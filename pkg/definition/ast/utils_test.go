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
	"regexp"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/stretchr/testify/require"
)

func TestGetFieldByPath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		found bool
		label string
	}{
		{
			name:  "Field does exist",
			path:  "component.attributes.status.health",
			found: true,
			label: "health",
		},
		{
			name:  "Field does not exist",
			path:  "component.attributes.status.missingField",
			found: false,
			label: "",
		},
		{
			name:  "Top level field can be found",
			path:  "component",
			found: true,
			label: "component",
		},
		{
			name:  "Empty path returns false",
			path:  "",
			found: false,
		},
	}

	src := `
	component: {
	  attributes: {
	    status: {
	      health: "ok"
	    }
	  }
	}`

	file, err := parser.ParseFile("-", src)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := GetFieldByPath(file, tt.path)
			require.Equal(t, tt.found, ok)

			if ok {
				require.Equal(t, tt.label, GetFieldLabel(field.Label))
			}
		})
	}
}

func TestGetNodeByPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		found      bool
		fieldLabel string
		nodeType   string
	}{
		{
			name:       "Node and field exist",
			path:       "component.attributes.status.health",
			found:      true,
			fieldLabel: "health",
			nodeType:   "*ast.BasicLit",
		},
		{
			name:  "Field does not exist",
			path:  "component.attributes.status.missingField",
			found: false,
		},
		{
			name:       "Top level field can be found",
			path:       "component",
			found:      true,
			fieldLabel: "component",
			nodeType:   "*ast.StructLit",
		},
		{
			name:  "Empty path",
			path:  "",
			found: false,
		},
		{
			name:       "Valid path that ends on struct",
			path:       "component.attributes.status",
			found:      true,
			fieldLabel: "status",
			nodeType:   "*ast.StructLit",
		},
	}

	src := `
	component: {
	  attributes: {
	    status: {
	      health: "ok"
	    }
	  }
	}`

	file, err := parser.ParseFile("-", src)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, field, ok := GetNodeByPath(file, tt.path)
			require.Equal(t, tt.found, ok)

			if ok {
				require.NotNil(t, field)
				require.Equal(t, tt.fieldLabel, GetFieldLabel(field.Label))
				if tt.nodeType != "" {
					require.Equal(t, tt.nodeType, fmt.Sprintf("%T", node))
				}
			}
		})
	}
}

func TestUpdateNodeByPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		newValue  string
		shouldSet bool
		expected  string
	}{
		{
			name:      "Update nested field",
			path:      "component.attributes.status.health",
			newValue:  `"updated"`,
			shouldSet: true,
			expected:  `"updated"`,
		},
		{
			name:      "Update of missing field",
			path:      "component.attributes.status.missingField",
			newValue:  `"missing!"`,
			shouldSet: false,
		},
		{
			name:      "Update top level field",
			path:      "component",
			newValue:  `"top"`,
			shouldSet: true,
			expected:  `"top"`,
		},
		{
			name:      "Empty path",
			path:      "",
			newValue:  `"shouldFail"`,
			shouldSet: false,
		},
	}

	src := `
	component: {
	  attributes: {
	    status: {
	      health: "ok"
	    }
	  }
	}`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", src)
			require.NoError(t, err)

			newExpr := &ast.BasicLit{Kind: token.STRING, Value: tt.newValue}
			ok := UpdateNodeByPath(file, tt.path, newExpr)
			require.Equal(t, tt.shouldSet, ok)

			if ok {
				field, found := GetFieldByPath(file, tt.path)
				require.True(t, found)

				basicLit, isLit := field.Value.(*ast.BasicLit)
				require.True(t, isLit)
				require.Equal(t, tt.expected, basicLit.Value)
			}
		})
	}
}

func TestStringifyStructLitAsCueString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		contains   []string
		shouldFail bool
	}{
		{
			name: "Simple string and number fields",
			input: `
			{
			  name: "example"
			  count: 42
			}`,
			contains: []string{
				`name:   "example"`,
				`count:   42`,
			},
		},
		{
			name: "Boolean and nested struct",
			input: `
			{
			  enabled: true
			  metadata: {
			    version: "v1"
			  }
			}`,
			contains: []string{
				`enabled:   true`,
				`metadata: {`,
				`version:   "v1"`,
			},
		},
		{
			name: "Empty struct",
			input: `
			{
			}`,
			contains: []string{
				`{}`,
			},
		},
		{
			name: "Struct with list",
			input: `
			{
			  items: [1, 2, 3]
			}`,
			contains: []string{
				`items:   [1, 2, 3]`,
			},
		},
		{
			name: "Invalid CUE input",
			input: `
			{
			  name: "missing comma"
			  123bad
			}`,
			shouldFail: true,
		},
		{
			name: "Conditional fields with if statements",
			input: `
			{
				if context.status.healthy {
					message: "Healthy! (\(context.status.details.replicaReadyRatio * 100)% pods running)"
				}

				if !context.status.healthy {
					message: "Unhealthy! (\(context.status.details.replicaReadyRatio * 100)% pods running)"
				}
			}`,
			contains: []string{
				`if context.status.healthy {`,
				`message: "Healthy! (\(context.status.details.replicaReadyRatio*100)% pods running)"`,
				`if !context.status.healthy {`,
				`message: "Unhealthy! (\(context.status.details.replicaReadyRatio*100)% pods running)"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.ParseExpr("-", tt.input)
			if tt.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			structLit, ok := expr.(*ast.StructLit)
			require.True(t, ok, "expected struct literal")

			strLit, err := StringifyStructLitAsCueString(structLit)
			require.NoError(t, err)

			for _, substr := range tt.contains {
				require.Contains(t, normalizeString(strLit.Value), normalizeString(substr))
			}
		})
	}
}

func TestGetFieldLabel(t *testing.T) {
	tests := []struct {
		name     string
		label    ast.Label
		expected string
	}{
		{
			name:     "Ident label",
			label:    &ast.Ident{Name: "foo"},
			expected: "foo",
		},
		{
			name:     "BasicLit string label",
			label:    &ast.BasicLit{Value: `"bar"`},
			expected: "bar",
		},
		{
			name:     "BasicLit empty string",
			label:    &ast.BasicLit{Value: `""`},
			expected: "",
		},
		{
			name:     "BasicLit with quotes and spaces",
			label:    &ast.BasicLit{Value: `"  spaced  "`},
			expected: "  spaced  ",
		},
		{
			name:     "Nil label",
			label:    nil,
			expected: "",
		},
		{
			name:     "Unsupported label type",
			label:    &ast.ListLit{}, // not a valid label
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFieldLabel(tt.label)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimCueRawString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Triple quoted raw string with hash",
			input:    `#"""foo bar"""#`,
			expected: `foo bar`,
		},
		{
			name:     "Triple quoted raw string without hash",
			input:    `"""foo bar"""`,
			expected: `foo bar`,
		},
		{
			name:     "Double quoted standard string",
			input:    `"foo bar"`,
			expected: `foo bar`,
		},
		{
			name:     "Unquoted fallback string",
			input:    `foo bar`,
			expected: `foo bar`,
		},
		{
			name:     "Triple quoted with leading/trailing spaces",
			input:    `   #"""foo bar"""#   `,
			expected: `foo bar`,
		},
		{
			name:     "Escaped quoted string",
			input:    `"line 1\nline 2"`,
			expected: "line 1\nline 2",
		},
		{
			name:     "Invalid quoted string (fallback)",
			input:    `"unterminated`,
			expected: `"unterminated`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := TrimCueRawString(tt.input)
			require.Equal(t, tt.expected, out)
		})
	}
}

func TestValidateCueStringLiteral(t *testing.T) {
	tests := []struct {
		name       string
		lit        *ast.BasicLit
		validator  func(*ast.StructLit) error
		expectErr  bool
		errContain string
	}{
		{
			name: "valid struct literal",
			lit: &ast.BasicLit{
				Kind: token.STRING,
				Value: `#"""
name: "test"
age: 30
"""#`,
			},
			validator: validStructValidator,
			expectErr: false,
		},
		{
			name: "empty struct literal fails validator",
			lit: &ast.BasicLit{
				Kind: token.STRING,
				Value: `#"""
"""#`,
			},
			validator:  validStructValidator,
			expectErr:  true,
			errContain: "struct is empty",
		},
		{
			name: "empty string passes",
			lit: &ast.BasicLit{
				Kind:  token.STRING,
				Value: `""`,
			},
			validator: validStructValidator,
			expectErr: false,
		},
		{
			name: "invalid cue content fails parse",
			lit: &ast.BasicLit{
				Kind: token.STRING,
				Value: `#"""
invalid: @@@
"""#`,
			},
			validator:  validStructValidator,
			expectErr:  true,
			errContain: "invalid cue content in string literal",
		},
		{
			name: "non-string literal fails",
			lit: &ast.BasicLit{
				Kind:  token.INT,
				Value: `123`,
			},
			validator:  validStructValidator,
			expectErr:  true,
			errContain: "not a string literal",
		},
		{
			name: "parsed type mismatch returns error",
			lit: &ast.BasicLit{
				Kind:  token.STRING,
				Value: `#""" """#`, // empty struct literal
			},
			validator: func(s *ast.StructLit) error {
				return fmt.Errorf("forced error")
			},
			expectErr:  true,
			errContain: "forced error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCueStringLiteral[*ast.StructLit](tt.lit, tt.validator)
			if tt.expectErr {
				require.Error(t, err)
				if tt.errContain != "" {
					require.Contains(t, err.Error(), tt.errContain)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func normalizeString(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}

func validStructValidator(slit *ast.StructLit) error {
	if len(slit.Elts) == 0 {
		return fmt.Errorf("struct is empty")
	}
	return nil
}

func TestWrapCueStruct(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple multiline string",
			input:    "field1: \"value1\"\nfield2: 42",
			expected: "{\nfield1: \"value1\"\nfield2: 42\n}",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "{\n\n}",
		},
		{
			name:     "single line",
			input:    "foo: bar",
			expected: "{\nfoo: bar\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapCueStruct(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestFindAndValidateField(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		fieldName          string
		expectedFound      bool
		expectedErrMessage string
		validator          fieldValidator
	}{
		{
			name: "find field at top level without validator",
			input: `{
				field1: "value1"
				field2: "value2"
			}`,
			fieldName:     "field1",
			expectedFound: true,
		},
		{
			name: "field not found at any level",
			input: `{
				field1: "value1"
				field2: "value2"
			}`,
			fieldName:     "field3",
			expectedFound: false,
		},
		{
			name: "find field at top level with validator that passes",
			input: `{
				message: "hello world"
				other: "value"
			}`,
			fieldName:     "message",
			expectedFound: true,
			validator: func(expr ast.Expr) error {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					if basicLit.Value != `"hello world"` {
						return fmt.Errorf("expected hello world, got %s", basicLit.Value)
					}
				}
				return nil
			},
		},
		{
			name: "find field at top level with validator that fails",
			input: `{
				message: "hello world"
				other: "value"
			}`,
			fieldName:          "message",
			expectedFound:      true,
			expectedErrMessage: "expected goodbye, got",
			validator: func(expr ast.Expr) error {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					if basicLit.Value != `"goodbye"` {
						return fmt.Errorf("expected goodbye, got %s", basicLit.Value)
					}
				}
				return nil
			},
		},
		{
			name: "find field in simple if statement",
			input: `{
				otherField: "value"
				if condition {
					message: "found in if"
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "find field in nested if statements",
			input: `{
				otherField: "value"
				if condition1 {
					if condition2 {
						message: "deeply nested"
					}
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "find field in if-else chain",
			input: `{
				status: "running"
				if status == "running" {
					message: "service is running"
				}
				if status == "stopped" {
					message: "service is stopped"
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "find field in complex nested conditionals",
			input: `{
				phase: context.output.status.phase
				replicas: context.output.status.replicas
				readyReplicas: *0 | int
				if context.output.status.readyReplicas != _|_ {
					readyReplicas: context.output.status.readyReplicas
				}
				if phase == "Running" {
					if readyReplicas == replicas {
						message: "All replicas are ready"
					}
					if readyReplicas < replicas {
						message: "Some replicas are not ready"
					}
				}
				if phase != "Running" {
					message: "Deployment is not running"
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "find field in if with validator",
			input: `{
				condition: true
				if condition {
					isHealth: true
				}
			}`,
			fieldName:     "isHealth",
			expectedFound: true,
			validator: func(expr ast.Expr) error {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					if basicLit.Value != "true" {
						return fmt.Errorf("expected true, got %s", basicLit.Value)
					}
				}
				return nil
			},
		},
		{
			name: "find field in nested if with failing validator",
			input: `{
				condition: true
				if condition {
					if nestedCondition {
						isHealth: false
					}
				}
			}`,
			fieldName:          "isHealth",
			expectedFound:      true,
			expectedErrMessage: "expected true, got",
			validator: func(expr ast.Expr) error {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					if basicLit.Value != "true" {
						return fmt.Errorf("expected true, got %s", basicLit.Value)
					}
				}
				return nil
			},
		},
		{
			name: "field not found in comprehension without if clause",
			input: `{
				items: [for x in list { value: x }]
			}`,
			fieldName:     "message",
			expectedFound: false,
		},
		{
			name: "find field with complex expressions in if",
			input: `{
				replicas: context.output.status.replicas
				readyReplicas: context.output.status.readyReplicas
				if (replicas | *0) != (readyReplicas | *0) {
					message: "not ready"
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "find field with quoted label in if statement",
			input: `{
				condition: true
				if condition {
					"field-with-dash": "value"
				}
			}`,
			fieldName:     "field-with-dash",
			expectedFound: true,
		},
		{
			name: "find field in multiple nested levels",
			input: `{
				level1: "value"
				if condition1 {
					level2: "value"
					if condition2 {
						level3: "value"
						if condition3 {
							message: "deeply nested field"
						}
					}
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
		},
		{
			name: "empty struct with if statements",
			input: `{
				if condition {
				}
			}`,
			fieldName:     "message",
			expectedFound: false,
		},
		{
			name: "field exists in both top level and if statement (finds top level first)",
			input: `{
				message: "top level"
				if condition {
					message: "in if"
				}
			}`,
			fieldName:     "message",
			expectedFound: true,
			validator: func(expr ast.Expr) error {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					if basicLit.Value == `"top level"` {
						return nil
					}
					return fmt.Errorf("expected top level message, got %s", basicLit.Value)
				}
				return fmt.Errorf("expected string literal")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.ParseExpr("-", tt.input)
			require.NoError(t, err)

			structLit, ok := expr.(*ast.StructLit)
			require.True(t, ok, "input should parse as struct literal")

			found, err := FindAndValidateField(structLit, tt.fieldName, tt.validator)

			require.Equal(t, tt.expectedFound, found, "found result should match expected")

			if tt.expectedErrMessage != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
