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

package main

import (
	"encoding/xml"
	"net/http/httptest"
	"testing"

	"github.com/oam-dev/kubevela/pkg/addon"
)

func TestOssHandlerServesFileBytesExactly(t *testing.T) {
	want, err := testData.ReadFile("testdata/sample/metadata.yaml")
	if err != nil {
		t.Fatalf("failed to read fixture directly: %v", err)
	}

	req := httptest.NewRequest("GET", "/sample/metadata.yaml", nil)
	rec := httptest.NewRecorder()
	ossHandler(rec, req)

	got := rec.Body.Bytes()
	if string(got) != string(want) {
		t.Fatalf("served bytes do not match source file exactly\nwant: %q\ngot:  %q", want, got)
	}
}

// TestOssHandlerServesTemplateLikeBytesUnchanged guards against the handler
// treating served file content as a Go template. A file containing an
// unbalanced "{{" fails html/template.Parse; the handler must still return
// the file's bytes untouched rather than a template-error render.
func TestOssHandlerServesTemplateLikeBytesUnchanged(t *testing.T) {
	want, err := testData.ReadFile("testdata/template-edge-case/broken.txt")
	if err != nil {
		t.Fatalf("failed to read fixture directly: %v", err)
	}

	req := httptest.NewRequest("GET", "/template-edge-case/broken.txt", nil)
	rec := httptest.NewRecorder()
	ossHandler(rec, req)

	got := rec.Body.Bytes()
	if string(got) != string(want) {
		t.Fatalf("served bytes do not match source file exactly\nwant: %q\ngot:  %q", want, got)
	}
}

func TestOssHandlerNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/does-not-exist.yaml", nil)
	rec := httptest.NewRecorder()
	ossHandler(rec, req)

	if got := rec.Body.String(); got != "not found" {
		t.Fatalf("expected \"not found\", got %q", got)
	}
}

func TestOssHandlerListReturnsValidXML(t *testing.T) {
	req := httptest.NewRequest("GET", "/?prefix=sample", nil)
	rec := httptest.NewRecorder()
	ossHandler(rec, req)

	var res addon.ListBucketResult
	if err := xml.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("response is not valid XML: %v\nbody: %s", err, rec.Body.String())
	}
	if res.Count == 0 {
		t.Fatalf("expected at least one file under prefix \"sample\", got 0")
	}
	for _, f := range res.Files {
		if f.Name[:len("sample")] != "sample" {
			t.Fatalf("file %q does not match requested prefix", f.Name)
		}
	}
}
