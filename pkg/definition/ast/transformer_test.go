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
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalAndUnmarshalMetadata(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectMarshalErr   string
		expectUnmarshalErr string
		expectContains     string
	}{
		{
			name: "valid scalar values",
			input: `
				attributes: {
					status: {
						details: {
							strVal: "ok"
							intVal: 42
							boolVal: true
							nullVal: null
							typeVal: string
						}
					}
				}
			`,
			expectContains: "strVal",
		},
		{
			name: "references to context output",
			input: `
				attributes: {
					status: {
						details: {
							val: context.output.status.running
						}
					}
				}
			`,
			expectContains: "val",
		},
		{
			name: "valid selector and call expressions",
			input: `
				attributes: {
					status: {
						details: {
							selector: context.output.status.running
						}
					}
				}
			`,
			expectContains: "selector",
		},
		{
			name: "unary expression value is valid",
			input: `
				attributes: {
					status: {
						details: {
							notFailing: !context.output.status.failing
						}
					}
				}
			`,
			expectContains: "notFailing",
		},
		{
			name: "struct value is invalid",
			input: `
				attributes: {
					status: {
						details: {
							data: { key: "value" }
						}
					}
				}
			`,
			expectMarshalErr: `unsupported expression type`,
		},
		{
			name: "list value is invalid",
			input: `
				attributes: {
					status: {
						details: {
							items: [1, 2, 3]
						}
					}
				}
			`,
			expectMarshalErr: `unsupported expression type`,
		},
		{
			name: "$key with struct is permitted",
			input: `
				attributes: {
					status: {
						details: {
							$raw: { key: "value" }
						}
					}
				}
			`,
			expectContains: "$raw",
		},
		{
			name: "$key with list is permitted",
			input: `
				attributes: {
					status: {
						details: {
							$items: [1, 2, 3]
						}
					}
				}
			`,
			expectContains: "$items",
		},
		{
			name: "valid stringified struct round trip",
			input: `
				attributes: {
					status: {
						details: #"""
							ok: true
							value: 99
						"""#
					}
				}
			`,
			expectContains: "value",
		},
		{
			name: "malformed stringified status fails validation",
			input: `
	        	attributes: {
	        		status: {
	        			details: #"""
	        				invalid cue: abc
	        			"""#
	        		}
	        	}
	        `,
			expectMarshalErr: "invalid cue content in string literal",
		},
		{
			name: "nested local struct is permitted",
			input: `
	        	attributes: {
	        		status: {
	        			details: {
                            $local: {
								nested: {
                                    deepNesting: {
                                       key: "value" 
                                    }
                                }
                            }
                            $anotherLocal: $local.nested.deepNesting.key
                            val: $anotherLocal
                        }
	        		}
	        	}
	        `,
			expectContains: "$local",
		},
		{
			name: "status details with import statement should work",
			input: `
				attributes: {
					status: {
						details: #"""
							import "strconv"
							replicas: strconv.Atoi(context.output.status.replicas)
						"""#
					}
				}
			`,
			expectContains: "import \"strconv\"",
		},
		{
			name: "status details with package declaration",
			input: `
				attributes: {
					status: {
						details: #"""
							package status
							
							ready: true
							phase: "Running"
						"""#
					}
				}
			`,
			expectContains: "package status",
		},
		{
			name: "status details with import cannot bypass validation",
			input: `
				attributes: {
					status: {
						details: #"""
							import "strings"
							data: { nested: "structure" }
						"""#
					}
				}
			`,
			expectMarshalErr: "unsupported expression type",
		},
	}

	for _, tt := range tests {
		tt := tt // Re-bind range variable to avoid capture
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			err = DecodeMetadata(rootField)
			if tt.expectUnmarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectUnmarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.expectContains != "" {
				statusField, ok := GetFieldByPath(rootField, "attributes.status.details")
				require.True(t, ok)

				switch v := statusField.Value.(type) {
				case *ast.BasicLit:
					require.Contains(t, v.Value, tt.expectContains)
				case *ast.StructLit:
					out, err := format.Node(v)
					require.NoError(t, err)
					require.Contains(t, string(out), tt.expectContains)
				default:
					t.Fatalf("unexpected status value type: %T", v)
				}
			}
		})
	}
}

func TestHasDisableValidation(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "field with @disableValidation()",
			input:    `details: { foo: "bar" } @disableValidation()`,
			expected: true,
		},
		{
			name:     "field without any attributes",
			input:    `details: { foo: "bar" }`,
			expected: false,
		},
		{
			name:     "field with a different attribute",
			input:    `details: { foo: "bar" } @someOtherAttr()`,
			expected: false,
		},
		{
			name:     "field with multiple attributes including @disableValidation()",
			input:    `details: { foo: "bar" } @someOtherAttr() @disableValidation()`,
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tc.input)
			require.NoError(t, err)

			var field *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					field = f
					break
				}
			}
			require.NotNil(t, field)
			assert.Equal(t, tc.expected, hasDisableValidation(field))
		})
	}
}

func TestDisableValidationAttribute(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectContains string
	}{
		{
			name: "details with @disableValidation() bypasses validation and is stringified",
			input: `
				attributes: {
					status: {
						details: {
							{for _, rule in context.outputs.ingress.spec.rules {
								"host.\(rule.host)": rule.host
							}}
						} @disableValidation()
					}
				}
			`,
			expectContains: "host.",
		},
		{
			name: "healthPolicy with @disableValidation() bypasses validation and is stringified",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							someComplexField: [for c in context.output.status.conditions { c.status }][0] == "True"
							isHealth: someComplexField
						} @disableValidation()
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "details stringified format with @disableValidation() bypasses validation on encode and decode",
			input: `
				attributes: {
					status: {
						details: #"""
							import "strings"
							{for _, rule in context.outputs.ingress.spec.rules {
								"host.\(rule.host)": rule.host
							}}
						"""# @disableValidation()
					}
				}
			`,
			expectContains: "import",
		},
		{
			name: "customStatus with @disableValidation() bypasses validation and is stringified",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: [for c in context.output.status.conditions { c.message }][0]
						} @disableValidation()
					}
				}
			`,
			expectContains: "message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			require.NoError(t, err)

			// After encoding, the field value should be a string literal
			// containing the original content.
			var fieldPath string
			switch {
			case strings.Contains(tt.input, "details:"):
				fieldPath = "attributes.status.details"
			case strings.Contains(tt.input, "healthPolicy:"):
				fieldPath = "attributes.status.healthPolicy"
			case strings.Contains(tt.input, "customStatus:"):
				fieldPath = "attributes.status.customStatus"
			}

			statusField, ok := GetFieldByPath(rootField, fieldPath)
			require.True(t, ok)
			basicLit, ok := statusField.Value.(*ast.BasicLit)
			require.True(t, ok, "expected field to be stringified to *ast.BasicLit after encoding, got %T", statusField.Value)
			require.Contains(t, basicLit.Value, tt.expectContains)
		})
	}
}

func TestDisableValidationDecodeRoundTrip(t *testing.T) {
	// Verifies that @disableValidation() skips validation on DecodeMetadata too,
	// so a stringified field with otherwise-invalid content survives a round trip.
	input := `
		attributes: {
			status: {
				details: #"""
					data: { nested: "structure" }
				"""# @disableValidation()
			}
		}
	`
	file, err := parser.ParseFile("-", input)
	require.NoError(t, err)

	var rootField *ast.Field
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.Field); ok {
			rootField = f
			break
		}
	}
	require.NotNil(t, rootField)

	// Encode: should pass despite nested struct (validation disabled)
	require.NoError(t, EncodeMetadata(rootField))

	require.NoError(t, DecodeMetadata(rootField))
}

func TestDisableValidationYAMLRoundTrip(t *testing.T) {
	// Simulates the full YAML storage round-trip: encode strips field attributes
	// (as YAML/gocodec does), but the cue-attr sentinel in the string value means
	// decode still skips validation correctly.
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "details with struct value and @disableValidation()",
			input: `
				attributes: {
					status: {
						details: {
							data: { nested: "structure" }
						} @disableValidation()
					}
				}
			`,
		},
		{
			name: "details with stringified value and @disableValidation()",
			input: `
				attributes: {
					status: {
						details: #"""
							data: { nested: "structure" }
						"""# @disableValidation()
					}
				}
			`,
		},
		{
			name: "healthPolicy with @disableValidation()",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: [for c in context.output.status.conditions { c.status }][0] == "True"
						} @disableValidation()
					}
				}
			`,
		},
		{
			name: "customStatus with @disableValidation()",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: [for c in context.output.status.conditions { c.message }][0]
						} @disableValidation()
					}
				}
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			require.NoError(t, EncodeMetadata(rootField))

			// Simulate YAML storage stripping field attributes from all status sub-fields.
			for _, path := range []string{
				"attributes.status.details",
				"attributes.status.healthPolicy",
				"attributes.status.customStatus",
			} {
				if f, ok := GetFieldByPath(rootField, path); ok {
					f.Attrs = nil
				}
			}

			// Decode must still succeed — the cue-attr sentinel in the string carries
			// the @disableValidation() intent across the storage boundary.
			require.NoError(t, DecodeMetadata(rootField))
		})
	}
}

func TestInjectAttrsMultipleAndIdempotent(t *testing.T) {
	t.Run("multiple attributes are all persisted and restored", func(t *testing.T) {
		// Use two attributes on the same field; both should survive the YAML round-trip.
		input := `
			attributes: {
				status: {
					details: {
						data: { nested: "structure" }
					} @disableValidation() @someOtherAttr(value)
				}
			}
		`
		file, err := parser.ParseFile("-", input)
		require.NoError(t, err)

		var rootField *ast.Field
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.Field); ok {
				rootField = f
				break
			}
		}
		require.NotNil(t, rootField)

		require.NoError(t, EncodeMetadata(rootField))

		// Verify both cue-attr lines are present in the stored string.
		f, ok := GetFieldByPath(rootField, "attributes.status.details")
		require.True(t, ok)
		bl, ok := f.Value.(*ast.BasicLit)
		require.True(t, ok)
		require.Contains(t, bl.Value, "// cue-attr:@disableValidation()")
		require.Contains(t, bl.Value, "// cue-attr:@someOtherAttr(value)")

		// Strip field attributes to simulate YAML storage.
		f.Attrs = nil

		// Decode must restore both attributes and succeed.
		require.NoError(t, DecodeMetadata(rootField))
		require.Len(t, f.Attrs, 2)
	})

	t.Run("encoding an already-stringified field with cue-attr does not double-inject", func(t *testing.T) {
		// Simulates a second EncodeMetadata call on an already-encoded field.
		input := `
			attributes: {
				status: {
					details: #"""
						// cue-attr:@disableValidation()
						data: { nested: "structure" }
					"""# @disableValidation()
				}
			}
		`
		file, err := parser.ParseFile("-", input)
		require.NoError(t, err)

		var rootField *ast.Field
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.Field); ok {
				rootField = f
				break
			}
		}
		require.NotNil(t, rootField)

		require.NoError(t, EncodeMetadata(rootField))

		f, ok := GetFieldByPath(rootField, "attributes.status.details")
		require.True(t, ok)
		bl, ok := f.Value.(*ast.BasicLit)
		require.True(t, ok)

		// Should appear exactly once, not twice.
		count := strings.Count(bl.Value, "// cue-attr:@disableValidation()")
		require.Equal(t, 1, count, "cue-attr sentinel should not be duplicated on re-encode")
	})
}

func TestComprehensionDynamicKeyErrorMessage(t *testing.T) {
	// Verifies that a comprehension with an invalid value type reports a
	// meaningful error rather than an empty label.
	input := `
		attributes: {
			status: {
				details: {
					{for _, rule in context.outputs.ingress.spec.rules {
						"host.\(rule.host)": { nested: "invalid" }
					}}
				}
			}
		}
	`
	file, err := parser.ParseFile("-", input)
	require.NoError(t, err)

	var rootField *ast.Field
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.Field); ok {
			rootField = f
			break
		}
	}
	require.NotNil(t, rootField)

	err = EncodeMetadata(rootField)
	require.Error(t, err)
	require.Contains(t, err.Error(), "<dynamic>")
}

func TestStatusDetailsWithDynamicKeys(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectMarshalErr string
		expectContains   string
	}{
		{
			name: "root comprehension generates dynamic keys from list",
			input: `
				attributes: {
					status: {
						details: {
							{for _, rule in context.outputs.ingress.spec.rules {
								"host.\(rule.host)": rule.host
							}}
						}
					}
				}
			`,
			expectContains: "host.",
		},
		{
			name: "call expression value with list comprehension arg",
			input: `
				attributes: {
					status: {
						details: {
							hosts: strings.Join([for rule in context.outputs.ingress.spec.rules { rule.host }], ",")
						}
					}
				}
			`,
			expectContains: "hosts",
		},
		{
			name: "local field embedded at root of details",
			input: `
				attributes: {
					status: {
						details: {
							$local: { key: "value" }
							$local
						}
					}
				}
			`,
			expectContains: "key",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			}
			require.NoError(t, err)

			err = DecodeMetadata(rootField)
			require.NoError(t, err)

			if tt.expectContains != "" {
				statusField, ok := GetFieldByPath(rootField, "attributes.status.details")
				require.True(t, ok)

				switch v := statusField.Value.(type) {
				case *ast.BasicLit:
					require.Contains(t, v.Value, tt.expectContains)
				case *ast.StructLit:
					out, err := format.Node(v)
					require.NoError(t, err)
					require.Contains(t, string(out), tt.expectContains)
				default:
					t.Fatalf("unexpected status value type: %T", v)
				}
			}
		})
	}
}

func TestMarshalAndUnmarshalHealthPolicy(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectMarshalErr   string
		expectUnmarshalErr string
		expectContains     string
	}{
		{
			name: "valid healthPolicy with boolean literal",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: true
						}
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "valid healthPolicy with binary expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: context.output.status.phase == "Running"
						}
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "valid healthPolicy with complex expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: context.output.status.ready && context.output.status.replicas > 0
						}
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "valid healthPolicy with selector expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: context.output.status.conditions[0].status
						}
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "valid healthPolicy with call expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: len(context.output.status.conditions) > 0
						}
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "healthPolicy missing isHealth field",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							someOtherField: true
						}
					}
				}
			`,
			expectMarshalErr: "healthPolicy must contain an 'isHealth' field",
		},
		{
			name: "healthPolicy with invalid isHealth type (struct)",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: { nested: true }
						}
					}
				}
			`,
			expectMarshalErr: "healthPolicy field 'isHealth' must be a boolean expression",
		},
		{
			name: "healthPolicy with invalid isHealth type (list)",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: [true, false]
						}
					}
				}
			`,
			expectMarshalErr: "healthPolicy field 'isHealth' must be a boolean expression",
		},
		{
			name: "valid stringified healthPolicy round trip",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							isHealth: context.output.status.phase == "Running"
						"""#
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "malformed stringified healthPolicy fails validation",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							invalid cue: abc
						"""#
					}
				}
			`,
			expectMarshalErr: "invalid cue content in string literal",
		},
		{
			name: "healthPolicy as plain string is valid",
			input: `
				attributes: {
					status: {
						healthPolicy: "isHealth: true"
					}
				}
			`,
			expectContains: "isHealth",
		},
		{
			name: "healthPolicy with package declaration",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							package health
							
							isHealth: context.output.status.phase == "Running"
						"""#
					}
				}
			`,
			expectContains: "package health",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			err = DecodeMetadata(rootField)
			if tt.expectUnmarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectUnmarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.expectContains != "" {
				healthPolicyField, ok := GetFieldByPath(rootField, "attributes.status.healthPolicy")
				require.True(t, ok)

				switch v := healthPolicyField.Value.(type) {
				case *ast.BasicLit:
					require.Contains(t, v.Value, tt.expectContains)
				case *ast.StructLit:
					out, err := format.Node(v)
					require.NoError(t, err)
					require.Contains(t, string(out), tt.expectContains)
				default:
					t.Fatalf("unexpected healthPolicy value type: %T", v)
				}
			}
		})
	}
}

func TestMarshalAndUnmarshalCustomStatus(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectMarshalErr   string
		expectUnmarshalErr string
		expectContains     string
	}{
		{
			name: "valid customStatus with string message",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: "Service is healthy"
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "valid customStatus with interpolation",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: "\(context.output.metadata.name) is running"
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "valid customStatus with selector expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: context.output.status.message
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "valid customStatus with binary expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: "Replicas: " + context.output.status.replicas
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "valid customStatus with call expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: strconv.FormatInt(context.output.status.replicas, 10)
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "valid customStatus with list expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: [for c in context.output.status.conditions if c.type == "Ready" { c.message }][0]
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "customStatus missing message field",
			input: `
				attributes: {
					status: {
						customStatus: {
							someOtherField: "value"
						}
					}
				}
			`,
			expectMarshalErr: "customStatus must contain a 'message' field",
		},
		{
			name: "customStatus with invalid message type (struct)",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: { nested: "value" }
						}
					}
				}
			`,
			expectMarshalErr: "customStatus field 'message' must be a string expression",
		},
		{
			name: "customStatus with integer literal message",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: 42
						}
					}
				}
			`,
			expectMarshalErr: "customStatus field 'message' must be a string",
		},
		{
			name: "valid stringified customStatus round trip",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							message: "Pod \(context.output.metadata.name) is running"
						"""#
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "malformed stringified customStatus fails validation",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							invalid cue: abc
						"""#
					}
				}
			`,
			expectMarshalErr: "invalid cue content in string literal",
		},
		{
			name: "customStatus with additional fields alongside message",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: "Service is healthy"
							severity: "info"
							timestamp: context.output.metadata.creationTimestamp
						}
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "customStatus as plain string is valid",
			input: `
				attributes: {
					status: {
						customStatus: "message: \"Hello\""
					}
				}
			`,
			expectContains: "message",
		},
		{
			name: "customStatus with import statement should work",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							import "strings"
							message: strings.Join(["hello", "world"], ",")
						"""#
					}
				}
			`,
			expectContains: "import \"strings\"",
		},
		{
			name: "customStatus with multiple imports",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							import "strings"
							import "strconv"
							count: strconv.Atoi("42")
							message: strings.Join(["Count", strconv.FormatInt(count, 10)], ": ")
						"""#
					}
				}
			`,
			expectContains: "import \"strconv\"",
		},
		{
			name: "customStatus with import alias",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							import str "strings"
							message: str.ToUpper(str.Join(["hello", "world"], " "))
						"""#
					}
				}
			`,
			expectContains: "import str \"strings\"",
		},
		{
			name: "customStatus with package declaration",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							package mytest
							
							message: "Package test"
						"""#
					}
				}
			`,
			expectContains: "package mytest",
		},
		{
			name: "customStatus with package and imports",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							package mytest
							
							import "strings"
							
							message: strings.ToUpper("hello world")
						"""#
					}
				}
			`,
			expectContains: "package mytest",
		},
		{
			name: "customStatus with import still requires message field",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							import "strings"
							someOtherField: "value"
						"""#
					}
				}
			`,
			expectMarshalErr: "customStatus must contain a 'message' field",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			err = DecodeMetadata(rootField)
			if tt.expectUnmarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectUnmarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.expectContains != "" {
				customStatusField, ok := GetFieldByPath(rootField, "attributes.status.customStatus")
				require.True(t, ok)

				switch v := customStatusField.Value.(type) {
				case *ast.BasicLit:
					require.Contains(t, v.Value, tt.expectContains)
				case *ast.StructLit:
					out, err := format.Node(v)
					require.NoError(t, err)
					require.Contains(t, string(out), tt.expectContains)
				default:
					t.Fatalf("unexpected customStatus value type: %T", v)
				}
			}
		})
	}
}

func TestHealthPolicyEdgeCases(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectMarshalErr   string
		expectUnmarshalErr string
	}{
		{
			name: "healthPolicy with unary expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: !context.output.status.failed
						}
					}
				}
			`,
		},
		{
			name: "healthPolicy with parenthesized expression",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: (context.output.status.phase == "Running" || context.output.status.phase == "Succeeded")
						}
					}
				}
			`,
		},
		{
			name: "healthPolicy with nested binary expressions",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: context.output.status.ready && (context.output.status.replicas > 0 || context.output.status.phase == "Ready")
						}
					}
				}
			`,
		},
		{
			name: "healthPolicy with string literal should fail",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: "true"
						}
					}
				}
			`,
			expectMarshalErr: "healthPolicy field 'isHealth' must be a boolean literal (true/false)",
		},
		{
			name: "healthPolicy with comprehension should fail",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: [for c in context.output.status.conditions if c.type == "Ready" { c.status }][0]
						}
					}
				}
			`,
			expectMarshalErr: "healthPolicy field 'isHealth' must be a boolean expression",
		},
		{
			name: "healthPolicy with empty struct should fail",
			input: `
				attributes: {
					status: {
						healthPolicy: {}
					}
				}
			`,
			expectMarshalErr: "healthPolicy must contain an 'isHealth' field",
		},
		{
			name: "healthPolicy with additional fields is allowed",
			input: `
				attributes: {
					status: {
						healthPolicy: {
							isHealth: true
							reason: "always healthy"
						}
					}
				}
			`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			err = DecodeMetadata(rootField)
			if tt.expectUnmarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectUnmarshalErr)
				return
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCustomStatusEdgeCases(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		expectMarshalErr   string
		expectUnmarshalErr string
	}{
		{
			name: "customStatus with comprehension expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: [for i, v in context.output.status.conditions { "\(i): \(v.message)" }][0]
						}
					}
				}
			`,
		},
		{
			name: "customStatus with index expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: context.output.status.conditions[0].message
						}
					}
				}
			`,
		},
		{
			name: "customStatus with slice expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: context.output.status.message[0:10]
						}
					}
				}
			`,
		},
		{
			name: "customStatus with parenthesized expression",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: ("Status: " + context.output.status.phase)
						}
					}
				}
			`,
		},
		{
			name: "customStatus with nested interpolation",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: "Pod \(context.output.metadata.name) has \(context.output.status.replicas) replicas"
						}
					}
				}
			`,
		},
		{
			name: "customStatus with boolean literal should fail",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: true
						}
					}
				}
			`,
			expectMarshalErr: "customStatus field 'message' must be a string",
		},
		{
			name: "customStatus with empty struct should fail",
			input: `
				attributes: {
					status: {
						customStatus: {}
					}
				}
			`,
			expectMarshalErr: "customStatus must contain a 'message' field",
		},
		{
			name: "customStatus with only non-message fields should fail",
			input: `
				attributes: {
					status: {
						customStatus: {
							severity: "error"
							code: 500
						}
					}
				}
			`,
			expectMarshalErr: "customStatus must contain a 'message' field",
		},
		{
			name: "customStatus with field expression should fail",
			input: `
				attributes: {
					status: {
						customStatus: {
							message: { template: "Hello" }
						}
					}
				}
			`,
			expectMarshalErr: "customStatus field 'message' must be a string expression",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			if tt.expectMarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectMarshalErr)
				return
			} else {
				require.NoError(t, err)
			}

			err = DecodeMetadata(rootField)
			if tt.expectUnmarshalErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectUnmarshalErr)
				return
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMixedFieldsWithAndWithoutImports(t *testing.T) {
	input := `
		attributes: {
			status: {
				healthPolicy: #"""
					isHealth: context.output.status.phase == "Running"
				"""#
				customStatus: #"""
					import "strings"
					message: strings.ToLower(context.output.status.phase)
				"""#
			}
		}
	`

	file, err := parser.ParseFile("-", input)
	require.NoError(t, err)

	var rootField *ast.Field
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.Field); ok {
			rootField = f
			break
		}
	}
	require.NotNil(t, rootField)

	// Encode (struct -> string)
	err = EncodeMetadata(rootField)
	require.NoError(t, err)

	// Decode (string -> struct/string based on imports)
	err = DecodeMetadata(rootField)
	require.NoError(t, err)

	// Check healthPolicy (no imports) - should be decoded to struct
	healthField, ok := GetFieldByPath(rootField, "attributes.status.healthPolicy")
	require.True(t, ok)
	_, isStruct := healthField.Value.(*ast.StructLit)
	assert.True(t, isStruct, "healthPolicy without imports should be decoded to struct")

	// Check customStatus (has imports) - should remain as string
	customField, ok := GetFieldByPath(rootField, "attributes.status.customStatus")
	require.True(t, ok)
	basicLit, isString := customField.Value.(*ast.BasicLit)
	assert.True(t, isString, "customStatus with imports should remain as string")
	if isString {
		assert.Contains(t, basicLit.Value, "import \"strings\"")
	}
}

func TestBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "existing worker component healthPolicy format",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							isHealth: context.output.status.readyReplicas > 0 && context.output.status.readyReplicas == context.output.status.replicas
						"""#
					}
				}
			`,
		},
		{
			name: "existing worker component customStatus format",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							appName:     context.appName
							internal:    "\($appName) is running"
							exposeType:  *"" | string
							if context.outputs.service != _|_ {
								exposeType: context.outputs.service.spec.type
							}
							if exposeType == "ClusterIP" {
								message: "\(appName) has ClusterIP service"
							}
							if exposeType == "NodePort" {
								message: "\(appName) has NodePort service"
							}
							if exposeType == "LoadBalancer" {
								message: "\(appName) has LoadBalancer service"
							}
							if exposeType == "" {
								message: internal
							}
						"""#
					}
				}
			`,
		},
		{
			name: "complex multi-field definition with both healthPolicy and customStatus",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							isHealth: context.output.status.phase == "Running" || (context.output.status.phase == "Succeeded" && context.output.spec.restartPolicy == "Never")
						"""#
						customStatus: #"""
							ready: [ for c in context.output.status.conditions if c.type == "Ready" { c.status }][0] == "True"
							message: "Pod \(context.output.metadata.name): phase=\(context.output.status.phase), ready=\(ready)"
						"""#
					}
				}
			`,
		},
		{
			name: "simple string format for healthPolicy",
			input: `
				attributes: {
					status: {
						healthPolicy: "isHealth: true"
					}
				}
			`,
		},
		{
			name: "simple string format for customStatus",
			input: `
				attributes: {
					status: {
						customStatus: "message: \"Service is healthy\""
					}
				}
			`,
		},
		{
			name: "healthPolicy with list comprehensions and complex conditions",
			input: `
				attributes: {
					status: {
						healthPolicy: #"""
							conditions: [ for c in context.output.status.conditions if c.type == "Ready" || c.type == "ContainersReady" { c.status }]
							isHealth: len($conditions) > 0 && ![ for c in conditions if c != "True" { c }] != []
						"""#
					}
				}
			`,
		},
		{
			name: "customStatus with nested conditionals and string interpolation",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							phase: context.output.status.phase
							replicas: context.output.status.replicas
							readyReplicas: *0 | int
							if context.output.status.readyReplicas != _|_ {
								readyReplicas: context.output.status.readyReplicas
							}
							if phase == "Running" {
								if readyReplicas == replicas {
									message: "All \(replicas) replicas are ready"
								}
								if readyReplicas < replicas {
									message: "Only \(readyReplicas) of \(replicas) replicas are ready"
								}
							}
							if phase != "Running" {
								message: "Deployment is in phase: \(phase)"
							}
						"""#
					}
				}
			`,
		},
		{
			name: "preserving local fields that start with $",
			input: `
				attributes: {
					status: {
						customStatus: #"""
							internal: "internal state"
							compute: context.output.status.replicas * 2
							debugInfo: {
								phase: context.output.status.phase
								replicas: context.output.status.replicas
							}
							message: "Status: \(internal), computed: \(compute)"
						"""#
					}
				}
			`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile("-", tt.input)
			require.NoError(t, err)

			var rootField *ast.Field
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.Field); ok {
					rootField = f
					break
				}
			}
			require.NotNil(t, rootField)

			err = EncodeMetadata(rootField)
			require.NoError(t, err)

			err = DecodeMetadata(rootField)
			require.NoError(t, err)
		})
	}
}
