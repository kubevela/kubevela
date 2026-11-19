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

// InheritField is the directive a child uses to leave out one of its parent's
// outputs.
const InheritField = "$inherit"

// droppable are the surface fields whose entries a child may leave out one by
// one. Every other surface field always merges: a definition that extends
// another is the parent with more on it, never something else.
var droppable = map[string]bool{"outputs": true, "patchOutputs": true}

// merged are the surface fields a child always merges onto its parent's, and
// which `$inherit` may therefore only mention to say so.
var merged = map[string]bool{"output": true, "patch": true, "processing": true}

// directives are the instructions read off a child's template. They come from
// the AST rather than the compiled value, since they steer how that value is
// assembled.
type directives struct {
	// drops is set by `$inherit: {outputs: {ingress: false}}`: per droppable
	// surface field, the entries the child leaves out of its parent's.
	drops map[string]map[string]bool
}

// dropped reports whether the child leaves out one entry of a parent's field.
func (d directives) dropped(field, key string) bool { return d.drops[field][key] }

// DroppedOutputs is the entries of a droppable surface field, such as
// `outputs`, that a level leaves out of its parent's with `$inherit`.
func DroppedOutputs(level Level, field string) (map[string]bool, error) {
	d, err := parseDirectives(level)
	if err != nil {
		return nil, err
	}
	return d.drops[field], nil
}

// parseDirectives reads `$inherit` off a template, refusing the forms that
// would replace what the parent renders.
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
			if !b {
				return d, fmt.Errorf(
					"`%s: false` is not allowed in %s: a definition that extends another adds to what "+
						"its parent renders and cannot replace it; leave out one output at a time with "+
						"`%s: {outputs: {<name>: false}}`",
					InheritField, level.Name, InheritField)
			}
		case *ast.StructLit:
			if err := d.addFields(level, v); err != nil {
				return d, err
			}
		default:
			return d, fmt.Errorf(
				"%s in %s must be a struct, for example `%s: {outputs: {ingress: false}}`",
				InheritField, level.Name, InheritField)
		}
	}
	return d, nil
}

// addFields reads the fields of `$inherit: {...}`. Added to rather than
// replaced, as CUE unifies two structs.
func (d *directives) addFields(level Level, st *ast.StructLit) error {
	for _, elt := range st.Elts {
		inner, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		name := labelName(inner.Label)
		switch {
		case droppable[name]:
			if err := d.addDrops(level, name, inner.Value); err != nil {
				return err
			}
		case merged[name]:
			b, err := fieldBool(level, name, inner.Value)
			if err != nil {
				return err
			}
			if !b {
				return fmt.Errorf(
					"`%s: {%s: false}` is not allowed in %s: a definition that extends another adds to "+
						"its parent's %s and cannot replace it; a definition that needs something else "+
						"in its place is a different definition",
					InheritField, name, level.Name, name)
			}
		default:
			return fmt.Errorf("%s in %s: unknown field %s; only outputs and patchOutputs take "+
				"per-output drops, for example `%s: {outputs: {ingress: false}}`",
				InheritField, level.Name, name, InheritField)
		}
	}
	return nil
}

// addDrops reads `outputs: {ingress: false}`. The field as a whole may only be
// said to merge.
func (d *directives) addDrops(level Level, field string, value ast.Expr) error {
	switch v := value.(type) {
	case *ast.StructLit:
		for _, elt := range v.Elts {
			entry, ok := elt.(*ast.Field)
			if !ok {
				continue
			}
			key, ok := outputName(entry.Label)
			if !ok {
				return fmt.Errorf("%s in %s: each entry of %s must name one output, "+
					"for example `%s: {%s: {ingress: false}}`; a pattern or computed label names none",
					InheritField, level.Name, field, InheritField, field)
			}
			b, err := fieldBool(level, field+"."+key, entry.Value)
			if err != nil {
				return err
			}
			if b {
				continue
			}
			if d.drops == nil {
				d.drops = map[string]map[string]bool{}
			}
			if d.drops[field] == nil {
				d.drops[field] = map[string]bool{}
			}
			d.drops[field][key] = true
		}
		return nil
	default:
		b, err := fieldBool(level, field, value)
		if err != nil {
			return err
		}
		if !b {
			return fmt.Errorf(
				"`%s: {%s: false}` is not allowed in %s: leave out one output at a time with "+
					"`%s: {%s: {<name>: false}}`",
				InheritField, field, level.Name, InheritField, field)
		}
		return nil
	}
}

// fieldBool reads a directive value that must be a boolean literal.
func fieldBool(level Level, name string, value ast.Expr) (bool, error) {
	if lit, ok := value.(*ast.BasicLit); ok {
		if b, err := boolLit(lit); err == nil {
			return b, nil
		}
	}
	return false, fmt.Errorf("%s in %s: %s must be true or false", InheritField, level.Name, name)
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

// outputName is the output a per-output drop names: a plain or quoted label
// that is not empty. A pattern, a computed or interpolated label, or "" names
// no single output.
func outputName(label ast.Label) (string, bool) {
	switch l := label.(type) {
	case *ast.Ident:
		return l.Name, l.Name != ""
	case *ast.BasicLit:
		if l.Kind != token.STRING {
			return "", false
		}
		name := labelName(l)
		return name, name != ""
	default:
		return "", false
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
