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

	cueast "cuelang.org/go/cue/ast"
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

			newExpr := &cueast.BasicLit{Kind: token.STRING, Value: tt.newValue}
			ok := UpdateNodeByPath(file, tt.path, newExpr)
			require.Equal(t, tt.shouldSet, ok)

			if ok {
				field, found := GetFieldByPath(file, tt.path)
				require.True(t, found)

				basicLit, isLit := field.Value.(*cueast.BasicLit)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.ParseExpr("-", tt.input)
			if tt.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			structLit, ok := expr.(*cueast.StructLit)
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
		label    cueast.Label
		expected string
	}{
		{
			name:     "Ident label",
			label:    &cueast.Ident{Name: "foo"},
			expected: "foo",
		},
		{
			name:     "BasicLit string label",
			label:    &cueast.BasicLit{Value: `"bar"`},
			expected: "bar",
		},
		{
			name:     "BasicLit empty string",
			label:    &cueast.BasicLit{Value: `""`},
			expected: "",
		},
		{
			name:     "BasicLit with quotes and spaces",
			label:    &cueast.BasicLit{Value: `"  spaced  "`},
			expected: "  spaced  ",
		},
		{
			name:     "Nil label",
			label:    nil,
			expected: "",
		},
		{
			name:     "Unsupported label type",
			label:    &cueast.ListLit{}, // not a valid label
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

func TestWrapCueStruct(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Basic indented struct",
			input:    `name: "foo"`,
			expected: "{\nname: \"foo\"\n}",
		},
		{
			name:     "Already formatted multiline",
			input:    "a: 1\nb: 2",
			expected: "{\na: 1\nb: 2\n}",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "{\n\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := WrapCueStruct(tt.input)
			require.Equal(t, tt.expected, out)
		})
	}
}

func normalizeString(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}
