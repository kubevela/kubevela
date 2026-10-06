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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkflowStepExecCases(t *testing.T) {
	testCluster(t)
	suites, err := Load("testdata/steps")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	require.Len(t, suites[0].Cases, 6)
	for _, c := range suites[0].Cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Subset(t, c.Labels, []string{"workflow-step", "exec"})
			require.Empty(t, c.Run())
		})
	}
}

func TestWorkflowStepExecMocksAreChecked(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "s.cue", "s: type: \"workflow-step\"\ntemplate: parameter: {}")
	write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#WorkflowStepExec & {definition: \"s\", mocks: {\"vela/builtin\": \"#Suspend\": {}}, expect: {}}")
	suites, err := Load(dir)
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, "vela/builtin drives the step's phase, so it runs for real")
}

// Each case runs a step afresh, however many ran before it in the same
// namespace: the workflow engine counts a step's failures across runs of one
// Application, and cases are not runs of one Application.
func TestStepCasesDoNotShareFailureCounts(t *testing.T) {
	testCluster(t)
	s := loadOne(t, writeHookSuite(t, `"fails": test.#WorkflowStepExec & {
	definition: "publish"
	context: namespace: "shared-failures"
	parameter: {name: "Not_A_Valid_Name", data: {}}
	expect: {phase: "failed", reason: "Execute"}
}
`))
	for i := 0; i < 12; i++ {
		require.Empty(t, s.Cases[0].Run(), "run %d", i+1)
	}
}
