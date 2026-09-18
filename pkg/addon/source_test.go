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
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockItem implements the Item interface for testing
type mockItem struct {
	path     string
	name     string
	typeName string
}

func (m mockItem) GetType() string { return m.typeName }
func (m mockItem) GetPath() string { return m.path }
func (m mockItem) GetName() string { return m.name }

// mockReader implements the AsyncReader interface for testing
type mockReader struct{}

func (m mockReader) ListAddonMeta() (map[string]SourceMeta, error) { return nil, nil }
func (m mockReader) ReadFile(path string) (string, error)          { return "", nil }
func (m mockReader) RelativePath(item Item) string                 { return item.GetPath() }

func TestClassifyItemByPattern(t *testing.T) {
	addonName := "my-addon"
	meta := &SourceMeta{
		Name: addonName,
		Items: []Item{
			mockItem{path: "my-addon/metadata.yaml"},
			mockItem{path: "my-addon/template.cue"},
			mockItem{path: "my-addon/definitions/def.cue"},
			mockItem{path: "my-addon/resources/res.yaml"},
			mockItem{path: "my-addon/schemas/schema.cue"},
			mockItem{path: "my-addon/views/view.cue"},
			mockItem{path: "my-addon/some-other-file.txt"}, // Should be ignored
		},
	}

	r := mockReader{}
	classified := ClassifyItemByPattern(meta, r)

	assert.Contains(t, classified, MetadataFileName)
	assert.Len(t, classified[MetadataFileName], 1)

	assert.Contains(t, classified, AppTemplateCueFileName)
	assert.Len(t, classified[AppTemplateCueFileName], 1)

	assert.Contains(t, classified, DefinitionsDirName)
	assert.Len(t, classified[DefinitionsDirName], 1)

	assert.Contains(t, classified, ResourcesDirName)
	assert.Len(t, classified[ResourcesDirName], 1)

	assert.Contains(t, classified, DefSchemaName)
	assert.Len(t, classified[DefSchemaName], 1)

	assert.Contains(t, classified, ViewDirName)
	assert.Len(t, classified[ViewDirName], 1)

	assert.NotContains(t, classified, "some-other-file.txt")
}
