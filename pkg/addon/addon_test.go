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
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"gotest.tools/assert"
)

var paths = []string{
	"example/metadata.yaml",
	"example/readme.md",
	"example/template.yaml",
	"example/definitions/helm.yaml",
	"example/resources/configmap.cue",
	"example/resources/parameter.cue",
	"example/resources/service/source-controller.yaml",
}

var ossHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
	queryPath := strings.TrimPrefix(req.URL.Path, "/")

	if strings.HasPrefix(req.URL.RawQuery, "prefix") {
		prefix := req.URL.Query().Get("prefix")
		res := ListBucketResult{
			Files: []string{},
			Count: 0,
		}
		for _, p := range paths {
			if strings.HasPrefix(p, prefix) {
				res.Files = append(res.Files, p)
				res.Count += 1
			}
		}
		data, err := xml.Marshal(res)
		if err != nil {
			rw.Write([]byte(err.Error()))
		}
		rw.Write(data)
	} else {
		found := false
		for _, p := range paths {
			if queryPath == p {
				file, err := os.ReadFile(path.Join("testdata", queryPath))
				if err != nil {
					rw.Write([]byte(err.Error()))
				}
				found = true
				rw.Write(file)
				break
			}
		}
		if !found {
			rw.Write([]byte("not found"))
		}
	}
}

func TestGetAddon(t *testing.T) {
	server := httptest.NewServer(ossHandler)
	defer server.Close()

	reader, err := NewAsyncReader(server.URL, "", "", ossType)

	assert.NilError(t, err)

	testAddonName := "example"
	assert.NilError(t, err)
	addon, err := GetSingleAddonFromReader(reader, testAddonName, EnableLevelOptions)
	assert.NilError(t, err)
	assert.Equal(t, addon.Name, testAddonName)
	assert.Assert(t, addon.Parameters != "")
	assert.Assert(t, len(addon.Definitions) > 0)

	addons, err := GetAddonsFromReader(reader, EnableLevelOptions)
	assert.NilError(t, err)
	assert.Assert(t, len(addons) == 1)
}
