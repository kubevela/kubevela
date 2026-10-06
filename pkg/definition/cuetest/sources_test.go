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

func TestSourceExecCases(t *testing.T) {
	testCluster(t)
	s := loadOne(t, "testdata/sources/sources_test.cue")
	var run []*Case
	for _, c := range s.Cases {
		require.Subset(t, c.Labels, []string{"source", "exec"})
		if !c.Pending {
			run = append(run, c)
		}
	}
	require.Len(t, run, len(s.Cases)-1, "one case waits on a fix to the shipped vela-addon")
	for i, o := range s.Evaluate(run, RunOptions{}) {
		require.Empty(t, o.Failures, run[i].Name)
	}
}

func TestSourceExecLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"a field no source reads": {
			src:  `context: stepName: "deploy"`,
			want: `context.stepName is not readable by a source consumed from component`,
		},
		"a consumer the definition does not allow": {
			src:  `consumer: "trait"`,
			want: `strict is consumable from component only, not trait`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			def, err := os.ReadFile("testdata/sources/strict.cue")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "strict.cue"), def, 0o600))
			file := filepath.Join(dir, "strict_test.cue")
			require.NoError(t, os.WriteFile(file, []byte(`import "vela/test"

"case": test.#SourceExec & {
	definition: "strict"
	parameter: count: 1
	`+tc.src+`
}
`), 0o600))
			suites, err := Load(file)
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.want)
		})
	}
}

func TestSourceExecReadsConfigs(t *testing.T) {
	testCluster(t)
	s := loadOne(t, "testdata/sources/configs_test.cue")
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures, s.Cases[i].Name)
	}
}
