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

	cueast "cuelang.org/go/cue/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/references/cuegen"
)

func TestGenerate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
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
		assert.Equal(t, string(expected), got.String())
	})

	t.Run("invalid", func(t *testing.T) {
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
	})

	t.Run("empty file", func(t *testing.T) {
		err := Generate(Options{
			File:   "",
			Writer: io.Discard,
		})
		assert.Error(t, err)
	})
}

func TestExtractProviders(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		g, err := cuegen.NewGenerator("testdata/valid.go")
		require.NoError(t, err)
		providers, err := extractProviders(g.Package())
		require.NoError(t, err)
		require.Len(t, providers, 4)
		assert.Equal(t, `"apply"`, providers[0].name)
		assert.Equal(t, "ResourceParams", providers[0].params)
		assert.Equal(t, "ResourceReturns", providers[0].returns)
		assert.Equal(t, "Apply", providers[0].do)
	})

	t.Run("no provider map", func(t *testing.T) {
		g, err := cuegen.NewGenerator("testdata/invalid/no_provider_map.go")
		require.NoError(t, err)
		_, err = extractProviders(g.Package())
		assert.EqualError(t, err, "no provider function map found like 'map[string]github.com/kubevela/pkg/cue/cuex/runtime.ProviderFn'")
	})
}

func TestModifyDecls(t *testing.T) {
	tests := []struct {
		name      string
		decls     []cuegen.Decl
		providers []provider
		wantLen   int
	}{
		{
			name: "valid",
			decls: []cuegen.Decl{
				&cuegen.Struct{CommonFields: cuegen.CommonFields{Name: "Params", Expr: &cueast.StructLit{Elts: []cueast.Decl{&cueast.Field{Label: cueast.NewIdent("p"), Value: cueast.NewIdent("string")}}}}},
				&cuegen.Struct{CommonFields: cuegen.CommonFields{Name: "Returns", Expr: &cueast.StructLit{Elts: []cueast.Decl{&cueast.Field{Label: cueast.NewIdent("r"), Value: cueast.NewIdent("string")}}}}},
			},
			providers: []provider{
				{name: `"my-do"`, params: "Params", returns: "Returns", do: "MyDo"},
			},
			wantLen: 1,
		},
		{
			name:      "no providers",
			decls:     []cuegen.Decl{},
			providers: []provider{},
			wantLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDecls, err := modifyDecls("my-provider", tt.decls, tt.providers)
			require.NoError(t, err)
			require.Len(t, newDecls, tt.wantLen)
			if tt.wantLen > 0 {
				s, ok := newDecls[0].(*cuegen.Struct)
				require.True(t, ok)
				assert.Equal(t, "#MyDo", s.Name)
				require.Len(t, s.Expr.(*cueast.StructLit).Elts, 4)
			}
		})
	}
}

func TestRecoverAssert(t *testing.T) {
	t.Run("panic recovery", func(t *testing.T) {
		var err error
		func() {
			defer recoverAssert(&err, "test panic")
			panic("panic message")
		}()
		assert.EqualError(t, err, "panic message: panic: test panic")
	})
}
