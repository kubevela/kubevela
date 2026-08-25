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

package application

import (
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	cueparser "cuelang.org/go/cue/parser"
)

type sourceSchemaValidator struct {
	schema cue.Value
	// schemaExpr is the schema block as source text, retained because typing an
	// expression needs a schema it can build sentinels from, and re-extracting it
	// would mean a second definition lookup.
	schemaExpr string
}

func (v *sourceSchemaValidator) HasPath(path string) bool {
	cur, ok := v.lookup(path)
	return ok && cur.Exists()
}

// lookup walks the dotted path through the schema value and returns the reached
// value plus whether every segment resolved. Optional fields (field?:) are not
// returned by LookupPath, so a failed struct lookup falls back to iterating the
// parent's fields (including optionals) to locate the segment.
func (v *sourceSchemaValidator) lookup(path string) (cue.Value, bool) {
	cur := v.schema
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return cur, false
		}
		// `content: _` or `properties: _` declares a field without declaring its
		// shape, so nothing below it is knowable here. TypeOf is what judges
		// those reads - it demands `& <type>` at the point of use - and this
		// check has nothing to add beyond rejecting a read the schema
		// deliberately left open.
		if cur.IncompleteKind() == cue.TopKind {
			return cur, true
		}
		if idx, err := strconv.Atoi(seg); err == nil {
			cur = cur.LookupPath(cue.MakePath(cue.Index(idx)))
			if !cur.Exists() {
				return cur, false
			}
			continue
		}
		next := cur.LookupPath(cue.MakePath(cue.Str(seg)))
		if !next.Exists() {
			if opt, ok := lookupOptionalField(cur, seg); ok {
				cur = opt
				continue
			}
			// An open map declares a pattern, never a key: `traits: [string]: {...}`
			// has no field called `scaler`, but reading one is exactly what the
			// map is for. Fall through to the pattern's type, which is what the
			// key will hold. Without this every key read out of a declared map -
			// traits, labels, a Config's outputs - was rejected as undeclared.
			if pattern := cur.LookupPath(cue.MakePath(cue.AnyString)); pattern.Exists() {
				cur = pattern
				continue
			}
			return next, false
		}
		cur = next
	}
	return cur, true
}

// lookupOptionalField finds a field by label in parent (including optional
// fields) and returns its value.
func lookupOptionalField(parent cue.Value, label string) (cue.Value, bool) {
	iter, err := parent.Fields(cue.Optional(true), cue.Definitions(false))
	if err != nil {
		return cue.Value{}, false
	}
	for iter.Next() {
		sel := iter.Selector()
		if sel.IsString() && sel.Unquoted() == label {
			return iter.Value(), true
		}
	}
	return cue.Value{}, false
}

func extractSourceSchemaExprForAdmission(template string) (string, error) {
	return extractTopLevelBlock(template, "schema")
}

// extractTopLevelBlock returns the CUE source of the top-level field named
// blockName (e.g. "schema" or "parameter") from a SourceDefinition template, or
// "" if absent. Static parse only; no evaluation.
func extractTopLevelBlock(template, blockName string) (string, error) {
	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name != blockName {
			continue
		}
		bt, err := cueformat.Node(field.Value)
		if err != nil {
			return "", err
		}
		return string(bt), nil
	}
	return "", nil
}
