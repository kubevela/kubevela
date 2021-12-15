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
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestPathWithParent(t *testing.T) {
	testCases := []struct {
		readPath       string
		parentPath     string
		actualReadPath string
	}{
		{
			readPath:       "example",
			parentPath:     "experimental",
			actualReadPath: "experimental/example",
		},
		{
			readPath:       "example/",
			parentPath:     "experimental",
			actualReadPath: "experimental/example/",
		},
	}
	for _, tc := range testCases {
		res := pathWithParent(tc.readPath, tc.parentPath)
		assert.Equal(t, res, tc.actualReadPath)
	}
}

func TestConvert2OssItem(t *testing.T) {
	reader, err := NewAsyncReader("ep-beijing.com", "bucket", "sub-addons", "", ossType)
	assert.NilError(t, err)
	o, ok := reader.(*ossReader)
	assert.Equal(t, ok, true)
	var testFiles = []File{
		{
			Name: "sub-addons/fluxcd",
			Size: 0,
		},
		{
			Name: "sub-addons/fluxcd/definitions/",
			Size: 0,
		},
		{
			Name: "sub-addons/fluxcd/definitions/helm-release.yaml",
			Size: 100,
		},
		{
			Name: "sub-addons/example/resources/configmap.yaml",
			Size: 100,
		},
	}
	var expectItemCase = map[string][]Item{
		"sub-addons": {
			OssItem{
				tp:   DirType,
				path: "fluxcd",
				name: "fluxcd",
			},
			OssItem{
				tp:   DirType,
				path: "example",
				name: "example",
			},
		},
		"sub-addons/fluxcd": {
			OssItem{
				tp:   DirType,
				path: "fluxcd/definitions",
				name: "definitions",
			},
		}}

	for nowPath, expectItems := range expectItemCase {
		var readFile []File
		for _, testFile := range testFiles {
			if strings.HasPrefix(testFile.Name, nowPath) {
				readFile = append(readFile, testFile)
			}
		}
		items := o.convert2OssItem(readFile, nowPath)
		assert.Equal(t, len(items), len(expectItems))
		for i, item := range items {
			ei := expectItems[i]
			assert.Equal(t, item.GetPath(), ei.GetPath())
			assert.Equal(t, item.GetName(), ei.GetName())
			assert.Equal(t, item.GetType(), ei.GetType())
		}
	}

}
