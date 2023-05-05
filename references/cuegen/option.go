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

import goast "go/ast"

// Type is a special cue type
type Type string

const (
	// TypeAny converts go type to _(top value) in cue
	TypeAny Type = "any"
	// TypeEllipsis converts go type to {...} in cue
	TypeEllipsis Type = "ellipsis"
)

type options struct {
	types      map[string]Type
	nullable   bool
	typeFilter func(typ *goast.TypeSpec) bool
}

// Option is a function that configures generation options
type Option func(opts *options)

func newDefaultOptions() *options {
	return &options{
		types: map[string]Type{
			"map[string]interface{}": TypeEllipsis, "map[string]any": TypeEllipsis,
			"interface{}": TypeAny, "any": TypeAny,
		},
		nullable:   false,
		typeFilter: func(_ *goast.TypeSpec) bool { return true },
	}
}

// WithTypes appends go types as specified cue types in generation
//
// Example:*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured, TypeEllipsis
//
//   - Default any types: interface{}, any
//   - Default ellipsis types: map[string]interface{}, map[string]any
func WithTypes(types map[string]Type) Option {
	return func(opts *options) {
		for k, v := range types {
			opts.types[k] = v
		}
	}
}

// WithNullable will generate null enum for pointer type
func WithNullable() Option {
	return func(opts *options) {
		opts.nullable = true
	}
}

// WithTypeFilter filters top struct types to be generated, and filter returns true to generate the type, otherwise false
func WithTypeFilter(filter func(typ *goast.TypeSpec) bool) Option {
	// return invalid option if filter is nil, so that it will not be applied
	if filter == nil {
		return nil
	}

	return func(opts *options) {
		opts.typeFilter = filter
	}
}
