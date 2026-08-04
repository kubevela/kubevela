/*
Copyright 2026 The KubeVela Authors.

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

package controllers_test

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

// newMockOSSAddonServer starts an in-process HTTP server that serves the addon
// fixtures under root using the same OSS bucket-listing protocol the real OSS
// registry reader (pkg/addon/reader_oss.go) speaks: a "?prefix=" query lists
// matching files as XML, a bare path returns raw file bytes. This lets tests
// register a real, working addon registry without depending on an externally
// started process, so there is no cross-step process-lifetime or ordering
// concern in CI.
func newMockOSSAddonServer(root string) (*httptest.Server, error) {
	type fileEntry struct {
		relPath string
		size    int64
	}

	var files []fileEntry
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, fileEntry{relPath: filepath.ToSlash(rel), size: info.Size()})
		return nil
	})
	if walkErr != nil {
		return nil, errors.Wrapf(walkErr, "failed to walk mock addon testdata %q", root)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		if prefixes, ok := req.URL.Query()["prefix"]; ok {
			prefix := ""
			if len(prefixes) > 0 {
				prefix = prefixes[0]
			}
			result := pkgaddon.ListBucketResult{}
			for _, f := range files {
				if strings.HasPrefix(f.relPath, prefix) {
					result.Files = append(result.Files, pkgaddon.File{Name: f.relPath, Size: int(f.size)})
				}
			}
			result.Count = len(result.Files)
			rw.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(rw).Encode(result)
			return
		}

		rel := strings.TrimPrefix(req.URL.Path, "/")
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // G304: serving a fixed local testdata directory, not user input
		if err != nil {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = rw.Write(content)
	})

	return httptest.NewServer(mux), nil
}
