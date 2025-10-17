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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/oam-dev/kubevela/pkg/utils"
)

var baseUrl = "/api/v4"

func gitlabSetup(t *testing.T) (*gitlab.Client, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseUrl+"/", http.StripPrefix(baseUrl, mux))
	server := httptest.NewServer(apiHandler)

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(server.URL+baseUrl+"/"))
	assert.NoError(t, err)

	return client, mux, server.Close
}

func TestGitlabReader_ReadFile(t *testing.T) {
	client, mux, teardown := gitlabSetup(t)
	defer teardown()

	// The gitlab client URL-encodes the file path, so we must match the encoded path.
	mux.HandleFunc("/projects/9999/repository/files/example%2Fmetadata.yaml", func(rw http.ResponseWriter, req *http.Request) {
		content := &gitlab.File{
			Content: base64.StdEncoding.EncodeToString([]byte("hello world")),
		}
		res, err := json.Marshal(content)
		assert.NoError(t, err)
		_, err = rw.Write(res)
		assert.NoError(t, err)
	})
	mux.HandleFunc("/projects/9999/repository/files/example%2Fnot%2Ffound.yaml", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})

	gith := &gitlabHelper{
		Client: client,
		Meta:   &utils.Content{GitlabContent: utils.GitlabContent{PId: 9999, Path: "example"}},
	}
	var r AsyncReader = &gitlabReader{h: gith}

	t.Run("success case", func(t *testing.T) {
		content, err := r.ReadFile("metadata.yaml")
		assert.NoError(t, err)
		assert.Equal(t, "hello world", content)
	})

	t.Run("not found case", func(t *testing.T) {
		_, err := r.ReadFile("not/found.yaml")
		assert.Error(t, err)
	})
}

func TestGitlabReader_ListAddonMeta(t *testing.T) {
	client, mux, teardown := gitlabSetup(t)
	defer teardown()

	projectID := 9999
	projectPath := "addons"

	mux.HandleFunc("/projects/"+strconv.Itoa(projectID)+"/repository/tree", func(rw http.ResponseWriter, req *http.Request) {
		pathParam := req.URL.Query().Get("path")
		pageParam := req.URL.Query().Get("page")
		if pageParam == "" {
			pageParam = "1"
		}

		var tree []*gitlab.TreeNode

		switch pathParam {
		case projectPath:
			rw.Header().Set("X-Total-Pages", "2")
			if pageParam == "1" {
				tree = []*gitlab.TreeNode{{ID: "1", Name: "fluxcd", Type: "tree", Path: "addons/fluxcd"}}
			} else if pageParam == "2" {
				tree = []*gitlab.TreeNode{{ID: "2", Name: "velaux", Type: "tree", Path: "addons/velaux"}}
			}
		case "addons/fluxcd":
			tree = []*gitlab.TreeNode{{ID: "3", Name: "metadata.yaml", Type: "blob", Path: "addons/fluxcd/metadata.yaml"}}
		case "addons/velaux":
			tree = []*gitlab.TreeNode{{ID: "4", Name: "template.cue", Type: "blob", Path: "addons/velaux/template.cue"}}
		default:
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		res, err := json.Marshal(tree)
		assert.NoError(t, err)
		_, err = rw.Write(res)
		assert.NoError(t, err)
	})

	gith := &gitlabHelper{
		Client: client,
		Meta: &utils.Content{GitlabContent: utils.GitlabContent{
			PId:  projectID,
			Path: projectPath,
		}},
	}
	r := &gitlabReader{h: gith}

	meta, err := r.ListAddonMeta()
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, 2, len(meta))

	expectedAddons := map[string]struct {
		itemCount int
		itemPath  string
	}{
		"fluxcd": {itemCount: 1, itemPath: "fluxcd/metadata.yaml"},
		"velaux": {itemCount: 1, itemPath: "velaux/template.cue"},
	}

	for name, expected := range expectedAddons {
		t.Run(name, func(t *testing.T) {
			addon, ok := meta[name]
			assert.True(t, ok, "addon not found in result")
			assert.Equal(t, name, addon.Name)
			assert.Equal(t, expected.itemCount, len(addon.Items))
			assert.Equal(t, expected.itemPath, addon.Items[0].GetPath())
		})
	}
}

func TestGitlabReader_Getters(t *testing.T) {
	t.Run("GetRef", func(t *testing.T) {
		githWithRef := &gitlabHelper{Meta: &utils.Content{GitlabContent: utils.GitlabContent{Ref: "develop"}}}
		rWithRef := &gitlabReader{h: githWithRef}
		assert.Equal(t, "develop", rWithRef.GetRef())

		githWithoutRef := &gitlabHelper{Meta: &utils.Content{GitlabContent: utils.GitlabContent{Ref: ""}}}
		rWithoutRef := &gitlabReader{h: githWithoutRef}
		assert.Equal(t, "master", rWithoutRef.GetRef())
	})

	t.Run("GetProjectID and GetProjectPath", func(t *testing.T) {
		gith := &gitlabHelper{
			Meta: &utils.Content{GitlabContent: utils.GitlabContent{
				PId:  12345,
				Path: "my/project/path",
			}},
		}
		r := &gitlabReader{h: gith}
		assert.Equal(t, 12345, r.GetProjectID())
		assert.Equal(t, "my/project/path", r.GetProjectPath())
	})

	t.Run("RelativePath", func(t *testing.T) {
		r := &gitlabReader{}
		item := &GitLabItem{
			basePath: "addons",
			path:     "addons/fluxcd/metadata.yaml",
		}
		assert.Equal(t, "fluxcd/metadata.yaml", r.RelativePath(item))
	})
}

func TestGitLabItem(t *testing.T) {
	t.Run("Getters", func(t *testing.T) {
		item := GitLabItem{
			tp:   "blob",
			name: "metadata.yaml",
		}
		assert.Equal(t, "blob", item.GetType())
		assert.Equal(t, "metadata.yaml", item.GetName())
	})

	t.Run("GetPath", func(t *testing.T) {
		testCases := map[string]struct {
			basePath string
			fullPath string
			expected string
		}{
			"no base path": {
				basePath: "",
				fullPath: "fluxcd/metadata.yaml",
				expected: "fluxcd/metadata.yaml",
			},
			"with base path": {
				basePath: "addons",
				fullPath: "addons/fluxcd/metadata.yaml",
				expected: "fluxcd/metadata.yaml",
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				item := GitLabItem{basePath: tc.basePath, path: tc.fullPath}
				assert.Equal(t, tc.expected, item.GetPath())
			})
		}
	})
}
