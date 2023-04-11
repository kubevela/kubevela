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
	"testing"

	cueast "cuelang.org/go/cue/ast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testGenerator(t *testing.T) *Generator {
	g, err := NewGenerator("testdata/valid.go")
	require.NoError(t, err)
	require.NotNil(t, g)
	require.Len(t, g.pkg.Errors, 0)

	return g
}

func TestNewGenerator(t *testing.T) {
	g := testGenerator(t)

	assert.NotNil(t, g.pkg)
	assert.NotNil(t, g.types)
	assert.Equal(t, g.opts.anyTypes, newDefaultOptions().anyTypes)
	assert.Equal(t, g.opts.nullable, newDefaultOptions().nullable)
	// assert can't compare function
	assert.True(t, g.opts.typeFilter(nil))

	assert.Greater(t, len(g.types), 0)
}

func TestGeneratorPackage(t *testing.T) {
	g := testGenerator(t)

	assert.Equal(t, g.Package(), g.pkg)
}

func TestGeneratorGenerate(t *testing.T) {
	g := testGenerator(t)

	decls, err := g.Generate(WithAnyTypes("foo", "bar"), nil)
	assert.NoError(t, err)
	assert.NotNil(t, decls)

	decls, err = g.Generate()
	assert.NoError(t, err)
	assert.NotNil(t, decls)
}

func TestGeneratorFormat(t *testing.T) {
	g := testGenerator(t)

	decls, err := g.Generate()
	assert.NoError(t, err)

	assert.NoError(t, g.Format(io.Discard, decls))
	assert.NoError(t, g.Format(io.Discard, []cueast.Decl{nil, nil}))
	assert.Error(t, g.Format(nil, decls))
	assert.Error(t, g.Format(io.Discard, nil))
	assert.Error(t, g.Format(io.Discard, []cueast.Decl{}))
}

func TestLoadPackage(t *testing.T) {
	pkg, err := loadPackage("testdata/valid.go")
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.Len(t, pkg.Errors, 0)
}

func TestGetTypeInfo(t *testing.T) {
	pkg, err := loadPackage("testdata/valid.go")
	require.NoError(t, err)

	require.Greater(t, len(getTypeInfo(pkg)), 0)
}
