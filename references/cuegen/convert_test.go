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
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cueast "cuelang.org/go/cue/ast"
	"github.com/stretchr/testify/assert"
)

func TestConvert(t *testing.T) {
	g, err := NewGenerator("testdata/valid.go")
	assert.NoError(t, err)

	got := &bytes.Buffer{}
	decls, err := g.Generate(
		WithTypes(map[string]Type{
			"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured": TypeEllipsis,
		}),
		WithTypeFilter(func(typ *goast.TypeSpec) bool {
			if typ.Name == nil {
				return true
			}
			return !strings.HasPrefix(typ.Name.Name, "TypeFilter")
		}))
	assert.NoError(t, err)
	assert.NoError(t, g.Format(got, decls))

	want, err := os.ReadFile("testdata/valid.cue")
	assert.NoError(t, err)

	assert.Equal(t, got.String(), string(want))
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

		decls, err := g.Generate()
		assert.Error(t, err, path)
		assert.Nil(t, decls)

		return nil
	}); err != nil {
		t.Error(err)
	}
}

func TestConvertNullable(t *testing.T) {
	g, err := NewGenerator("testdata/nullable.go")
	assert.NoError(t, err)

	got := &bytes.Buffer{}
	decls, err := g.Generate(WithNullable())
	assert.NoError(t, err)
	assert.NoError(t, g.Format(got, decls))

	want, err := os.ReadFile("testdata/nullable.cue")
	assert.NoError(t, err)

	assert.Equal(t, got.String(), string(want))
}

func TestMakeComment(t *testing.T) {
	cases := []struct {
		name string
		in   *goast.CommentGroup
		out  []string
	}{
		{
			name: "nil comment",
			in:   nil,
			out:  nil,
		},
		{
			name: "empty comment",
			in:   &goast.CommentGroup{},
			out:  nil,
		},
		{
			name: "line comment",
			in: &goast.CommentGroup{
				List: []*goast.Comment{
					{Text: "// hello"},
					{Text: "// world"},
				},
			},
			out: []string{"// hello", "// world"},
		},
		{
			name: "block comment",
			in: &goast.CommentGroup{
				List: []*goast.Comment{
					{Text: "/* hello world */"},
				},
			},
			out: []string{"// hello world "},
		},
		{
			name: "multiline block comment",
			in: &goast.CommentGroup{
				List: []*goast.Comment{
					{Text: `/*
 * hello
 * world
 */`},
				},
			},
			out: []string{"// * hello", "// * world", "//"},
		},
		{
			name: "multiline block comment with no space",
			in: &goast.CommentGroup{
				List: []*goast.Comment{
					{Text: `/*
hello
world
*/`},
				},
			},
			out: []string{"// hello", "// world", "//"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cg := makeComment(tc.in)
			if cg == nil {
				assert.Nil(t, tc.out)
				return
			}
			var comments []string
			for _, c := range cg.List {
				comments = append(comments, c.Text)
			}
			assert.Equal(t, tc.out, comments)
		})
	}
}

func typeFromSource(t *testing.T, src string) types.Type {
	fset := token.NewFileSet()
	fullSrc := "package p\n\n" + src
	f, err := parser.ParseFile(fset, "src.go", fullSrc, 0)
	assert.NoError(t, err)

	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("p", fset, []*goast.File{f}, nil)
	assert.NoError(t, err)

	obj := pkg.Scope().Lookup("T")
	assert.NotNil(t, obj, "type T not found in source")
	return obj.Type()
}

func TestSupportedType(t *testing.T) {
	cases := []struct {
		name          string
		src           string
		shouldError   bool
		errorContains string
	}{
		{name: "string", src: "type T string", shouldError: false},
		{name: "pointer", src: "type T *string", shouldError: false},
		{name: "slice", src: "type T []int", shouldError: false},
		{name: "map", src: "type T map[string]bool", shouldError: false},
		{name: "struct", src: "type T struct{ F string }", shouldError: false},
		{name: "interface", src: "type T interface{}", shouldError: false},
		{name: "recursive pointer", src: "type T *T", shouldError: true, errorContains: "recursive type"},
		{name: "recursive struct field", src: "type T struct{ F *T }", shouldError: true, errorContains: "recursive type"},
		{name: "map with non-string key", src: "type T map[int]string", shouldError: true, errorContains: "unsupported map key type"},
		{name: "map with struct key", src: `type U struct{}
		type T map[U]string`, shouldError: true, errorContains: "unsupported map key type"}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ := typeFromSource(t, tc.src)
			err := supportedType(nil, typ)

			if tc.shouldError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnumField(t *testing.T) {
	// Create a dummy generator. The actual fields of Generator are not used by enumField
	// except for g.opts.types, which is empty here.
	g := &Generator{}

	defVal1 := "val1"
	def1 := "1"

	cases := []struct {
		name        string
		typSrc      string
		opts        *tagOptions
		expectedErr bool
		expectedCue cueast.Expr
	}{
		{
			name:        "string enum",
			typSrc:      "type T string",
			opts:        &tagOptions{Enum: []string{"val1", "val2"}},
			expectedErr: true,
		},
		{
			name:        "int enum",
			typSrc:      "type T int",
			opts:        &tagOptions{Enum: []string{"1", "2"}},
			expectedErr: true,
		},
		{
			name:        "string enum with default",
			typSrc:      "type T string",
			opts:        &tagOptions{Enum: []string{"val1", "val2"}, Default: &defVal1},
			expectedErr: true,
		},
		{
			name:        "int enum with default",
			typSrc:      "type T int",
			opts:        &tagOptions{Enum: []string{"1", "2"}, Default: &def1},
			expectedErr: true,
		},
		{
			name:        "unsupported type for enum",
			typSrc:      "type T struct{}",
			opts:        &tagOptions{Enum: []string{"val1"}},
			expectedErr: true,
		},
		{
			name:        "invalid enum value for int type",
			typSrc:      "type T int",
			opts:        &tagOptions{Enum: []string{"not_an_int"}},
			expectedErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ := typeFromSource(t, tc.typSrc)
			expr, err := g.enumField(typ, tc.opts)

			if tc.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, expr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCue, expr)
			}
		})
	}
}
