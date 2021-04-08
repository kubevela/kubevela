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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
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

func toString(v cue.Value) (string, error) {
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
		b, err := format.Node(f)
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

// ToFile convert ast.Node to ast.File
func ToFile(n ast.Node) (*ast.File, error) {
	return toFile(n)
}

func toFile(n ast.Node) (*ast.File, error) {
	switch x := n.(type) {
	case nil:
		return nil, nil
	case *ast.StructLit:
		return &ast.File{Decls: x.Elts}, nil
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}, nil
	case *ast.File:
		return x, nil
	default:
		return nil, errors.Errorf("Unsupported node type %T", x)
	}
}
