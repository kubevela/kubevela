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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
	subPath := "sub-addons"
	reader, err := NewAsyncReader("ep-beijing.com", "bucket", subPath, "", ossType)

	assert.NoError(t, err)

	o, ok := reader.(*ossReader)
	assert.Equal(t, ok, true)
	var testFiles = []File{
		{
			Name: "sub-addons/fluxcd",
			Size: 0,
		},
		{
			Name: "sub-addons/fluxcd/metadata.yaml",
			Size: 100,
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
		{
			Name: "sub-addons/example/metadata.yaml",
			Size: 100,
		},
	}
	var expectItemCase = []SourceMeta{
		{
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   DirType,
					path: "fluxcd",
					name: "fluxcd",
				},
				&OSSItem{
					tp:   DirType,
					path: "example",
					name: "example",
				},
			},
		},
		{
			Name: "example",
			Items: []Item{
				&OSSItem{
					tp:   DirType,
					path: "fluxcd",
					name: "fluxcd",
				},
				&OSSItem{
					tp:   DirType,
					path: "example",
					name: "example",
				},
			},
		},
	}
	addonMetas := o.convertOSSFiles2Addons(testFiles, subPath)
	assert.Equal(t, expectItemCase, addonMetas)

}

func TestAliyunOSS(t *testing.T) {

	var source Source = &OSSAddonSource{
		Endpoint: "https://addons.kubevela.net",
		Bucket:   "",
		Path:     "",
	}
	addons, err := source.ListRegistryMeta()
	if err != nil {
		t.Error(err)
	}

	for _, d := range addons {
		ui, err := source.GetUIMeta(&d, UIMetaOptions)
		if err != nil {
			t.Error(err)
		}

		pk, err := source.GetInstallPackage(&d, ui)
		if err != nil {
			t.Error(err)
		}
		fmt.Println(pk.Name, "XXXXXX", pk.CUETemplates, "YYYYYYYY", pk.YAMLTemplates)
	}

}
