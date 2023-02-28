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
			Label: cueast.NewString(typeSpec.Name.Name),
			Value: lit,
		}
		// there is no doc for typeSpec, so we only add x.Doc
		makeComments(field, &commentUnion{comment: nil, doc: x.Doc})

		cueast.SetRelPos(field, cuetoken.Blank)
		decls = append(decls, field)
	}

	return decls, nil
}

func (g *Generator) convert(typ gotypes.Type) (cueast.Expr, error) {
	// if type is registered as any, return {...}
	if _, ok := g.anyTypes[typ.String()]; ok {
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
		return &cueast.BinaryExpr{
			X:  cueast.NewNull(),
			Op: cuetoken.OR,
			Y:  expr,
		}, nil
	case *gotypes.Slice:
		if t.Elem().String() == "byte" {
			return ident("bytes", false), nil
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
			return ident("bytes", false), nil
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
			Label: cueast.NewList(ident("string", false)),
			Value: expr,
		}
		return &cueast.StructLit{
			Elts: []cueast.Decl{f},
		}, nil
	case *gotypes.Interface:
		// we don't process interface
		return ident("_", false), nil
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
		// empty enum will be parsed as [""], so we need to check the first element
		case opts.Enum != nil && len(opts.Enum) > 0 && opts.Enum[0] != "":
			expr, err = g.enumField(field.Type(), opts)
		// process normal field
		default:
			expr, err = g.normalField(field.Type(), opts)
		}
		if err != nil {
			return fmt.Errorf("field '%s': %w", opts.Name, err)
		}

		f := &cueast.Field{
			Label: cueast.NewString(opts.Name),
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
