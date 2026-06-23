/*
Copyright 2024 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package upgrade

import (
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
)

var v1_11 = Version{Major: 1, Minor: 11}

// errorFieldLabelRe matches an unquoted `error` used as a field label, i.e. `error` followed by
// optional whitespace and a colon. This avoids false positives on identifiers like `errorMessage`.
var errorFieldLabelRe = regexp.MustCompile(`\berror\s*:`)

func init() {
	// list arithmetic (+ and *) was deprecated in CUE v0.11 and became a hard error in v0.14.
	// Associated with KubeVela 1.11 which first shipped CUE >= v0.14.
	RegisterUpgrade(CUEUpgradeFunc{
		ID:                    "list-arithmetic",
		CUEVersion:            Version{0, 14},
		AssociatedVelaVersion: v1_11,
		Reason:                "contains deprecated list operators (+ or *) that need upgrading to list.Concat() or list.Repeat()",
		Precheck:              func(s string) bool { return strings.Contains(s, "+") || strings.Contains(s, "*") },
		Upgrade:               upgradeListConcatenation,
	})

	// The `error` built-in was introduced in CUE v0.14; unquoted `error` field labels now conflict.
	RegisterUpgrade(CUEUpgradeFunc{
		ID:                    "error-field-label",
		CUEVersion:            Version{0, 14},
		AssociatedVelaVersion: v1_11,
		Reason:                `contains field named 'error' which conflicts with the CUE 0.14 built-in; must be quoted as "error"`,
		Precheck:              func(s string) bool { return errorFieldLabelRe.MatchString(s) },
		Upgrade:               upgradeErrorFieldLabel,
	})
}

// upgradeListConcatenation handles:
// - list1 + list2 -> list.Concat([list1, list2])
// - list * n -> list.Repeat(list, n)
func upgradeListConcatenation(cueStr string, file *ast.File) (string, error) {
	transformed := upgradeListConcatenationAST(file)

	result, err := format.Node(transformed)
	if err != nil {
		return "", fmt.Errorf("failed to format CUE: %w", err)
	}

	return strings.TrimRight(string(result), "\n"), nil
}

func upgradeListConcatenationAST(file *ast.File) *ast.File {
	listRegistry := collectListDeclarations(file)

	needsListImport := false

	result := astutil.Apply(file, func(cursor astutil.Cursor) bool {
		if binExpr, ok := cursor.Node().(*ast.BinaryExpr); ok {
			if binExpr.Op.String() == "+" {
				// Collect all operands from a left-associative + chain so that
				// a + b + c + d becomes list.Concat([a, b, c, d]) in one pass
				// rather than nested list.Concat(list.Concat(...)) calls.
				operands := collectAddChain(binExpr, listRegistry)
				if len(operands) >= 2 {
					callExpr := &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: "list"},
							Sel: &ast.Ident{Name: "Concat"},
						},
						Args: []ast.Expr{
							&ast.ListLit{Elts: operands},
						},
					}
					cursor.Replace(callExpr)
					needsListImport = true
				}
			}

			if binExpr.Op.String() == "*" {
				var listExpr, countExpr ast.Expr

				if isListExpression(binExpr.X, listRegistry) && isNumericExpression(binExpr.Y, listRegistry) {
					listExpr = binExpr.X
					countExpr = binExpr.Y
				} else if isNumericExpression(binExpr.X, listRegistry) && isListExpression(binExpr.Y, listRegistry) {
					countExpr = binExpr.X
					listExpr = binExpr.Y
				}

				if listExpr != nil && countExpr != nil {
					ast.SetRelPos(listExpr, 0)
					ast.SetRelPos(countExpr, 0)

					callExpr := &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: "list"},
							Sel: &ast.Ident{Name: "Repeat"},
						},
						Args: []ast.Expr{listExpr, countExpr},
					}

					cursor.Replace(callExpr)
					needsListImport = true
				}
			}
		}
		return true
	}, nil)

	if file, ok := result.(*ast.File); ok && needsListImport {
		ensureListImport(file)
		return file
	}

	return file
}

// collectAddChain flattens a left-associative + chain into a flat slice of operands,
// returning nil if any operand is not a list expression (so non-list + is left alone).
// If any operand is itself a list.Concat([...]) call (from a prior upgrade pass),
// its inner elements are inlined so the result is always a single flat list.Concat call.
func collectAddChain(expr ast.Expr, listRegistry map[string]bool) []ast.Expr {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "+" {
		if isListExpression(expr, listRegistry) {
			return extractListConcatArgs(expr)
		}
		return nil
	}
	left := collectAddChain(bin.X, listRegistry)
	if left == nil {
		return nil
	}
	if !isListExpression(bin.Y, listRegistry) {
		return nil
	}
	return append(left, extractListConcatArgs(bin.Y)...)
}

// extractListConcatArgs returns the inner elements of a list.Concat([...]) call,
// or wraps expr in a single-element slice if it is not such a call.
// This allows repeated upgrade passes to produce a flat rather than nested result.
func extractListConcatArgs(expr ast.Expr) []ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return []ast.Expr{expr}
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return []ast.Expr{expr}
	}
	base, ok := sel.X.(*ast.Ident)
	if !ok || base.Name != "list" {
		return []ast.Expr{expr}
	}
	selName, ok := sel.Sel.(*ast.Ident)
	if !ok || selName.Name != "Concat" {
		return []ast.Expr{expr}
	}
	listLit, ok := call.Args[0].(*ast.ListLit)
	if !ok {
		return []ast.Expr{expr}
	}
	return listLit.Elts
}

func ensureListImport(file *ast.File) {
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == "\"list\"" {
			return
		}
	}

	for _, decl := range file.Decls {
		if importDecl, ok := decl.(*ast.ImportDecl); ok {
			for _, spec := range importDecl.Specs {
				if spec.Path != nil && spec.Path.Value == "\"list\"" {
					return
				}
			}
		}
	}

	if file.Imports != nil || len(file.Decls) > 0 {
		listImport := &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: "\"list\"",
			},
		}

		file.Imports = append([]*ast.ImportSpec{listImport}, file.Imports...)

		importDecl := &ast.ImportDecl{
			Specs: []*ast.ImportSpec{listImport},
		}

		file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
	}
}

func collectListDeclarations(file *ast.File) map[string]bool {
	listRegistry := make(map[string]bool)

	astutil.Apply(file, func(cursor astutil.Cursor) bool {
		if node, ok := cursor.Node().(*ast.Field); ok {
			if label, ok := node.Label.(*ast.Ident); ok {
				if isListLiteral(node.Value) {
					listRegistry[label.Name] = true
				} else if structLit, ok := node.Value.(*ast.StructLit); ok {
					prefix := label.Name
					collectNestedListDeclarationsFirstPass(structLit, prefix, listRegistry)
				}
			}
		}
		return true
	}, nil)

	// Second pass: iteratively collect fields that are results of list operations
	changed := true
	for changed {
		changed = false
		astutil.Apply(file, func(cursor astutil.Cursor) bool {
			if node, ok := cursor.Node().(*ast.Field); ok {
				if label, ok := node.Label.(*ast.Ident); ok {
					if !listRegistry[label.Name] && isListOperationResult(node.Value, listRegistry) {
						listRegistry[label.Name] = true
						changed = true
					} else if structLit, ok := node.Value.(*ast.StructLit); ok {
						prefix := label.Name
						if collectNestedListDeclarationsSecondPass(structLit, prefix, listRegistry) {
							changed = true
						}
					}
				}
			}
			return true
		}, nil)
	}

	return listRegistry
}

func collectNestedListDeclarationsFirstPass(structLit *ast.StructLit, prefix string, listRegistry map[string]bool) {
	for _, elt := range structLit.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if label, ok := field.Label.(*ast.Ident); ok {
				qualifiedName := prefix + "." + label.Name
				if isListLiteral(field.Value) {
					listRegistry[qualifiedName] = true
					listRegistry[label.Name] = true
				} else if nestedStruct, ok := field.Value.(*ast.StructLit); ok {
					collectNestedListDeclarationsFirstPass(nestedStruct, qualifiedName, listRegistry)
				}
			}
		}
	}
}

func collectNestedListDeclarationsSecondPass(structLit *ast.StructLit, prefix string, listRegistry map[string]bool) bool {
	changed := false
	for _, elt := range structLit.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if label, ok := field.Label.(*ast.Ident); ok {
				qualifiedName := prefix + "." + label.Name
				if !listRegistry[qualifiedName] && isListOperationResult(field.Value, listRegistry) {
					listRegistry[qualifiedName] = true
					listRegistry[label.Name] = true
					changed = true
				} else if nestedStruct, ok := field.Value.(*ast.StructLit); ok {
					if collectNestedListDeclarationsSecondPass(nestedStruct, qualifiedName, listRegistry) {
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func isListLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.ListLit:
		return true
	case *ast.Comprehension:
		return true
	case *ast.Ellipsis:
		return true
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "list" {
				return true
			}
		}
		return false
	case *ast.BinaryExpr:
		// Handle disjunctions like `*[] | [...string]` — if either side is a list, treat as list
		if e.Op.String() == "|" {
			return isListLiteral(e.X) || isListLiteral(e.Y)
		}
		return false
	case *ast.UnaryExpr:
		// Handle default markers like `*[]`
		return isListLiteral(e.X)
	}
	return false
}

func isListExpression(expr ast.Expr, listRegistry map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.ListLit:
		return true

	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "list" {
				return true
			}
		}
		return false

	case *ast.Ident:
		return listRegistry[e.Name]

	case *ast.SelectorExpr:
		if base, ok := e.X.(*ast.Ident); ok {
			if sel, ok := e.Sel.(*ast.Ident); ok {
				qualifiedName := base.Name + "." + sel.Name
				return listRegistry[qualifiedName]
			}
		}
		return false
	}

	return false
}

// isNumericExpression checks if an expression is a numeric literal or identifier that is not a known list.
func isNumericExpression(expr ast.Expr, listRegistry map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT || e.Kind == token.FLOAT
	case *ast.Ident:
		return !listRegistry[e.Name]
	case *ast.UnaryExpr:
		return isNumericExpression(e.X, listRegistry)
	}
	return false
}

// isListOperationResult checks if an expression is the result of a list operation
func isListOperationResult(expr ast.Expr, listRegistry map[string]bool) bool {
	if binExpr, ok := expr.(*ast.BinaryExpr); ok {
		if binExpr.Op.String() == "+" {
			return isListExpression(binExpr.X, listRegistry) && isListExpression(binExpr.Y, listRegistry)
		}
		if binExpr.Op.String() == "*" {
			return (isListExpression(binExpr.X, listRegistry) && isNumericExpression(binExpr.Y, listRegistry)) ||
				(isNumericExpression(binExpr.X, listRegistry) && isListExpression(binExpr.Y, listRegistry))
		}
	}
	if callExpr, ok := expr.(*ast.CallExpr); ok {
		if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "list" {
				if selName, ok := sel.Sel.(*ast.Ident); ok {
					return selName.Name == "Concat" || selName.Name == "Repeat"
				}
			}
		}
	}
	return false
}

// upgradeErrorFieldLabel rewrites any field label named `error` (an unquoted identifier)
// to `"error"` (a quoted string label) to avoid conflict with the CUE 0.14 built-in.
func upgradeErrorFieldLabel(cueStr string, file *ast.File) (string, error) {
	astutil.Apply(file, func(cursor astutil.Cursor) bool {
		field, ok := cursor.Node().(*ast.Field)
		if !ok {
			return true
		}
		ident, ok := field.Label.(*ast.Ident)
		if !ok || ident.Name != "error" {
			return true
		}
		field.Label = ast.NewString("error")
		return true
	}, nil)

	result, err := format.Node(file)
	if err != nil {
		return "", fmt.Errorf("failed to format CUE: %w", err)
	}

	return strings.TrimRight(string(result), "\n"), nil
}
