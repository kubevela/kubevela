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

package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageModuleRefusesASymlinkInTheTree(t *testing.T) {
	dir := minimalModuleDir(t)
	outside := filepath.Join(t.TempDir(), "secret.yaml")
	require.NoError(t, os.WriteFile(outside, []byte("kind: Secret\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "v1", "auxiliary", "link.yaml")))

	_, err := PackageModule(dir, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `package module "minimal"`)
	assert.Contains(t, err.Error(), "contains a symlink at v1/auxiliary/link.yaml", "packaging must not reach outside the tree")
}

func TestPackageModuleDropsGitDirectoriesAtAnyDepth(t *testing.T) {
	dir := minimalModuleDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "v1", ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "v1", ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))

	art, err := PackageModule(dir, "")
	require.NoError(t, err)
	files := archiveFS(t, art.Archive, "minimal").(fstestMapFS)
	for name := range files {
		assert.False(t, strings.HasPrefix(name, ".git/") || strings.Contains(name, "/.git/"), "repository state %q must not be published", name)
	}
	_, kept := files["v1/definitions/widget.yaml"]
	assert.True(t, kept, "the module content itself is still there")
}
