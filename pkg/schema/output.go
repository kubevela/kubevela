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
	"context"
	"strconv"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

const (
	// OutputFieldName is the block of a component template declaring its workload.
	OutputFieldName = "output"
	// OutputsFieldName is the block of a component or trait template declaring
	// the other resources it applies, by name.
	OutputsFieldName = "outputs"
	// statusFieldName is the part of an applied object its controller writes.
	statusFieldName = "status"
)

// OutputSchemas are the shapes of what a component or trait template applies,
// as component.<name>.output and component.<name>.outputs.<resource> read them.
type OutputSchemas struct {
	// Output is the workload; nil for a template that declares none, such as a trait's.
	Output *openapi3.Schema
	// Outputs are the other resources, by the name the template gives each.
	Outputs map[string]*openapi3.Schema
}

// GenerateOutputSchemas reads a template's `output` and `outputs` into the
// schema of each object: the fields the template writes, typed by the values or
// parameters it writes them from, and a `status` of any type, since the object's
// controller writes it and a reader casts what it takes from there. Fields
// written inside an `if` are included, typed from the template's source, since
// the evaluated value leaves them out while the parameter tested is unknown. A
// resource whose name the template computes is left out.
func GenerateOutputSchemas(ctx context.Context, template string) (*OutputSchemas, error) {
	val, _, err := compilePruned(ctx, template, process.ParameterFieldName, OutputFieldName, OutputsFieldName)
	if err != nil {
		return nil, err
	}
	var params *openapi3.Schema
	if ps, err := GenerateParameterSchemas(ctx, template); err == nil {
		params = ps.OpenAPI
	}
	out := &OutputSchemas{}
	if v := val.LookupPath(cue.ParsePath(OutputFieldName)); v.Exists() {
		out.Output = valueSchema(v, 0)
	}
	if v := val.LookupPath(cue.ParsePath(OutputsFieldName)); v.Exists() {
		for name, ref := range valueSchema(v, 0).Properties {
			if out.Outputs == nil {
				out.Outputs = map[string]*openapi3.Schema{}
			}
			out.Outputs[name] = ref.Value
		}
	}
	addWrittenFields(template, params, out)
	if out.Output != nil {
		withStatus(out.Output)
	}
	for _, s := range out.Outputs {
		withStatus(s)
	}
	return out, nil
}

// maxOutputDepth bounds how far into an object its schema goes.
const maxOutputDepth = 12

// valueSchema is the schema of an evaluated value by its kind. A struct holding
// an unresolved `if` still lists the fields outside it.
func valueSchema(v cue.Value, depth int) *openapi3.Schema {
	switch v.IncompleteKind() &^ cue.NullKind {
	case cue.StringKind:
		return openapi3.NewStringSchema()
	case cue.IntKind:
		return openapi3.NewIntegerSchema()
	case cue.FloatKind, cue.NumberKind:
		return openapi3.NewFloat64Schema()
	case cue.BoolKind:
		return openapi3.NewBoolSchema()
	case cue.ListKind:
		s := openapi3.NewArraySchema()
		if depth >= maxOutputDepth {
			return s
		}
		if it, err := v.List(); err == nil && it.Next() {
			s.Items = openapi3.NewSchemaRef("", valueSchema(it.Value(), depth+1))
		} else if elem := v.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
			s.Items = openapi3.NewSchemaRef("", valueSchema(elem, depth+1))
		}
		return s
	case cue.StructKind:
		s := openapi3.NewObjectSchema()
		if depth >= maxOutputDepth {
			return s
		}
		it, err := v.Fields(cue.Optional(true))
		if err != nil {
			return s
		}
		for it.Next() {
			if name := labelName(it.Selector()); name != "" {
				s.Properties[name] = openapi3.NewSchemaRef("", valueSchema(it.Value(), depth+1))
			}
		}
		return s
	default:
		// Other kinds have no schema of their own.
	}
	return &openapi3.Schema{}
}

// addWrittenFields adds what the template's source writes and the evaluated
// value lacks: the fields inside `if` bodies, and resources declared in one.
func addWrittenFields(template string, params *openapi3.Schema, out *OutputSchemas) {
	f, err := parser.ParseFile("", template)
	if err != nil {
		return
	}
	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, ok := astLabelName(field.Label)
		if !ok {
			continue
		}
		lit, ok := field.Value.(*ast.StructLit)
		if !ok {
			continue
		}
		w := writtenFields{params: params}
		switch name {
		case OutputFieldName:
			if out.Output == nil {
				out.Output = openapi3.NewObjectSchema()
			}
			w.walkStruct(lit, out.Output)
		case OutputsFieldName:
			resources := openapi3.NewObjectSchema()
			for k, v := range out.Outputs {
				resources.Properties[k] = openapi3.NewSchemaRef("", v)
			}
			w.walkStruct(lit, resources)
			for k, ref := range resources.Properties {
				if out.Outputs == nil {
					out.Outputs = map[string]*openapi3.Schema{}
				}
				out.Outputs[k] = ref.Value
			}
		}
	}
}

// writtenFields walks a template's source, adding each field it writes to a
// schema that does not have it yet.
type writtenFields struct {
	params *openapi3.Schema
}

func (w writtenFields) walkStruct(lit *ast.StructLit, s *openapi3.Schema) {
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.Field:
			name, ok := astLabelName(e.Label)
			if !ok {
				continue
			}
			w.walkValue(e.Value, s, name)
		case *ast.Comprehension:
			body, ok := e.Value.(*ast.StructLit)
			if ok && onlyIfClauses(e.Clauses) {
				w.walkStruct(body, s)
			}
		}
	}
}

func (w writtenFields) walkValue(expr ast.Expr, parent *openapi3.Schema, name string) {
	if parent.Properties == nil {
		parent.Properties = openapi3.Schemas{}
	}
	existing := parent.Properties[name]
	switch v := expr.(type) {
	case *ast.StructLit:
		if existing == nil || existing.Value == nil {
			existing = openapi3.NewSchemaRef("", openapi3.NewObjectSchema())
			parent.Properties[name] = existing
		}
		if typeIs(existing.Value, openapi3.TypeObject) {
			w.walkStruct(v, existing.Value)
		}
	case *ast.ListLit:
		if existing == nil || existing.Value == nil {
			existing = openapi3.NewSchemaRef("", openapi3.NewArraySchema())
			parent.Properties[name] = existing
		}
		if !typeIs(existing.Value, openapi3.TypeArray) {
			return
		}
		for _, elem := range v.Elts {
			lit, ok := elem.(*ast.StructLit)
			if !ok {
				continue
			}
			if existing.Value.Items == nil || existing.Value.Items.Value == nil {
				existing.Value.Items = openapi3.NewSchemaRef("", openapi3.NewObjectSchema())
			}
			if typeIs(existing.Value.Items.Value, openapi3.TypeObject) {
				w.walkStruct(lit, existing.Value.Items.Value)
			}
		}
	default:
		if existing == nil || existing.Value == nil {
			parent.Properties[name] = openapi3.NewSchemaRef("", w.typeOf(v))
		}
	}
}

// typeOf is the schema of a value written as an expression: a literal's kind,
// a parameter's schema, a context field's string, or any type.
func (w writtenFields) typeOf(expr ast.Expr) *openapi3.Schema {
	switch v := expr.(type) {
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			return openapi3.NewStringSchema()
		case token.INT:
			return openapi3.NewIntegerSchema()
		case token.FLOAT:
			return openapi3.NewFloat64Schema()
		case token.TRUE, token.FALSE:
			return openapi3.NewBoolSchema()
		default:
			// Other kinds need nothing here.
		}
	case *ast.Interpolation:
		return openapi3.NewStringSchema()
	case *ast.UnaryExpr:
		if v.Op == token.MUL {
			return w.typeOf(v.X)
		}
	case *ast.BinaryExpr:
		if v.Op == token.OR {
			if s := w.typeOf(v.X); s.Type != nil {
				return s
			}
			return w.typeOf(v.Y)
		}
	case *ast.SelectorExpr, *ast.IndexExpr:
		root, path := selectorPath(v)
		switch root {
		case process.ParameterFieldName:
			if s := lookup(w.params, path); s != nil {
				c := *s
				return &c
			}
		case "context":
			return openapi3.NewStringSchema()
		}
	}
	return &openapi3.Schema{}
}

// step is one selection below a path's root: a field or key by name, or an
// index whose key is not a literal string.
type step struct {
	name  string
	index bool
}

// selectorPath reads `a.b["k"][0].c` as its root and the steps below it.
func selectorPath(expr ast.Expr) (string, []step) {
	var path []step
	for {
		switch v := expr.(type) {
		case *ast.SelectorExpr:
			name, ok := astLabelName(v.Sel)
			if !ok {
				return "", nil
			}
			path = append([]step{{name: name}}, path...)
			expr = v.X
		case *ast.IndexExpr:
			s := step{index: true}
			if lit, ok := v.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if key, err := strconv.Unquote(lit.Value); err == nil {
					s = step{name: key}
				}
			}
			path = append([]step{s}, path...)
			expr = v.X
		case *ast.Ident:
			return v.Name, path
		default:
			return "", nil
		}
	}
}

// lookup follows a path through a schema: a name to the property, or to a
// map's values where there is none, and an index to a list's items or a
// map's values.
func lookup(s *openapi3.Schema, path []step) *openapi3.Schema {
	for _, p := range path {
		if s == nil {
			return nil
		}
		if ref, ok := s.Properties[p.name]; ok && !p.index {
			if ref == nil {
				return nil
			}
			s = ref.Value
			continue
		}
		switch {
		case p.index && s.Items != nil:
			s = s.Items.Value
		case s.AdditionalProperties.Schema != nil:
			s = s.AdditionalProperties.Schema.Value
		default:
			return nil
		}
	}
	return s
}

func onlyIfClauses(clauses []ast.Clause) bool {
	for _, c := range clauses {
		if _, ok := c.(*ast.IfClause); !ok {
			return false
		}
	}
	return len(clauses) > 0
}

// astLabelName is a field's name where the template writes it literally; a
// computed name, a definition or a hidden field has none a reader can use.
func astLabelName(l ast.Label) (string, bool) {
	switch v := l.(type) {
	case *ast.Ident:
		if len(v.Name) == 0 || v.Name[0] == '#' || v.Name[0] == '_' {
			return "", false
		}
		return v.Name, true
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		name, err := strconv.Unquote(v.Value)
		return name, err == nil
	}
	return "", false
}

func typeIs(s *openapi3.Schema, t string) bool {
	return s.Type != nil && s.Type.Is(t)
}

// withStatus sets an object's status to any type, replacing whatever the
// template may have written there.
func withStatus(s *openapi3.Schema) {
	if s.Properties == nil {
		s.Properties = openapi3.Schemas{}
	}
	s.Properties[statusFieldName] = openapi3.NewSchemaRef("", &openapi3.Schema{})
}

// Extend lays the output schemas of a definition that extends this one over
// this one's, as its render merges them: field by field, with outputs joining
// by name. output and outputs say whether the child inherits each; one it does
// not is the child's alone. A field the child writes untyped, such as one read
// from `$super`, keeps the type this one gives it.
func (s *OutputSchemas) Extend(child *OutputSchemas, output, outputs bool) *OutputSchemas {
	if s == nil {
		s = &OutputSchemas{}
	}
	if child == nil {
		child = &OutputSchemas{}
	}
	out := &OutputSchemas{Output: child.Output, Outputs: child.Outputs}
	if output {
		out.Output = overlay(s.Output, child.Output)
	}
	if outputs && len(s.Outputs) > 0 {
		out.Outputs = map[string]*openapi3.Schema{}
		for k, v := range s.Outputs {
			out.Outputs[k] = v
		}
		for k, v := range child.Outputs {
			out.Outputs[k] = overlay(s.Outputs[k], v)
		}
	}
	return out
}

// overlay is child laid over parent: the child's type where it gives one, and
// the fields of both.
func overlay(parent, child *openapi3.Schema) *openapi3.Schema {
	if parent == nil {
		return child
	}
	if child == nil {
		return parent
	}
	out := *child
	if out.Type == nil {
		out.Type = parent.Type
	}
	if len(parent.Properties) > 0 {
		out.Properties = openapi3.Schemas{}
		for k, ref := range parent.Properties {
			out.Properties[k] = ref
		}
		for k, ref := range child.Properties {
			out.Properties[k] = openapi3.NewSchemaRef("", overlay(refValue(parent.Properties[k]), refValue(ref)))
		}
	}
	if parent.Items != nil {
		out.Items = openapi3.NewSchemaRef("", overlay(refValue(parent.Items), refValue(child.Items)))
	}
	return &out
}

func refValue(ref *openapi3.SchemaRef) *openapi3.Schema {
	if ref == nil {
		return nil
	}
	return ref.Value
}
