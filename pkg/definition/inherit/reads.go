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
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
)

// superReads lists which fields of `$super` a template reads, so a render fills
// only those: moving values between cue.Contexts is most of what a chain costs,
// and merging happens in Go.
//
// nil means everything, for a template mentioning `$super` in a way this cannot
// follow. Filling too much costs time; filling too little breaks a render.
func superReads(template string) map[string]bool {
	file, err := parser.ParseFile("-", template, parser.ParseComments)
	if err != nil {
		return nil
	}

	// `$super: properties: {...}` declares rather than reads, and a selector's
	// name belongs to whatever it follows. Both are skipped by identity.
	skip := map[ast.Node]bool{}
	ast.Walk(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Field:
			skip[e.Label] = true
		case *ast.SelectorExpr:
			skip[e.Sel] = true
		}
		return true
	}, nil)

	reads := map[string]bool{}
	wholesale := false
	ast.Walk(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := e.X.(*ast.Ident); ok && id.Name == SuperField {
				if sel, ok := e.Sel.(*ast.Ident); ok {
					reads[sel.Name] = true
					return false
				}
				wholesale = true
				return false
			}
		case *ast.Ident:
			if e.Name == SuperField && !skip[ast.Node(e)] {
				wholesale = true
				return false
			}
		}
		return true
	}, nil)

	if wholesale {
		return nil
	}
	return reads
}

// wants reports whether a template reads a particular field of `$super`.
func wants(reads map[string]bool, field string) bool {
	return reads == nil || reads[field]
}
