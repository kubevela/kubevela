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

func TestChecks(t *testing.T) {
	testCluster(t)
	s := loadOne(t, "testdata/checks/checks_test.cue")
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures, s.Cases[i].Name)
	}
}

func TestCheckFailures(t *testing.T) {
	testCluster(t)
	caseWith := func(checks string) string {
		return `
_cm: {apiVersion: "v1", kind: "ConfigMap"}
"publishes": test.#WorkflowStepExec & {
	definition: "publish"
	context: namespace: "check-failures"
	parameter: {name: "cfg", data: a: "2"}
	expect: checks: ` + checks + `
}
`
	}
	for name, tc := range map[string]struct{ checks, want string }{
		"a mismatch names the path": {
			checks: `"cfg has a": {call: kube.#Read & {$params: value: _cm & {metadata: {name: "cfg", namespace: "check-failures"}}}, returns: value: data: a: "1"}`,
			want:   `checks."cfg has a".returns.value.data.a: expected "1", got "2"`,
		},
		"an object expected absent": {
			checks: `gone: {call: kube.#Read & {$params: value: _cm & {metadata: {name: "cfg", namespace: "check-failures"}}}, returns: err: =~"not found"}`,
			want:   `checks.gone.returns.err: missing`,
		},
		// kube.#List reads $params.value in kubevela/workflow, not the
		// $params.resource its schema declares, and panics without it.
		"a provider that panics": {
			checks: `list: {call: kube.#List & {$params: resource: {apiVersion: "v1", kind: "ConfigMap"}}}`,
			want:   `checks.list: provider kube.list panicked: `,
		},
		"@not when it matches": {
			checks: `not: {call: kube.#Read & {$params: value: _cm & {metadata: {name: "cfg", namespace: "check-failures"}}}, returns: value: data: a: "2" @not()}`,
			want:   `checks.not.returns.value.data.a: expected not to match "2"`,
		},
		"@contains with no match": {
			checks: `has: {call: kube.#Read & {$params: value: _cm & {metadata: {name: "cfg", namespace: "check-failures"}}}, returns: value: metadata: managedFields: [{manager: "nobody"}] @contains()}`,
			want:   `checks.has.returns.value.metadata.managedFields: no element matches`,
		},
		"a call that reaches outside the cluster": {
			checks: `ping: {call: http.#HTTPDo & {$params: {method: "GET", url: "https://example.com"}}}`,
			want:   `checks.ping: `,
		},
	} {
		t.Run(name, func(t *testing.T) {
			src := caseWith(tc.checks)
			if name == "a call that reaches outside the cluster" {
				src = "import \"vela/http\"\n" + src
			}
			s := loadOne(t, writeHookSuite(t, src))
			failures := s.Evaluate(s.Cases, RunOptions{})[0].Failures
			require.Len(t, failures, 1)
			require.Contains(t, failures[0], tc.want)
		})
	}
}
