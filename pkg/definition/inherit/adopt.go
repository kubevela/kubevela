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

package inherit

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
)

// adopt rebuilds a value in another value's cue.Context.
//
// Each level compiles in a context of its own, and CUE panics with "values are
// not from the same runtime" if values from two are unified. So a parent's
// result goes back to syntax and is rebuilt where its child lives.
//
// Each option earns its place. Doc comments are kept, since a trait's
// `+patchKey` and `+patchStrategy` live on them. Nothing asks for concreteness,
// since a field derived from a defaulted parameter must arrive as a disjunction
// for a child to override it.
//
// ResolveReferences is deprecated on the grounds that Syntax resolves dangling
// references itself. It does not for a value whose `$super` was filled from
// another context, which is every level above the first: the reference reaches
// the child unresolved and CUE nil-derefs on it. Keep it until Syntax does.
func adopt(target, v cue.Value) (cue.Value, error) {
	node := v.Syntax(cue.All(), cue.Docs(true), cue.ResolveReferences(true)) //nolint:staticcheck // see above

	expr, err := asExpr(node)
	if err != nil {
		return cue.Value{}, err
	}
	out := target.Context().BuildExpr(expr)
	if err := out.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("rebuilding a parent's result for its child: %w", err)
	}
	return out, nil
}

// asExpr coerces what Syntax returned into something BuildExpr accepts.
//
// A struct comes back as a StructLit, a whole template as a File, which is a set
// of declarations rather than an expression; wrapping those in a struct says the
// same thing in a form that can be built.
func asExpr(node ast.Node) (ast.Expr, error) {
	switch n := node.(type) {
	case ast.Expr:
		return n, nil
	case *ast.File:
		return &ast.StructLit{Elts: n.Decls}, nil
	default:
		return nil, fmt.Errorf("cannot rebuild %T across cue contexts", node)
	}
}
