package main

import (
	"embed"
	"encoding/xml"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/mock/utils"
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
		log.Fatal("Apply mock server config to ConfigMap fail")
	}
	http.HandleFunc("/", ossHandler)
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
		if err != nil {
			_, _ = rw.Write([]byte(err.Error()))
		}
		_, _ = rw.Write(data)
	} else {
		found := false
		for _, p := range paths {
			if queryPath == p.path {
				file, err := testData.ReadFile(path.Join("testdata", queryPath))
				if err != nil {
					_, _ = rw.Write([]byte(err.Error()))
				}
				found = true
				_, _ = rw.Write(file)
				break
			}
		}
		if !found {
			_, _ = rw.Write([]byte("not found"))
		}
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
		fmt.Println(path, size)

		paths = append(paths, struct {
			path   string
			length int64
		}{path: path, length: size})
		return nil
	})
}
