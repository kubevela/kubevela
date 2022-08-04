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
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

type nodewalker struct {
	pos     []string
	tags    map[string]string
	process walkProcess
}

type walkCtx interface {
	Pos() []string
	Tags() map[string]string
}

type walkProcess func(node ast.Node, ctx walkCtx)

func newWalker(process walkProcess) *nodewalker {
	return &nodewalker{
		pos:     []string{},
		process: process,
		tags:    map[string]string{},
	}
}

func (nwk *nodewalker) walk(node ast.Node) {
	if nwk.process != nil {
		nwk.process(node, nwk)
	}
	switch n := node.(type) {

	case *ast.Field:
		label := labelStr(n.Label)
		if label == "" || strings.HasPrefix(label, "#") {
			return
		}
		if n.Value != nil {
			origin := nwk.pos
			oriTags := nwk.tags
			nwk.tags = map[string]string{}
			for k, v := range oriTags {
				nwk.tags[k] = v
			}
			nwk.pos = append(nwk.pos, labelStr(n.Label))
			tags := findCommentTag(n.Comments())
			for tk, tv := range tags {
				nwk.tags[tk] = tv
			}

			nwk.walk(n.Value)
			nwk.tags = oriTags
			nwk.pos = origin
		}

	case *ast.StructLit:
		nwk.walkDeclList(n.Elts)

	case *ast.ListLit:
		nwk.walkExprList(n.Elts)

	case *ast.BinaryExpr:
		nwk.walk(n.X)
		nwk.walk(n.Y)

	case *ast.UnaryExpr:
		nwk.walk(n.X)

	case *ast.EmbedDecl:
		nwk.walk(n.Expr)

	case *ast.Comprehension:
		nwk.walk(n.Value)

	// Files
	case *ast.File:
		nwk.walkDeclList(n.Decls)

	case *ast.SliceExpr:
		if list, ok := n.X.(*ast.ListLit); ok {
			nwk.walkExprSlice(list.Elts, n.Low, n.High)
		}

	case *ast.CallExpr:
		// close func need to be ignored
		if it, ok := n.Fun.(*ast.Ident); ok && it.Name == "close" && len(n.Args) == 1 {
			nwk.walk(n.Args[0])
		} else {
			nwk.walkExprList(n.Args)
		}

	default:

	}

}

func (nwk *nodewalker) walkExprList(list []ast.Expr) {
	for i, x := range list {
		origin := nwk.pos
		nwk.pos = append(nwk.pos, strconv.Itoa(i))
		nwk.walk(x)
		nwk.pos = origin
	}
}

func (nwk *nodewalker) walkExprSlice(list []ast.Expr, low ast.Expr, high ast.Expr) {
	var (
		lowIndex  = 0
		highIndex = len(list)
	)
	if v, ok := low.(*ast.BasicLit); ok && v.Kind == token.INT {
		lowIndex, _ = strconv.Atoi(v.Value)
	}
	if v, ok := high.(*ast.BasicLit); ok && v.Kind == token.INT {
		highIndex, _ = strconv.Atoi(v.Value)
	}
	for i, x := range list {
		if i < lowIndex || i >= highIndex {
			continue
		}
		origin := nwk.pos
		nwk.pos = append(nwk.pos, strconv.Itoa(i-lowIndex))
		nwk.walk(x)
		nwk.pos = origin
	}
}

func (nwk *nodewalker) walkDeclList(list []ast.Decl) {
	for _, x := range list {
		nwk.walk(x)
	}
}

func (nwk *nodewalker) Pos() []string {
	return nwk.pos
}

func (nwk *nodewalker) Tags() map[string]string {
	return nwk.tags
}
