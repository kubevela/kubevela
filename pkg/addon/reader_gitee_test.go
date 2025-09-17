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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/google/go-github/v32/github"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/utils"
)

func giteeSetup() (client *Client, mux *http.ServeMux, teardown func()) {
	// mux is the HTTP request multiplexer used with the test server.
	mux = http.NewServeMux()

	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseURLPath+"/", http.StripPrefix(baseURLPath, mux))

	// server is a test HTTP server used to provide mock API responses.
	server := httptest.NewServer(apiHandler)

	// client is the GitHub client being tested and is
	// configured to use test server.
	URL, _ := url.Parse(server.URL + baseURLPath + "/")
	httpClient := &http.Client{}
	client = NewGiteeClient(httpClient, URL)

	return client, mux, server.Close
}

func TestGiteeReader(t *testing.T) {
	client, mux, teardown := giteeSetup()
	giteePattern := "/repos/o/r/contents/"
	mux.HandleFunc(giteePattern, func(rw http.ResponseWriter, req *http.Request) {
		queryPath := strings.TrimPrefix(req.URL.Path, giteePattern)
		localPath := path.Join(testdataPrefix, queryPath)
		file, err := testdata.ReadFile(localPath)
		// test if it's a file
		if err == nil {
			content := &github.RepositoryContent{Type: String("file"), Name: String(path.Base(queryPath)), Size: Int(len(file)), Encoding: String(""), Path: String(queryPath), Content: String(string(file))}
			res, _ := json.Marshal(content)
			rw.Write(res)
			return
		}

		// otherwise, it could be directory
		dir, err := testdata.ReadDir(localPath)
		if err == nil {
			contents := make([]*github.RepositoryContent, 0)
			for _, item := range dir {
				tp := "file"
				if item.IsDir() {
					tp = "dir"
				}
				contents = append(contents, &github.RepositoryContent{Type: String(tp), Name: String(item.Name()), Path: String(path.Join(queryPath, item.Name()))})
			}
			dRes, _ := json.Marshal(contents)
			rw.Write(dRes)
			return
		}

		rw.Write([]byte("invalid gitee query"))
	})
	defer teardown()

	gith := &giteeHelper{
		Client: client,
		Meta: &utils.Content{GiteeContent: utils.GiteeContent{
			Owner: "o",
			Repo:  "r",
		}},
	}
	var r AsyncReader = &giteeReader{gith}
	_, err := r.ReadFile("example/metadata.yaml")
	assert.NoError(t, err)

}

func TestNewGiteeClient(t *testing.T) {
	defaultURL, _ := url.Parse(DefaultGiteeURL)

	testCases := map[string]struct {
		httpClient *http.Client
		baseURL    *url.URL
		wantClient *http.Client
		wantURL    *url.URL
	}{
		"Nil inputs": {
			httpClient: nil,
			baseURL:    nil,
			wantClient: &http.Client{},
			wantURL:    defaultURL,
		},
		"Custom inputs": {
			httpClient: &http.Client{Timeout: 10},
			baseURL:    &url.URL{Host: "my-gitee.com"},
			wantClient: &http.Client{Timeout: 10},
			wantURL:    &url.URL{Host: "my-gitee.com"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client := NewGiteeClient(tc.httpClient, tc.baseURL)
			assert.Equal(t, tc.wantClient.Timeout, client.Client.Timeout)
			assert.Equal(t, tc.wantURL.Host, client.BaseURL.Host)
		})
	}
}

func TestGiteeReaderRelativePath(t *testing.T) {
	testCases := map[string]struct {
		basePath     string
		itemPath     string
		expectedPath string
	}{
		"No base path": {
			basePath:     "",
			itemPath:     "fluxcd/metadata.yaml",
			expectedPath: "fluxcd/metadata.yaml",
		},
		"With base path": {
			basePath:     "addons",
			itemPath:     "addons/fluxcd/metadata.yaml",
			expectedPath: "fluxcd/metadata.yaml",
		},
		"With deep base path": {
			basePath:     "official/addons",
			itemPath:     "official/addons/fluxcd/template.cue",
			expectedPath: "fluxcd/template.cue",
		},
		"Item at root of base path": {
			basePath:     "addons",
			itemPath:     "addons/README.md",
			expectedPath: "README.md",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gith := &giteeHelper{
				Meta: &utils.Content{GiteeContent: utils.GiteeContent{
					Path: tc.basePath,
				}},
			}
			r := &giteeReader{h: gith}
			item := &github.RepositoryContent{Path: &tc.itemPath}

			result := r.RelativePath(item)
			assert.Equal(t, tc.expectedPath, result)
		})
	}
}

func TestGiteeReader_ListAddonMeta(t *testing.T) {
	client, mux, teardown := giteeSetup()
	defer teardown()

	giteePattern := "/repos/o/r/contents/"
	mux.HandleFunc(giteePattern, func(rw http.ResponseWriter, req *http.Request) {
		var contents []*github.RepositoryContent
		queryPath := strings.TrimPrefix(req.URL.Path, giteePattern)

		switch queryPath {
		case "": // Root directory
			contents = []*github.RepositoryContent{
				{Type: String("dir"), Name: String("fluxcd"), Path: String("fluxcd")},
				{Type: String("dir"), Name: String("velaux"), Path: String("velaux")},
				{Type: String("file"), Name: String("README.md"), Path: String("README.md")},
			}
		case "fluxcd":
			contents = []*github.RepositoryContent{
				{Type: String("file"), Name: String("metadata.yaml"), Path: String("fluxcd/metadata.yaml")},
				{Type: String("dir"), Name: String("resources"), Path: String("fluxcd/resources")},
			}
		case "fluxcd/resources":
			contents = []*github.RepositoryContent{
				{Type: String("file"), Name: String("parameter.cue"), Path: String("fluxcd/resources/parameter.cue")},
			}
		case "velaux":
			contents = []*github.RepositoryContent{
				{Type: String("file"), Name: String("metadata.yaml"), Path: String("velaux/metadata.yaml")},
			}
		default:
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		res, _ := json.Marshal(contents)
		rw.Write(res)
	})

	gith := &giteeHelper{
		Client: client,
		Meta: &utils.Content{GiteeContent: utils.GiteeContent{
			Owner: "o",
			Repo:  "r",
		}},
	}
	r := &giteeReader{h: gith}

	meta, err := r.ListAddonMeta()
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, 2, len(meta), "Expected to find 2 addons, root files should be ignored")

	t.Run("fluxcd addon discovery", func(t *testing.T) {
		addon, ok := meta["fluxcd"]
		assert.True(t, ok, "fluxcd addon should be discovered")
		assert.Equal(t, "fluxcd", addon.Name)

		// Should find 2 items recursively: metadata.yaml and resources/parameter.cue
		assert.Equal(t, 2, len(addon.Items), "fluxcd should contain 2 files")

		foundPaths := make(map[string]bool)
		for _, item := range addon.Items {
			foundPaths[item.GetPath()] = true
		}
		assert.True(t, foundPaths["fluxcd/metadata.yaml"], "should find fluxcd/metadata.yaml")
		assert.True(t, foundPaths["fluxcd/resources/parameter.cue"], "should find fluxcd/resources/parameter.cue")
	})

	t.Run("velaux addon discovery", func(t *testing.T) {
		addon, ok := meta["velaux"]
		assert.True(t, ok, "velaux addon should be discovered")
		assert.Equal(t, "velaux", addon.Name)
		assert.Equal(t, 1, len(addon.Items), "velaux should contain 1 file")
		assert.Equal(t, "velaux/metadata.yaml", addon.Items[0].GetPath())
	})
}
