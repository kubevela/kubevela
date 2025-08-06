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

	var sb strings.Builder
	sb.WriteString(`#"""`)
	sb.WriteString("\n")

	for _, elt := range structLit.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		var labelStr, valueStr string

		switch l := field.Label.(type) {
		case *ast.Ident:
			labelStr = l.Name
		case *ast.BasicLit:
			labelStr = strings.Trim(l.Value, `"`)
		default:
			labelStr = "<unknown>"
		}

		for _, attr := range field.Attrs {
			sb.WriteString("  " + attr.Text + "\n")
		}

		b, err := format.Node(field.Value)
		if err != nil {
			valueStr = "<complex>"
		} else {
			lines := strings.Split(string(b), "\n")
			for i := range lines {
				lines[i] = "  " + lines[i]
			}
			valueStr = strings.Join(lines, "\n")
		}

		sb.WriteString(fmt.Sprintf("  %s: %s\n", labelStr, valueStr))
	}

	sb.WriteString(`"""#`)
	val := strings.ReplaceAll(sb.String(), "\t", "  ")
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: val,
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

	wrapped := WrapCueStruct(raw)

	expr, err := parser.ParseExpr("-", wrapped)
	if err != nil {
		return fmt.Errorf("invalid cue content in string literal: %w", err)
	}

	node, ok := expr.(T)
	if !ok {
		return fmt.Errorf("parsed expression is not of expected type %T", *new(T))
	}

	return validator(node)
}

// TrimCueRawString trims a CUE raw string literal
func TrimCueRawString(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `#"""`) && strings.HasSuffix(s, `"""#`) {
		return strings.TrimSuffix(strings.TrimPrefix(s, `#"""`), `"""#`)
	}
	if strings.HasPrefix(s, `"""`) && strings.HasSuffix(s, `"""`) {
		return strings.TrimSuffix(strings.TrimPrefix(s, `"""`), `"""`)
	}
	fallback, err := strconv.Unquote(s)
	if err != nil {
		return s
	}
	return fallback
}

// WrapCueStruct wraps a string in a CUE struct format
func WrapCueStruct(s string) string {
	return fmt.Sprintf("{\n%s\n}", s)
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
