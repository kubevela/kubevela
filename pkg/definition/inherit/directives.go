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

package inherit

import (
	"fmt"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// InheritField is the directive a child uses to turn merging off.
const InheritField = "$inherit"

// directives are the instructions read off a child's template. They come from
// the AST rather than the compiled value, since they steer how that value is
// assembled.
type directives struct {
	// all is set by `$inherit: false`, which turns every field off.
	all *bool
	// fields is set by `$inherit: {output: false}`, per surface field.
	fields map[string]bool
}

// inherits reports whether a surface field should be merged onto the parent's.
// Silence inherits: only a child that means to take over has to say so.
func (d directives) inherits(field string) bool {
	if v, ok := d.fields[field]; ok {
		return v
	}
	if d.all != nil {
		return *d.all
	}
	return true
}

// parseDirectives reads `$inherit` off a template.
func parseDirectives(level Level) (directives, error) {
	var d directives
	file, err := parser.ParseFile(level.Name+".cue", level.Template, parser.ParseComments)
	if err != nil {
		// A template that will not parse fails later with a better message from
		// the compiler, which knows the kind and the call site.
		return d, nil //nolint:nilerr // reported by the compiler, not here
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if labelName(field.Label) != InheritField {
			continue
		}
		switch v := field.Value.(type) {
		case *ast.BasicLit:
			b, err := boolLit(v)
			if err != nil {
				return d, fmt.Errorf("%s in %s: %w", InheritField, level.Name, err)
			}
			d.all = &b
		case *ast.StructLit:
			// Added to rather than replaced, as CUE unifies two structs.
			if d.fields == nil {
				d.fields = map[string]bool{}
			}
			for _, elt := range v.Elts {
				inner, ok := elt.(*ast.Field)
				if !ok {
					continue
				}
				lit, ok := inner.Value.(*ast.BasicLit)
				if !ok {
					return d, fmt.Errorf(
						"%s in %s: %s must be true or false",
						InheritField, level.Name, labelName(inner.Label))
				}
				b, err := boolLit(lit)
				if err != nil {
					return d, fmt.Errorf("%s in %s: %s: %w", InheritField, level.Name, labelName(inner.Label), err)
				}
				d.fields[labelName(inner.Label)] = b
			}
		default:
			return d, fmt.Errorf(
				"%s in %s must be a boolean or a struct of booleans, for example `%s: false` or `%s: {output: false}`",
				InheritField, level.Name, InheritField, InheritField)
		}
	}
	return d, nil
}

func boolLit(lit *ast.BasicLit) (bool, error) {
	switch lit.Kind {
	case token.TRUE:
		return true, nil
	case token.FALSE:
		return false, nil
	default:
		return false, fmt.Errorf("expected true or false, got %s", lit.Value)
	}
}

// labelName is the field a label names.
//
// A quoted label is decoded rather than trimmed: CUE reads "out\u0070ut" as
// `output`, and a directive that compared the raw text would name a field that
// does not exist and quietly do nothing.
func labelName(label ast.Label) string {
	switch l := label.(type) {
	case *ast.Ident:
		return l.Name
	case *ast.BasicLit:
		if l.Kind == token.STRING {
			if decoded, err := literal.Unquote(l.Value); err == nil {
				return decoded
			}
		}
		return l.Value
	default:
		return ""
	}
}
