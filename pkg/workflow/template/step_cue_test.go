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

package template

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// workflowstepDefDir is the directory of the shipped workflow step definitions.
func workflowstepDefDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// pkg/workflow/template -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../.."))
	return filepath.Join(root, "vela-templates/definitions/internal/workflowstep")
}

// readWorkflowStepCue reads a shipped workflow step definition's CUE.
func readWorkflowStepCue(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(workflowstepDefDir(t), name)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(data)
}
