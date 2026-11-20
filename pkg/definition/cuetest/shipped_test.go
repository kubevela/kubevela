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

package cuetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// shippedDefinitions are KubeVela's built-in definitions, which ship in the
// vela-core chart.
const shippedDefinitions = "../../../vela-templates/definitions/internal"

// shippedDefinitionTestsEnv opts into the tests of the shipped definitions,
// which CI runs in their own job (make test-definitions), not with the unit tests.
const shippedDefinitionTestsEnv = "VELA_SHIPPED_DEFINITION_TESTS"

func requireShippedDefinitionTests(t *testing.T) {
	t.Helper()
	if os.Getenv(shippedDefinitionTestsEnv) == "" {
		t.Skipf("set %s=1 to run, or use make test-definitions", shippedDefinitionTestsEnv)
	}
}

// untested are the shipped definitions with no test file, and why.
var untested = map[string]string{
	"component/ref-objects":        "the controller fetches the objects it names; its template only passes them through",
	"workflowstep/apply-component": "the controller runs its builtin apply-component task; its template only declares parameters",
	"policy/apply-once":            "read by the controller, not rendered",
	"policy/garbage-collect":       "read by the controller, not rendered",
	"policy/override":              "read by the controller, not rendered",
	"policy/read-only":             "read by the controller, not rendered",
	"policy/replication":           "read by the controller, not rendered",
	"policy/resource-update":       "read by the controller, not rendered",
	"policy/shared-resource":       "read by the controller, not rendered",
	"policy/take-over":             "read by the controller, not rendered",
	"policy/topology":              "read by the controller, not rendered",
}

// Every shipped definition has a test file beside it, unless it is listed as
// untested with why; a listed one that gains tests must come off the list.
func TestShippedDefinitionsAreTested(t *testing.T) {
	requireShippedDefinitionTests(t)
	var missing, stale []string
	seen := map[string]bool{}
	err := filepath.WalkDir(shippedDefinitions, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".cue" || utils.IsCUETestFile(p) {
			return err
		}
		rel, _ := filepath.Rel(shippedDefinitions, strings.TrimSuffix(p, ".cue"))
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		_, statErr := os.Stat(strings.TrimSuffix(p, ".cue") + utils.CUETestFileSuffix)
		_, excused := untested[rel]
		switch {
		case statErr != nil && !excused:
			missing = append(missing, rel)
		case statErr == nil && excused:
			stale = append(stale, rel)
		}
		return nil
	})
	require.NoError(t, err)
	for rel := range untested {
		if !seen[rel] {
			stale = append(stale, rel+" (no such definition)")
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	require.Empty(t, missing, "shipped definitions without a %s beside them", utils.CUETestFileSuffix)
	require.Empty(t, stale, "listed as untested, but tested or gone")
}

// The shipped definitions' own tests pass, pending ones aside.
func TestShippedDefinitions(t *testing.T) {
	requireShippedDefinitionTests(t)
	suites, err := Load(shippedDefinitions)
	if err != nil && strings.Contains(err.Error(), "no *"+utils.CUETestFileSuffix) {
		t.Skip("no shipped definition has tests yet")
	}
	require.NoError(t, err)
	for _, s := range suites {
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		t.Run(rel, func(t *testing.T) {
			require.NoError(t, s.Err)
			var run []*Case
			for _, c := range s.Cases {
				if !c.Pending {
					run = append(run, c)
				}
			}
			for i, o := range s.Evaluate(run, RunOptions{}) {
				require.Emptyf(t, o.Failures, "%s", run[i].Name)
			}
		})
	}
}

// uncovered are declared parameters no case sets, and why: a whole
// definition as "<kind>/<name>", or one parameter as "<kind>/<name>:<path>".
// A listed one that gains a case must come off the list.
var uncovered = map[string]string{
	"trait/k8s-update-strategy:targetAPIVersion": "declared but never read by the template, so a case has nothing to assert",
}

// coverageDepth is how deep into a parameter's fields the coverage check looks.
const coverageDepth = 2

// Every parameter a shipped definition declares, to coverageDepth, is set by
// a case that runs.
func TestShippedParametersAreCovered(t *testing.T) {
	requireShippedDefinitionTests(t)
	suites, err := Load(shippedDefinitions)
	require.NoError(t, err)
	compiler, err := testCompiler()
	require.NoError(t, err)
	var missing, stale []string
	listed := map[string]bool{}
	for _, s := range suites {
		require.NoError(t, s.Err)
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		def := strings.TrimSuffix(rel, utils.CUETestFileSuffix)
		if len(s.Cases) == 0 {
			continue
		}
		if _, skip := uncovered[def]; skip {
			listed[def] = true
			continue
		}
		// the controller supplies context when it renders; here it only needs to resolve
		v, err := compiler.CompileStringWithOptions(context.Background(), s.Cases[0].Subject.Template+"\ncontext: {...}\n", cuex.DisableResolveProviderFunctions{})
		require.NoError(t, err, def)
		param := v.LookupPath(cue.ParsePath("parameter"))
		if !param.Exists() {
			require.NoErrorf(t, v.Err(), "%s: its template does not evaluate, so its parameters cannot be read", def)
			continue
		}
		declared := map[string]bool{}
		declaredPaths(param, "", 1, declared)
		set := map[string]bool{}
		for _, c := range s.Cases {
			if !c.Pending {
				setPaths(c.Input.Parameter, "", 1, set)
			}
		}
		for p := range declared {
			key := def + ":" + p
			_, excused := uncovered[key]
			switch {
			case excused:
				listed[key] = true
			case !set[p]:
				missing = append(missing, key)
			}
		}
	}
	for key := range uncovered {
		if !listed[key] {
			stale = append(stale, key)
		}
	}
	for key := range listed {
		if def, p, ok := strings.Cut(key, ":"); ok && coveredElsewhere(suites, def, p) {
			stale = append(stale, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	require.Empty(t, missing, "declared parameters no running case sets; add a case, or list them in uncovered with why")
	require.Empty(t, stale, "listed as uncovered, but covered or gone")
}

// declaredPaths adds the dotted path of each field v declares, optional ones
// included, descending into structs and list elements. A disjunction declares
// the fields of all its branches.
func declaredPaths(v cue.Value, prefix string, depth int, into map[string]bool) {
	if op, args := v.Expr(); op == cue.OrOp {
		for _, a := range args {
			declaredPaths(a, prefix, depth, into)
		}
		return
	}
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		return
	}
	for it.Next() {
		p := prefix + it.Selector().Unquoted()
		into[p] = true
		if depth >= coverageDepth {
			continue
		}
		child := it.Value()
		if elem := child.LookupPath(cue.MakePath(cue.AnyIndex)); child.IncompleteKind()&cue.ListKind != 0 && elem.Exists() {
			child = elem
		}
		declaredPaths(child, p+".", depth+1, into)
	}
}

// setPaths adds the dotted path of each field a case's parameter sets, the
// way declaredPaths names them.
func setPaths(v any, prefix string, depth int, into map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			p := prefix + k
			into[p] = true
			if depth < coverageDepth {
				setPaths(child, p+".", depth+1, into)
			}
		}
	case []any:
		for _, e := range v {
			setPaths(e, prefix, depth, into)
		}
	}
}

// coveredElsewhere reports whether a running case of def sets path p.
func coveredElsewhere(suites []*Suite, def, p string) bool {
	for _, s := range suites {
		rel, _ := filepath.Rel(shippedDefinitions, s.File)
		if strings.TrimSuffix(rel, utils.CUETestFileSuffix) != def {
			continue
		}
		set := map[string]bool{}
		for _, c := range s.Cases {
			if !c.Pending {
				setPaths(c.Input.Parameter, "", 1, set)
			}
		}
		return set[p]
	}
	return false
}

// shippedGolden holds what each shipped definition's render cases produce, so a change to what
// a definition renders shows in review, and in the release notes, rather than on the first
// edit of an Application after an upgrade.
const shippedGolden = "testdata/golden"

// updateGoldenEnv rewrites the golden files from the current renders.
const updateGoldenEnv = "UPDATE_GOLDEN"

// goldenDefinitionsEnv and goldenReportEnv compare another release's definitions with the
// golden renders, writing the differences to a report.
const (
	goldenDefinitionsEnv = "GOLDEN_DEFINITIONS_DIR"
	goldenReportEnv      = "GOLDEN_REPORT"
	// goldenRendersEnv, with goldenReportEnv, keeps each differing render in that directory,
	// laid out like the golden files.
	goldenRendersEnv = "GOLDEN_RENDERS_DIR"
)

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// The shipped definitions' successful render cases produce what their golden files hold. A
// case that expects an error has none: error text changes without the render changing.
//
// With goldenReportEnv set it compares another release's definitions instead, laid beside these
// test files in goldenDefinitionsEnv by hack/definition-render-diff.sh, and writes what renders
// differently to the report rather than failing.
func TestShippedDefinitionsMatchTheirGoldenRenders(t *testing.T) {
	requireShippedDefinitionTests(t)
	definitions := shippedDefinitions
	if dir := os.Getenv(goldenDefinitionsEnv); dir != "" {
		definitions = dir
	}
	report := os.Getenv(goldenReportEnv)
	var differs []string
	if report != "" {
		defer func() {
			sort.Strings(differs)
			require.NoError(t, os.WriteFile(report, []byte(strings.Join(differs, "\n")+"\n"), 0o600))
		}()
	}
	suites, err := Load(definitions)
	require.NoError(t, err)
	update := os.Getenv(updateGoldenEnv) != ""
	for _, s := range suites {
		rel, _ := filepath.Rel(definitions, s.File)
		dir := filepath.Join(shippedGolden, strings.TrimSuffix(rel, utils.CUETestFileSuffix))
		var cases []*Case
		for _, c := range s.Cases {
			if !c.Pending && tests[c.Test].mode != "exec" && !c.Expect.LookupPath(cue.ParsePath("error")).Exists() {
				cases = append(cases, c)
			}
		}
		if len(cases) == 0 {
			continue
		}
		t.Run(rel, func(t *testing.T) {
			require.NoError(t, s.RunBefore())
			defer func() { require.NoError(t, s.RunAfter()) }()
			for _, c := range cases {
				result, err := c.result()
				if report != "" && err != nil {
					differs = append(differs, fmt.Sprintf("- %s: %q does not render: %v", strings.TrimSuffix(rel, utils.CUETestFileSuffix), c.Name, firstLine(err.Error())))
					continue
				}
				require.NoError(t, err, c.Name)
				// What reaches the cluster: the context a template reads is an input, and
				// listing it would rewrite every file whenever a context field is added.
				delete(result, "context")
				got, err := yaml.Marshal(result)
				require.NoError(t, err, c.Name)
				file := filepath.Join(dir, goldenName(c.Name)+".yaml")
				if update {
					require.NoError(t, os.MkdirAll(dir, 0o755))
					require.NoError(t, os.WriteFile(file, got, 0o600))
					continue
				}
				want, err := os.ReadFile(file)
				require.NoError(t, err, "%s: no golden render; run with %s=1 to write it", c.Name, updateGoldenEnv)
				if report != "" {
					if string(want) != string(got) {
						differs = append(differs, fmt.Sprintf("- %s: %q renders differently", strings.TrimSuffix(rel, utils.CUETestFileSuffix), c.Name))
						if out := os.Getenv(goldenRendersEnv); out != "" {
							keep := filepath.Join(out, strings.TrimSuffix(rel, utils.CUETestFileSuffix))
							require.NoError(t, os.MkdirAll(keep, 0o755))
							require.NoError(t, os.WriteFile(filepath.Join(keep, goldenName(c.Name)+".yaml"), got, 0o600))
						}
					}
					continue
				}
				require.Equal(t, string(want), string(got),
					"%s renders differently from %s: if intended, rewrite it with %s=1 and note the change for the release", c.Name, file, updateGoldenEnv)
			}
		})
	}
}

// goldenName turns a case name into a file name.
func goldenName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
