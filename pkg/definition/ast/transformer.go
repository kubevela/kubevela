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
	"cuelang.org/go/cue/token"
)

const (
	// status is the path to the status field in the metadata
	status       = "attributes.status.details"
	healthPolicy = "attributes.status.healthPolicy"
	customStatus = "attributes.status.customStatus"
	// localFieldPrefix is the prefix for local fields not output to the status
	localFieldPrefix = "$"
)

// EncodeMetadata encodes native CUE in the metadata fields to a CUE string literal
func EncodeMetadata(field *ast.Field) error {
	if err := marshalField[*ast.StructLit](field, healthPolicy, validateHealthPolicyField); err != nil {
		return err
	}
	if err := marshalField[*ast.StructLit](field, customStatus, validateCustomStatusField); err != nil {
		return err
	}
	if err := marshalField[*ast.StructLit](field, status, validateStatusField); err != nil {
		return err
	}
	return nil
}

// DecodeMetadata decodes a CUE string literal in the metadata fields to native CUE expressions
func DecodeMetadata(field *ast.Field) error {
	if err := unmarshalField[*ast.StructLit](field, healthPolicy, validateHealthPolicyField); err != nil {
		return err
	}
	if err := unmarshalField[*ast.StructLit](field, customStatus, validateCustomStatusField); err != nil {
		return err
	}
	if err := unmarshalField[*ast.StructLit](field, status, validateStatusField); err != nil {
		return err
	}
	return nil
}

func marshalField[T ast.Node](field *ast.Field, key string, validator func(T) error) error {
	if statusField, ok := GetFieldByPath(field, key); ok {
		switch expr := statusField.Value.(type) {
		case *ast.BasicLit:
			if expr.Kind != token.STRING {
				return fmt.Errorf("expected %s field to be string, got %v", key, expr.Kind)
			}
			if err := ValidateCueStringLiteral[T](expr, validator); err != nil {
				return fmt.Errorf("%s field failed validation: %w", key, err)
			}
			return nil

		case *ast.StructLit:
			structLit := expr
			v, ok := ast.Node(structLit).(T)
			if !ok {
				return fmt.Errorf("%s field: cannot convert *ast.StructLit to expected type", key)
			}
			err := validator(v)
			if err != nil {
				return err
			}
			strLit, err := StringifyStructLitAsCueString(structLit)
			if err != nil {
				return err
			}
			UpdateNodeByPath(field, key, strLit)
			return nil

		default:
			return fmt.Errorf("unexpected type for %s field: %T", key, expr)
		}
	}
	return nil
}

func unmarshalField[T ast.Node](field *ast.Field, key string, validator func(T) error) error {
	if statusField, ok := GetFieldByPath(field, key); ok {
		basicLit, ok := statusField.Value.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			return fmt.Errorf("%s field is not a string literal", key)
		}

		err := ValidateCueStringLiteral[T](basicLit, validator)
		if err != nil {
			return fmt.Errorf("%s field failed validation: %w", key, err)
		}

		unquoted := strings.TrimSpace(TrimCueRawString(basicLit.Value))

		structLit, hasImports, hasPackage, parseErr := ParseCueContent(unquoted)
		if parseErr != nil {
			return fmt.Errorf("unexpected error re-parsing validated %s string: %w", key, parseErr)
		}

		if hasImports || hasPackage {
			// Keep as string literal to preserve imports/package
			return nil
		}

		statusField.Value = structLit
	}
	return nil
}

func validateStatusField(sl *ast.StructLit) error {
	for _, elt := range sl.Elts {
		f, ok := elt.(*ast.Field)
		if !ok {
			return fmt.Errorf("status.details contains non-field element")
		}

		label := GetFieldLabel(f.Label)

		if strings.HasPrefix(label, localFieldPrefix) {
			continue
		}

		switch f.Value.(type) {
		case *ast.BasicLit,
			*ast.Ident,
			*ast.SelectorExpr,
			*ast.CallExpr,
			*ast.BinaryExpr:
			continue
		default:
			return fmt.Errorf("status.details field %q contains unsupported expression type %T", label, f.Value)
		}
	}
	return nil
}

func validateCustomStatusField(sl *ast.StructLit) error {
	validator := func(expr ast.Expr) error {
		switch v := expr.(type) {
		case *ast.BasicLit:
			if v.Kind != token.STRING {
				return fmt.Errorf("customStatus field 'message' must be a string, got %v", v.Kind)
			}
		case *ast.Interpolation, *ast.CallExpr, *ast.SelectorExpr, *ast.Ident, *ast.BinaryExpr, *ast.ParenExpr,
			*ast.ListLit, *ast.IndexExpr, *ast.SliceExpr, *ast.Comprehension:
		default:
			return fmt.Errorf("customStatus field 'message' must be a string expression, got %T", v)
		}
		return nil
	}

	found, err := FindAndValidateField(sl, "message", validator)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("customStatus must contain a 'message' field")
	}

	return nil
}

func validateHealthPolicyField(sl *ast.StructLit) error {
	validator := func(expr ast.Expr) error {
		switch v := expr.(type) {
		case *ast.Ident:
		case *ast.BasicLit:
			if v.Kind != token.TRUE && v.Kind != token.FALSE {
				return fmt.Errorf("healthPolicy field 'isHealth' must be a boolean literal (true/false), got %v", v.Kind)
			}
		case *ast.BinaryExpr, *ast.UnaryExpr, *ast.CallExpr, *ast.SelectorExpr, *ast.ParenExpr:
		default:
			return fmt.Errorf("healthPolicy field 'isHealth' must be a boolean expression, got %T", v)
		}
		return nil
	}

	found, err := FindAndValidateField(sl, "isHealth", validator)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("healthPolicy must contain an 'isHealth' field")
	}

	return nil
}
