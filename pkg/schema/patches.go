/*
Copyright 2026 The KubeVela Authors.

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

package schema

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
)

const (
	patchFieldName        = "patch"
	patchOutputsFieldName = "patchOutputs"
)

// Patches reports whether a trait template patches its component's workload
// (`patch`) and its other resources (`patchOutputs`), anywhere at the top level,
// under an `if` or a `for` included. It is read from the syntax, since a template
// depends on parameters that do not exist until it renders. A template that does
// not parse, or a top-level label only a render can name, may patch either.
func Patches(template string) (output, outputs bool) {
	f, err := parser.ParseFile("-", template)
	if err != nil {
		return true, true
	}
	walkTopLevel(f.Decls, func(name string, known bool) {
		switch {
		case !known:
			output, outputs = true, true
		case name == patchFieldName:
			output = true
		case name == patchOutputsFieldName:
			outputs = true
		}
	})
	return output, outputs
}

// walkTopLevel calls fn with each top-level field's name, descending into
// comprehensions and embedded structs; known is false for a label whose name
// only a render decides, such as a computed or interpolated one.
func walkTopLevel(decls []ast.Decl, fn func(name string, known bool)) {
	for _, d := range decls {
		switch x := d.(type) {
		case *ast.Field:
			name, _, err := ast.LabelName(x.Label)
			fn(name, err == nil)
		case *ast.Comprehension:
			if s, ok := x.Value.(*ast.StructLit); ok {
				walkTopLevel(s.Elts, fn)
			}
		case *ast.EmbedDecl:
			if s, ok := x.Expr.(*ast.StructLit); ok {
				walkTopLevel(s.Elts, fn)
			}
		}
	}
}
