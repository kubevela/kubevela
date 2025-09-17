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

package addon

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalReader(t *testing.T) {
	r := localReader{name: "local", dir: "./testdata/local"}

	t.Run("ListAddonMeta", func(t *testing.T) {
		m, err := r.ListAddonMeta()
		assert.NoError(t, err)
		assert.NotNil(t, m["local"])
		assert.Equal(t, 2, len(m["local"].Items))

		// Check that the correct files are found, regardless of order.
		foundPaths := make(map[string]bool)
		for _, item := range m["local"].Items {
			// Normalize path separators for consistent checking
			foundPaths[filepath.ToSlash(item.GetPath())] = true
		}
		assert.True(t, foundPaths[filepath.ToSlash("testdata/local/metadata.yaml")])
		assert.True(t, foundPaths[filepath.ToSlash("testdata/local/resources/parameter.cue")])
	})

	t.Run("ReadFile", func(t *testing.T) {
		t.Run("read root file", func(t *testing.T) {
			file, err := r.ReadFile("metadata.yaml")
			assert.NoError(t, err)
			assert.Equal(t, file, metaFile)
		})
		t.Run("read nested file", func(t *testing.T) {
			file, err := r.ReadFile("resources/parameter.cue")
			assert.NoError(t, err)
			assert.True(t, strings.Contains(file, parameterFile))
		})
	})
}

func TestLocalReader_RelativePath(t *testing.T) {
	testCases := map[string]struct {
		dir       string
		addonName string
		itemPath  string
		expected  string
	}{
		"item in root": {
			dir:       "./testdata/local",
			addonName: "my-addon",
			itemPath:  filepath.Join("./testdata/local", "metadata.yaml"),
			expected:  filepath.Join("my-addon", "metadata.yaml"),
		},
		"item in subdirectory": {
			dir:       "./testdata/local",
			addonName: "my-addon",
			itemPath:  filepath.Join("./testdata/local", "resources", "parameter.cue"),
			expected:  filepath.Join("my-addon", "resources", "parameter.cue"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := localReader{name: tc.addonName, dir: tc.dir}
			item := OSSItem{path: tc.itemPath}
			result := r.RelativePath(item)
			assert.Equal(t, tc.expected, result)
		})
	}
}

const (
	metaFile = `name: test-local-addon
version: 1.0.0
description: This is a addon for test when install addon from local
icon: https://www.terraform.io/assets/images/logo-text-8c3ba8a6.svg
url: https://terraform.io/

tags: []

deployTo:
  control_plane: true
  runtime_cluster: false

dependencies: []

invisible: false`

	parameterFile = `parameter: {
	// test wrong parameter
	example: *"default"
}`
)
