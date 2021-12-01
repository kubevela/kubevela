package addon

import (
	"encoding/xml"
	"gotest.tools/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
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
	var reader AsyncReader
	var err error
	var server *httptest.Server
	server = httptest.NewServer(ossHandler)
	defer server.Close()

	reader, err = NewAsyncReader(server.URL, "", "", ossType)

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
