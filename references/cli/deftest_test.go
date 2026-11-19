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
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const defTestDefinition = `web: {
	type: "component"
	attributes: {
		workload: definition: {apiVersion: "apps/v1", kind: "Deployment"}
		status: healthPolicy: "isHealth: context.output.status.readyReplicas == context.output.spec.replicas"
	}
}
template: {
	output: {apiVersion: "apps/v1", kind: "Deployment", spec: replicas: parameter.replicas}
	parameter: replicas: *1 | int
}
`

const defTestCases = `import "vela/test"

"defaults to one replica": test.#ComponentRender & {definition: "web", expect: output: spec: replicas: 1}
"fails on purpose": test.#ComponentRender & {definition: "web", expect: output: spec: replicas: 2}
"healthy once ready": test.#ComponentStatus & {
	definition: "web"
	observed: output: status: readyReplicas: 1
	expect: healthy: true
}
`

func runDefTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	dir := writeDefWithTests(t)
	cmd := NewDefinitionTestCommand()
	initCommand(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{dir}, args...))
	err := cmd.Execute()
	return string(bytes.ReplaceAll(out.Bytes(), []byte(dir+string(filepath.Separator)), nil)), err
}

func TestDefinitionTestCommand(t *testing.T) {
	out, err := runDefTest(t)
	require.EqualError(t, err, "1 of 3 definition tests failed")
	require.Equal(t, "FAIL web_test.cue:4 web / fails on purpose\n"+
		"  output.spec.replicas: expected 2, got 1\n"+
		"2 passed, 1 failed\n", out)
}

func TestDefinitionTestCommandLoadFailure(t *testing.T) {
	dir := writeDefWithTests(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken_test.cue"), []byte("tests: {}"), 0o600))
	cmd := NewDefinitionTestCommand()
	initCommand(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{dir})
	err := cmd.Execute()
	got := strings.ReplaceAll(out.String(), dir+string(filepath.Separator), "")
	require.EqualError(t, err, "1 of 3 definition tests failed; 1 test file failed to load")
	require.Contains(t, got, "FAIL broken_test.cue (load)\n  no cases: declare each with a function from \"vela/test\"\n")
	require.Contains(t, got, "FAIL web_test.cue:4 web / fails on purpose\n", "the other files still run")
	require.True(t, strings.HasSuffix(got, "2 passed, 1 failed, 1 failed to load\n"), got)
}

func TestDefinitionTestCommandDefaultsToCurrentDir(t *testing.T) {
	dir := writeDefWithTests(t)
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { require.NoError(t, os.Chdir(wd)) }()
	cmd := NewDefinitionTestCommand()
	initCommand(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	require.EqualError(t, cmd.Execute(), "1 of 3 definition tests failed")
	require.Contains(t, out.String(), "FAIL web_test.cue:4 web / fails on purpose")
}

func TestDefinitionTestCommandVerbose(t *testing.T) {
	out, _ := runDefTest(t, "-v")
	require.Contains(t, out, "ok   web_test.cue:3 web / defaults to one replica\n")
}

func TestDefinitionTestCommandJSON(t *testing.T) {
	out, err := runDefTest(t, "--json")
	require.Error(t, err)
	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	require.Len(t, results, 3)
	require.Equal(t, map[string]any{
		"file":       "web_test.cue",
		"line":       float64(4),
		"definition": "web",
		"test":       "component-render",
		"labels":     []any{"component", "render"},
		"case":       "fails on purpose",
		"passed":     false,
		"failures":   []any{"output.spec.replicas: expected 2, got 1"},
	}, results[1])
}

// writeDefWithTests writes a definition and its test file to a new directory.
func writeDefWithTests(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "web.cue"), []byte(defTestDefinition), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "web_test.cue"), []byte(defTestCases), 0o600))
	return dir
}

func TestDefinitionRenderSkipsTestFiles(t *testing.T) {
	dir, out := writeDefWithTests(t), t.TempDir()
	cmd := NewDefinitionRenderCommand(initArgs())
	initCommand(cmd)
	cmd.SetArgs([]string{dir, "-o", out})
	require.NoError(t, cmd.Execute())
	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Equal(t, []string{"web.yaml"}, names)
}

func TestDryRunDefinitionsSkipTestFiles(t *testing.T) {
	objs, err := ReadDefinitionsFromFile(writeDefWithTests(t), util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
	require.NoError(t, err)
	require.Len(t, objs, 1)
	require.Equal(t, "web", objs[0].GetName())
}

func TestDefinitionTestCommandFilters(t *testing.T) {
	cases := map[string]struct {
		args    []string
		summary string
		err     string
	}{
		"label":            {[]string{"--label-filter", "status"}, "1 passed, 0 failed, 2 skipped\n", ""},
		"label expression": {[]string{"--label-filter", "render && !status"}, "1 passed, 1 failed, 1 skipped\n", "1 of 2 definition tests failed"},
		"focus":            {[]string{"--focus", "on purpose"}, "0 passed, 1 failed, 2 skipped\n", "1 of 1 definition tests failed"},
		"skip":             {[]string{"--skip", "on purpose"}, "2 passed, 0 failed, 1 skipped\n", ""},
		"focus file line":  {[]string{"--focus-file", "web_test.cue:3"}, "1 passed, 0 failed, 2 skipped\n", ""},
		"nothing selected": {[]string{"--focus", "nope"}, "0 passed, 0 failed, 3 skipped\n", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runDefTest(t, tc.args...)
			if tc.err == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.err)
			}
			require.True(t, strings.HasSuffix(out, tc.summary), out)
		})
	}
}

func TestDefinitionTestCommandBadFilter(t *testing.T) {
	_, err := runDefTest(t, "--label-filter", "a &&")
	require.ErrorContains(t, err, "--label-filter: ")
}

func TestDefinitionTestCommandJSONSkipped(t *testing.T) {
	out, err := runDefTest(t, "--json", "--label-filter", "status")
	require.NoError(t, err)
	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	require.Len(t, results, 3)
	require.Equal(t, true, results[0]["skipped"])
	require.Equal(t, []any{"component", "render"}, results[0]["labels"])
	require.Equal(t, true, results[2]["passed"])
}

func TestDefinitionTestCommandPending(t *testing.T) {
	const parked = `import "vela/test"
"parked": test.#ComponentRender & {definition: "web", expect: output: spec: replicas: 99} @pending(waiting on a fix)
`
	run := func(args ...string) (string, error) {
		dir := writeDefWithTests(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "parked_test.cue"), []byte(parked), 0o600))
		cmd := NewDefinitionTestCommand()
		initCommand(cmd)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{dir, "--skip", "on purpose"}, args...))
		err := cmd.Execute()
		return strings.ReplaceAll(out.String(), dir+string(filepath.Separator), ""), err
	}

	out, err := run("-v")
	require.NoError(t, err, "a pending case does not run, so its failing expectation does not count")
	require.Contains(t, out, "pend parked_test.cue:2 web / parked\n  waiting on a fix\n")
	require.True(t, strings.HasSuffix(out, "2 passed, 0 failed, 1 skipped, 1 pending\n"), out)

	_, err = run("--fail-on-pending")
	require.EqualError(t, err, "1 definition test is pending")

	out, err = run("--focus", "defaults")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(out, "1 passed, 0 failed, 3 skipped\n"), "a pending case that is filtered out is skipped: "+out)

	out, err = run("--json")
	require.NoError(t, err)
	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	require.Equal(t, true, results[0]["pending"])
	require.Equal(t, "waiting on a fix", results[0]["reason"])
}

func TestDefinitionTestCommandCheckPending(t *testing.T) {
	const parked = `import "vela/test"
"still broken": test.#ComponentRender & {definition: "web", expect: output: spec: replicas: 99} @pending(waiting on a fix)
"since fixed": test.#ComponentRender & {definition: "web", expect: output: spec: replicas: 1} @pending(fixed upstream)
`
	run := func(args ...string) (string, error) {
		dir := writeDefWithTests(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "parked_test.cue"), []byte(parked), 0o600))
		cmd := NewDefinitionTestCommand()
		initCommand(cmd)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{dir, "--skip", "on purpose"}, args...))
		err := cmd.Execute()
		return strings.ReplaceAll(out.String(), dir+string(filepath.Separator), ""), err
	}

	out, err := run()
	require.NoError(t, err, "without the flag, pending cases do not run")
	require.True(t, strings.HasSuffix(out, "2 passed, 0 failed, 1 skipped, 2 pending\n"), out)

	out, err = run("--check-pending")
	require.EqualError(t, err, "1 pending definition test now passes; remove its @pending marker")
	require.Contains(t, out, "FAIL parked_test.cue:3 web / since fixed\n"+
		"  passes, but is marked @pending(fixed upstream): the defect is fixed, so remove the marker\n")
	require.NotContains(t, out, "still broken", "a pending case that still fails stays quiet")
	require.True(t, strings.HasSuffix(out, "2 passed, 0 failed, 1 skipped, 1 pending, 1 no longer pending\n"), out)

	out, err = run("--check-pending", "--json")
	require.Error(t, err)
	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	byCase := map[string]map[string]any{}
	for _, r := range results {
		byCase[r["case"].(string)] = r
	}
	require.Equal(t, true, byCase["still broken"]["pending"])
	require.Nil(t, byCase["still broken"]["failures"])
	require.Equal(t, true, byCase["since fixed"]["pending"])
	require.Equal(t, false, byCase["since fixed"]["passed"])
	require.Equal(t, "fixed upstream", byCase["since fixed"]["reason"])
}

func TestDefinitionTestCommandUpgrade(t *testing.T) {
	run := func(args ...string) (string, error) {
		dir := t.TempDir()
		for _, f := range []string{"legacy.cue", "legacy_test.cue"} {
			b, err := os.ReadFile(filepath.Join("../../pkg/definition/cuetest/testdata/upgrade", f))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, f), b, 0o600))
		}
		cmd := NewDefinitionTestCommand()
		initCommand(cmd)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{dir}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	out, err := run()
	require.Error(t, err, "definitions are tested as written")
	require.Contains(t, out, "Addition of lists is superseded by list.Concat")

	out, err = run("--upgrade")
	require.NoError(t, err, "--upgrade runs the upgrader first")
	require.Equal(t, "1 passed, 0 failed\n", out)
}

func TestDefinitionTestCommandUpgradeMarker(t *testing.T) {
	run := func(args ...string) (string, error) {
		dir := t.TempDir()
		src := "../../pkg/definition/cuetest/testdata/upgrade-marked"
		entries, err := os.ReadDir(src)
		require.NoError(t, err)
		for _, e := range entries {
			b, err := os.ReadFile(filepath.Join(src, e.Name()))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600))
		}
		cmd := NewDefinitionTestCommand()
		initCommand(cmd)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{dir}, args...))
		err = cmd.Execute()
		return strings.ReplaceAll(out.String(), dir+string(filepath.Separator), ""), err
	}

	out, err := run("--focus", "needs the upgrader", "-v")
	require.NoError(t, err, "a known upgrader dependency does not block")
	require.Contains(t, out, "upgd marked_test.cue:8 legacy / needs the upgrader\n  args use list addition\n")
	require.True(t, strings.HasSuffix(out, "0 passed, 0 failed, 3 skipped, 1 upgraded\n"), out)

	out, err = run("--focus", "needs the upgrader")
	require.NoError(t, err)
	require.Contains(t, out, "upgd marked_test.cue:8 legacy / needs the upgrader\n  args use list addition\n",
		"an upgraded case is named with its reason, like a failure, without -v")

	_, err = run("--focus", "needs the upgrader", "--fail-on-upgrade")
	require.EqualError(t, err, "1 of 1 definition tests failed")

	out, err = run("--focus", "fixed but still marked")
	require.Error(t, err)
	require.Contains(t, out, "passes as written: remove @upgrade")

	out, err = run("--focus", "needs the upgrader", "--json")
	require.NoError(t, err)
	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	found := false
	for _, r := range results {
		if r["case"] == "needs the upgrader" {
			found = true
			require.Equal(t, true, r["upgraded"])
			require.Equal(t, "args use list addition", r["reason"])
		}
	}
	require.True(t, found, "the upgraded case is in the JSON")
}

func TestSignalExitCode(t *testing.T) {
	require.Equal(t, 130, signalExitCode(os.Interrupt))
	require.Equal(t, 143, signalExitCode(syscall.SIGTERM))
	require.Equal(t, 130, signalExitCode(otherSignal{}), "a signal with no number exits as an interrupt does")
}

// otherSignal is an os.Signal that is not a syscall.Signal.
type otherSignal struct{}

func (otherSignal) String() string { return "other" }
func (otherSignal) Signal()        {}

func writeStepCase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"publish.cue"} {
		b, err := os.ReadFile(filepath.Join("../../pkg/definition/cuetest/testdata/steps", f))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), b, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "publish_test.cue"), []byte(`import "vela/test"
"publishes": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "cfg", data: a: "1"}
	expect: {phase: "succeeded", resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "cfg", data: a: "1"}]}
}
`), 0o600))
	return dir
}

func TestDefinitionTestCommandExec(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("workflow step tests need envtest's binaries (KUBEBUILDER_ASSETS)")
	}
	cmd := NewDefinitionTestCommand()
	initCommand(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{writeStepCase(t), "--crds", "../../charts/vela-core/crds"})
	require.NoError(t, cmd.Execute(), out.String())
	require.Equal(t, "1 passed, 0 failed\n", out.String())
}

func TestDefinitionTestCommandHooks(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("workflow step tests need envtest's binaries (KUBEBUILDER_ASSETS)")
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "2 passed, 0 failed\n"},
		{[]string{"--focus", "runs each hook again"}, "1 passed, 0 failed, 1 skipped\n"},
	} {
		cmd := NewDefinitionTestCommand()
		initCommand(cmd)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{"../../pkg/definition/cuetest/testdata/hooks", "--crds", "../../charts/vela-core/crds"}, tc.args...))
		require.NoError(t, cmd.Execute(), out.String())
		require.Equal(t, tc.want, out.String())
	}
}

func TestDefinitionTestCommandExecWithoutBinaries(t *testing.T) {
	cmd := NewDefinitionTestCommand()
	initCommand(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{writeStepCase(t), "--envtest-assets", filepath.Join(t.TempDir(), "none")})
	require.Error(t, cmd.Execute())
	require.Contains(t, out.String(), "setup-envtest")
}

func TestDefinitionTestCommandRenderNeedsNoCluster(t *testing.T) {
	out, err := runDefTest(t, "--envtest-assets", "/nonexistent", "--skip", "on purpose")
	require.NoError(t, err, "a run with no workflow step cases never starts a cluster")
	require.Equal(t, "2 passed, 0 failed, 1 skipped\n", out)
}
