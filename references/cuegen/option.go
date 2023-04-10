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

type options struct {
	anyTypes   map[string]struct{}
	nullable   bool
	typeFilter func(typ *goast.TypeSpec) bool
}

// Option is a function that configures generation options
type Option func(opts *options)

func newDefaultOptions() *options {
	return &options{
		anyTypes: map[string]struct{}{
			"map[string]interface{}": {}, "map[string]any": {},
			"interface{}": {}, "any": {},
		},
		nullable:   false,
		typeFilter: func(_ *goast.TypeSpec) bool { return true },
	}
}

// WithAnyTypes appends go types as any type({...}) in CUE
//
// Example:*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured
//
// Default any types are: map[string]interface{}, map[string]any, interface{}, any
func WithAnyTypes(types ...string) Option {
	return func(opts *options) {
		for _, t := range types {
			opts.anyTypes[t] = struct{}{}
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
