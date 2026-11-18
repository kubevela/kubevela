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
// reads, as `context.outputs.svc` or `context.outputs["svc"]`. all is set when
// it reads them by an index only known at evaluation, or the whole map.
func OutputsRead(snippet string) (keys map[string]bool, all bool) {
	keys = map[string]bool{}
	file, err := parser.ParseFile("-", snippet)
	if err != nil {
		// A snippet that does not parse reads nothing that can be named; it fails
		// on its own when the status is evaluated.
		return keys, false
	}
	// A `context.outputs` read through a named entry is seen first, as the
	// parent of the bare `context.outputs` under it, which it then accounts for.
	named := map[ast.Expr]bool{}
	ast.Walk(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if isContextOutputs(e.X) {
				keys[selectorName(e.Sel)] = true
				named[e.X] = true
			} else if isContextOutputs(e) && !named[e] {
				// The whole map: ranged over, bound to a name, or passed on.
				all = true
			}
		case *ast.IndexExpr:
			if !isContextOutputs(e.X) {
				return true
			}
			named[e.X] = true
			if lit, ok := e.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := literal.Unquote(lit.Value); err == nil {
					keys[s] = true
					return true
				}
			}
			all = true
		}
		return true
	}, nil)
	return keys, all
}

// isContextOutputs reports whether an expression is `context.outputs`.
func isContextOutputs(x ast.Expr) bool {
	sel, ok := x.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "context" && selectorName(sel.Sel) == "outputs"
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
