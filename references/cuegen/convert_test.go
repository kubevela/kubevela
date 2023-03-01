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

package cuegen

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvert(t *testing.T) {
	if err := filepath.Walk("testdata/valid", func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}

		// skip directories
		if info.IsDir() {
			return nil
		}

		// skip cue files
		if filepath.Ext(path) != ".go" {
			return nil
		}

		g, err := NewGenerator(path)
		assert.NoError(t, err)
		g.RegisterAny(
			"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured",
		)

		got := &bytes.Buffer{}
		assert.NoError(t, g.Generate(got))

		// remove .go suffix and add .cue suffix
		cuePath := strings.TrimSuffix(path, filepath.Ext(path)) + ".cue"
		want, err := os.ReadFile(cuePath)
		assert.NoError(t, err)

		assert.Equal(t, got.String(), string(want), path)
		return nil
	}); err != nil {
		t.Error(err)
	}
}

func TestConvertInvalid(t *testing.T) {
	if err := filepath.Walk("testdata/invalid", func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}

		if info.IsDir() {
			return nil
		}

		g, err := NewGenerator(path)
		assert.NoError(t, err)

		assert.Error(t, g.Generate(io.Discard), path)

		return nil
	}); err != nil {
		t.Error(err)
	}
}
