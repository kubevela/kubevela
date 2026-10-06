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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyRenderCases(t *testing.T) {
	s := loadOne(t, "testdata/policies/policies_test.cue")
	for _, c := range s.Cases {
		require.Subset(t, c.Labels, []string{"policy", "render"})
		require.Empty(t, c.Run(), c.Name)
	}
}

func TestPolicyRenderLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct{ def, want string }{
		"an Application-scoped policy": {
			def:  "cost-tag",
			want: `"cost-tag" is Application-scoped, so #PolicyRender cannot test it; use #ApplicationPolicyRender`,
		},
		"a policy with nothing to render": {
			def:  "../../../../../vela-templates/definitions/internal/policy/apply-once",
			want: `"apply-once" renders nothing: its template has no output`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := filepath.Abs("testdata/policies")
			require.NoError(t, err)
			tmp := t.TempDir()
			file := filepath.Join(tmp, "p_test.cue")
			def, err := filepath.Rel(tmp, filepath.Join(dir, tc.def))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, []byte(`import "vela/test"

"case": test.#PolicyRender & {definition: "`+filepath.ToSlash(def)+`"}
`), 0o600))
			suites, err := Load(file)
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.want)
		})
	}
}

// A policy's objects are dispatched to the hub, so its context.cluster is
// always local and a case cannot set it.
func TestPolicyRenderClusterIsTheHub(t *testing.T) {
	suites, err := Load(writeCase(t, "testdata/policies", `"case": test.#PolicyRender & {
	definition: "network-guard"
	context: cluster: "prod"
	parameter: allowFrom: "ingress"
}`))
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, "context.cluster is derived: a policy's objects are dispatched to the hub, so it is local")
}
