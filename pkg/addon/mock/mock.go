package main

import (
	"encoding/xml"
	"github.com/oam-dev/kubevela/pkg/addon"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

func main() {
	http.HandleFunc("/", ossHandler)
	err := http.ListenAndServe(":9098", nil)
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
			if strings.HasPrefix(p, prefix) {
				// Size 100 is for mock a file
				res.Files = append(res.Files, addon.File{Name: p, Size: 100})
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

var paths = []string{
	"example/metadata.yaml",
	"example/readme.md",
	"example/template.yaml",
	"example/definitions/helm.yaml",
	"example/resources/configmap.cue",
	"example/resources/parameter.cue",
	"example/resources/service/source-controller.yaml",

	"terraform/metadata.yaml",
	"terraform-alibaba/metadata.yaml",

	"test-error-addon/metadata.yaml",
	"test-error-addon/resources/parameter.cue",
}
