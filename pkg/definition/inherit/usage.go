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
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// parameterUse records how a template uses `parameter`.
type parameterUse struct {
	// wholesale is set when `parameter` is used as a value in its own right, as
	// in `for k, v in parameter`. No individual field can then be called unused.
	wholesale bool
	// fields are the names selected off it.
	fields map[string]bool
}

// scanParameterUse reads which parameters a template uses, so one that is
// published and then goes nowhere can be reported.
func scanParameterUse(sources ...string) parameterUse {
	use := parameterUse{fields: map[string]bool{}}
	for _, src := range sources {
		if src == "" {
			continue
		}
		file, err := parser.ParseFile("-", src, parser.ParseComments)
		if err != nil {
			// Unparseable CUE fails with a better message elsewhere; treating it
			// as wholesale use says nothing about it here.
			return parameterUse{wholesale: true}
		}
		scanFile(file, &use)
		if use.wholesale {
			return use
		}
	}
	return use
}

func scanFile(file *ast.File, use *parameterUse) {
	// A field's label declares rather than reads, and a selector's name belongs
	// to whatever it follows, as in `$super.parameter`. Both skipped by identity.
	labels := map[ast.Node]bool{}
	ast.Walk(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Field:
			labels[e.Label] = true
		case *ast.SelectorExpr:
			labels[e.Sel] = true
		}
		return true
	}, nil)

	ast.Walk(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			if isParameter(e.X) {
				if sel, ok := e.Sel.(*ast.Ident); ok {
					use.fields[sel.Name] = true
					return false
				}
			}
		case *ast.IndexExpr:
			if isParameter(e.X) {
				if lit, ok := e.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if name, err := strconv.Unquote(lit.Value); err == nil {
						use.fields[name] = true
						return false
					}
				}
				// An index that is not a literal string selects a parameter
				// nobody can name here.
				use.wholesale = true
				return false
			}
		case *ast.Ident:
			if e.Name == ParameterField && !labels[ast.Node(e)] {
				use.wholesale = true
				return false
			}
		}
		return true
	}, nil)
}

func isParameter(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == ParameterField
}

// forwardsWholesale reports whether a child hands over its whole parameter
// struct, as in `$super: properties: parameter`. Naming nothing means nothing can
// be misnamed, so the undeclared-name check stands down: such a child carries
// parameters of its own and a parent's struct is open.
func forwardsWholesale(template string) bool {
	file, err := parser.ParseFile("-", template, parser.ParseComments)
	if err != nil {
		return false
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok || labelName(field.Label) != SuperField {
			continue
		}
		// Only what is supplied counts; reading `$super.output` elsewhere in the
		// block forwards nothing.
		supplied := suppliedValue(field.Value)
		if supplied == nil {
			continue
		}
		var wholesale bool
		ast.Walk(supplied, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.SelectorExpr:
				if isParameter(e.X) {
					return false
				}
			case *ast.IndexExpr:
				if isParameter(e.X) {
					return false
				}
			case *ast.Ident:
				if e.Name == ParameterField {
					wholesale = true
					return false
				}
			}
			return true
		}, nil)
		if wholesale {
			return true
		}
	}
	return false
}

// suppliedValue digs `properties` out of a `$super` block.
func suppliedValue(value ast.Expr) ast.Expr {
	lit, ok := value.(*ast.StructLit)
	if !ok {
		return nil
	}
	for _, elt := range lit.Elts {
		if f, ok := elt.(*ast.Field); ok && labelName(f.Label) == PropertiesField {
			return f.Value
		}
	}
	return nil
}
