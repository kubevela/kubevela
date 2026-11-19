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

package sourcedefinition

import (
	"fmt"
	"slices"
	"strings"

	"cuelang.org/go/cue/ast"
	cueliteral "cuelang.org/go/cue/literal"
	cueparser "cuelang.org/go/cue/parser"
	cuetoken "cuelang.org/go/cue/token"

	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// ValidateSourceStorage checks the `storage:` block of a SourceDefinition template.
//
// Every SourceDefinition must declare a storage key: it names the backing Config
// cache entry and defines the sharing boundary between Applications. Without one
// the controller has no deterministic cache identity, so an absent or empty key is
// rejected here rather than papered over at resolution time.
//
// The key is usually a CUE interpolation resolved per-binding at runtime
// (`"cfg-\(context.cluster)"`), so it cannot be fully evaluated at admission. What
// is statically knowable is checked:
//   - a fully literal key is validated in full (charset and length)
//   - an interpolated key has its literal segments validated, which catches
//     characters no interpolated value could ever make legal
//
// The resolved key is validated again at resolution time, where interpolated
// values are concrete.
func ValidateSourceStorage(template string) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("SourceDefinition must declare a cue template")
	}

	internal, err := topLevelField(template, cachekey.InternalField)
	if err != nil {
		return err
	}
	if internal == nil {
		return fmt.Errorf("SourceDefinition must declare a %s block with a %s field; apply it with "+
			"`vela def apply` and one will be generated", cachekey.InternalField, cachekey.KeyField)
	}

	structLit, ok := internal.Value.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("%s: must be a struct declaring a %s field", cachekey.InternalField, cachekey.KeyField)
	}

	keyField := fieldByName(structLit.Elts, cachekey.KeyField)
	if keyField == nil {
		return fmt.Errorf("%s: must declare a %s field naming the cache entry",
			cachekey.InternalField, cachekey.KeyField)
	}

	return validateKeyExpr(keyField.Value)
}

// ValidateSourceSchema checks the `schema:` block of a SourceDefinition template.
//
// schema: is the contract between the platform engineer and the application
// author, and it is load-bearing for the feature's security properties: admission
// validates every a source read path against it, and the resolver validates the
// resolved output against it. Both checks are skipped when it is absent, so a
// schema-less SourceDefinition would let an application read any field of the
// resolved output with no validation at either layer. It is therefore required.
func ValidateSourceSchema(template string) error {
	schema, err := topLevelField(template, "schema")
	if err != nil {
		return err
	}
	if schema == nil {
		return fmt.Errorf("SourceDefinition must declare a schema: block")
	}
	structLit, ok := schema.Value.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("schema: must be a struct declaring the fields an Application may read")
	}
	// `...` is an element but not a declaration. A schema that is only open
	// declares nothing, so admission has no path to check a read against and
	// close(schema) at resolution admits no output field either: the definition
	// is accepted and then cannot be read from at all.
	declared := 0
	for _, elt := range structLit.Elts {
		if _, ok := elt.(*ast.Field); ok {
			declared++
		}
	}
	if declared == 0 {
		return fmt.Errorf("schema: must declare at least one field; an empty or open schema exposes nothing to a source read")
	}
	return nil
}

// validateKeyExpr validates whatever statically-knowable text the key expression
// carries. Non-string expressions (e.g. a bare reference) are left alone: they
// cannot be checked here and are caught at resolution time.
func validateKeyExpr(expr ast.Expr) error {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != cuetoken.STRING {
			return fmt.Errorf("%s.%s must be a string, got %s", cachekey.InternalField, cachekey.KeyField, v.Kind)
		}
		key, err := cueliteral.Unquote(v.Value)
		if err != nil {
			return fmt.Errorf("%s.%s is not a valid string literal: %w", cachekey.InternalField, cachekey.KeyField, err)
		}
		return cachekey.ValidateCacheKey(key)
	case *ast.Interpolation:
		// Elts alternate literal, expression, literal, ... Only the literals are
		// knowable now; an interpolated value cannot rescue an illegal literal.
		literal := false
		for _, elt := range v.Elts {
			literal = !literal
			if !literal {
				continue
			}
			lit, ok := elt.(*ast.BasicLit)
			if !ok || lit.Kind != cuetoken.STRING {
				continue
			}
			// Interpolation fragments carry their delimiters — a fragment looks like
			// `"head\(`, `)middle\(` or `)tail"` — so strip that punctuation before
			// checking. Trim only touches the ends, so an illegal character *inside*
			// the literal text is still caught.
			text := strings.Trim(lit.Value, `"\()`)
			if bad := cachekey.InvalidCacheKeyChars(text); bad != "" {
				return fmt.Errorf("%s.%s literal segment %q contains characters not allowed in a cache key (%s); only lowercase letters, digits and '-' are permitted", cachekey.InternalField, cachekey.KeyField, text, bad)
			}
		}
	}
	// Any other expression - a call, a binary op, a bare reference - is accepted
	// here on purpose. Its key exists only once the template runs, so there is
	// nothing to check yet; resolveCachePolicy validates the computed key at
	// render, which is the first moment it is knowable.
	return nil
}

// topLevelField returns the named top-level field of a definition template, or
// nil when it is absent.
func topLevelField(template, name string) (*ast.Field, error) {
	file, err := cueparser.ParseFile("-", template, cueparser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse cue template: %w", err)
	}
	return fieldByName(file.Decls, name), nil
}

// fieldByName finds a field with the given label among the declarations.
func fieldByName(decls []ast.Decl, name string) *ast.Field {
	for _, decl := range decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		label, _, err := ast.LabelName(field.Label)
		if err != nil || label != name {
			continue
		}
		return field
	}
	return nil
}

// ParseConsumableFrom reads the optional `consumableFrom:` block of a
// SourceDefinition template.
//
// It returns the declared surfaces, or nil when the block is absent - meaning
// every surface that supports a source read. Restricting a source is opt-in: the
// common case declares nothing.
func ParseConsumableFrom(template string) ([]string, error) {
	field, err := topLevelField(template, "consumableFrom")
	if err != nil || field == nil {
		return nil, err
	}

	switch v := field.Value.(type) {
	case *ast.ListLit:
		var surfaces []string
		for _, elt := range v.Elts {
			lit, ok := elt.(*ast.BasicLit)
			if !ok {
				return nil, fmt.Errorf("consumableFrom entries must be strings")
			}
			value, err := literalString(lit)
			if err != nil {
				return nil, fmt.Errorf("consumableFrom entries must be strings: %w", err)
			}
			if !slices.Contains(sources.ConsumableSurfaces, value) {
				return nil, fmt.Errorf("consumableFrom entry %q is not a surface that supports a source read; expected one of %v",
					value, sources.ConsumableSurfaces)
			}
			surfaces = append(surfaces, value)
		}
		if len(surfaces) == 0 {
			return nil, fmt.Errorf("consumableFrom must not be empty; omit it to allow every supported surface")
		}
		return surfaces, nil
	default:
		return nil, fmt.Errorf("consumableFrom must be a list of surfaces, for example [\"component\"]; omit it to allow every supported surface")
	}
}

// ValidateSurfaceCompatibility rejects a definition that can never resolve where
// it says it can be consumed.
//
// A source's context is its call site's, narrowed to what the cache-key rules
// allow. A template reading a field some surfaces lack is not usable from those
// surfaces, and one reading fields no single surface has is not usable anywhere.
// Saying so when the definition is created beats saying it when someone first
// binds the source.
//
// Inert while the rules permit only universally-available fields.
func ValidateSurfaceCompatibility(template string, consumable []string) error {
	fields, err := cachekey.RequiredContext(template)
	if err != nil {
		// The template's context reads are validated by the cache-key check,
		// which reports this better than repeating it here would.
		//nolint:nilerr // reported elsewhere, deliberately not twice
		return nil
	}
	if len(fields) == 0 {
		return nil
	}

	// Naming a surface is an assertion that the source works there, so every
	// named surface has to supply what the template reads. Naming none asserts
	// nothing, and one working surface is enough: the check at bind time is what
	// refuses the rest.
	declared := consumable
	needAll := len(declared) > 0
	if !needAll {
		declared = sources.ConsumableSurfaces
	}

	supported := cachekey.SurfacesSupporting(fields, declared)
	if needAll && len(supported) == len(declared) {
		return nil
	}
	if !needAll && len(supported) > 0 {
		return nil
	}

	// Report against a surface that actually fails, rather than repeating the
	// same sentence per surface, and name where it would work if anywhere does.
	reason := cachekey.CheckSurface(fields, firstUnsupported(fields, declared))
	if elsewhere := cachekey.SurfacesSupporting(fields, sources.ConsumableSurfaces); len(elsewhere) > 0 {
		return fmt.Errorf("this source %w, where it declares it may be consumed; it is available in %s",
			reason, strings.Join(pluralise(elsewhere), ", "))
	}
	return fmt.Errorf("this source %w, and is available in no surface that resolves sources", reason)
}

// firstUnsupported names a surface the fields cannot be read from, in the order
// the author declared them so the diagnostic matches what they wrote.
func firstUnsupported(fields, declared []string) string {
	for _, surface := range declared {
		if cachekey.CheckSurface(fields, surface) != nil {
			return surface
		}
	}
	return declared[0]
}

// SurfaceAllowed reports whether a source declaring the given surfaces may be
// consumed from surface. Nil surfaces means unrestricted.
func SurfaceAllowed(surfaces []string, surface string) bool {
	if len(surfaces) == 0 {
		return true
	}
	return slices.Contains(surfaces, surface)
}

func literalString(lit *ast.BasicLit) (string, error) {
	if lit.Kind != cuetoken.STRING {
		return "", fmt.Errorf("expected a string, got %s", lit.Kind)
	}
	value, err := cueliteral.Unquote(lit.Value)
	if err != nil {
		return "", fmt.Errorf("invalid string literal %s: %w", lit.Value, err)
	}
	return value, nil
}

// pluralise names surfaces the way a message reads them.
func pluralise(surfaces []string) []string {
	out := make([]string, 0, len(surfaces))
	for _, s := range surfaces {
		out = append(out, propexpr.SurfacePlural(s))
	}
	return out
}
