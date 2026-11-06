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

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/openapi"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// boundedDefaultError is how CUE's OpenAPI encoder fails on a number that has
// both a default and a bound, such as `*1 | int & >=1 & <=5`, when references
// are expanded (cue-lang/cue#4305, fixed in CUE releases after the one KubeVela
// uses). The encoder peels the default off and hands the remaining conjunction
// to its number builder unsplit.
const boundedDefaultError = "unsupported op for number"

var expandRefs = &openapi.Config{ExpandReferences: true}

// encodeOpenAPI encodes params, the parameter value refine made from template,
// as OpenAPI with references expanded. When the encoder hits the bounded-default
// bug, the template is encoded again with its number defaults taken out, which
// the encoder handles, and those defaults are written back into the schema. Any
// other error, or a failure of that second pass, returns the original error.
func encodeOpenAPI(template, params cue.Value, refine func(cue.Value) (cue.Value, error)) ([]byte, error) {
	b, err := openapi.Gen(params, expandRefs)
	if err == nil || !strings.Contains(err.Error(), boundedDefaultError) {
		return b, err
	}
	out, ferr := encodeWithoutNumberDefaults(template, params, refine)
	if ferr != nil {
		return nil, err
	}
	return out, nil
}

// encodeWithoutNumberDefaults rebuilds the template's parameter and its helper
// definitions without the number defaults that have a bound, encodes that the
// usual way, and restores the defaults from params. Everything else, shapes, descriptions and bounds,
// comes from the encoder as before.
func encodeWithoutNumberDefaults(template, params cue.Value, refine func(cue.Value) (cue.Value, error)) ([]byte, error) {
	if !template.LookupPath(cue.ParsePath(process.ParameterFieldName)).Exists() {
		return openapi.Gen(params, expandRefs)
	}
	src, err := parameterSourceWithoutNumberDefaults(template)
	if err != nil {
		return nil, err
	}
	stripped := template.Context().CompileBytes(src)
	if stripped.Err() != nil {
		return nil, stripped.Err()
	}
	strippedParams, err := refine(stripped)
	if err != nil {
		return nil, err
	}
	b, err := openapi.Gen(strippedParams, expandRefs)
	if err != nil {
		return nil, err
	}
	return restoreNumberDefaults(b, params)
}

// parameterSourceWithoutNumberDefaults prints the template's parameter field
// and top-level definitions as CUE, with every bounded number default removed.
// Unbounded ones, such as `*0 | int`, encode fine and stay, so a struct used as
// a default keeps concrete values. Fields
// the parameter refers to outside those are not printed, so such a template
// fails to compile here and keeps its original error.
func parameterSourceWithoutNumberDefaults(template cue.Value) ([]byte, error) {
	file := &ast.File{}
	imports := map[string]*ast.ImportSpec{}
	add := func(label string, v cue.Value) {
		expr, specs := splitSyntax(v.Syntax(cue.Docs(true)))
		for _, s := range specs {
			imports[s.Path.Value] = s
		}
		field := &ast.Field{Label: ast.NewIdent(label), Value: expr}
		// The field's own comments, such as the parameter block's description,
		// belong to the label rather than the value.
		for _, cg := range v.Doc() {
			ast.AddComment(field, cg)
		}
		file.Decls = append(file.Decls, field)
	}
	param := template.LookupPath(cue.ParsePath(process.ParameterFieldName))
	if !param.Exists() {
		return nil, fmt.Errorf("template has no %s", process.ParameterFieldName)
	}
	add(process.ParameterFieldName, param)
	it, err := template.Fields(cue.Definitions(true))
	if err != nil {
		return nil, err
	}
	for it.Next() {
		if it.Selector().IsDefinition() {
			add(it.Selector().String(), it.Value())
		}
	}
	if len(imports) > 0 {
		decl := &ast.ImportDecl{}
		for _, s := range imports {
			decl.Specs = append(decl.Specs, s)
		}
		file.Decls = append([]ast.Decl{decl}, file.Decls...)
	}
	astutil.Apply(file, func(c astutil.Cursor) bool {
		if e, ok := c.Node().(*ast.BinaryExpr); ok && e.Op == token.OR {
			if rest, removed := withoutNumberDefault(e); removed {
				c.Replace(rest)
			}
		}
		return true
	}, nil)
	return format.Node(file)
}

// splitSyntax returns a value's syntax as an expression and the imports it
// needs. A value that uses packages prints as a file with import declarations.
func splitSyntax(n ast.Node) (ast.Expr, []*ast.ImportSpec) {
	f, ok := n.(*ast.File)
	if !ok {
		if e, ok := n.(ast.Expr); ok {
			return e, nil
		}
		return &ast.StructLit{}, nil
	}
	var specs []*ast.ImportSpec
	lit := &ast.StructLit{}
	for _, d := range f.Decls {
		if imp, ok := d.(*ast.ImportDecl); ok {
			specs = append(specs, imp.Specs...)
			continue
		}
		lit.Elts = append(lit.Elts, d)
	}
	return lit, specs
}

// withoutNumberDefault drops a number default from a disjunction whose other
// side has a bound, so `*1 | int & >=1` becomes `int & >=1`. Other defaults are
// kept: the encoder only fails on bounded ones.
func withoutNumberDefault(e *ast.BinaryExpr) (ast.Expr, bool) {
	ds := disjuncts(e)
	bounded := false
	for _, d := range ds {
		if hasBound(d) {
			bounded = true
		}
	}
	if !bounded {
		return e, false
	}
	var rest []ast.Expr
	removed := false
	for _, d := range ds {
		if u, ok := d.(*ast.UnaryExpr); ok && u.Op == token.MUL && isNumberLit(u.X) {
			removed = true
			continue
		}
		rest = append(rest, d)
	}
	if !removed || len(rest) == 0 {
		return e, false
	}
	out := rest[0]
	for _, d := range rest[1:] {
		out = &ast.BinaryExpr{X: out, Op: token.OR, Y: d}
	}
	return out, true
}

func disjuncts(e ast.Expr) []ast.Expr {
	switch x := e.(type) {
	case *ast.BinaryExpr:
		if x.Op == token.OR {
			return append(disjuncts(x.X), disjuncts(x.Y)...)
		}
	case *ast.ParenExpr:
		if b, ok := x.X.(*ast.BinaryExpr); ok && b.Op == token.OR {
			return disjuncts(b)
		}
	}
	return []ast.Expr{e}
}

// hasBound reports whether e constrains a value with a comparison, such as >=1.
func hasBound(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.UnaryExpr:
		switch x.Op {
		case token.GEQ, token.LEQ, token.GTR, token.LSS, token.NEQ:
			return true
		}
	case *ast.BinaryExpr:
		return x.Op == token.AND && (hasBound(x.X) || hasBound(x.Y))
	case *ast.ParenExpr:
		return hasBound(x.X)
	}
	return false
}

func isNumberLit(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BasicLit:
		return x.Kind == token.INT || x.Kind == token.FLOAT
	case *ast.UnaryExpr:
		return (x.Op == token.SUB || x.Op == token.ADD) && isNumberLit(x.X)
	case *ast.ParenExpr:
		return isNumberLit(x.X)
	}
	return false
}

// restoreNumberDefaults writes the number defaults of params back into the
// parameter schema, where the encoder puts them: on the property itself.
func restoreNumberDefaults(doc []byte, params cue.Value) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var d map[string]any
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	components, _ := d["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	schema, ok := schemas[process.ParameterFieldName]
	if !ok {
		return nil, fmt.Errorf("schema has no %s", process.ParameterFieldName)
	}
	setNumberDefaults(params.LookupPath(cue.MakePath(cue.Def(process.ParameterFieldName))), schema)
	return json.Marshal(d)
}

func setNumberDefaults(v cue.Value, schema any) {
	s, ok := schema.(map[string]any)
	if !ok || !v.Exists() {
		return
	}
	if props, ok := s["properties"].(map[string]any); ok {
		if it, err := v.Fields(cue.Optional(true)); err == nil {
			for it.Next() {
				prop, ok := props[it.Selector().Unquoted()].(map[string]any)
				if !ok {
					continue
				}
				if _, has := prop["default"]; !has {
					if def, ok := it.Value().Default(); ok && def.IsConcrete() && def.Kind()&cue.NumberKind != 0 {
						if raw, err := def.MarshalJSON(); err == nil {
							prop["default"] = json.Number(raw)
						}
					}
				}
				setNumberDefaults(it.Value(), prop)
			}
		}
	}
	if items, ok := s["items"]; ok {
		setNumberDefaults(v.LookupPath(cue.MakePath(cue.AnyIndex)), items)
	}
}
