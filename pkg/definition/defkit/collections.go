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

package defkit

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

// CollectionOp represents an operation on a collection (array/list).
type CollectionOp struct {
	source Value
	ops    []collectionOperation
	guard  Condition // optional guard: wraps entire comprehension in if cond
}

type collectionOperation interface {
	apply(items []any) []any
}

// Each creates a collection operation pipeline from a list parameter.
// Usage: ports.Each().Filter(...).Map(...).Pick(...)
func Each(source Value) *CollectionOp {
	return &CollectionOp{
		source: source,
		ops:    make([]collectionOperation, 0),
	}
}

// From creates a collection operation pipeline from a source value.
// This is an alias for Each, providing a more readable API for transformations.
// Usage: From(volumes).Map(FieldMap{"mountPath": F("path"), "name": F("name")})
func From(source Value) *CollectionOp {
	return Each(source)
}

// F creates a field reference for use in Map operations.
// This is a convenience function that creates a FieldRef.
// Usage: From(volumes).Map(FieldMap{"mountPath": F("path")})
func F(name string) FieldRef {
	return FieldRef(name)
}

func (c *CollectionOp) expr()  {}
func (c *CollectionOp) value() {}

// Guard adds a guard condition that wraps the entire comprehension.
// When the guard is set, the generated CUE is:
//
//	[if guard for v in source if filter {v}]
//
// This is used when the source may not exist and needs a
// protective condition (e.g., if parameter.privileges != _|_).
func (c *CollectionOp) Guard(cond Condition) *CollectionOp {
	c.guard = cond
	return c
}

// GetGuard returns the guard condition, if any.
func (c *CollectionOp) GetGuard() Condition { return c.guard }

// Filter keeps only items matching the predicate.
// Usage: .Filter(Field("expose").Eq(true))
func (c *CollectionOp) Filter(pred Predicate) *CollectionOp {
	c.ops = append(c.ops, &filterOp{pred: pred})
	return c
}

// Map transforms each item using the given field mappings.
// Usage: .Map(Fields{"containerPort": Field("port"), "name": Field("name")})
func (c *CollectionOp) Map(mappings FieldMap) *CollectionOp {
	c.ops = append(c.ops, &mapOp{mappings: mappings})
	return c
}

// MapVariant adds a conditional field mapping that only applies when the iteration
// variable's discriminator field equals the given variant name.
// Generates: if v.discriminator == "variantName" { ...mappings... }
// Usage: .MapVariant("type", "pvc", FieldMap{"persistentVolumeClaim.claimName": FieldRef("claimName")})
func (c *CollectionOp) MapVariant(discriminator, variantName string, mappings FieldMap) *CollectionOp {
	c.ops = append(c.ops, &mapVariantOp{
		discriminator: discriminator,
		variantName:   variantName,
		mappings:      mappings,
	})
	return c
}

// Pick selects only the specified fields from each item.
// Usage: .Pick("name", "mountPath")
func (c *CollectionOp) Pick(fields ...string) *CollectionOp {
	c.ops = append(c.ops, &pickOp{fields: fields})
	return c
}

// Rename renames a field in each item.
// Usage: .Rename("port", "containerPort")
func (c *CollectionOp) Rename(from, to string) *CollectionOp {
	c.ops = append(c.ops, &renameOp{from: from, to: to})
	return c
}

// Wrap wraps each item value under a new key.
// Usage: .Wrap("name") transforms "secret1" to {name: "secret1"}
func (c *CollectionOp) Wrap(key string) *CollectionOp {
	c.ops = append(c.ops, &wrapOp{key: key})
	return c
}

// DefaultField sets a default value for a field if not present.
// Usage: .DefaultField("name", Format("port-%d", Field("port")))
func (c *CollectionOp) DefaultField(field string, defaultVal FieldValue) *CollectionOp {
	c.ops = append(c.ops, &defaultFieldOp{field: field, defaultVal: defaultVal})
	return c
}

// Flatten flattens nested collections from multiple sources.
// Used for volumeMounts which has pvc, configMap, secret, etc.
func (c *CollectionOp) Flatten() *CollectionOp {
	c.ops = append(c.ops, &flattenOp{})
	return c
}

// Source returns the source value.
func (c *CollectionOp) Source() Value { return c.source }

// Operations returns the operations to apply.
func (c *CollectionOp) Operations() []collectionOperation { return c.ops }

// --- Go 1.23 Iterator Methods (iter.Seq) ---

// All returns an iterator over all items after applying operations.
// This is a Go 1.23 range-over-function iterator that can be used in for-range loops.
// Example: for item := range col.All() { ... }
func (c *CollectionOp) All(items []any) iter.Seq[map[string]any] {
	return func(yield func(map[string]any) bool) {
		// Apply all operations in sequence
		result := items
		for _, op := range c.ops {
			result = op.apply(result)
		}

		// Yield each item
		for _, item := range result {
			if m, ok := item.(map[string]any); ok {
				if !yield(m) {
					return
				}
			}
		}
	}
}

// AllPairs returns a key-value iterator with index and item.
// This is a Go 1.23 iter.Seq2 that provides (index, item) pairs.
// Example: for i, item := range col.AllPairs() { ... }
func (c *CollectionOp) AllPairs(items []any) iter.Seq2[int, map[string]any] {
	return func(yield func(int, map[string]any) bool) {
		// Apply all operations in sequence
		result := items
		for _, op := range c.ops {
			result = op.apply(result)
		}

		// Yield each item with index
		for i, item := range result {
			if m, ok := item.(map[string]any); ok {
				if !yield(i, m) {
					return
				}
			}
		}
	}
}

// Collect materializes the iterator into a slice.
// Uses Go 1.23 slices.Collect for efficient collection.
func (c *CollectionOp) Collect(items []any) []map[string]any {
	return slices.Collect(c.All(items))
}

// Count returns the number of items after applying operations.
func (c *CollectionOp) Count(items []any) int {
	count := 0
	for range c.All(items) {
		count++
	}
	return count
}

// First returns the first item after applying operations, or nil if empty.
func (c *CollectionOp) First(items []any) map[string]any {
	for item := range c.All(items) {
		return item
	}
	return nil
}

// FieldMap defines field mappings for Map operation.
type FieldMap map[string]FieldValue

// --- Go 1.23 Iterator Methods for FieldMap (maps package) ---

// All returns an iterator over all key-value pairs using Go 1.23 maps.All.
// Example: for key, val := range fm.All() { ... }
func (fm FieldMap) All() iter.Seq2[string, FieldValue] {
	return maps.All(fm)
}

// Keys returns an iterator over all keys using Go 1.23 maps.Keys.
// Example: for key := range fm.Keys() { ... }
func (fm FieldMap) Keys() iter.Seq[string] {
	return maps.Keys(fm)
}

// Values returns an iterator over all values using Go 1.23 maps.Values.
// Example: for val := range fm.Values() { ... }
func (fm FieldMap) Values() iter.Seq[FieldValue] {
	return maps.Values(fm)
}

// FieldValue represents a value to use in field mapping.
type FieldValue interface {
	resolve(item map[string]any) any
}

// FieldRef references a field from the current item in collection operations.
type FieldRef string

func (f FieldRef) resolve(item map[string]any) any {
	return item[string(f)]
}

// Or provides a fallback if the field is nil or empty.
// Generates CUE like: *v.field | fallback
func (f FieldRef) Or(fallback FieldValue) *OrFieldRef {
	return &OrFieldRef{primary: f, fallback: fallback}
}

// OrConditional provides a fallback using if/else blocks instead of default syntax.
// Generates CUE like:
//
//	if v.field != _|_ { name: v.field }
//	if v.field == _|_ { name: fallbackExpr }
func (f FieldRef) OrConditional(fallback FieldValue) *ConditionalOrFieldRef {
	return &ConditionalOrFieldRef{primary: f, fallback: fallback}
}

// ConditionalOrFieldRef represents a field reference with a conditional fallback.
// Instead of generating CUE default syntax (*v.field | fallback), it generates
// two if/else blocks for the field.
type ConditionalOrFieldRef struct {
	primary  FieldRef
	fallback FieldValue
}

func (c *ConditionalOrFieldRef) resolve(item map[string]any) any {
	val := item[string(c.primary)]
	if val == nil || val == "" {
		return c.fallback.resolve(item)
	}
	return val
}

// OrFieldRef represents a field reference with a fallback value.
type OrFieldRef struct {
	primary  FieldRef
	fallback FieldValue
}

func (o *OrFieldRef) resolve(item map[string]any) any {
	val := item[string(o.primary)]
	if val == nil || val == "" {
		return o.fallback.resolve(item)
	}
	return val
}

// LitVal is a literal value for field mapping.
type LitVal struct {
	val any
}

func (l LitVal) resolve(_ map[string]any) any {
	return l.val
}

// LitField creates a literal field value.
func LitField(val any) LitVal {
	return LitVal{val: val}
}

// FormatField formats a string using item fields.
// Usage: FormatField("port-%v", Field("port"))
type FormatField struct {
	format string
	args   []FieldValue
}

func (f *FormatField) resolve(item map[string]any) any {
	args := make([]any, len(f.args))
	for i, arg := range f.args {
		args[i] = arg.resolve(item)
	}
	return fmt.Sprintf(f.format, args...)
}

// Format creates a formatted string field value.
func Format(format string, args ...FieldValue) *FormatField {
	return &FormatField{format: format, args: args}
}

// RequiredImports returns the CUE imports required by FormatField.
// FormatField typically uses strconv.FormatInt for numeric formatting.
func (f *FormatField) RequiredImports() []string {
	// Check if any args are numeric field references that would need strconv
	// The format "port-%v" with numeric args generates strconv.FormatInt
	for _, arg := range f.args {
		switch arg.(type) {
		case FieldRef, *OrFieldRef:
			// Field references with numeric formatting use strconv
			if strings.Contains(f.format, "%v") || strings.Contains(f.format, "%d") {
				return []string{"strconv"}
			}
		}
	}
	return nil
}

// Predicate represents a filter condition.
type Predicate interface {
	matches(item map[string]any) bool
}

// FieldEq checks if a field equals a value.
type FieldEq struct {
	field string
	value any
}

func (f FieldEq) matches(item map[string]any) bool {
	return item[f.field] == f.value
}

// FieldEquals creates a field equality predicate.
func FieldEquals(field string, value any) FieldEq {
	return FieldEq{field: field, value: value}
}

// FieldIsSet checks if a field is set (not nil).
type FieldIsSet struct {
	field string
}

func (f FieldIsSet) matches(item map[string]any) bool {
	val, exists := item[f.field]
	return exists && val != nil
}

// FieldExists creates a field-is-set predicate.
func FieldExists(field string) FieldIsSet {
	return FieldIsSet{field: field}
}

// --- Internal operation implementations ---

type filterOp struct {
	pred Predicate
}

func (f *filterOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if f.pred.matches(m) {
				result = append(result, item)
			}
		}
	}
	return result
}

type mapOp struct {
	mappings FieldMap
}

func (m *mapOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if itemMap, ok := item.(map[string]any); ok {
			newItem := make(map[string]any)
			for newKey, fieldVal := range m.mappings {
				resolved := fieldVal.resolve(itemMap)
				// Skip nil values from optional field types — they should be
				// omitted from the output entirely, not included as nil.
				if resolved == nil {
					if _, isOpt := fieldVal.(*OptionalField); isOpt {
						continue
					}
					if _, isCompOpt := fieldVal.(*CompoundOptionalField); isCompOpt {
						continue
					}
				}
				newItem[newKey] = resolved
			}
			result = append(result, newItem)
		}
	}
	return result
}

// mapVariantOp represents a conditional map operation based on a discriminator field value.
// When the iteration variable's discriminator field equals the variant name,
// the variant's field mappings are included.
// Generates: if v.discriminator == "variantName" { ...mappings... }
type mapVariantOp struct {
	discriminator string
	variantName   string
	mappings      FieldMap
}

func (m *mapVariantOp) apply(items []any) []any {
	// Runtime apply: merge variant mappings into matching items, pass others through.
	// This mirrors CUE semantics where each variant condition is evaluated for every
	// item in the loop body — non-matching items are kept so later MapVariant ops
	// can process them.
	result := make([]any, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			result = append(result, item)
			continue
		}
		disc, exists := itemMap[m.discriminator]
		if !exists || fmt.Sprintf("%v", disc) != m.variantName {
			// Non-matching: pass through unchanged
			result = append(result, item)
			continue
		}
		// Matching: copy existing fields and merge variant mappings
		newItem := make(map[string]any, len(itemMap)+len(m.mappings))
		for k, v := range itemMap {
			newItem[k] = v
		}
		for newKey, fieldVal := range m.mappings {
			newItem[newKey] = fieldVal.resolve(itemMap)
		}
		result = append(result, newItem)
	}
	return result
}

type pickOp struct {
	fields []string
}

func (p *pickOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			newItem := make(map[string]any)
			for _, field := range p.fields {
				if val, exists := m[field]; exists {
					newItem[field] = val
				}
			}
			result = append(result, newItem)
		}
	}
	return result
}

type renameOp struct {
	from, to string
}

func (r *renameOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			newItem := make(map[string]any)
			for k, v := range m {
				if k == r.from {
					newItem[r.to] = v
				} else {
					newItem[k] = v
				}
			}
			result = append(result, newItem)
		}
	}
	return result
}

type wrapOp struct {
	key string
}

func (w *wrapOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{w.key: item})
	}
	return result
}

type defaultFieldOp struct {
	field      string
	defaultVal FieldValue
}

func (d *defaultFieldOp) apply(items []any) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			newItem := make(map[string]any)
			for k, v := range m {
				newItem[k] = v
			}
			if newItem[d.field] == nil || newItem[d.field] == "" {
				newItem[d.field] = d.defaultVal.resolve(m)
			}
			result = append(result, newItem)
		}
	}
	return result
}

type flattenOp struct{}

func (f *flattenOp) apply(items []any) []any {
	result := make([]any, 0)
	for _, item := range items {
		if arr, ok := item.([]any); ok {
			result = append(result, arr...)
		} else {
			result = append(result, item)
		}
	}
	return result
}

// --- Multi-source collection for volumeMounts pattern ---

// MultiSource combines items from multiple named sources.
type MultiSource struct {
	source      Value
	sources     []string
	ops         []collectionOperation
	mapBySource map[string]FieldMap // Per-source-type mappings
}

func (m *MultiSource) expr()  {}
func (m *MultiSource) value() {}

// FromFields creates a multi-source collection from named fields of an object.
// Usage: FromFields(volumeMounts, "pvc", "configMap", "secret", "emptyDir", "hostPath")
func FromFields(source Value, fields ...string) *MultiSource {
	return &MultiSource{
		source:      source,
		sources:     fields,
		ops:         make([]collectionOperation, 0),
		mapBySource: make(map[string]FieldMap),
	}
}

// Pick selects only specified fields from each item.
func (m *MultiSource) Pick(fields ...string) *MultiSource {
	m.ops = append(m.ops, &pickOp{fields: fields})
	return m
}

// Dedupe removes duplicate items by a key field.
func (m *MultiSource) Dedupe(keyField string) *MultiSource {
	m.ops = append(m.ops, &dedupeOp{keyField: keyField})
	return m
}

// MapBySource applies different field mappings based on the source field name.
// This is useful for volumeMounts where pvc, configMap, secret each have different output formats.
//
// Usage:
//
//	FromFields(volumeMounts, "pvc", "configMap", "secret", "emptyDir", "hostPath").
//	    MapBySource(map[string]FieldMap{
//	        "pvc": {"name": FieldRef("name"), "persistentVolumeClaim": Nested(FieldMap{"claimName": FieldRef("claimName")})},
//	        "configMap": {"name": FieldRef("name"), "configMap": Nested(FieldMap{"name": FieldRef("cmName"), "defaultMode": FieldRef("defaultMode")})},
//	        ...
//	    }).
//	    Dedupe("name")
func (m *MultiSource) MapBySource(mappings map[string]FieldMap) *MultiSource {
	m.mapBySource = mappings
	return m
}

// Source returns the source value.
func (m *MultiSource) Source() Value { return m.source }

// Sources returns the source field names.
func (m *MultiSource) Sources() []string { return m.sources }

// Operations returns the operations.
func (m *MultiSource) Operations() []collectionOperation { return m.ops }

// MapBySourceMappings returns the per-source-type field mappings.
func (m *MultiSource) MapBySourceMappings() map[string]FieldMap { return m.mapBySource }

// --- Go 1.23 Iterator Methods for MultiSource (iter.Seq) ---

// All returns an iterator over all items from all sources after applying operations.
// Items are collected from each source field and transformed according to MapBySource mappings.
// Example: for item := range ms.All(sourceData) { ... }
func (m *MultiSource) All(sourceData map[string]any) iter.Seq[map[string]any] {
	return func(yield func(map[string]any) bool) {
		// Collect items from all sources
		var allItems []any
		for _, sourceName := range m.sources {
			if sourceItems, ok := sourceData[sourceName]; ok {
				if arr, ok := sourceItems.([]map[string]any); ok {
					// Apply per-source mapping if defined
					if mapping, hasMapping := m.mapBySource[sourceName]; hasMapping {
						for _, item := range arr {
							transformed := applyFieldMap(item, mapping)
							allItems = append(allItems, transformed)
						}
					} else {
						for _, item := range arr {
							allItems = append(allItems, item)
						}
					}
				} else if arr, ok := sourceItems.([]any); ok {
					// Apply per-source mapping if defined
					if mapping, hasMapping := m.mapBySource[sourceName]; hasMapping {
						for _, item := range arr {
							if itemMap, ok := item.(map[string]any); ok {
								transformed := applyFieldMap(itemMap, mapping)
								allItems = append(allItems, transformed)
							}
						}
					} else {
						allItems = append(allItems, arr...)
					}
				}
			}
		}

		// Apply operations
		result := allItems
		for _, op := range m.ops {
			result = op.apply(result)
		}

		// Yield each item
		for _, item := range result {
			if itemMap, ok := item.(map[string]any); ok {
				if !yield(itemMap) {
					return
				}
			}
		}
	}
}

// AllPairs returns a key-value iterator with index and item from all sources.
// Example: for i, item := range ms.AllPairs(sourceData) { ... }
func (m *MultiSource) AllPairs(sourceData map[string]any) iter.Seq2[int, map[string]any] {
	return func(yield func(int, map[string]any) bool) {
		i := 0
		for item := range m.All(sourceData) {
			if !yield(i, item) {
				return
			}
			i++
		}
	}
}

// Collect materializes the iterator into a slice.
// Uses Go 1.23 slices.Collect for efficient collection.
func (m *MultiSource) Collect(sourceData map[string]any) []map[string]any {
	return slices.Collect(m.All(sourceData))
}

// Count returns the number of items from all sources after applying operations.
func (m *MultiSource) Count(sourceData map[string]any) int {
	count := 0
	for range m.All(sourceData) {
		count++
	}
	return count
}

type dedupeOp struct {
	keyField string
}

func (d *dedupeOp) apply(items []any) []any {
	seen := make(map[any]bool)
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			key := m[d.keyField]
			if !seen[key] {
				seen[key] = true
				result = append(result, item)
			}
		}
	}
	return result
}

//lint:ignore U1000 planned for future use
type transformByTypeOp struct {
	transforms map[string]FieldMap
}

//lint:ignore U1000 planned for future use
func (t *transformByTypeOp) apply(items []any) []any {
	// This is applied during resolution, not here
	return items
}

// --- Nested Object Field Mapping ---

// NestedField creates a nested object with the given field mappings.
// Usage: Map(FieldMap{"persistentVolumeClaim": Nested(FieldMap{"claimName": FieldRef("claimName")})})
type NestedField struct {
	mapping FieldMap
}

func (n *NestedField) resolve(item map[string]any) any {
	result := make(map[string]any)
	for k, v := range n.mapping {
		resolved := v.resolve(item)
		if resolved != nil {
			result[k] = resolved
		}
	}
	return result
}

// Nested creates a nested object from field mappings.
// Usage: "persistentVolumeClaim": Nested(FieldMap{"claimName": FieldRef("claimName")})
func Nested(mapping FieldMap) *NestedField {
	return &NestedField{mapping: mapping}
}

// --- Optional Field Value ---

// OptionalField includes a field value only if it exists and is not nil.
type OptionalField struct {
	field string
}

func (o *OptionalField) resolve(item map[string]any) any {
	val, exists := item[o.field]
	if !exists || val == nil {
		return nil
	}
	return val
}

// Optional creates a field reference that returns nil if the field doesn't exist.
// This is useful for optional fields that should only appear in output if present.
func Optional(field string) *OptionalField {
	return &OptionalField{field: field}
}

// OptionalFieldRef is an alias for Optional, providing a clearer name for the pattern.
// Usage: defkit.OptionalFieldRef("subPath")
func OptionalFieldRef(field string) *OptionalField {
	return &OptionalField{field: field}
}

// CompoundOptionalField includes a field value only if the field exists AND an additional condition is met.
// Generates CUE: if v.field != _|_ if additionalCond { fieldName: v.field }
type CompoundOptionalField struct {
	field          string
	additionalCond Condition
}

func (o *CompoundOptionalField) resolve(item map[string]any) any {
	val, exists := item[o.field]
	if !exists || val == nil {
		return nil
	}
	return val
}

// OptionalFieldWithCond creates a field reference that includes the field only when
// both the field exists and the additional condition is satisfied.
// Generates CUE: if v.field != _|_ if cond { fieldName: v.field }
func OptionalFieldWithCond(field string, cond Condition) *CompoundOptionalField {
	return &CompoundOptionalField{field: field, additionalCond: cond}
}

// NestedFieldMap creates a nested object from field mappings.
// This is an alias for Nested, providing a clearer name for struct array helpers.
// Usage: defkit.NestedFieldMap(defkit.FieldMap{"claimName": defkit.FieldRef("claimName")})
func NestedFieldMap(mapping FieldMap) *NestedField {
	return &NestedField{mapping: mapping}
}

// ConcatExprValue represents a concatenation expression for array helpers.
// This is used for generating: mountsArray.pvc + mountsArray.configMap + ...
type ConcatExprValue struct {
	source *StructArrayHelper
	fields []string
}

func (c *ConcatExprValue) value() {}
func (c *ConcatExprValue) expr()  {}

// Source returns the source struct array helper.
func (c *ConcatExprValue) Source() *StructArrayHelper { return c.source }

// Fields returns the field names to concatenate.
func (c *ConcatExprValue) Fields() []string { return c.fields }

// ConcatExpr creates a concatenation expression from a struct array helper.
// This generates CUE like: mountsArray.pvc + mountsArray.configMap + mountsArray.secret + ...
// Used when setting volumeMounts on containers.
//
// Usage:
//
//	SetIf(volumeMounts.IsSet(), "spec.containers[0].volumeMounts",
//	    defkit.ConcatExpr(mountsArray, "pvc", "configMap", "secret", "emptyDir", "hostPath"))
func ConcatExpr(source *StructArrayHelper, fields ...string) *ConcatExprValue {
	return &ConcatExprValue{
		source: source,
		fields: fields,
	}
}
