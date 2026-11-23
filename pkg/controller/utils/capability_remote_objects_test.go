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

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

// moduleRepo makes a local Git repository holding files, and symlinks (path to target), in one
// commit, and returns its file:// URL.
func moduleRepo(t *testing.T, files map[string]string, symlinks map[string]string) string {
	t.Helper()
	return moduleRepoWithMessage(t, files, symlinks, "module")
}

// moduleRepoWithMessage is moduleRepo with the commit message given.
func moduleRepoWithMessage(t *testing.T, files map[string]string, symlinks map[string]string, message string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	for path, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(content), 0o600))
		_, err = wt.Add(path)
		require.NoError(t, err)
	}
	for path, target := range symlinks {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755))
		require.NoError(t, os.Symlink(target, filepath.Join(dir, path)))
		_, err = wt.Add(path)
		require.NoError(t, err)
	}
	_, err = wt.Commit(message, &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()}})
	require.NoError(t, err)
	return "file://" + dir
}

const moduleVars = `variable "region" {
  type = string
}
`

// A remote module is read from Git's objects, never checked out: a repository of large files
// costs only its compressed pack on disk, and only variables.tf or main.tf is ever read.
func TestARemoteModuleIsNotCheckedOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	url := moduleRepo(t, map[string]string{
		"mod/variables.tf": moduleVars,
		"big.bin":          strings.Repeat("\x00", 4<<20),
	}, nil)

	conf, err := GetTerraformConfigurationFromRemote("big", url, "mod", nil)
	require.NoError(t, err)
	require.Equal(t, moduleVars, conf)
	cache := filepath.Join(home, ".vela", "terraform", "big")
	_, err = os.Stat(filepath.Join(cache, "big.bin"))
	require.True(t, os.IsNotExist(err), "no file of the module is written out")
	_, err = os.Stat(filepath.Join(cache, "mod", "variables.tf"))
	require.True(t, os.IsNotExist(err))

	conf, err = GetTerraformConfigurationFromRemote("big", url, "mod", nil)
	require.NoError(t, err, "the cached clone serves the next read")
	require.Equal(t, moduleVars, conf)
}

func TestARemoteModuleIsReadOnlyFromItsOwnTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Run("falls back to main.tf", func(t *testing.T) {
		conf, err := GetTerraformConfigurationFromRemote("main", moduleRepo(t, map[string]string{"main.tf": moduleVars}, nil), "", nil)
		require.NoError(t, err)
		require.Equal(t, moduleVars, conf)
	})
	t.Run("neither file", func(t *testing.T) {
		_, err := GetTerraformConfigurationFromRemote("none", moduleRepo(t, map[string]string{"README.md": "x"}, nil), "", nil)
		require.ErrorContains(t, err, "failed to find main.tf or variables.tf")
	})
	t.Run("a symlinked variables.tf", func(t *testing.T) {
		_, err := GetTerraformConfigurationFromRemote("link", moduleRepo(t, nil, map[string]string{"variables.tf": "/etc/passwd"}), "", nil)
		require.ErrorContains(t, err, "refusing to read symlinked Terraform configuration")
	})
	t.Run("a path out of the module", func(t *testing.T) {
		for _, path := range []string{"../other", "/../other", "mod/../../other"} {
			_, err := GetTerraformConfigurationFromRemote("escape", moduleRepo(t, map[string]string{"mod/variables.tf": moduleVars}, nil), path, nil)
			require.ErrorContains(t, err, "outside the module", path)
		}
	})
	t.Run("a leading slash means the module's root", func(t *testing.T) {
		conf, err := GetTerraformConfigurationFromRemote("rooted", moduleRepo(t, map[string]string{"mod/variables.tf": moduleVars}, nil), "/mod", nil)
		require.NoError(t, err)
		require.Equal(t, moduleVars, conf)
	})
	t.Run("a tree too large to decode", func(t *testing.T) {
		restore := maxTreeObjectBytes
		maxTreeObjectBytes = 512
		t.Cleanup(func() { maxTreeObjectBytes = restore })
		files := map[string]string{"variables.tf": moduleVars}
		for i := 0; i < 40; i++ {
			files[fmt.Sprintf("padding-%02d.txt", i)] = "x"
		}
		_, err := GetTerraformConfigurationFromRemote("widetree", moduleRepo(t, files, nil), "", nil)
		require.ErrorContains(t, err, "too large")
	})
	t.Run("a commit too large to decode", func(t *testing.T) {
		restore := maxCommitObjectBytes
		maxCommitObjectBytes = 512
		t.Cleanup(func() { maxCommitObjectBytes = restore })
		repo := moduleRepoWithMessage(t, map[string]string{"variables.tf": moduleVars}, nil, strings.Repeat("m", 4096))
		_, err := GetTerraformConfigurationFromRemote("bigcommit", repo, "", nil)
		require.ErrorContains(t, err, "too large")
	})
	t.Run("a symlinked directory", func(t *testing.T) {
		repo := moduleRepo(t, map[string]string{"real/variables.tf": moduleVars}, map[string]string{"mod": "real"})
		_, err := GetTerraformConfigurationFromRemote("dirlink", repo, "mod", nil)
		require.ErrorContains(t, err, "failed to find main.tf or variables.tf", "a symlink is not a directory of the module")
	})
	t.Run("a file exactly at the cap", func(t *testing.T) {
		content := strings.Repeat("#", int(maxTerraformConfigBytes))
		conf, err := GetTerraformConfigurationFromRemote("atcap", moduleRepo(t, map[string]string{"variables.tf": content}, nil), "", nil)
		require.NoError(t, err)
		require.Len(t, conf, int(maxTerraformConfigBytes))
	})
	t.Run("a file over the cap", func(t *testing.T) {
		repo := moduleRepo(t, map[string]string{"variables.tf": strings.Repeat("#", int(maxTerraformConfigBytes)+1)}, nil)
		_, err := GetTerraformConfigurationFromRemote("huge", repo, "", nil)
		require.ErrorContains(t, err, "exceeds the maximum allowed size")
	})
}
