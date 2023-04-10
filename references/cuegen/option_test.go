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
	goast "go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithAnyTypes(t *testing.T) {
	tests := []struct {
		name  string
		opts  []Option
		extra map[string]struct{}
	}{
		{
			name:  "default",
			opts:  nil,
			extra: map[string]struct{}{},
		},
		{
			name:  "single",
			opts:  []Option{WithAnyTypes("foo", "bar")},
			extra: map[string]struct{}{"foo": {}, "bar": {}},
		},
		{
			name: "multiple",
			opts: []Option{WithAnyTypes("foo", "bar"),
				WithAnyTypes("baz", "qux")},
			extra: map[string]struct{}{"foo": {}, "bar": {}, "baz": {}, "qux": {}},
		},
	}

	for _, tt := range tests {
		opts := options{anyTypes: map[string]struct{}{}}
		for _, opt := range tt.opts {
			opt(&opts)
		}
		assert.Equal(t, opts.anyTypes, tt.extra, tt.name)
	}
}

func TestWithNullable(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want bool
	}{
		{name: "default", opts: nil, want: false},
		{name: "true", opts: []Option{WithNullable()}, want: true},
	}

	for _, tt := range tests {
		opts := options{nullable: false}
		for _, opt := range tt.opts {
			opt(&opts)
		}
		assert.Equal(t, opts.nullable, tt.want, tt.name)
	}
}

func TestWithTypeFilter(t *testing.T) {
	tests := []struct {
		name  string
		opts  []Option
		true  []string
		false []string
	}{
		{
			name:  "default",
			opts:  nil,
			true:  []string{"foo", "bar"},
			false: []string{},
		},
		{
			name: "nil",
			opts: []Option{WithTypeFilter(nil)},
			true: []string{"foo", "bar"},
		},
		{
			name:  "single",
			opts:  []Option{WithTypeFilter(func(typ *goast.TypeSpec) bool { return typ.Name.Name == "foo" })},
			true:  []string{"foo"},
			false: []string{"bar", "baz"},
		},
		{
			name: "multiple",
			opts: []Option{WithTypeFilter(func(typ *goast.TypeSpec) bool { return typ.Name.Name == "foo" }),
				WithTypeFilter(func(typ *goast.TypeSpec) bool { return typ.Name.Name == "bar" })},
			true:  []string{"bar"},
			false: []string{"foo", "baz"},
		},
	}

	for _, tt := range tests {
		opts := options{typeFilter: func(_ *goast.TypeSpec) bool { return true }}
		for _, opt := range tt.opts {
			if opt != nil {
				opt(&opts)
			}
		}
		for _, typ := range tt.true {
			assert.True(t, opts.typeFilter(&goast.TypeSpec{Name: &goast.Ident{Name: typ}}), tt.name)
		}
		for _, typ := range tt.false {
			assert.False(t, opts.typeFilter(&goast.TypeSpec{Name: &goast.Ident{Name: typ}}), tt.name)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := newDefaultOptions()

	assert.Equal(t, opts.anyTypes, map[string]struct{}{
		"map[string]interface{}": {}, "map[string]any": {},
		"interface{}": {}, "any": {},
	})
	assert.Equal(t, opts.nullable, false)
	// assert can't compare function
	assert.True(t, opts.typeFilter(nil))
}
