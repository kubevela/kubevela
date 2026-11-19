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

// Steps reach the providers the controller wires up for them, and the harness
// records, mocks and cleans up around each as it does for any other.
func TestStepHarness(t *testing.T) {
	testCluster(t)
	s := loadOne(t, "testdata/harness/harness_test.cue")
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		t.Run(s.Cases[i].Name, func(t *testing.T) {
			require.Empty(t, o.Failures)
		})
	}
}

// A case that could not be set up fails for that, whatever error it expects:
// the definition never ran.
func TestSetupFailureIsNotTheDefinitions(t *testing.T) {
	testCluster(t)
	s := loadOne(t, writeHookSuite(t, `"expects any error": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "mine", data: {}}
	resources: [{apiVersion: "v1", kind: "Nonsense", metadata: name: "x"}]
	expect: error: _
}
`))
	o := s.Evaluate(s.Cases, RunOptions{})
	require.Len(t, o[0].Failures, 1)
	require.Contains(t, o[0].Failures[0], "setting up the case: ")
}

// A case whose expectations cannot be read back fails for that, after its step
// ran, whatever error it expects.
func TestReadBackFailureIsNotTheDefinitions(t *testing.T) {
	testCluster(t)
	s := loadOne(t, writeHookSuite(t, `"expects any error": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "mine", data: {}}
	expect: {
		error: _
		resources: [{apiVersion: "v1", kind: string, metadata: name: "mine"}]
	}
}
`))
	o := s.Evaluate(s.Cases, RunOptions{})
	require.Len(t, o[0].Failures, 1)
	require.Contains(t, o[0].Failures[0], "reading back expect.resources: ")
}
