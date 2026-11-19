/*
Copyright 2025 The KubeVela Authors.

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

package ast

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/pkg/strconv"
)

// GetFieldByPath retrieves a field by its path in the AST
func GetFieldByPath(node ast.Node, path string) (*ast.Field, bool) {
	_, field, ok := GetNodeByPath(node, path)
	return field, ok
}

// GetNodeByPath retrieves a node by its path in the AST
func GetNodeByPath(node ast.Node, path string) (ast.Node, *ast.Field, bool) {
	if path == "" || node == nil {
		return nil, nil, false
	}

	pathParts := strings.Split(path, ".")

	switch n := node.(type) {
	case *ast.File:
		startField, val, ok := lookupTopLevelField(n, pathParts[0])
		if !ok {
			return nil, nil, false
		}
		if len(pathParts) == 1 {
			return val, startField, true
		}
		return traversePath(val, pathParts[1:], startField)

	case *ast.Field:
		label := GetFieldLabel(n.Label)
		if label == pathParts[0] {
			if len(pathParts) == 1 {
				return n, n, true
			}
			return traversePath(n.Value, pathParts[1:], n)
		}
		return traversePath(n.Value, pathParts, nil)

	case *ast.StructLit:
		return traversePath(n, pathParts, nil)

	default:
		return nil, nil, false
	}
}

// UpdateNodeByPath updates a node in the AST by its path
func UpdateNodeByPath(root ast.Node, path string, newExpr ast.Expr) bool {
	_, field, ok := GetNodeByPath(root, path)
	if !ok || field == nil {
		return false
	}
	field.Value = newExpr
	return true
}

// GetFieldLabel retrieves the label of a field in the AST
func GetFieldLabel(label ast.Label) string {
	if label == nil {
		return ""
	}
	switch l := label.(type) {
	case *ast.Ident:
		return l.Name
	case *ast.BasicLit:
		return strings.Trim(l.Value, `"`)
	default:
		return ""
	}
}

// StringifyStructLitAsCueString converts a StructLit to a CUE string literal representation
func StringifyStructLitAsCueString(structLit *ast.StructLit) (*ast.BasicLit, error) {
	if len(structLit.Elts) == 0 {
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"{}"`,
		}, nil
	}

	formatted, err := format.Node(structLit)
	if err != nil {
		return nil, fmt.Errorf("failed to format struct: %w", err)
	}

	content := string(formatted)

	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		content = strings.TrimPrefix(content, "{")
		content = strings.TrimSuffix(content, "}")
		content = strings.Trim(content, "\n")
	}

	if content == "" {
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"{}"`,
		}, nil
	}
	lines := strings.Split(content, "\n")

	var sb strings.Builder
	sb.WriteString(`#"""`)
	sb.WriteString("\n")
	sb.WriteString(strings.Join(lines, "\n"))
	sb.WriteString("\n")
	sb.WriteString(`"""#`)

	result := strings.ReplaceAll(sb.String(), "\t", "  ")

	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: result,
	}, nil
}

// ValidateCueStringLiteral validates a CUE string literal by parsing it and applying a custom validator function
func ValidateCueStringLiteral[T ast.Node](lit *ast.BasicLit, validator func(T) error) error {
	if lit.Kind != token.STRING {
		return fmt.Errorf("not a string literal")
	}

	raw := TrimCueRawString(lit.Value)
	if raw == "" {
		return nil
	}

	structLit, _, _, err := ParseCueContent(raw)
	if err != nil {
		return fmt.Errorf("invalid cue content in string literal: %w", err)
	}

	node, ok := ast.Node(structLit).(T)
	if !ok {
		return fmt.Errorf("parsed expression is not of expected type %T", *new(T))
	}
	return validator(node)
}

// TrimCueRawString trims a CUE raw string literal and handles escape sequences
func TrimCueRawString(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, `#"""`) && strings.HasSuffix(s, `"""#`):
		s = strings.TrimSuffix(strings.TrimPrefix(s, `#"""`), `"""#`)
	case strings.HasPrefix(s, `"""`) && strings.HasSuffix(s, `"""`):
		s = strings.TrimSuffix(strings.TrimPrefix(s, `"""`), `"""`)
	default:
		fallback, err := strconv.Unquote(s)
		if err == nil {
			s = fallback
		}
	}

	// Handle escape sequences for backward compatibility with existing definitions
	// For quoted strings (after strconv.Unquote): replace actual tab characters
	s = strings.ReplaceAll(s, "\t", "  ")
	// For raw strings (not unquoted): replace literal \t
	s = strings.ReplaceAll(s, "\\t", "  ")
	s = strings.ReplaceAll(s, "\\\\", "\\")

	return s
}

// WrapCueStruct wraps a string in a CUE struct format
func WrapCueStruct(s string) string {
	return fmt.Sprintf("{\n%s\n}", s)
}

// ParseCueContent parses CUE content and extracts struct fields, skipping imports/packages
func ParseCueContent(content string) (*ast.StructLit, bool, bool, error) {
	if strings.TrimSpace(content) == "" {
		return &ast.StructLit{Elts: []ast.Decl{}}, false, false, nil
	}

	file, err := parser.ParseFile("-", content)
	if err != nil {
		return nil, false, false, err
	}

	hasImports := len(file.Imports) > 0
	hasPackage := file.PackageName() != ""

	structLit := &ast.StructLit{
		Elts: []ast.Decl{},
	}

	for _, decl := range file.Decls {
		switch decl.(type) {
		case *ast.ImportDecl, *ast.Package:
			// Skip imports and package declarations
		default:
			structLit.Elts = append(structLit.Elts, decl)
		}
	}

	return structLit, hasImports, hasPackage, nil
}

// FindAndValidateField searches for a field at the top level or within top-level if statements
func FindAndValidateField(sl *ast.StructLit, fieldName string, validator fieldValidator) (found bool, err error) {
	// First check top-level fields
	for _, elt := range sl.Elts {
		if field, ok := elt.(*ast.Field); ok {
			label := GetFieldLabel(field.Label)
			if label == fieldName {
				found = true
				if validator != nil {
					err = validator(field.Value)
				}
				return found, err
			}
		}
	}

	// If not found at top level, check within top-level if statements
	for _, elt := range sl.Elts {
		if comp, ok := elt.(*ast.Comprehension); ok {
			// Check if this comprehension has if clauses (conditional fields)
			hasIfClause := false
			for _, clause := range comp.Clauses {
				if _, ok := clause.(*ast.IfClause); ok {
					hasIfClause = true
					break
				}
			}

			// If it has an if clause and the value is a struct, search within it
			if hasIfClause {
				if structLit, ok := comp.Value.(*ast.StructLit); ok {
					if innerFound, innerErr := FindAndValidateField(structLit, fieldName, validator); innerFound {
						return true, innerErr
					}
				}
			}
		}
	}

	return found, err
}

func lookupTopLevelField(node ast.Node, key string) (*ast.Field, ast.Expr, bool) {
	switch n := node.(type) {
	case *ast.Field:
		if GetFieldLabel(n.Label) == key {
			return n, n.Value, true
		}
	case *ast.StructLit:
		for _, decl := range n.Elts {
			if f, ok := decl.(*ast.Field); ok && GetFieldLabel(f.Label) == key {
				return f, f.Value, true
			}
		}
	case *ast.File:
		for _, decl := range n.Decls {
			if f, ok := decl.(*ast.Field); ok && GetFieldLabel(f.Label) == key {
				return f, f.Value, true
			}
		}
	}
	return nil, nil, false
}

// fieldValidator is a function that validates a field's value
type fieldValidator func(ast.Expr) error

func traversePath(val ast.Expr, pathParts []string, lastField *ast.Field) (ast.Node, *ast.Field, bool) {
	currentField := lastField
	currentVal := val

	for _, part := range pathParts {
		structLit, ok := currentVal.(*ast.StructLit)
		if !ok {
			return nil, nil, false
		}

		found := false
		for _, decl := range structLit.Elts {
			f, ok := decl.(*ast.Field)
			if !ok {
				continue
			}
			if GetFieldLabel(f.Label) == part {
				currentVal = f.Value
				currentField = f
				found = true
				break
			}
		}

		if !found {
			return nil, nil, false
		}
	}

	return currentVal, currentField, true
}
