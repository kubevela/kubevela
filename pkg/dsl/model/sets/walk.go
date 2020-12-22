package sets

import (
	"strconv"

	"cuelang.org/go/cue/ast"
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
		if n.Value != nil {
			origin := nwk.pos
			oriTags := nwk.tags
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

	case *ast.Interpolation:

	case *ast.ListLit:
		nwk.walkExprList(n.Elts)

	case *ast.BinaryExpr:
		nwk.walk(n.X)
		nwk.walk(n.Y)

	case *ast.EmbedDecl:
		nwk.walk(n.Expr)

	case *ast.Comprehension:
		nwk.walk(n.Value)

	// Files and packages
	case *ast.File:
		nwk.walkDeclList(n.Decls)

	case *ast.Package:

	case *ast.ListComprehension:
		nwk.walk(n.Expr)

	case *ast.ForClause:

	case *ast.IfClause:

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
