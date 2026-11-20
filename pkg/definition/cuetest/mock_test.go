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

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
)

func TestMocks(t *testing.T) {
	before := velacuex.WorkloadCompiler.Get()
	suites, err := Load("testdata/mocks")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	for _, c := range suites[0].Cases {
		require.Empty(t, c.Run(), c.Name)
	}
	require.Same(t, before, velacuex.WorkloadCompiler.Get(), "the real compiler is put back")
}

func TestMockErrors(t *testing.T) {
	cases := map[string]struct {
		mocks, err string
	}{
		"unknown package":  {`"vela/kub": "#Get": {}`, `mocks."vela/kub": not a provider; mockable: vela/addon, vela/config, vela/helm, vela/http, vela/kube, vela/module, vela/registry, vela/velaconfig`},
		"pure package":     {`"vela/base64": "#Encode": {}`, `mocks."vela/base64": vela/base64 has no side effects, so it runs for real`},
		"unknown function": {`"vela/kube": "#Gett": {}`, `mocks."vela/kube"."#Gett": vela/kube has no #Gett; it has #Apply, #Get, #List, #Patch`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {apiVersion: \"v1\", kind: \"X\"}, parameter: {}}")
			write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", mocks: {"+tc.mocks+"}, expect: {}}")
			suites, err := Load(dir)
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.err)
		})
	}
}
