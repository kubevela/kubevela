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

func TestNewAsyncReader(t *testing.T) {
	testCases := map[string]struct {
		baseURL  string
		bucket   string
		repo     string
		subPath  string
		token    string
		rdType   ReaderType
		wantType interface{}
		wantErr  bool
	}{
		"git type": {
			baseURL:  "https://github.com/kubevela/catalog",
			subPath:  "addons",
			rdType:   gitType,
			wantType: &gitReader{},
			wantErr:  false,
		},
		"gitee type": {
			baseURL:  "https://gitee.com/kubevela/catalog",
			subPath:  "addons",
			rdType:   giteeType,
			wantType: &giteeReader{},
			wantErr:  false,
		},
		"oss type": {
			baseURL:  "oss-cn-hangzhou.aliyuncs.com",
			bucket:   "kubevela-addons",
			rdType:   ossType,
			wantType: &ossReader{},
			wantErr:  false,
		},
		"invalid url": {
			baseURL: "://invalid-url",
			rdType:  gitType,
			wantErr: true,
		},
		"invalid type": {
			baseURL: "https://github.com/kubevela/catalog",
			rdType:  "invalid",
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Note: This test does not cover the gitlab case as it requires a live API call
			// or a complex mock setup, which is beyond the scope of this unit test.
			if tc.rdType == gitlabType {
				t.Skip("Skipping gitlab test in this unit test suite.")
			}

			reader, err := NewAsyncReader(tc.baseURL, tc.bucket, tc.repo, tc.subPath, tc.token, tc.rdType)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.IsType(t, tc.wantType, reader)
			}
		})
	}
}

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

func TestTokenSource(t *testing.T) {
	t.Run("GitAddonSource", func(t *testing.T) {
		source := &GitAddonSource{}
		assert.Equal(t, "", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetToken("test-token")
		assert.Equal(t, "test-token", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetTokenSecretRef("test-secret")
		assert.Equal(t, "test-secret", source.GetTokenSecretRef())
		assert.Equal(t, "", source.GetToken())
	})

	t.Run("GiteeAddonSource", func(t *testing.T) {
		source := &GiteeAddonSource{}
		assert.Equal(t, "", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetToken("test-token")
		assert.Equal(t, "test-token", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetTokenSecretRef("test-secret")
		assert.Equal(t, "test-secret", source.GetTokenSecretRef())
		assert.Equal(t, "", source.GetToken())
	})

	t.Run("GitlabAddonSource", func(t *testing.T) {
		source := &GitlabAddonSource{}
		assert.Equal(t, "", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetToken("test-token")
		assert.Equal(t, "test-token", source.GetToken())
		assert.Equal(t, "", source.GetTokenSecretRef())

		source.SetTokenSecretRef("test-secret")
		assert.Equal(t, "test-secret", source.GetTokenSecretRef())
		assert.Equal(t, "", source.GetToken())
	})
}
