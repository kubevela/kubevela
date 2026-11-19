/*
Copyright 2026 The KubeVela Authors.

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

package schema

import (
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"

	"github.com/oam-dev/kubevela/pkg/appfile"
)

// ImmutableFieldsFromTemplate parses a CUE template string and returns the set of
// dotted parameter field paths that are marked with the +immutable comment marker.
func ImmutableFieldsFromTemplate(templateStr string) map[string]bool {
	if templateStr == "" {
		return nil
	}
	f, err := parser.ParseFile("-", templateStr, parser.ParseComments)
	if err != nil {
		return nil
	}

	paramStruct := getParameterStruct(f)
	if paramStruct == nil {
		return nil
	}

	immutableFields := make(map[string]bool)
	collectImmutableFields(paramStruct, "", immutableFields)
	if len(immutableFields) == 0 {
		return nil
	}
	return immutableFields
}

// collectImmutableFields recursively walks a struct, collecting dotted
// paths of fields marked with +immutable into the result map.
func collectImmutableFields(structLit *ast.StructLit, prefix string, result map[string]bool) {
	for _, element := range structLit.Elts {
		field, ok := element.(*ast.Field)
		if !ok {
			continue
		}
		name := extractFieldName(field)
		if name == "" {
			continue
		}
		fieldPath := name
		if prefix != "" {
			fieldPath = prefix + "." + name
		}
		if hasImmutableComment(field) {
			result[fieldPath] = true
			continue
		}
		if nested, ok := field.Value.(*ast.StructLit); ok {
			collectImmutableFields(nested, fieldPath, result)
		}
	}
}

// getParameterStruct gets the top-level `parameter` struct
func getParameterStruct(f *ast.File) *ast.StructLit {
	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok || extractFieldName(field) != "parameter" {
			continue
		}
		if structLit, ok := field.Value.(*ast.StructLit); ok {
			return structLit
		}
	}
	return nil
}

// hasImmutableComment reports whether any comment group attached to field
// contains a line matching the +immutable marker.
func hasImmutableComment(field *ast.Field) bool {
	for _, commentGroup := range field.Comments() {
		for _, comment := range commentGroup.List {
			marker := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if marker == appfile.ImmutableTag {
				return true
			}
		}
	}
	return false
}

func extractFieldName(field *ast.Field) string {
	switch label := field.Label.(type) {
	case *ast.Ident:
		return label.Name
	case *ast.BasicLit:
		return strings.Trim(label.Value, `"`)
	}
	return ""
}
