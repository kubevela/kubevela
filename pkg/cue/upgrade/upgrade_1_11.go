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

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
)

func init() {
	RegisterUpgrade("1.11", upgradeListConcatenation)
}

func requires111Upgrade(cueStr string) (bool, []string, error) {
	file, err := parser.ParseFile("", cueStr, parser.ParseComments)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse CUE: %w", err)
	}
	
	var reasons []string
	if hasOldListConcatenation(file) {
		reasons = append(reasons, "contains old list concatenation syntax (list + list) that needs upgrading to list.Concat()")
	}
	
	return len(reasons) > 0, reasons, nil
}

func hasOldListConcatenation(file *ast.File) bool {
	listRegistry := collectListDeclarations(file)
	
	found := false
	astutil.Apply(file, func(cursor astutil.Cursor) bool {
		if binExpr, ok := cursor.Node().(*ast.BinaryExpr); ok {
			if binExpr.Op.String() == "+" {
				if isListExpression(binExpr.X, listRegistry) && isListExpression(binExpr.Y, listRegistry) {
					found = true
					return false
				}
			}
		}
		return true
	}, nil)
	
	return found
}

// upgradeListConcatenation handles list1 + list2 -> list.Concat([list1, list2])
func upgradeListConcatenation(cueStr string) (string, error) {
	file, err := parser.ParseFile("", cueStr, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse CUE: %w", err)
	}
	
	transformed := upgradeListConcatenationAST(file)
	
	result, err := format.Node(transformed)
	if err != nil {
		return "", fmt.Errorf("failed to format CUE: %w", err)
	}
	
	return string(result), nil
}

func upgradeListConcatenationAST(file *ast.File) *ast.File {
	listRegistry := collectListDeclarations(file)
	
	needsListImport := false
	
	result := astutil.Apply(file, func(cursor astutil.Cursor) bool {
		if binExpr, ok := cursor.Node().(*ast.BinaryExpr); ok {
			if binExpr.Op.String() == "+" {
				if isListExpression(binExpr.X, listRegistry) && isListExpression(binExpr.Y, listRegistry) {
					callExpr := &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: "list"},
							Sel: &ast.Ident{Name: "Concat"},
						},
						Args: []ast.Expr{
							&ast.ListLit{
								Elts: []ast.Expr{binExpr.X, binExpr.Y},
							},
						},
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
				Kind:  11, // token.STRING
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
		switch node := cursor.Node().(type) {
		case *ast.Field:
			if label, ok := node.Label.(*ast.Ident); ok {
				if isListLiteral(node.Value) {
					listRegistry[label.Name] = true
				} else if structLit, ok := node.Value.(*ast.StructLit); ok {
					prefix := label.Name
					collectNestedListDeclarations(structLit, prefix, listRegistry)
				}
			}
		}
		return true
	}, nil)
	
	return listRegistry
}

func collectNestedListDeclarations(structLit *ast.StructLit, prefix string, listRegistry map[string]bool) {
	for _, elt := range structLit.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if label, ok := field.Label.(*ast.Ident); ok {
				qualifiedName := prefix + "." + label.Name
				if isListLiteral(field.Value) {
					listRegistry[qualifiedName] = true
				} else if nestedStruct, ok := field.Value.(*ast.StructLit); ok {
					collectNestedListDeclarations(nestedStruct, qualifiedName, listRegistry)
				}
			}
		}
	}
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

