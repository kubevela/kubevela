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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModuleInitScaffoldsExpectedFiles verifies the command writes the full
// mandatory file set with the module name pre-filled.
func TestModuleInitScaffoldsExpectedFiles(t *testing.T) {
	tmp := t.TempDir()
	o := &moduleInitOptions{name: "s3", path: tmp}
	require.NoError(t, o.run(&bytes.Buffer{}))

	root := filepath.Join(tmp, "s3")
	for _, rel := range []string{
		"_module.cue",
		"README.md",
		"v1/_version.cue",
		"v1/definitions/example.cue",
	} {
		_, err := os.Stat(filepath.Join(root, rel))
		assert.NoError(t, err, "expected scaffolded file %s", rel)
	}

	// auxiliary/ and v1/auxiliary/ are created empty; README.md documents
	// what goes in each rather than a placeholder object living there.
	for _, dir := range []string{"auxiliary", "v1/auxiliary"} {
		info, err := os.Stat(filepath.Join(root, dir))
		require.NoError(t, err, "expected empty directory %s", dir)
		assert.True(t, info.IsDir())
		entries, err := os.ReadDir(filepath.Join(root, dir))
		require.NoError(t, err)
		assert.Empty(t, entries, "%s should be created empty", dir)
	}

	mod, err := os.ReadFile(filepath.Join(root, "_module.cue"))
	require.NoError(t, err)
	assert.Contains(t, string(mod), `module:`)
	assert.Contains(t, string(mod), `"s3"`)

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readme), "s3", "the module name is substituted into the README")
	assert.Contains(t, string(readme), "auxiliary/", "the README explains the auxiliary folders")
}

// TestModuleInitNonEmptyDirFails refuses to write into an existing non-empty
// directory, with no override.
func TestModuleInitNonEmptyDirFails(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "s3")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "existing.txt"), []byte("keep"), 0o644))

	o := &moduleInitOptions{name: "s3", path: tmp}
	err := o.run(&bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// The pre-existing file is untouched.
	data, readErr := os.ReadFile(filepath.Join(target, "existing.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(data))
}

// TestModuleInitInvalidName rejects a name that is not a DNS-1123 label.
func TestModuleInitInvalidName(t *testing.T) {
	tmp := t.TempDir()
	o := &moduleInitOptions{name: "S3_Bad", path: tmp}
	err := o.run(&bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid module name")
}
