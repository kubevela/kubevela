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
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chart/loader"
)

var files = []*loader.BufferedFile{
	{
		Name: "metadata.yaml",
		Data: []byte(`name: test-helm-addon
version: 1.0.0
description: This is a addon for test when install addon from helm repo
icon: https://www.terraform.io/assets/images/logo-text-8c3ba8a6.svg
url: https://terraform.io/

tags: []

deployTo:
  control_plane: true
  runtime_cluster: false

dependencies: []

invisible: false`),
	},
	{
		Name: "resources/parameter.cue",
		Data: []byte(`parameter: {
	// test wrong parameter
	example: *"default"
}`),
	},
}

func TestMemoryReader(t *testing.T) {
	m := MemoryReader{
		Name:  "fluxcd",
		Files: files,
	}

	t.Run("ListAddonMeta", func(t *testing.T) {
		meta, err := m.ListAddonMeta()
		assert.NoError(t, err)
		assert.Equal(t, 2, len(meta["fluxcd"].Items))
		// Verify the internal fileData map was populated
		_, metadataExists := m.fileData["metadata.yaml"]
		assert.True(t, metadataExists, "metadata.yaml should exist in fileData map")
		_, parameterExists := m.fileData["resources/parameter.cue"]
		assert.True(t, parameterExists, "resources/parameter.cue should exist in fileData map")
	})

	t.Run("ReadFile", func(t *testing.T) {
		// Ensure ListAddonMeta has been called to populate the internal map
		_, _ = m.ListAddonMeta()

		t.Run("read by exact name", func(t *testing.T) {
			metaFile, err := m.ReadFile("metadata.yaml")
			assert.NoError(t, err)
			assert.NotEmpty(t, metaFile)
		})

		t.Run("read by prefixed name", func(t *testing.T) {
			parameterData, err := m.ReadFile("fluxcd/resources/parameter.cue")
			assert.NoError(t, err)
			assert.NotEmpty(t, parameterData)
		})
	})
}

func TestMemoryReader_RelativePath(t *testing.T) {
	testCases := map[string]struct {
		addonName string
		itemName  string
		expected  string
	}{
		"name without prefix": {
			addonName: "my-addon",
			itemName:  "metadata.yaml",
			expected:  filepath.Join("my-addon", "metadata.yaml"),
		},
		"name with prefix": {
			addonName: "my-addon",
			itemName:  "my-addon/template.cue",
			expected:  "my-addon/template.cue",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := MemoryReader{Name: tc.addonName}
			item := OSSItem{name: tc.itemName}
			result := r.RelativePath(item)
			assert.Equal(t, tc.expected, result)
		})
	}
}
