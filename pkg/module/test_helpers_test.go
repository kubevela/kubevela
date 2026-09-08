/*
Copyright 2021 The KubeVela Authors.

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

package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func minimalModuleDir(t *testing.T) string {
	t.Helper()
	return writeModuleTree(t, map[string]string{
		"_module.cue": "module:  \"minimal\"\nversion: \"1.0.0\"\n",
		"auxiliary/xrd.yaml": `apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: xwidgets.example.com
`,
		"v1/_version.cue": "apiVersion: \"v1\"\n",
		"v1/auxiliary/composition.yaml": `apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: widgets.example.com
`,
		"v1/definitions/widget.yaml": `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: widget
  namespace: vela-system
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
`,
	})
}

// writeModuleTree writes files (keyed by slash-separated relative path) into a
// fresh temp directory and returns it.
func writeModuleTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return dir
}
