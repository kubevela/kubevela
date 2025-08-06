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
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

const (
	// status is the path to the status field in the metadata
	status = "attributes.status.details"
	// localFieldPrefix is the prefix for local fields not output to the status
	localFieldPrefix = "$"
)

// EncodeMetadata encodes native CUE in the metadata fields to a CUE string literal
func EncodeMetadata(field *ast.Field) error {
	if err := marshalStatusDetailsField(field); err != nil {
		return err
	}
	return nil
}

// DecodeMetadata decodes a CUE string literal in the metadata fields to native CUE expressions
func DecodeMetadata(field *ast.Field) error {
	if err := unmarshalStatusDetailsField(field); err != nil {
		return err
	}
	return nil
}

func marshalStatusDetailsField(field *ast.Field) error {
	if statusField, ok := GetFieldByPath(field, status); ok {
		switch expr := statusField.Value.(type) {
		case *ast.BasicLit:
			if expr.Kind != token.STRING {
				return fmt.Errorf("expected status field to be string, got %v", expr.Kind)
			}
			if err := ValidateCueStringLiteral[*ast.StructLit](expr, validateStatusField); err != nil {
				return fmt.Errorf("status.details field failed validation: %w", err)
			}
			return nil

		case *ast.StructLit:
			v, _ := statusField.Value.(*ast.StructLit)
			err := validateStatusField(v)
			if err != nil {
				return err
			}
			strLit, err := StringifyStructLitAsCueString(v)
			if err != nil {
				return err
			}
			UpdateNodeByPath(field, status, strLit)
			return nil

		default:
			return fmt.Errorf("unexpected type for status field: %T", expr)
		}
	}
	return nil
}

func unmarshalStatusDetailsField(field *ast.Field) error {
	if statusField, ok := GetFieldByPath(field, status); ok {
		basicLit, ok := statusField.Value.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			return fmt.Errorf("status.details field is not a string literal")
		}

		err := ValidateCueStringLiteral[*ast.StructLit](basicLit, validateStatusField)
		if err != nil {
			return fmt.Errorf("status field failed validation: %w", err)
		}

		unquoted := strings.TrimSpace(TrimCueRawString(basicLit.Value))
		expr, err := parser.ParseExpr("-", WrapCueStruct(unquoted))
		if err != nil {
			return fmt.Errorf("unexpected error re-parsing validated string: %w", err)
		}

		structLit, ok := expr.(*ast.StructLit)
		if !ok {
			return fmt.Errorf("expected struct after validation")
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
