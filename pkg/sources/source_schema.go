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

package sources

import (
	"fmt"
	"math"

	cueformat "cuelang.org/go/cue/format"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueparser "cuelang.org/go/cue/parser"
	"k8s.io/utils/lru"
)

// typeToSchema returns values retyped by the source's schema.
//
// A cached value has been through JSON, where 2.0 and 2 are the same text and
// the int/float distinction is lost. Unifying with the schema and decoding
// restores it, so a cache hit carries the same Go types a fresh resolution does
// and an expression means the same thing either way.
//
// Values that will not unify are returned unchanged: this is a retyping, not a
// second validation, and a cached value that no longer matches its schema is
// reported by the paths that already do that.
func (r *sourceResolver) typeToSchema(sourceTemplate string, values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return values
	}
	schemaExpr, err := extractSourceSchemaExpr(sourceTemplate)
	if err != nil || schemaExpr == "" {
		return values
	}
	schema := cuecontext.New().CompileString(schemaExpr)
	if schema.Err() != nil {
		return values
	}
	typed, ok := retypeAgainst(schema, values).(map[string]interface{})
	if !ok {
		return values
	}
	return typed
}

// retypeAgainst walks a value alongside the schema that declares it, converting
// each number to the kind the schema asks for.
//
// Only numbers, and only where the schema says which. A field the schema leaves
// open, or one it does not mention, is left exactly as it arrived: this restores
// what JSON dropped, it does not decide anything the schema did not.
func retypeAgainst(schema cue.Value, v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			out[k] = retypeAgainst(fieldOf(schema, k), child)
		}
		return out
	case []interface{}:
		elem := elementOf(schema)
		out := make([]interface{}, len(t))
		for i, child := range t {
			out[i] = retypeAgainst(elem, child)
		}
		return out
	case float64:
		kind := schema.IncompleteKind()
		// IntKind and FloatKind both, and NumberKind is their union: a schema
		// saying `number` has not chosen, so neither does this.
		if kind == cue.IntKind && t == math.Trunc(t) && !math.IsInf(t, 0) {
			return int64(t)
		}
		if kind == cue.FloatKind {
			return t
		}
		// No declaration to go on. A JSON number with no fractional part was
		// most likely an integer, which is the reading celexpr makes for a value
		// with no schema at all.
		if !schema.Exists() && t == math.Trunc(t) && !math.IsInf(t, 0) {
			return int64(t)
		}
		return t
	default:
		return v
	}
}

// fieldOf resolves one field of a schema struct, optional fields included.
func fieldOf(schema cue.Value, name string) cue.Value {
	if !schema.Exists() {
		return schema
	}
	if f := schema.LookupPath(cue.MakePath(cue.Str(name))); f.Exists() {
		return f
	}
	if f := schema.LookupPath(cue.MakePath(cue.Str(name).Optional())); f.Exists() {
		return f
	}
	// An open map - [string]: int - declares the value type at every key.
	return schema.LookupPath(cue.MakePath(cue.AnyString))
}

// elementOf resolves the element type of a schema list.
func elementOf(schema cue.Value) cue.Value {
	if !schema.Exists() {
		return schema
	}
	// `[...int]` has an element type but no concrete element, so iterating gives
	// nothing; LookupPath with an index reaches the constraint either way.
	if elem := schema.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
		return elem
	}
	if it, err := schema.List(); err == nil && it.Next() {
		return it.Value()
	}
	return cue.Value{}
}

func (r *sourceResolver) validateResolvedOutput(sourceType, sourceTemplate string, output map[string]interface{}) error {
	schemaExpr := r.sourceSchemas[sourceType]
	if schemaExpr == "" {
		extracted, err := extractSourceSchemaExpr(sourceTemplate)
		if err != nil {
			return err
		}
		if extracted == "" {
			return nil
		}
		schemaExpr = extracted
		r.sourceSchemas[sourceType] = extracted
	}
	// Encoded from the Go value rather than rendered to JSON text and parsed
	// back. A float64 with no fractional part marshals as `2`, which CUE reads
	// as an int, so a schema declaring `ratio: float` rejected its own source's
	// output with "conflicting values 2 and float". Encode keeps the Go type.
	ctx := cuecontext.New()
	data := ctx.Encode(output)
	if data.Err() != nil {
		return data.Err()
	}
	// close() refuses an output field the schema does not declare, which is what
	// keeps the schema a contract rather than a suggestion.
	schema := ctx.CompileString(fmt.Sprintf("close(%s)", schemaExpr))
	if schema.Err() != nil {
		return schema.Err()
	}
	out := schema.Unify(data)
	if out.Err() != nil {
		return out.Err()
	}
	return out.Validate(cue.Concrete(true))
}

// schemaExprCacheSize bounds the number of distinct definition templates kept.
// Unbounded, every edit to every SourceDefinition would retain another copy of
// its whole template for the life of the process.
const schemaExprCacheSize = 512

// schemaExprCache memoises the extracted schema block, which is fixed for the
// life of a definition. A pure function of the template, so the text is the key.
//
// Keyed on the text rather than a hash of it, and bounded, for the same reasons
// policyCache is.
var schemaExprCache = lru.New(schemaExprCacheSize)

func extractSourceSchemaExpr(template string) (string, error) {
	if hit, ok := schemaExprCache.Get(template); ok {
		if expr, ok := hit.(string); ok {
			return expr, nil
		}
	}
	expr, err := parseSourceSchemaExpr(template)
	if err != nil {
		// Not cached: a template that fails to parse is a condition worth
		// reporting again rather than remembering.
		return "", err
	}
	schemaExprCache.Add(template, expr)
	return expr, nil
}

func parseSourceSchemaExpr(template string) (string, error) {
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
		if err != nil || name != "schema" {
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
