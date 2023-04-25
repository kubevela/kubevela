/*
Copyright 2023 The KubeVela Authors.

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

package provider

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/references/cuegen"
)

func TestGenerate(t *testing.T) {
	got := bytes.Buffer{}
	err := Generate(Options{
		File:   "testdata/valid.go",
		Writer: &got,
		Types: map[string]cuegen.Type{
			"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured":     cuegen.TypeEllipsis,
			"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.UnstructuredList": cuegen.TypeEllipsis,
		},
		Nullable: false,
	})
	require.NoError(t, err)

	expected, err := os.ReadFile("testdata/valid.cue")
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, string(expected), got.String())
}

func TestGenerateInvalid(t *testing.T) {
	if err := filepath.Walk("testdata/invalid", func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}

		if info.IsDir() {
			return nil
		}

		err := Generate(Options{
			File:   path,
			Writer: io.Discard,
		})
		assert.Error(t, err)

		return nil
	}); err != nil {
		t.Error(err)
	}
}

func TestGenerateEmptyError(t *testing.T) {
	err := Generate(Options{
		File:   "",
		Writer: io.Discard,
	})
	assert.Error(t, err)
}
