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
	"path"
	"strings"
	"testing"

	"github.com/xanzy/go-gitlab"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/utils"
)

var baseUrl = "/api/v4"

func gitlabSetup() (client *gitlab.Client, mux *http.ServeMux, teardown func()) {
	// mux is the HTTP request multiplexer used with the test server.
	mux = http.NewServeMux()

	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseUrl+"/", http.StripPrefix(baseUrl, mux))

	// server is a test HTTP server used to provide mock API responses.
	server := httptest.NewServer(apiHandler)

	// client is the GitHub client being tested and is
	// configured to use test server.

	client, err := gitlab.NewClient("", gitlab.WithBaseURL(server.URL+baseUrl+"/"))
	if err != nil {
		return
	}

	return client, mux, server.Close
}

func TestGitlabReader(t *testing.T) {
	client, mux, teardown := gitlabSetup()
	gitlabPattern := "/projects/9999/repository/files/"
	mux.HandleFunc(gitlabPattern, func(rw http.ResponseWriter, req *http.Request) {
		queryPath := strings.TrimPrefix(req.URL.Path, gitlabPattern)
		localPath := path.Join(testdataPrefix, queryPath)
		file, err := testdata.ReadFile(localPath)
		// test if it's a file
		if err == nil {
			content := &gitlab.File{
				FilePath: localPath,
				FileName: path.Base(queryPath),
				Size:     *Int(len(file)),
				Encoding: "base64",
				Ref:      "master",
				Content:  base64.StdEncoding.EncodeToString(file),
			}
			res, _ := json.Marshal(content)
			rw.Write(res)
			return
		}

		// otherwise, it could be directory
		dir, err := testdata.ReadDir(localPath)
		if err == nil {
			contents := make([]*gitlab.TreeNode, 0)
			for _, item := range dir {
				tp := "file"
				if item.IsDir() {
					tp = "dir"
				}
				contents = append(contents, &gitlab.TreeNode{
					ID:   "",
					Name: item.Name(),
					Type: tp,
					Path: localPath + "/" + item.Name(),
					Mode: "",
				})
			}
			dRes, _ := json.Marshal(contents)
			rw.Write(dRes)
			return
		}

		rw.Write([]byte("invalid gitlab query"))
	})
	defer teardown()

	gith := &gitlabHelper{
		Client: client,
		Meta: &utils.Content{GitlabContent: utils.GitlabContent{
			PId: 9999,
		}},
	}
	var r AsyncReader = &gitlabReader{gith}
	_, err := r.ReadFile("example/metadata.yaml")
	assert.NoError(t, err)
}
