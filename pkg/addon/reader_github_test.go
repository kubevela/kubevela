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
	"embed"
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

const (
	// baseURLPath is a non-empty Client.BaseURL path to use during tests,
	// to ensure relative URLs are used for all endpoints. See issue #752.
	baseURLPath = "/api-v3"
)

var (
	//go:embed testdata
	testdata       embed.FS
	testdataPrefix = "testdata"
)

func setup() (client *github.Client, mux *http.ServeMux, teardown func()) {
	// mux is the HTTP request multiplexer used with the test server.
	mux = http.NewServeMux()

	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseURLPath+"/", http.StripPrefix(baseURLPath, mux))

	// server is a test HTTP server used to provide mock API responses.
	server := httptest.NewServer(apiHandler)

	// client is the GitHub client being tested and is
	// configured to use test server.
	client = github.NewClient(nil)
	URL, _ := url.Parse(server.URL + baseURLPath + "/")
	client.BaseURL = URL
	client.UploadURL = URL

	return client, mux, server.Close
}

func TestGitHubReader(t *testing.T) {
	client, mux, teardown := setup()
	githubPattern := "/repos/o/r/contents/"
	mux.HandleFunc(githubPattern, func(rw http.ResponseWriter, req *http.Request) {
		queryPath := strings.TrimPrefix(req.URL.Path, githubPattern)
		localPath := path.Join(testdataPrefix, queryPath)
		file, err := testdata.ReadFile(localPath)
		// test if it's a file
		if err == nil {
			content := &github.RepositoryContent{Type: String("file"), Name: String(path.Base(queryPath)), Size: Int(len(file)), Encoding: String(""), Path: String(queryPath), Content: String(string(file))}
			res, _ := json.Marshal(content)
			rw.Write(res)
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
		}

		rw.Write([]byte("invalid github query"))
	})
	defer teardown()

	gith := &gitHelper{
		Client: client,
		Meta: &utils.Content{GithubContent: utils.GithubContent{
			Owner: "o",
			Repo:  "r",
		}},
	}
	var r AsyncReader = &gitReader{gith}
	_, err := r.ReadFile("example/metadata.yaml")
	assert.NoError(t, err)

}

// Int is a helper routine that allocates a new int value
// to store v and returns a pointer to it.
func Int(v int) *int { return &v }

// String is a helper routine that allocates a new string value
// to store v and returns a pointer to it.
func String(v string) *string { return &v }
