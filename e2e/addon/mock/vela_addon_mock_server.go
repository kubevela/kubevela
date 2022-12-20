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

package main

import (
	"embed"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/oam-dev/kubevela/e2e/addon/mock/utils"
	"github.com/oam-dev/kubevela/pkg/addon"
)

var (
	//go:embed testdata
	testData embed.FS
	paths    []struct {
		path   string
		length int64
	}
)

func main() {
	err := utils.ApplyMockServerConfig()
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", ossHandler)
	http.HandleFunc("/helm/", helmHandler)
	err = http.ListenAndServe(fmt.Sprintf(":%d", utils.Port), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

var ossHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
	queryPath := strings.TrimPrefix(req.URL.Path, "/")

	if strings.Contains(req.URL.RawQuery, "prefix") {
		prefix := req.URL.Query().Get("prefix")
		res := addon.ListBucketResult{
			Files: []addon.File{},
			Count: 0,
		}
		for _, p := range paths {
			if strings.HasPrefix(p.path, prefix) {
				res.Files = append(res.Files, addon.File{Name: p.path, Size: int(p.length)})
				res.Count++
			}
		}
		data, err := xml.Marshal(res)
		error := map[string]error{"error": err}
		// Make and parse the data
		t, err := template.New("").Parse(string(data))
		if err != nil {
			// Render the data
			t.Execute(rw, error)
		}
		// Render the data
		t.Execute(rw, data)

	} else {
		found := false
		for _, p := range paths {
			if queryPath == p.path {
				file, err := testData.ReadFile(path.Join("testdata", queryPath))
				error := map[string]error{"error": err}
				// Make and parse the data
				t, err := template.New("").Parse(string(file))
				if err != nil {
					// Render the data
					t.Execute(rw, error)
				}
				found = true
				t.Execute(rw, file)
				break
			}
		}
		if !found {
			nf := "not found"
			t, _ := template.New("").Parse(nf)
			t.Execute(rw, nf)
		}
	}
}

var helmHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
	switch {
	case strings.Contains(req.URL.Path, "index.yaml"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/index.yaml")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "fluxcd-test-version-1.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/fluxcd-test-version-1.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "fluxcd-test-version-2.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/fluxcd-test-version-2.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "vela-workflow-v0.3.5.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/vela-workflow-v0.3.5.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "foo-v1.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/foo-v1.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "bar-v1.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/bar-v1.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "bar-v2.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/bar-v2.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	case strings.Contains(req.URL.Path, "mock-be-dep-addon-v1.0.0.tgz"):
		file, err := os.ReadFile("./e2e/addon/mock/testrepo/helm-repo/mock-be-dep-addon-v1.0.0.tgz")
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		rw.Write(file)
	}

}

func init() {
	_ = fs.WalkDir(testData, "testdata", func(path string, d fs.DirEntry, err error) error {
		path = strings.TrimPrefix(path, "testdata/")
		path = strings.TrimPrefix(path, "testdata")

		info, _ := d.Info()
		size := info.Size()
		if path == "" {
			return nil
		}
		if size == 0 {
			path += "/"
		}
		paths = append(paths, struct {
			path   string
			length int64
		}{path: path, length: size})
		return nil
	})
}
