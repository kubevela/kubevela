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
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
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
