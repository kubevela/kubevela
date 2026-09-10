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

package cachekey

import (
	"fmt"
	"sort"

	"cuelang.org/go/cue/ast"
	cueliteral "cuelang.org/go/cue/literal"
	cueparser "cuelang.org/go/cue/parser"
	cuetoken "cuelang.org/go/cue/token"
)

// contextIdent is the CUE identifier a template reads runtime values from.
const contextIdent = "context"

// InternalField is the block holding everything the tooling generates, kept
// apart from the authored storage: block so that what a user may edit needs no
// explaining: one block is theirs, the other is not.
//
// The $ prefix follows the convention CueX already uses for $params and
// $returns. A leading underscore would be the CUE idiom for "internal", but
// hidden fields are dropped from exported output, and these have to stay
// visible - GitOps diffs them and admission re-derives them.
const InternalField = "$internal"

const (
	// KeyField is the generated cache key, inside InternalField.
	KeyField = "key"
)

// Dimension is one context value a cache key is built from.
type Dimension struct {
	// Field is the context field, e.g. "cluster".
	Field string
	// Index is the literal key for an indexed read, e.g. the label name in
	// context.appLabels["team"]. Empty for a plain field.
	Index string

	order int
}

// String renders a dimension the way the tests and error messages name it:
// "cluster", or "appLabels[team]".
func (d Dimension) String() string {
	if d.Index == "" {
		return d.Field
	}
	return fmt.Sprintf("%s[%s]", d.Field, d.Index)
}

// Infer returns the context dimensions a template's resolution depends on, in the
// order they contribute to the key.
//
// The whole template is scanned, not just output:. A value reaches the output
// through aliases and helper fields, and deciding which reads actually influence
// it is not tractable - so a read anywhere counts. Over-inclusion costs a narrower
// cache; under-inclusion serves one consumer another's data.
func Infer(template string, rules *Rules) ([]Dimension, error) {
	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse cue template: %w", err)
	}

	// $internal is generated from these reads, so it cannot be one of them:
	// scanning it would make a regenerated key depend on the previous key rather
	// than on the resolution logic. Everything else, storage: included, is
	// authored and is scanned normally.
	stripGenerated(file)

	found := map[string]Dimension{}
	// Reads of an indexed field, keyed by the selector node, so the pass below
	// can remove the ones that turned out to carry an index and leave the bare
	// ones behind.
	indexedReads := map[*ast.SelectorExpr]string{}
	var scanErr error

	ast.Walk(file, func(n ast.Node) bool {
		if scanErr != nil {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !isContextIdent(sel.X) {
			return true
		}
		field, err := selectorName(sel.Sel)
		if err != nil {
			scanErr = err
			return false
		}
		entry, keyed := rules.keyedEntry(field)
		if !keyed {
			scanErr = unsupportedContext(field)
			return false
		}
		if entry.Indexed {
			// The value contributing to the key is at an index, so the index has
			// to be knowable now. Handled where the index is visible, below; this
			// records the read so a bare one can be told from an indexed one.
			indexedReads[sel] = field
			return true
		}
		d := Dimension{Field: field, order: entry.Order}
		found[d.String()] = d
		return true
	}, nil)
	if scanErr != nil {
		return nil, scanErr
	}

	// Indexed reads are a level up the tree: context.appLabels["x"] is an IndexExpr
	// wrapping the selector, so it needs its own pass to see the index.
	ast.Walk(file, func(n ast.Node) bool {
		if scanErr != nil {
			return false
		}
		idx, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		sel, ok := idx.X.(*ast.SelectorExpr)
		if !ok || !isContextIdent(sel.X) {
			return true
		}
		field, err := selectorName(sel.Sel)
		if err != nil {
			scanErr = err
			return false
		}
		entry, keyed := rules.keyedEntry(field)
		if !keyed || !entry.Indexed {
			return true
		}
		delete(indexedReads, sel)
		lit, ok := idx.Index.(*ast.BasicLit)
		if !ok || lit.Kind != cuetoken.STRING {
			scanErr = fmt.Errorf("context.%s must be read with a literal index so the key can be "+
				"determined before the template runs; a computed index is not knowable", field)
			return false
		}
		key, err := cueliteral.Unquote(lit.Value)
		if err != nil {
			scanErr = fmt.Errorf("context.%s has an unreadable index %s: %w", field, lit.Value, err)
			return false
		}
		d := Dimension{Field: field, Index: key, order: entry.Order}
		found[d.String()] = d
		return true
	}, nil)
	if scanErr != nil {
		return nil, scanErr
	}

	// A read of the whole map contributes nothing to the key while the resolver
	// hands the template the whole map, so two Applications with different
	// labels would share one cached result. The key can only carry a value it
	// can name, so the read has to name one.
	for _, field := range indexedReads {
		return nil, fmt.Errorf("context.%s must be read at a literal key - context.%s[\"team\"] - "+
			"so the value it contributes to the cache key is knowable; reading the whole map would "+
			"let two Applications with different %s share one cached result", field, field, field)
	}

	dims := make([]Dimension, 0, len(found))
	for _, d := range found {
		dims = append(dims, d)
	}
	// Sorted by the rules' order, then by index, so the same template always
	// produces the same key regardless of how it was written or walked.
	sort.Slice(dims, func(i, j int) bool {
		if dims[i].order != dims[j].order {
			return dims[i].order < dims[j].order
		}
		return dims[i].Index < dims[j].Index
	})
	if len(dims) == 0 {
		return nil, nil
	}
	return dims, nil
}

// stripGeneratedKey removes storage.key from the tree before scanning.
// stripGenerated removes the $internal block, so inference sees only what the
// author wrote. Dropping the whole block is the point of having one: there is no
// per-field list to keep in step with what Stamp writes.
func stripGenerated(file *ast.File) {
	kept := make([]ast.Decl, 0, len(file.Decls))
	for _, decl := range file.Decls {
		if field, ok := decl.(*ast.Field); ok {
			if name, _, err := ast.LabelName(field.Label); err == nil && name == InternalField {
				continue
			}
		}
		kept = append(kept, decl)
	}
	file.Decls = kept
}

// unsupportedContext is the single rejection for context a source may not read.
// Anything outside the keyed list is unsupported, and the answer is always the
// same - pass it in as a property - so there is one message rather than a reason
// maintained per field.
func unsupportedContext(field string) error {
	return fmt.Errorf("context.%s is not a supported value in SourceDefinitions; "+
		"additional data can be passed through properties", field)
}

func isContextIdent(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == contextIdent
}

func selectorName(l ast.Label) (string, error) {
	name, _, err := ast.LabelName(l)
	if err != nil {
		return "", fmt.Errorf("unreadable context field: %w", err)
	}
	return name, nil
}
