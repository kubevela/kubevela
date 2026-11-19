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

package health

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// Inherits reports whether a status snippet composes with its parent's or takes
// it over with `$inherit: false`.
func Inherits(snippet string) bool { return inherits(snippet) }

// OutputsRead reports which entries of `context.outputs` a status snippet
// reads, as `context.outputs.svc`, `context.outputs["svc"]` or
// `context["outputs"]["svc"]`. all is set when it reads them by an index only
// known at evaluation, or the whole map, or when `context` itself is used in a
// way that cannot be followed here, such as through an alias or as a whole
// value. Reads of other context fields, such as `context.output`, name nothing.
func OutputsRead(snippet string) (keys map[string]bool, all bool) {
	keys = map[string]bool{}
	file, err := parser.ParseFile("-", snippet)
	if err != nil {
		// A snippet that does not parse reads nothing that can be named; it fails
		// on its own when the status is evaluated.
		return keys, false
	}
	// The nodes enclosing the one being visited, nearest last.
	var stack []ast.Node
	ast.Walk(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "context" && len(stack) > 0 && isReference(id, stack[len(stack)-1]) {
			if !outputsReadAt(id, stack, keys) {
				all = true
			}
		}
		stack = append(stack, n)
		return true
	}, func(ast.Node) { stack = stack[:len(stack)-1] })
	return keys, all
}

// outputsReadAt accounts for one reference to `context`, whose enclosing nodes
// are stack. It records the output the reference reads, if any, and reports
// false when what it reads cannot be told, which then counts as all of them.
func outputsReadAt(id *ast.Ident, stack []ast.Node, keys map[string]bool) bool {
	parent := stack[len(stack)-1]
	field, ok := memberOf(parent, id)
	if !ok {
		return false
	}
	if field != "outputs" {
		return true
	}
	if len(stack) < 2 {
		return false
	}
	key, ok := memberOf(stack[len(stack)-2], parent.(ast.Expr))
	if !ok {
		return false
	}
	keys[key] = true
	return true
}

// memberOf names the field outer reads off x, as `x.name` or `x["name"]`. It
// reports false when outer does not read a field off x by a name known here:
// x is used as a whole value, or indexed by something only known at
// evaluation.
func memberOf(outer ast.Node, x ast.Expr) (string, bool) {
	switch e := outer.(type) {
	case *ast.SelectorExpr:
		if e.X == x {
			return selectorName(e.Sel), true
		}
	case *ast.IndexExpr:
		if e.X != x {
			return "", false
		}
		if lit, ok := e.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := literal.Unquote(lit.Value); err == nil {
				return s, true
			}
		}
	}
	return "", false
}

// isReference reports whether id, found under parent, refers to a value rather
// than naming one: not a field label, a selector, or a name being declared.
func isReference(id *ast.Ident, parent ast.Node) bool {
	switch p := parent.(type) {
	case *ast.SelectorExpr:
		return p.Sel != id
	case *ast.Field:
		return p.Label != id
	case *ast.LetClause:
		return p.Ident != id
	case *ast.Alias:
		return p.Ident != id
	case *ast.ForClause:
		return p.Key != id && p.Value != id
	case *ast.ImportSpec:
		return p.Name != id
	}
	return true
}

func selectorName(l ast.Label) string {
	switch v := l.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		if s, err := literal.Unquote(v.Value); err == nil {
			return s
		}
		return v.Value
	}
	return ""
}
