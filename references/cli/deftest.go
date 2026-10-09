/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/definition/cuetest"
)

type defTestResult struct {
	File       string   `json:"file"`
	Line       int      `json:"line,omitempty"`
	Definition string   `json:"definition,omitempty"`
	Test       string   `json:"test,omitempty"`
	Case       string   `json:"case,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	// Load marks a test file that failed to load, rather than a case.
	Load bool `json:"load,omitempty"`
	// Skipped marks a case the filters did not select.
	Skipped bool `json:"skipped,omitempty"`
	// Pending marks a selected @pending case, which was not run.
	Pending bool `json:"pending,omitempty"`
	// Upgraded marks an @upgrade case that failed as written and passed once
	// upgraded: a known issue, not a failure.
	Upgraded bool `json:"upgraded,omitempty"`
	// Reason is the @pending or @upgrade reason.
	Reason   string   `json:"reason,omitempty"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
}

// NewDefinitionTestCommand creates the `vela def test` command.
func NewDefinitionTestCommand() *cobra.Command {
	var verbose, asJSON, failOnPending, useUpgrades, failOnUpgrade bool
	var envtestAssets string
	var crds []string
	var filterOpts cuetest.FilterOptions
	cmd := &cobra.Command{
		Use:   "test [PATH]",
		Short: "Run CUE definition tests.",
		Long: "Run the *_test.cue files at PATH, a file or a directory searched recursively; PATH defaults to the " +
			"current directory. " +
			"A test file imports \"vela/test\" and declares each case with one of its functions: " +
			"#ComponentRender and #TraitRender check what a definition renders from given parameters and context; " +
			"#ComponentStatus and #TraitStatus check its health, message and details given the state the cluster " +
			"reports for the rendered objects; #PolicyRender and #ApplicationPolicyRender render a policy; " +
			"#AddonRender renders an addon as enabling it would; #WorkflowStepExec runs a workflow step and " +
			"#SourceExec resolves a source. A case's definition is the path of its .cue file, relative to the " +
			"test file, with \".cue\" optional. " +
			"A case passes when its expectation subsumes the result. Label cases with @label(...) and select them with " +
			"the Ginkgo-compatible --label-filter, --focus, --skip and --focus-file. A case marked @pending(reason) " +
			"is not run, and is counted as pending. Definitions are tested as written; --upgrade first applies every " +
			"rewrite of KubeVela's CUE upgrader, to see whether it would rescue one. A case marked @upgrade(reason=...) " +
			"is a known dependency on the upgrader: if it fails as written it is retried upgraded and counted as " +
			"upgraded, and if it passes as written it fails until the marker is removed. " +
			"Render, status, policy and addon cases contact no cluster. Step and source cases run against a local " +
			"API server with no controllers, started on first use and stopped on exit: its etcd and kube-apiserver " +
			"binaries come from --envtest-assets or KUBEBUILDER_ASSETS, and its CRDs from --crds or, by default, " +
			"the vela-core chart found above the test files.",
		Example: "# Run every definition test under ./definitions\n" +
			"> vela def test ./definitions\n" +
			"# List passing cases too\n" +
			"> vela def test ./definitions -v\n" +
			"# Machine-readable results\n" +
			"> vela def test ./definitions --json\n" +
			"# Status tests that are not labelled slow, or one case by line\n" +
			"> vela def test --label-filter 'status && !slow'\n" +
			"> vela def test --focus-file webservice_test.cue:12\n" +
			"# Step and source cases, with envtest's binaries installed by setup-envtest\n" +
			"> vela def test ./workflowsteps --envtest-assets \"$(setup-envtest use -p path)\"",
		Args: cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeDefManagement,
			types.TagCommandOrder: "6",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// The render path logs its errors through klog; they are reported below.
			klog.SetLogger(logr.Discard())
			cuex.EnableExternalPackageForDefaultCompiler = false
			runOpts := cuetest.RunOptions{Upgrades: useUpgrades, FailOnUpgrade: failOnUpgrade}

			// Step and source cases start a local API server on first use; its
			// processes outlive this one unless stopped, interrupted or not.
			cuetest.ExecClusterOptions = cuetest.ClusterOptions{Assets: envtestAssets}
			for _, dir := range crds {
				abs, err := filepath.Abs(dir)
				if err != nil {
					return err
				}
				cuetest.ExecClusterOptions.CRDs = append(cuetest.ExecClusterOptions.CRDs, abs)
			}
			defer func() { _ = cuetest.StopExecCluster() }()
			interrupted := make(chan os.Signal, 1)
			signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(interrupted)
			go func() {
				if sig, ok := <-interrupted; ok {
					_ = cuetest.StopExecCluster()
					os.Exit(signalExitCode(sig))
				}
			}()

			filter, err := cuetest.NewFilter(filterOpts)
			if err != nil {
				return err
			}
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			suites, err := cuetest.Load(path)
			if err != nil {
				return err
			}
			var results []defTestResult
			var passed, failed, skipped, pending, upgraded, loadFailed int
			for _, s := range suites {
				if s.Err != nil {
					loadFailed++
					results = append(results, defTestResult{File: s.File, Load: true, Failures: []string{s.Err.Error()}})
					continue
				}
				// Cases to run go through the suite, between its hooks, and
				// are reported in file order with the rest.
				var run []*cuetest.Case
				for _, c := range s.Cases {
					if filter.Match(c) && !c.Pending {
						run = append(run, c)
					}
				}
				outcomes := map[*cuetest.Case]cuetest.Outcome{}
				for i, o := range s.Evaluate(run, runOpts) {
					outcomes[run[i]] = o
				}
				for _, c := range s.Cases {
					result := defTestResult{
						File: c.File, Line: c.Line, Definition: c.Subject.Name, Test: string(c.Test), Case: c.Name, Labels: c.Labels,
					}
					if !filter.Match(c) {
						skipped++
						result.Skipped = true
						results = append(results, result)
						continue
					}
					if c.Pending {
						pending++
						result.Pending, result.Reason = true, c.PendingReason
						results = append(results, result)
						continue
					}
					outcome := outcomes[c]
					switch {
					case len(outcome.Failures) > 0:
						failed++
					case outcome.Upgraded:
						upgraded++
						result.Upgraded, result.Reason = true, c.Upgrade.Reason
					default:
						passed++
					}
					result.Passed, result.Failures = len(outcome.Failures) == 0, outcome.Failures
					results = append(results, result)
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(results); err != nil {
					return err
				}
			} else {
				printDefTestResults(out, results, verbose)
				summary := fmt.Sprintf("%d passed, %d failed", passed, failed)
				if skipped > 0 {
					summary += fmt.Sprintf(", %d skipped", skipped)
				}
				if pending > 0 {
					summary += fmt.Sprintf(", %d pending", pending)
				}
				if upgraded > 0 {
					summary += fmt.Sprintf(", %d upgraded", upgraded)
				}
				if loadFailed > 0 {
					summary += fmt.Sprintf(", %d failed to load", loadFailed)
				}
				fmt.Fprintln(out, summary)
			}
			var problems []string
			if failed > 0 {
				problems = append(problems, fmt.Sprintf("%d of %d definition tests failed", failed, passed+failed+upgraded))
			}
			if loadFailed > 0 {
				problems = append(problems, fmt.Sprintf("%d test %s failed to load", loadFailed, plural(loadFailed, "file", "files")))
			}
			if failOnPending && pending > 0 {
				problems = append(problems, fmt.Sprintf("%d definition %s pending", pending, plural(pending, "test is", "tests are")))
			}
			if len(problems) > 0 {
				return errors.New(strings.Join(problems, "; "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list passing cases as well as failures")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print results as JSON")
	cmd.Flags().BoolVar(&failOnPending, "fail-on-pending", false, "fail if any selected case is @pending")
	cmd.Flags().BoolVar(&useUpgrades, "upgrade", false, "run definitions through KubeVela's CUE upgrader, with every rewrite pass on, instead of testing them as written")
	cmd.Flags().StringVar(&envtestAssets, "envtest-assets", "", "directory holding etcd and kube-apiserver for step and source cases (default: KUBEBUILDER_ASSETS)")
	cmd.Flags().StringArrayVar(&crds, "crds", nil, "CRD file or directory to install for step and source cases (default: the vela-core chart's, when found); repeatable")
	cmd.Flags().BoolVar(&failOnUpgrade, "fail-on-upgrade", false, "fail @upgrade cases that fail as written, instead of retrying them upgraded")
	cmd.Flags().StringVar(&filterOpts.LabelFilter, "label-filter", "", "run only cases whose labels match this Ginkgo label query, e.g. 'status && !slow'")
	cmd.Flags().StringArrayVar(&filterOpts.Focus, "focus", nil, "run only cases whose full text ('<file> <definition> / <case>') matches this regexp; repeatable")
	cmd.Flags().StringArrayVar(&filterOpts.Skip, "skip", nil, "skip cases whose full text matches this regexp; repeatable")
	cmd.Flags().StringArrayVar(&filterOpts.FocusFiles, "focus-file", nil, "run only cases in files matching this regexp, optionally with lines: file:12, file:3,9 or file:1-20; repeatable")
	return cmd
}

func printDefTestResults(out io.Writer, results []defTestResult, verbose bool) {
	for _, r := range results {
		switch {
		case r.Load:
			fmt.Fprintf(out, "FAIL %s (load)\n", r.File)
		case r.Upgraded:
			// Named even without -v: it passes only because of the upgrader.
			fmt.Fprintf(out, "upgd %s:%d %s / %s\n", r.File, r.Line, r.Definition, r.Case)
			if r.Reason != "" {
				fmt.Fprintf(out, "  %s\n", r.Reason)
			}
		case (r.Passed || r.Skipped || r.Pending) && !verbose:
			continue
		case r.Pending:
			fmt.Fprintf(out, "pend %s:%d %s / %s\n", r.File, r.Line, r.Definition, r.Case)
			if r.Reason != "" {
				fmt.Fprintf(out, "  %s\n", r.Reason)
			}
		case r.Skipped:
			fmt.Fprintf(out, "skip %s:%d %s / %s\n", r.File, r.Line, r.Definition, r.Case)
		case r.Passed:
			fmt.Fprintf(out, "ok   %s:%d %s / %s\n", r.File, r.Line, r.Definition, r.Case)
		default:
			fmt.Fprintf(out, "FAIL %s:%d %s / %s\n", r.File, r.Line, r.Definition, r.Case)
		}
		for _, f := range r.Failures {
			fmt.Fprintf(out, "  %s\n", strings.ReplaceAll(f, "\n", "\n  "))
		}
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// signalExitCode is the shell's exit status for a process killed by sig.
func signalExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 130
}
