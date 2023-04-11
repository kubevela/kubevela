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
	"fmt"
	goast "go/ast"
	gotoken "go/token"
	gotypes "go/types"
	"strconv"
	"strings"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
)

func (g *Generator) convertDecls(x *goast.GenDecl) (decls []cueast.Decl, _ error) {
	// TODO(iyear): currently only support 'type'
	if x.Tok != gotoken.TYPE {
		return
	}

	for _, spec := range x.Specs {
		typeSpec, ok := spec.(*goast.TypeSpec)
		if !ok {
			continue
		}

		if g.opts.typeFilter != nil && !g.opts.typeFilter(typeSpec) {
			continue
		}

		// only process struct
		typ := g.pkg.TypesInfo.TypeOf(typeSpec.Name)

		if err := supportedType(nil, typ); err != nil {
			return nil, fmt.Errorf("unsupported type %s: %w", typeSpec.Name.Name, err)
		}

		named, ok := typ.(*gotypes.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*gotypes.Struct)
		if !ok {
			continue
		}

		lit, err := g.convert(st)
		if err != nil {
			return nil, err
		}

		field := &cueast.Field{
			Label: Ident(typeSpec.Name.Name, false),
			Value: lit,
		}
		// there is no doc for typeSpec, so we only add x.Doc
		makeComments(field, &commentUnion{comment: nil, doc: x.Doc})

		cueast.SetRelPos(field, cuetoken.Newline)
		decls = append(decls, field)
	}

	return decls, nil
}

func (g *Generator) convert(typ gotypes.Type) (cueast.Expr, error) {
	// if type is registered as any, return {...}
	if _, ok := g.opts.anyTypes[typ.String()]; ok {
		return anyLit(), nil
	}

	switch t := typ.(type) {
	case *gotypes.Basic:
		return basicType(t), nil
	case *gotypes.Named:
		return g.convert(t.Underlying())
	case *gotypes.Struct:
		return g.makeStructLit(t)
	case *gotypes.Pointer:
		expr, err := g.convert(t.Elem())
		if err != nil {
			return nil, err
		}

		// generate null enum for pointer type
		if g.opts.nullable {
			return &cueast.BinaryExpr{
				X:  cueast.NewNull(),
				Op: cuetoken.OR,
				Y:  expr,
			}, nil
		}
		return expr, nil
	case *gotypes.Slice:
		if t.Elem().String() == "byte" {
			return Ident("bytes", false), nil
		}
		expr, err := g.convert(t.Elem())
		if err != nil {
			return nil, err
		}
		return cueast.NewList(&cueast.Ellipsis{Type: expr}), nil
	case *gotypes.Array:
		if t.Elem().String() == "byte" {
			// TODO: no way to constraint lengths of bytes for now, as regexps
			// operate on Unicode, not bytes. So we need
			//     fmt.Fprint(e.w, fmt.Sprintf("=~ '^\C{%d}$'", x.Len())),
			// but regexp does not support that.
			// But translate to bytes, instead of [...byte] to be consistent.
			return Ident("bytes", false), nil
		}

		expr, err := g.convert(t.Elem())
		if err != nil {
			return nil, err
		}
		return &cueast.BinaryExpr{
			X: &cueast.BasicLit{
				Kind:  cuetoken.INT,
				Value: strconv.Itoa(int(t.Len())),
			},
			Op: cuetoken.MUL,
			Y:  cueast.NewList(expr),
		}, nil
	case *gotypes.Map:
		// cue map only support string as key
		if b, ok := t.Key().Underlying().(*gotypes.Basic); !ok || b.Kind() != gotypes.String {
			return nil, fmt.Errorf("unsupported map key type %s of %s", t.Key(), t)
		}

		expr, err := g.convert(t.Elem())
		if err != nil {
			return nil, err
		}

		f := &cueast.Field{
			Label: cueast.NewList(Ident("string", false)),
			Value: expr,
		}
		return &cueast.StructLit{
			Elts: []cueast.Decl{f},
		}, nil
	case *gotypes.Interface:
		// we don't process interface
		return Ident("_", false), nil
	}

	return nil, fmt.Errorf("unsupported type %s", typ)
}

func (g *Generator) makeStructLit(x *gotypes.Struct) (*cueast.StructLit, error) {
	st := &cueast.StructLit{
		Elts: make([]cueast.Decl, 0),
	}

	// if num of fields is 1, we don't need braces. Keep it simple.
	if x.NumFields() > 1 {
		st.Lbrace = cuetoken.Blank.Pos()
		st.Rbrace = cuetoken.Newline.Pos()
	}

	err := g.addFields(st, x, map[string]struct{}{})
	if err != nil {
		return nil, err
	}

	return st, nil
}

// addFields converts fields of go struct to CUE fields and add them to cue StructLit.
func (g *Generator) addFields(st *cueast.StructLit, x *gotypes.Struct, names map[string]struct{}) error {
	comments := g.fieldComments(x)

	for i := 0; i < x.NumFields(); i++ {
		field := x.Field(i)

		// skip unexported fields
		if !field.Exported() {
			continue
		}

		// TODO(iyear): support more complex tags and usages
		opts := g.parseTag(x.Tag(i))

		// skip fields with "-" tag
		if opts.Name == "-" {
			continue
		}

		// if field name tag is empty, use Go field name
		if opts.Name == "" {
			opts.Name = field.Name()
		}

		// can't decl same field in the same scope
		if _, ok := names[opts.Name]; ok {
			return fmt.Errorf("field '%s' already exists, can not declare duplicate field name", opts.Name)
		}
		names[opts.Name] = struct{}{}

		// process anonymous field with inline tag
		if field.Anonymous() && opts.Inline {
			if t, ok := field.Type().Underlying().(*gotypes.Struct); ok {
				if err := g.addFields(st, t, names); err != nil {
					return err
				}
			}
			continue
		}

		var (
			expr cueast.Expr
			err  error
		)
		switch {
		// process field with enum tag
		case opts.Enum != nil && len(opts.Enum) > 0:
			expr, err = g.enumField(field.Type(), opts)
		// process normal field
		default:
			expr, err = g.normalField(field.Type(), opts)
		}
		if err != nil {
			return fmt.Errorf("field '%s': %w", opts.Name, err)
		}

		f := &cueast.Field{
			Label: Ident(opts.Name, false),
			Value: expr,
		}

		// process field with optional tag(omitempty in json tag)
		if opts.Optional {
			f.Token = cuetoken.COLON
			f.Optional = cuetoken.Blank.Pos()
		}

		makeComments(f, comments[i])

		st.Elts = append(st.Elts, f)
	}

	return nil
}

func (g *Generator) enumField(typ gotypes.Type, opts *tagOptions) (cueast.Expr, error) {
	tt, ok := typ.(*gotypes.Basic)
	if !ok {
		// TODO(iyear): support more types
		return nil, fmt.Errorf("enum value only support [int, float, string, bool]")
	}

	expr, err := basicLabel(tt, opts.Enum[0])
	if err != nil {
		return nil, err
	}

	for _, v := range opts.Enum[1:] {
		enumExpr, err := basicLabel(tt, v)
		if err != nil {
			return nil, err
		}

		// default value should be marked with *
		if opts.Default != nil && *opts.Default == v {
			enumExpr = &cueast.UnaryExpr{Op: cuetoken.MUL, X: enumExpr}
		}

		expr = &cueast.BinaryExpr{
			X:  expr,
			Op: cuetoken.OR,
			Y:  enumExpr,
		}
	}

	return expr, nil
}

func (g *Generator) normalField(typ gotypes.Type, opts *tagOptions) (cueast.Expr, error) {
	expr, err := g.convert(typ)
	if err != nil {
		return nil, err
	}

	// process field with default tag
	if opts.Default != nil {
		tt, ok := typ.(*gotypes.Basic)
		if !ok {
			// TODO(iyear): support more types
			return nil, fmt.Errorf("default value only support [int, float, string, bool]")
		}

		defaultExpr, err := basicLabel(tt, *opts.Default)
		if err != nil {
			return nil, err
		}
		expr = &cueast.BinaryExpr{
			// default value should be marked with *
			X:  &cueast.UnaryExpr{Op: cuetoken.MUL, X: defaultExpr},
			Op: cuetoken.OR,
			Y:  expr,
		}
	}

	return expr, nil
}

func supportedType(stack []gotypes.Type, t gotypes.Type) error {
	// we expand structures recursively, so we can't support recursive types
	for _, t0 := range stack {
		if t0 == t {
			return fmt.Errorf("recursive type %s", t)
		}
	}
	stack = append(stack, t)

	t = t.Underlying()
	switch x := t.(type) {
	case *gotypes.Basic:
		if x.String() != "invalid type" {
			return nil
		}
		return fmt.Errorf("unsupported type %s", t)
	case *gotypes.Named:
		return nil
	case *gotypes.Pointer:
		return supportedType(stack, x.Elem())
	case *gotypes.Slice:
		return supportedType(stack, x.Elem())
	case *gotypes.Array:
		return supportedType(stack, x.Elem())
	case *gotypes.Map:
		if b, ok := x.Key().Underlying().(*gotypes.Basic); !ok || b.Kind() != gotypes.String {
			return fmt.Errorf("unsupported map key type %s of %s", x.Key(), t)
		}
		return supportedType(stack, x.Elem())
	case *gotypes.Struct:
		// Eliminate structs with fields for which all fields are filtered.
		if x.NumFields() == 0 {
			return nil
		}
		for i := 0; i < x.NumFields(); i++ {
			f := x.Field(i)
			if f.Exported() {
				if err := supportedType(stack, f.Type()); err != nil {
					return err
				}
			}
		}
		return nil
	case *gotypes.Interface:
		return nil
	}
	return fmt.Errorf("unsupported type %s", t)
}

// ----------comment----------

type commentUnion struct {
	comment *goast.CommentGroup
	doc     *goast.CommentGroup
}

// fieldComments returns the comments for each field in a go struct.
//
// The comments are same order as the fields.
func (g *Generator) fieldComments(x *gotypes.Struct) []*commentUnion {
	comments := make([]*commentUnion, x.NumFields())

	st, ok := g.types[x]
	if !ok {
		return comments
	}

	for i, field := range st.Fields.List {
		comments[i] = &commentUnion{comment: field.Comment, doc: field.Doc}
	}

	return comments
}

// makeComments adds comments to a cue node.
//
// go docs/comments are converted to cue comments.
func makeComments(node cueast.Node, c *commentUnion) {
	if c == nil {
		return
	}
	cg := make([]*cueast.Comment, 0)

	if comment := makeComment(c.comment); comment != nil && len(comment.List) > 0 {
		cg = append(cg, comment.List...)
	}
	if doc := makeComment(c.doc); doc != nil && len(doc.List) > 0 {
		cg = append(cg, doc.List...)
	}

	// avoid nil comment groups which will cause panics
	if len(cg) > 0 {
		cueast.AddComment(node, &cueast.CommentGroup{List: cg})
	}
}

// makeComment converts a go CommentGroup to a cue CommentGroup.
//
// All /*-style comments are converted to //-style comments.
func makeComment(cg *goast.CommentGroup) *cueast.CommentGroup {
	if cg == nil {
		return nil
	}

	var comments []*cueast.Comment

	for _, comment := range cg.List {
		c := comment.Text

		if len(c) < 2 {
			continue
		}

		// Remove comment markers.
		// The parser has given us exactly the comment text.
		switch c[1] {
		case '/':
			// -style comment (no newline at the end)
			comments = append(comments, &cueast.Comment{Text: c})

		case '*':
			/*-style comment */
			c = c[2 : len(c)-2]
			if len(c) > 0 && c[0] == '\n' {
				c = c[1:]
			}

			lines := strings.Split(c, "\n")

			// Find common space prefix
			i := 0
			line := lines[0]
			for ; i < len(line); i++ {
				if c := line[i]; c != ' ' && c != '\t' {
					break
				}
			}

			for _, l := range lines {
				for j := 0; j < i && j < len(l); j++ {
					if line[j] != l[j] {
						i = j
						break
					}
				}
			}

			// Strip last line if empty.
			if n := len(lines); n > 1 && len(lines[n-1]) < i {
				lines = lines[:n-1]
			}

			// Print lines.
			for _, l := range lines {
				if i >= len(l) {
					comments = append(comments, &cueast.Comment{Text: "//"})
					continue
				}
				comments = append(comments, &cueast.Comment{Text: "// " + l[i:]})
			}
		}
	}

	return &cueast.CommentGroup{List: comments}
}
