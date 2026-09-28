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

func TestApplicationPolicyRenderCases(t *testing.T) {
	s := loadOne(t, "testdata/apppolicies/apppolicies_test.cue")
	for _, c := range s.Cases {
		require.Subset(t, c.Labels, []string{"application-policy", "render"})
		require.Empty(t, c.Run(), c.Name)
	}
}

func TestApplicationPolicyRenderLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"a workload-bearing policy": {
			src:  `"case": test.#ApplicationPolicyRender & {definition: "../policies/network-guard"}`,
			want: `"network-guard" is a policy definition, so #ApplicationPolicyRender cannot test it; use #PolicyRender`,
		},
		"the spec set through context": {
			src:  `"case": test.#ApplicationPolicyRender & {definition: "cost-tag", context: appComponents: []}`,
			want: `context.appComponents is not offered to application-scoped policies`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			suites, err := Load(writeCase(t, "testdata/apppolicies", tc.src))
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.want)
		})
	}
}

// writeCase writes a test file of src into dir, within a copy of testdata,
// so its definitions resolve as they would beside the real ones.
func writeCase(t *testing.T, dir, src string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(root, "testdata"), os.DirFS("testdata")))
	rel, err := filepath.Rel("testdata", dir)
	require.NoError(t, err)
	file := filepath.Join(root, "testdata", rel, "case_test.cue")
	require.NoError(t, os.WriteFile(file, []byte("import \"vela/test\"\n\n"+src+"\n"), 0o600))
	return file
}

// An Application-scoped policy is given exactly the packages the controller
// compiles one with: CueX's internal packages, external ones being off.
func TestScopedPolicyProvidersAreTheControllers(t *testing.T) {
	var got []string
	for _, p := range scopedPolicyProviders.packages() {
		got = append(got, p.GetPath())
	}
	require.ElementsMatch(t, []string{"vela/base64", "vela/cue", "vela/http", "vela/kube", "vela/util"}, got)
}
