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
	reader, err := NewAsyncReader("ep-beijing.com", "bucket", "", subPath, "", ossType)

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
	var expectItemCase = map[string]SourceMeta{
		"fluxcd": {
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/definitions/helm-release.yaml",
					name: "helm-release.yaml",
				},
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/metadata.yaml",
					name: "metadata.yaml",
				},
			},
		},
		"example": {
			Name: "example",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "example/metadata.yaml",
					name: "metadata.yaml",
				},
				&OSSItem{
					tp:   FileType,
					path: "example/resources/configmap.yaml",
					name: "configmap.yaml",
				},
			},
		},
	}
	addonMetas := o.convertOSSFiles2Addons(testFiles)
	assert.Equal(t, expectItemCase, addonMetas)

}

func TestSafeCopy(t *testing.T) {
	var git *GitAddonSource
	sgit := git.SafeCopy()
	assert.Nil(t, sgit)
	git = &GitAddonSource{URL: "http://github.com/kubevela", Path: "addons", Token: "123456"}
	sgit = git.SafeCopy()
	assert.Empty(t, sgit.Token)
	assert.Equal(t, "http://github.com/kubevela", sgit.URL)
	assert.Equal(t, "addons", sgit.Path)

	var gitee *GiteeAddonSource
	sgitee := gitee.SafeCopy()
	assert.Nil(t, sgitee)
	gitee = &GiteeAddonSource{URL: "http://gitee.com/kubevela", Path: "addons", Token: "123456"}
	sgitee = gitee.SafeCopy()
	assert.Empty(t, sgitee.Token)
	assert.Equal(t, "http://gitee.com/kubevela", sgitee.URL)
	assert.Equal(t, "addons", sgitee.Path)

	var gitlab *GitlabAddonSource
	sgitlab := gitlab.SafeCopy()
	assert.Nil(t, sgitlab)
	gitlab = &GitlabAddonSource{URL: "http://gitlab.com/kubevela", Repo: "vela", Path: "addons", Token: "123456"}
	sgitlab = gitlab.SafeCopy()
	assert.Empty(t, sgitlab.Token)
	assert.Equal(t, "http://gitlab.com/kubevela", sgitlab.URL)
	assert.Equal(t, "addons", sgitlab.Path)
	assert.Equal(t, "vela", sgitlab.Repo)

	var helm *HelmSource
	shelm := helm.SafeCopy()
	assert.Nil(t, shelm)
	helm = &HelmSource{URL: "https://hub.vela.com/chartrepo/addons", Username: "user123", Password: "pass456"}
	shelm = helm.SafeCopy()
	assert.Empty(t, shelm.Username)
	assert.Empty(t, shelm.Password)
	assert.Equal(t, "https://hub.vela.com/chartrepo/addons", shelm.URL)
}
