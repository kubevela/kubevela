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
	gotypes "go/types"
	"math"
	"strconv"
	"testing"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
	"github.com/stretchr/testify/assert"
)

func TestIdent(t *testing.T) {
	tests := []struct {
		name  string
		isDef bool
		want  string
	}{
		{name: "test", isDef: true, want: "#test"},
		{name: "test", isDef: false, want: "test"},
		{name: "Test", isDef: true, want: "#Test"},
		{name: "Test", isDef: false, want: "Test"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, Ident(tt.name, tt.isDef).String())
	}
}

func TestBasicType(t *testing.T) {
	tests := []struct {
		name string
		typ  *gotypes.Basic
		want string
	}{
		{name: "uintptr", typ: gotypes.Typ[gotypes.Uintptr], want: "uint64"},
		{name: "byte", typ: gotypes.Typ[gotypes.Byte], want: "uint8"},
		{name: "int", typ: gotypes.Typ[gotypes.Int], want: "int"},
		{name: "int8", typ: gotypes.Typ[gotypes.Int8], want: "int8"},
		{name: "int16", typ: gotypes.Typ[gotypes.Int16], want: "int16"},
		{name: "int32", typ: gotypes.Typ[gotypes.Int32], want: "int32"},
		{name: "int64", typ: gotypes.Typ[gotypes.Int64], want: "int64"},
		{name: "uint", typ: gotypes.Typ[gotypes.Uint], want: "uint"},
		{name: "uint8", typ: gotypes.Typ[gotypes.Uint8], want: "uint8"},
		{name: "uint16", typ: gotypes.Typ[gotypes.Uint16], want: "uint16"},
		{name: "uint32", typ: gotypes.Typ[gotypes.Uint32], want: "uint32"},
		{name: "uint64", typ: gotypes.Typ[gotypes.Uint64], want: "uint64"},
		{name: "float32", typ: gotypes.Typ[gotypes.Float32], want: "float32"},
		{name: "float64", typ: gotypes.Typ[gotypes.Float64], want: "float64"},
		{name: "string", typ: gotypes.Typ[gotypes.String], want: "string"},
		{name: "bool", typ: gotypes.Typ[gotypes.Bool], want: "bool"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, basicType(tt.typ).(*cueast.Ident).String(), tt.name)
	}
}

func TestAnyLit(t *testing.T) {
	assert.Equal(t, anyLit(), &cueast.StructLit{Elts: []cueast.Decl{&cueast.Ellipsis{}}})
}

func TestBasicLabel(t *testing.T) {
	overflowInt64 := strconv.FormatInt(math.MaxInt64, 10) + "0"
	overflowUint64 := strconv.FormatUint(math.MaxUint64, 10) + "0"
	overflowFloat64 := strconv.FormatFloat(math.MaxFloat64, 'f', -1, 64) + "0"

	tests := []struct {
		name    string
		typ     *gotypes.Basic
		v       string
		wantErr bool
		want    *cueast.BasicLit
	}{
		{name: "int", typ: gotypes.Typ[gotypes.Int], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "int8", typ: gotypes.Typ[gotypes.Int8], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "int16", typ: gotypes.Typ[gotypes.Int16], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "int32", typ: gotypes.Typ[gotypes.Int32], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "int64", typ: gotypes.Typ[gotypes.Int64], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "uint", typ: gotypes.Typ[gotypes.Uint], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "uint8", typ: gotypes.Typ[gotypes.Uint8], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "uint16", typ: gotypes.Typ[gotypes.Uint16], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "uint32", typ: gotypes.Typ[gotypes.Uint32], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "uint64", typ: gotypes.Typ[gotypes.Uint64], v: "123", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.INT, Value: "123"}},
		{name: "float32", typ: gotypes.Typ[gotypes.Float32], v: "123.456", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.FLOAT, Value: "123.456"}},
		{name: "float64", typ: gotypes.Typ[gotypes.Float64], v: "123.456", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.FLOAT, Value: "123.456"}},
		{name: "string", typ: gotypes.Typ[gotypes.String], v: "abc", wantErr: false, want: &cueast.BasicLit{Kind: cuetoken.STRING, Value: `"abc"`}},
		{name: "bool", typ: gotypes.Typ[gotypes.Bool], v: "true", wantErr: false, want: cueast.NewBool(true)},
		{name: "bool", typ: gotypes.Typ[gotypes.Bool], v: "false", wantErr: false, want: cueast.NewBool(false)},
		{name: "int_error", typ: gotypes.Typ[gotypes.Int], v: "abc", wantErr: true},
		{name: "uint_error", typ: gotypes.Typ[gotypes.Uint], v: "abc", wantErr: true},
		{name: "float_error", typ: gotypes.Typ[gotypes.Float64], v: "abc", wantErr: true},
		{name: "bool_error", typ: gotypes.Typ[gotypes.Bool], v: "abc", wantErr: true},
		{name: "type_error", typ: gotypes.Typ[gotypes.Complex64], v: "abc", wantErr: true},
		{name: "int_overflow", typ: gotypes.Typ[gotypes.Int], v: overflowInt64, wantErr: true},
		{name: "uint_overflow", typ: gotypes.Typ[gotypes.Uint], v: overflowUint64, wantErr: true},
		{name: "int64_overflow", typ: gotypes.Typ[gotypes.Int64], v: overflowInt64, wantErr: true},
		{name: "uint64_overflow", typ: gotypes.Typ[gotypes.Uint64], v: overflowUint64, wantErr: true},
		{name: "float64_overflow", typ: gotypes.Typ[gotypes.Float64], v: overflowFloat64, wantErr: true},
	}

	for _, tt := range tests {
		got, err := basicLabel(tt.typ, tt.v)
		if tt.wantErr {
			assert.Error(t, err, tt.name)
		} else {
			assert.NoError(t, err, tt.name)
			assert.Equal(t, tt.want, got, tt.name)
		}
	}
}
