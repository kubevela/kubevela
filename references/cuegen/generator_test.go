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
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorWriter is an io.Writer that always returns an error.
type errorWriter struct{}

func (ew *errorWriter) Write(p []byte) (n int, err error) {
	return 0, assert.AnError
}

// testGenerator is a helper function to create a valid Generator for tests.
func testGenerator(t *testing.T) *Generator {
	g, err := NewGenerator("testdata/valid.go")
	require.NoError(t, err)
	require.NotNil(t, g)
	require.Len(t, g.pkg.Errors, 0)

	return g
}

func TestNewGenerator(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		expectedErr bool
		errContains string
	}{
		{
			name:        "valid package",
			path:        "testdata/valid.go",
			expectedErr: false,
		},
		{
			name:        "non-existent package",
			path:        "testdata/non_existent.go",
			expectedErr: true,
			errContains: "could not load Go packages",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewGenerator(tc.path)

			if tc.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, g)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, g)
				assert.NotNil(t, g.pkg)
				assert.NotNil(t, g.types)
				assert.Equal(t, g.opts.types, newDefaultOptions().types)
				assert.Equal(t, g.opts.nullable, newDefaultOptions().nullable)
				assert.True(t, g.opts.typeFilter(nil))
				assert.Greater(t, len(g.types), 0)
			}
		})
	}
}

func TestGeneratorPackage(t *testing.T) {
	g := testGenerator(t)

	assert.Equal(t, g.Package(), g.pkg)
}

func TestGeneratorGenerate(t *testing.T) {
	g := testGenerator(t)

	cases := []struct {
		name        string
		opts        []Option
		expectedErr bool
		expectedLen int // Expected number of Decls
	}{
		{
			name:        "no options",
			opts:        nil,
			expectedErr: false,
			expectedLen: 26,
		},
		{
			name: "with types option",
			opts: []Option{WithTypes(map[string]Type{
				"foo": TypeAny,
				"bar": TypeAny,
			})},
			expectedErr: false,
			expectedLen: 26,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decls, err := g.Generate(tc.opts...)

			if tc.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, decls)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, decls)
				assert.Len(t, decls, tc.expectedLen)
			}
		})
	}
}

func TestGeneratorFormat(t *testing.T) {
	g := testGenerator(t)

	decls, err := g.Generate()
	require.NoError(t, err)
	require.NotNil(t, decls)

	cases := []struct {
		name        string
		writer      io.Writer
		decls       []Decl
		expectedErr bool
		errContains string
	}{
		{
			name:        "valid format",
			writer:      io.Discard,
			decls:       decls,
			expectedErr: false,
		},
		{
			name:        "nil writer",
			writer:      nil,
			decls:       decls,
			expectedErr: true,
			errContains: "nil writer",
		},
		{
			name:        "empty decls",
			writer:      io.Discard,
			decls:       []Decl{},
			expectedErr: true,
			errContains: "invalid decls",
		},
		{
			name:        "writer error",
			writer:      &errorWriter{},
			decls:       decls,
			expectedErr: true,
			errContains: assert.AnError.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := g.Format(tc.writer, tc.decls)

			if tc.expectedErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadPackage(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		expectedErr bool
		errContains string
	}{
		{
			name:        "valid package",
			path:        "testdata/valid.go",
			expectedErr: false,
		},
		{
			name:        "non-existent package",
			path:        "testdata/non_existent.go",
			expectedErr: true,
			errContains: "could not load Go packages",
		},
		{
			name:        "package with syntax error",
			path:        "testdata/invalid_syntax.go",
			expectedErr: true,
			errContains: "could not load Go packages",
		},
	}

	// Create a temporary file with syntax errors for "package with syntax error" case
	invalidGoContent := `package main

func main { // Missing parentheses
	fmt.Println("Hello")
}`
	err := os.WriteFile("testdata/invalid_syntax.go", []byte(invalidGoContent), 0644)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove("testdata/invalid_syntax.go")
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := loadPackage(tc.path)

			if tc.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, pkg)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pkg)
				assert.Len(t, pkg.Errors, 0)
			}
		})
	}
}

func TestGetTypeInfo(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		expectedLen int
	}{
		{
			name:        "valid package",
			path:        "testdata/valid.go",
			expectedLen: 40,
		}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := loadPackage(tc.path)
			require.NoError(t, err)

			typeInfo := getTypeInfo(pkg)
			assert.Len(t, typeInfo, tc.expectedLen)
		})
	}
}
