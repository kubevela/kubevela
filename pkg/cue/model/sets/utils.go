/*
Copyright 2021 The KubeVela Authors.

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

package sets

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue/parser"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/token"
	"github.com/pkg/errors"
)

func lookUp(node ast.Node, paths ...string) (ast.Node, error) {
	if len(paths) == 0 {
		return peelCloseExpr(node), nil
	}
	key := paths[0]
	switch x := node.(type) {
	case *ast.File:
		for _, decl := range x.Decls {
			nnode := lookField(decl, key)
			if nnode != nil {
				return lookUp(nnode, paths[1:]...)
			}
		}
	case *ast.ListLit:
		for index, elt := range x.Elts {
			if strconv.Itoa(index) == key {
				return lookUp(elt, paths[1:]...)
			}
		}
	case *ast.StructLit:
		for _, elt := range x.Elts {
			nnode := lookField(elt, key)
			if nnode != nil {
				return lookUp(nnode, paths[1:]...)
			}
		}
	case *ast.CallExpr:
		if it, ok := x.Fun.(*ast.Ident); ok && it.Name == "close" && len(x.Args) == 1 {
			return lookUp(x.Args[0], paths...)
		}
		for index, arg := range x.Args {
			if strconv.Itoa(index) == key {
				return lookUp(arg, paths[1:]...)
			}
		}
	}
	return nil, notFoundErr
}

func lookUpAll(node ast.Node, paths ...string) []ast.Node {
	if len(paths) == 0 {
		return []ast.Node{node}
	}
	key := paths[0]
	var nodes []ast.Node
	switch x := node.(type) {
	case *ast.File:
		for _, decl := range x.Decls {
			nnode := lookField(decl, key)
			if nnode != nil {
				nodes = append(nodes, lookUpAll(nnode, paths[1:]...)...)
			}
		}

	case *ast.StructLit:
		for _, elt := range x.Elts {
			nnode := lookField(elt, key)
			if nnode != nil {
				nodes = append(nodes, lookUpAll(nnode, paths[1:]...)...)
			}
		}
	case *ast.ListLit:
		for index, elt := range x.Elts {
			if strconv.Itoa(index) == key {
				return lookUpAll(elt, paths[1:]...)
			}
		}
	}
	return nodes
}

// PreprocessBuiltinFunc preprocess builtin function in cue file.
func PreprocessBuiltinFunc(root ast.Node, name string, process func(values []ast.Node) (ast.Expr, error)) error {
	var gerr error
	ast.Walk(root, func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.EmbedDecl:
			if fname, args := extractFuncName(v.Expr); fname == name && len(args) > 0 {
				expr, err := doBuiltinFunc(root, args[0], process)
				if err != nil {
					gerr = err
					return false
				}
				v.Expr = expr
			}
		case *ast.Field:
			if fname, args := extractFuncName(v.Value); fname == name && len(args) > 0 {
				expr, err := doBuiltinFunc(root, args[0], process)
				if err != nil {
					gerr = err
					return false
				}
				v.Value = expr
			}
		}
		return true
	}, nil)
	return gerr
}

func doBuiltinFunc(root ast.Node, pathSel ast.Expr, do func(values []ast.Node) (ast.Expr, error)) (ast.Expr, error) {
	paths := getPaths(pathSel)
	if len(paths) == 0 {
		return nil, errors.New("path resolve error")
	}
	values := lookUpAll(root, paths...)
	return do(values)
}

func extractFuncName(expr ast.Expr) (string, []ast.Expr) {
	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) > 0 {
		if ident, ok := call.Fun.(*ast.Ident); ok {
			return ident.Name, call.Args
		}
	}
	return "", nil
}

func getPaths(node ast.Expr) []string {
	switch v := node.(type) {
	case *ast.SelectorExpr:
		return append(getPaths(v.X), v.Sel.Name)
	case *ast.Ident:
		return []string{v.Name}
	case *ast.BasicLit:
		s, err := literal.Unquote(v.Value)
		if err != nil {
			return nil
		}
		return []string{s}
	case *ast.IndexExpr:
		return append(getPaths(v.X), getPaths(v.Index)...)
	}
	return nil
}

func peelCloseExpr(node ast.Node) ast.Node {
	x, ok := node.(*ast.CallExpr)
	if !ok {
		return node
	}
	if it, ok := x.Fun.(*ast.Ident); ok && it.Name == "close" && len(x.Args) == 1 {
		return x.Args[0]
	}
	return node
}

func lookField(node ast.Node, key string) ast.Node {
	if field, ok := node.(*ast.Field); ok {
		if labelStr(field.Label) == key {
			return field.Value
		}
	}
	return nil
}

func labelStr(label ast.Label) string {
	switch v := label.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	}
	return ""
}

func toString(v cue.Value, opts ...func(node ast.Node) ast.Node) (string, error) {
	v = v.Eval()
	syopts := []cue.Option{cue.All(), cue.DisallowCycles(true), cue.ResolveReferences(true), cue.Docs(true)}

	var w bytes.Buffer
	useSep := false
	format := func(name string, n ast.Node) error {
		if name != "" {
			fmt.Fprintf(&w, "// %s\n", filepath.Base(name))
		} else if useSep {
			fmt.Fprintf(&w, "// ---")
		}
		useSep = true

		f, err := toFile(n)
		if err != nil {
			return err
		}
		var node ast.Node = f
		for _, opt := range opts {
			node = opt(node)
		}
		b, err := format.Node(node, format.UseSpaces(2), format.TabIndent(false))
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}

	if err := format("", v.Syntax(syopts...)); err != nil {
		return "", err
	}
	instStr := w.String()
	return instStr, nil
}

// ToString convert cue.Value to string
func ToString(v cue.Value, opts ...func(node ast.Node) ast.Node) (string, error) {
	return toString(v, opts...)
}

// ToFile convert ast.Node to ast.File
func ToFile(n ast.Node) (*ast.File, error) {
	return toFile(n)
}

func toFile(n ast.Node) (*ast.File, error) {
	switch x := n.(type) {
	case nil:
		return nil, nil
	case *ast.StructLit:
		decls := []ast.Decl{}
		for _, elt := range x.Elts {
			if _, ok := elt.(*ast.Ellipsis); ok {
				continue
			}
			decls = append(decls, elt)
		}
		return &ast.File{Decls: decls}, nil
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}, nil
	case *ast.File:
		return x, nil
	default:
		return nil, errors.Errorf("Unsupported node type %T", x)
	}
}

// OptBytesToString convert cue bytes to string.
func OptBytesToString(node ast.Node) ast.Node {
	ast.Walk(node, nil, func(node ast.Node) {
		basic, ok := node.(*ast.BasicLit)
		if ok {
			if basic.Kind == token.STRING {
				s := basic.Value
				if strings.HasPrefix(s, "'") {
					info, nStart, _, err := literal.ParseQuotes(s, s)
					if err != nil {
						return
					}
					if !info.IsDouble() {
						s = s[nStart:]
						s, err := info.Unquote(s)
						if err == nil {
							basic.Value = fmt.Sprintf(`"%s"`, s)
						}
					}
				}
			}
		}
	})
	return node
}

// OpenBaiscLit make that the basicLit can be modified.
func OpenBaiscLit(s string) (string, error) {
	f, err := parser.ParseFile("-", s, parser.ParseComments)
	if err != nil {
		return "", err
	}
	openBaiscLit(f)
	b, err := format.Node(f)
	return string(b), err
}

func openBaiscLit(root ast.Node) {
	ast.Walk(root, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if ok {
			v := field.Value
			switch lit := v.(type) {
			case *ast.BasicLit:
				field.Value = ast.NewBinExpr(token.OR, &ast.UnaryExpr{X: lit, Op: token.MUL}, ast.NewIdent("_"))
			case *ast.ListLit:
				field.Value = ast.NewBinExpr(token.OR, &ast.UnaryExpr{X: lit, Op: token.MUL}, ast.NewList(&ast.Ellipsis{}))
			}
		}
		return true
	}, nil)
}

// ListOpen enable the cue list can add elements.
func ListOpen(expr ast.Node) ast.Node {
	listOpen(expr)
	return expr
}

func listOpen(expr ast.Node) {
	switch v := expr.(type) {
	case *ast.File:
		for _, decl := range v.Decls {
			listOpen(decl)
		}
	case *ast.Field:
		listOpen(v.Value)
	case *ast.StructLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
	case *ast.BinaryExpr:
		listOpen(v.X)
		listOpen(v.Y)
	case *ast.EmbedDecl:
		listOpen(v.Expr)
	case *ast.Comprehension:
		listOpen(v.Value)
	case *ast.ListLit:
		for _, elt := range v.Elts {
			listOpen(elt)
		}
		if len(v.Elts) > 0 {
			if _, ok := v.Elts[len(v.Elts)-1].(*ast.Ellipsis); !ok {
				v.Elts = append(v.Elts, &ast.Ellipsis{})
			}
		}
	}
}
