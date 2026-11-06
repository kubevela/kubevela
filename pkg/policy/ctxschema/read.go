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

// Package ctxschema reads what an Application-scoped policy declares about the
// context it supplies: the `schema: ctx` block of its template, which
// components and traits see as context.custom.
//
// Everything here reads the template's syntax and never evaluates it, so it is
// safe at admission: a policy that calls a provider is not made to call it.
package ctxschema

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

const (
	outputField   = "output"
	ctxField      = "ctx"
	policiesField = "policies"
	schemaField   = "schema"
	configField   = "config"
	enabledField  = "enabled"
)

// Info is what a policy template says about the context it supplies.
type Info struct {
	// SetsCtx is whether the template may set output.ctx. A template whose
	// output cannot be read without evaluating it counts as setting it.
	SetsCtx bool
	// SetsPolicies is whether the template may add policies through
	// output.policies, which admission does not see.
	SetsPolicies bool
	// Schema is the `schema: ctx` block as a self-contained CUE file, for
	// Compile, or empty when the template declares none.
	Schema string
	// Conditional is whether the policy may not apply: its enabled flag is
	// anything but the literal true.
	Conditional bool
	// SchemaUnread is whether the template has a schema field whose ctx
	// admission cannot read from syntax (a reference, a comprehension or an
	// embedding), which the render still evaluates.
	SchemaUnread bool
}

// Read reads a policy template's context declarations from its syntax.
func Read(template string) (Info, error) {
	f, err := parser.ParseFile("", template)
	if err != nil {
		return Info{}, err
	}
	var info Info
	var schemas []ast.Expr
	readDecls(f.Decls, false, &info, &schemas)
	if len(schemas) > 0 {
		info.Schema, err = schemaFile(f, schemas)
		if err != nil {
			return Info{}, err
		}
	}
	return info, nil
}

// readDecls reads the top-level declarations. Those under a comprehension only
// apply when it does, so a config there makes the policy conditional, and a
// schema there is not read: a schema that may not exist is not one admission
// can judge by.
func readDecls(decls []ast.Decl, underComprehension bool, info *Info, schemas *[]ast.Expr) {
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.Field:
			name, ok := staticLabel(d.Label)
			if !ok {
				// A computed or pattern label may be any field: assume it is each of them.
				info.SetsCtx, info.SetsPolicies, info.Conditional = true, true, true
				info.SchemaUnread = info.SchemaUnread || mayHaveField(d.Value, ctxField)
				break
			}
			switch name {
			case outputField:
				info.SetsCtx = info.SetsCtx || mayHaveField(d.Value, ctxField)
				info.SetsPolicies = info.SetsPolicies || mayHaveField(d.Value, policiesField)
			case schemaField:
				if underComprehension {
					info.SchemaUnread = info.SchemaUnread || mayHaveField(d.Value, ctxField)
					break
				}
				exprs, unread := schemaCtxExprs(d.Value)
				*schemas = append(*schemas, exprs...)
				info.SchemaUnread = info.SchemaUnread || unread
			case enabledField:
				info.Conditional = info.Conditional || underComprehension || !isTrue(d.Value)
			case configField:
				info.Conditional = info.Conditional || underComprehension || configConditional(d.Value)
			}
		case *ast.Comprehension:
			if st, ok := d.Value.(*ast.StructLit); ok {
				readDecls(st.Elts, true, info, schemas)
			}
		case *ast.EmbedDecl:
			// An embedded value may carry output or config: unknown, so assume both.
			info.SetsCtx = true
			info.SetsPolicies = true
			info.Conditional = true
		}
	}
}

// mayHaveField reports whether a struct may have the named field, counting
// anything that cannot be read without evaluation as having it.
func mayHaveField(v ast.Expr, name string) bool {
	switch x := v.(type) {
	case *ast.StructLit:
		for _, elt := range x.Elts {
			switch e := elt.(type) {
			case *ast.Field:
				if label, ok := staticLabel(e.Label); !ok || label == name {
					return true
				}
			case *ast.Comprehension:
				if mayHaveField(e.Value, name) {
					return true
				}
			case *ast.EmbedDecl:
				return true
			}
		}
		return false
	case *ast.BinaryExpr:
		if x.Op == token.AND {
			return mayHaveField(x.X, name) || mayHaveField(x.Y, name)
		}
	case *ast.ParenExpr:
		return mayHaveField(x.X, name)
	}
	return true
}

// configConditional reports whether a config block may make the policy not
// apply. A config without enabled leaves the policy on.
func configConditional(v ast.Expr) bool {
	st, ok := v.(*ast.StructLit)
	if !ok {
		return true
	}
	for _, elt := range st.Elts {
		switch e := elt.(type) {
		case *ast.Field:
			if name, ok := staticLabel(e.Label); !ok || name == enabledField && !isTrue(e.Value) {
				return true
			}
		case *ast.Comprehension:
			if mayHaveField(e.Value, enabledField) {
				return true
			}
		case *ast.EmbedDecl:
			return true
		}
	}
	return false
}

func isTrue(v ast.Expr) bool {
	lit, ok := v.(*ast.BasicLit)
	return ok && lit.Kind == token.TRUE
}

// schemaCtxExprs returns the ctx expressions a schema block declares as fields
// of a struct literal, and whether it may declare ctx in a way syntax does not
// show: as a value other than a struct literal, through a comprehension or an
// embedding, or under a label computed at render.
func schemaCtxExprs(v ast.Expr) ([]ast.Expr, bool) {
	st, ok := v.(*ast.StructLit)
	if !ok {
		return nil, true
	}
	var out []ast.Expr
	unread := false
	for _, elt := range st.Elts {
		switch e := elt.(type) {
		case *ast.Field:
			name, ok := staticLabel(e.Label)
			if !ok {
				unread = true
			} else if name == ctxField {
				out = append(out, e.Value)
			}
		case *ast.Comprehension:
			unread = unread || mayHaveField(e.Value, ctxField)
		case *ast.EmbedDecl:
			unread = true
		}
	}
	return out, unread
}

// schemaFile writes the schema as a file of its own: the imports it uses, and
// the ctx expressions unified under a ctx field.
func schemaFile(f *ast.File, exprs []ast.Expr) (string, error) {
	used := identsIn(exprs)
	var b strings.Builder
	for _, spec := range f.Imports {
		name := importName(spec)
		if !used[name] {
			continue
		}
		if spec.Name != nil {
			fmt.Fprintf(&b, "import %s %s\n", spec.Name.Name, spec.Path.Value)
		} else {
			fmt.Fprintf(&b, "import %s\n", spec.Path.Value)
		}
	}
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		src, err := cueformat.Node(e)
		if err != nil {
			return "", err
		}
		parts = append(parts, "("+string(src)+")")
	}
	fmt.Fprintf(&b, "%s: %s\n", ctxField, strings.Join(parts, " & "))
	return b.String(), nil
}

// identsIn returns the identifiers the expressions reference, leaving out field
// labels, which do not refer to an import.
func identsIn(exprs []ast.Expr) map[string]bool {
	used := map[string]bool{}
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			ast.Walk(x.Value, visit, nil)
			return false
		case *ast.Ident:
			used[x.Name] = true
		}
		return true
	}
	for _, e := range exprs {
		ast.Walk(e, visit, nil)
	}
	return used
}

// importName is the name an import is referred to by: its alias, or the last
// element of its path. A schema compiles on its own, so only the standard
// library is of use to it, and its paths carry no qualifier.
func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	p, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return ""
	}
	return path.Base(p)
}

// staticLabel returns a field's label and whether syntax gives it: a label
// computed at render, such as (k) or an interpolation, and a pattern such as
// [string] have none.
func staticLabel(l ast.Label) (string, bool) {
	name, _, err := ast.LabelName(l)
	return name, err == nil
}

// Compile compiles a schema Read returned. It refuses a schema that does not
// stand alone, that contradicts itself, or that is not a struct.
func Compile(cctx *cue.Context, schema string) (cue.Value, error) {
	v := cctx.CompileString(schema)
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("schema.ctx: %w", err)
	}
	s := v.LookupPath(cue.ParsePath(ctxField))
	if s.IncompleteKind() != cue.StructKind {
		return cue.Value{}, fmt.Errorf("schema.ctx must be a struct, not %v", s.IncompleteKind())
	}
	return s, nil
}
