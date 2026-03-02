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

import "fmt"

// HelperVar is a type-safe reference to a template-level helper definition.
// When used in a Resource.Set() call, the CUE generator will emit a reference
// to the helper name rather than inlining the collection.
type HelperVar struct {
	name        string
	collection  Value     // The collection operation (CollectionOp or MultiSource)
	template    *Template // Back-reference to parent template
	afterOutput bool      // If true, helper appears after output: block (used for expose helpers)
	guard       Condition // Outer guard condition for list comprehension
}

// Ensure HelperVar implements Value interface
var _ Value = (*HelperVar)(nil)

func (h *HelperVar) value() {}
func (h *HelperVar) expr()  {}

// Name returns the helper name.
func (h *HelperVar) Name() string { return h.name }

// Collection returns the underlying collection for inspection and CUE generation.
func (h *HelperVar) Collection() Value { return h.collection }

// IsAfterOutput returns true if this helper should be placed after the output: block.
func (h *HelperVar) IsAfterOutput() bool { return h.afterOutput }

// Guard returns the outer guard condition, if any.
func (h *HelperVar) Guard() Condition { return h.guard }

// String implements Stringer for debugging.
func (h *HelperVar) String() string {
	return fmt.Sprintf("Helper(%s)", h.name)
}

// NotEmpty returns a condition that checks if len(helper) != 0.
// This is used for conditional outputs like Service which should only
// be created when there are exposed ports.
//
// Example:
//
//	exposePorts := tpl.Helper("exposePorts").
//	    From(ports).
//	    Filter(...).
//	    Build()
//	tpl.OutputsIf(exposePorts.NotEmpty(), "service", svc)
func (h *HelperVar) NotEmpty() Condition {
	return &LenNotZeroCondition{source: h}
}

// LenNotZeroCondition checks if len(value) != 0.
type LenNotZeroCondition struct {
	baseCondition
	source Value
}

// Source returns the value being checked for non-zero length.
func (l *LenNotZeroCondition) Source() Value { return l.source }

// HelperBuilder provides a fluent API for building template helpers.
type HelperBuilder struct {
	name        string
	template    *Template
	source      helperSource
	ops         []helperOperation
	afterOutput bool      // If true, helper appears after output: block
	guard       Condition // Outer guard condition (if param != _|_ for v in ...)
}

// helperSource represents the source for a helper (single or multi-source).
type helperSource interface {
	isHelperSource()
}

// singleSource wraps a single Value source.
type singleSource struct {
	source Value
}

func (s *singleSource) isHelperSource() {}

// multiSourceDef represents multiple named fields from a source.
type multiSourceDef struct {
	source Value
	fields []string
}

func (m *multiSourceDef) isHelperSource() {}

// helperOperation represents an operation to apply in the builder.
type helperOperation interface {
	isHelperOperation()
}

// eachOp transforms each element with a function.
type eachOp struct {
	fn func(Value) Value
}

func (e *eachOp) isHelperOperation() {}

// helperPickIfOp conditionally picks a field.
type helperPickIfOp struct {
	cond  Condition
	field string
}

func (p *helperPickIfOp) isHelperOperation() {}

// mapBySourceOp applies different mappings per source type.
type mapBySourceOp struct {
	mappings map[string]FieldMap
}

func (m *mapBySourceOp) isHelperOperation() {}

// filterCondOp filters items by a condition.
type filterCondOp struct {
	cond Condition
}

func (f *filterCondOp) isHelperOperation() {}

// helperPickOp wraps pickOp for the builder.
type helperPickOp struct {
	fields []string
}

func (h *helperPickOp) isHelperOperation() {}

// helperMapOp wraps mapOp for the builder.
type helperMapOp struct {
	mappings FieldMap
}

func (h *helperMapOp) isHelperOperation() {}

// helperWrapOp wraps wrapOp for the builder.
type helperWrapOp struct {
	key string
}

func (h *helperWrapOp) isHelperOperation() {}

// helperDedupeOp wraps dedupeOp for the builder.
type helperDedupeOp struct {
	keyField string
}

func (h *helperDedupeOp) isHelperOperation() {}

// helperDefaultFieldOp wraps defaultFieldOp for the builder.
type helperDefaultFieldOp struct {
	field      string
	defaultVal FieldValue
}

func (h *helperDefaultFieldOp) isHelperOperation() {}

// helperRenameOp wraps renameOp for the builder.
type helperRenameOp struct {
	from, to string
}

func (h *helperRenameOp) isHelperOperation() {}

// helperFilterOp wraps filterOp for the builder.
type helperFilterOp struct {
	pred Predicate
}

func (h *helperFilterOp) isHelperOperation() {}

// Helper starts building a named helper.
// The returned HelperBuilder provides a fluent API for defining the helper's
// source and operations.
//
// Example:
//
//	mounts := tpl.Helper("mountsArray").
//	    FromFields(Param("volumeMounts"), "pvc", "configMap", "secret").
//	    Pick("name", "mountPath").
//	    PickIf(IsSet(Field("subPath")), "subPath").
//	    Build()
func (t *Template) Helper(name string) *HelperBuilder {
	return &HelperBuilder{
		name:     name,
		template: t,
		ops:      make([]helperOperation, 0),
	}
}

// Guard sets an outer condition that wraps the for comprehension.
// This generates `if condition for v in source` pattern in CUE.
//
// Example:
//
//	exposePorts := tpl.Helper("exposePorts").
//	    From(ports).
//	    Guard(ports.IsSet()). // generates: if parameter.ports != _|_ for v in ...
//	    FilterPred(FieldEquals("expose", true)).
//	    Build()
func (hb *HelperBuilder) Guard(cond Condition) *HelperBuilder {
	hb.guard = cond
	return hb
}

// From sets a single source for the helper.
//
// Example:
//
//	ports := tpl.Helper("portsArray").
//	    From(Param("ports")).
//	    Pick("port", "name", "protocol").
//	    Build()
func (hb *HelperBuilder) From(source Value) *HelperBuilder {
	hb.source = &singleSource{source: source}
	return hb
}

// FromFields sets multiple named fields as the source.
// This is used for patterns like volumeMounts where items come from
// multiple sub-fields (pvc, configMap, secret, emptyDir, hostPath).
//
// Example:
//
//	mounts := tpl.Helper("mountsArray").
//	    FromFields(Param("volumeMounts"), "pvc", "configMap", "secret", "emptyDir", "hostPath").
//	    Pick("name", "mountPath").
//	    Build()
func (hb *HelperBuilder) FromFields(source Value, fields ...string) *HelperBuilder {
	hb.source = &multiSourceDef{source: source, fields: fields}
	return hb
}

// FromArray uses a pre-built ArrayBuilder as the helper source.
// This enables complex iteration patterns (ForEachWithGuardedFiltered)
// that can't be expressed through the standard From/Filter/Map pipeline.
func (hb *HelperBuilder) FromArray(ab *ArrayBuilder) *HelperBuilder {
	hb.source = &arrayBuilderSource{builder: ab}
	return hb
}

// FromHelper references another helper as the source.
// This enables helper chaining for patterns like deduplication.
//
// Example:
//
//	dedupedVolumes := tpl.Helper("deDupVolumesArray").
//	    FromHelper(volumesList).
//	    Dedupe("name").
//	    Build()
func (hb *HelperBuilder) FromHelper(helper *HelperVar) *HelperBuilder {
	hb.source = &helperRefSource{helper: helper}
	return hb
}

// helperRefSource references another helper.
type helperRefSource struct {
	helper *HelperVar
}

func (h *helperRefSource) isHelperSource() {}

// arrayBuilderSource wraps an ArrayBuilder as a helper source.
type arrayBuilderSource struct {
	builder *ArrayBuilder
}

func (a *arrayBuilderSource) isHelperSource() {}

// Each applies a transformation function to each element.
// The function receives a Value representing the current item and returns
// a transformed Value.
//
// Example:
//
//	mounts := tpl.Helper("mountsArray").
//	    FromFields(Param("volumeMounts"), "pvc", "configMap").
//	    Each(func(v Value) Value {
//	        return Struct(
//	            Field("name", v.Get("name")),
//	            Field("mountPath", v.Get("mountPath")),
//	        )
//	    }).
//	    Build()
func (hb *HelperBuilder) Each(fn func(Value) Value) *HelperBuilder {
	hb.ops = append(hb.ops, &eachOp{fn: fn})
	return hb
}

// Pick selects only the specified fields from each element.
//
// Example:
//
//	.Pick("name", "mountPath", "subPath")
func (hb *HelperBuilder) Pick(fields ...string) *HelperBuilder {
	hb.ops = append(hb.ops, &helperPickOp{fields: fields})
	return hb
}

// PickIf conditionally includes a field if the condition is true.
//
// Example:
//
//	.PickIf(IsSet(Field("subPath")), "subPath")
func (hb *HelperBuilder) PickIf(cond Condition, field string) *HelperBuilder {
	hb.ops = append(hb.ops, &helperPickIfOp{cond: cond, field: field})
	return hb
}

// MapBySource applies different field mappings based on the source field name.
// This is essential for volumeMounts where pvc, configMap, secret each have
// different output structures.
//
// Example:
//
//	volumes := tpl.Helper("volumesList").
//	    FromFields(Param("volumeMounts"), "pvc", "configMap", "secret", "emptyDir", "hostPath").
//	    MapBySource(map[string]FieldMap{
//	        "pvc":       {"name": FieldRef("name"), "persistentVolumeClaim.claimName": FieldRef("claimName")},
//	        "configMap": {"name": FieldRef("name"), "configMap.name": FieldRef("cmName")},
//	        "secret":    {"name": FieldRef("name"), "secret.secretName": FieldRef("secretName")},
//	        "emptyDir":  {"name": FieldRef("name"), "emptyDir.medium": FieldRef("medium")},
//	        "hostPath":  {"name": FieldRef("name"), "hostPath.path": FieldRef("path")},
//	    }).
//	    Build()
func (hb *HelperBuilder) MapBySource(mappings map[string]FieldMap) *HelperBuilder {
	hb.ops = append(hb.ops, &mapBySourceOp{mappings: mappings})
	return hb
}

// Map transforms each element using the given field mappings.
//
// Example:
//
//	.Map(FieldMap{"containerPort": FieldRef("port"), "name": FieldRef("name")})
func (hb *HelperBuilder) Map(mappings FieldMap) *HelperBuilder {
	hb.ops = append(hb.ops, &helperMapOp{mappings: mappings})
	return hb
}

// Filter keeps only items matching the condition.
//
// Example:
//
//	exposedPorts := tpl.Helper("exposedPorts").
//	    From(Param("ports")).
//	    Filter(Eq(Field("expose"), true)).
//	    Build()
func (hb *HelperBuilder) Filter(cond Condition) *HelperBuilder {
	hb.ops = append(hb.ops, &filterCondOp{cond: cond})
	return hb
}

// FilterPred keeps only items matching the predicate.
func (hb *HelperBuilder) FilterPred(pred Predicate) *HelperBuilder {
	hb.ops = append(hb.ops, &helperFilterOp{pred: pred})
	return hb
}

// Wrap wraps each item value under a new key.
//
// Example:
//
//	// Transforms ["secret1", "secret2"] to [{name: "secret1"}, {name: "secret2"}]
//	.Wrap("name")
func (hb *HelperBuilder) Wrap(key string) *HelperBuilder {
	hb.ops = append(hb.ops, &helperWrapOp{key: key})
	return hb
}

// Dedupe removes duplicate items by a key field.
//
// Example:
//
//	.Dedupe("name")
func (hb *HelperBuilder) Dedupe(keyField string) *HelperBuilder {
	hb.ops = append(hb.ops, &helperDedupeOp{keyField: keyField})
	return hb
}

// DefaultField sets a default value for a field if not present.
//
// Example:
//
//	.DefaultField("name", Format("port-%d", FieldRef("port")))
func (hb *HelperBuilder) DefaultField(field string, defaultVal FieldValue) *HelperBuilder {
	hb.ops = append(hb.ops, &helperDefaultFieldOp{field: field, defaultVal: defaultVal})
	return hb
}

// Rename renames a field in each item.
//
// Example:
//
//	.Rename("port", "containerPort")
func (hb *HelperBuilder) Rename(from, to string) *HelperBuilder {
	hb.ops = append(hb.ops, &helperRenameOp{from: from, to: to})
	return hb
}

// AfterOutput marks this helper to appear after the output: block in generated CUE.
// Use this for helpers that are primarily used by outputs: (auxiliary resources)
// rather than the main output: resource.
//
// In KubeVela CUE definitions, the structure is typically:
//
//	template: {
//	    mountsArray: [...]   // Primary helpers
//	    volumesList: [...]
//	    output: {...}         // Main resource
//	    exposePorts: [...]    // Auxiliary helpers (AfterOutput)
//	    outputs: {...}        // Auxiliary resources
//	    parameter: {...}
//	}
//
// Example:
//
//	exposePorts := tpl.Helper("exposePorts").
//	    From(ports).
//	    FilterPred(FieldEquals("expose", true)).
//	    Map(...).
//	    AfterOutput().  // Place after output:
//	    Build()
func (hb *HelperBuilder) AfterOutput() *HelperBuilder {
	hb.afterOutput = true
	return hb
}

// Build finalizes the helper and registers it with the template.
// Returns a type-safe HelperVar that can be used in Resource.Set() calls.
func (hb *HelperBuilder) Build() *HelperVar {
	// Build the collection operation from builder state
	collection := hb.buildCollection()

	// Create typed reference
	helper := &HelperVar{
		name:        hb.name,
		collection:  collection,
		template:    hb.template,
		afterOutput: hb.afterOutput,
		guard:       hb.guard,
	}

	// Register with template
	hb.template.registerHelper(helper)

	return helper
}

// buildCollection converts builder state to the appropriate collection type.
func (hb *HelperBuilder) buildCollection() Value {
	switch src := hb.source.(type) {
	case *multiSourceDef:
		// Create MultiSource
		ms := FromFields(src.source, src.fields...)
		hb.applyOpsToMultiSource(ms)
		return ms

	case *singleSource:
		// Create CollectionOp
		col := Each(src.source)
		hb.applyOpsToCollection(col)
		return col

	case *helperRefSource:
		// Create CollectionOp from helper reference
		col := Each(src.helper)
		hb.applyOpsToCollection(col)
		return col

	case *arrayBuilderSource:
		return src.builder

	default:
		// Default: empty collection
		return Each(Lit([]any{}))
	}
}

// applyOpsToMultiSource applies builder operations to a MultiSource.
func (hb *HelperBuilder) applyOpsToMultiSource(ms *MultiSource) {
	for _, op := range hb.ops {
		switch o := op.(type) {
		case *helperPickOp:
			ms.Pick(o.fields...)
		case *helperDedupeOp:
			ms.Dedupe(o.keyField)
		case *mapBySourceOp:
			ms.MapBySource(o.mappings)
		case *helperPickIfOp:
			// pickIf is handled specially in CUE generation
			ms.ops = append(ms.ops, &pickIfCollectionOp{cond: o.cond, field: o.field})
		}
	}
}

// applyOpsToCollection applies builder operations to a CollectionOp.
func (hb *HelperBuilder) applyOpsToCollection(col *CollectionOp) {
	for _, op := range hb.ops {
		switch o := op.(type) {
		case *helperPickOp:
			col.Pick(o.fields...)
		case *helperMapOp:
			col.Map(o.mappings)
		case *helperWrapOp:
			col.Wrap(o.key)
		case *helperDedupeOp:
			col.ops = append(col.ops, &dedupeOp{keyField: o.keyField})
		case *helperDefaultFieldOp:
			col.DefaultField(o.field, o.defaultVal)
		case *helperRenameOp:
			col.Rename(o.from, o.to)
		case *helperFilterOp:
			col.Filter(o.pred)
		case *helperPickIfOp:
			// pickIf is handled specially in CUE generation
			col.ops = append(col.ops, &pickIfCollectionOp{cond: o.cond, field: o.field})
		case *eachOp:
			// Store the transform function for CUE generation
			col.ops = append(col.ops, &eachTransformOp{fn: o.fn})
		}
	}
}

// pickIfCollectionOp represents a conditional field pick in a collection.
type pickIfCollectionOp struct {
	cond  Condition
	field string
}

func (p *pickIfCollectionOp) apply(items []any) []any {
	// Runtime behavior: conditionally add field
	result := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			newItem := make(map[string]any)
			for k, v := range m {
				newItem[k] = v
			}
			// Check if field exists and condition would be met
			if _, exists := m[p.field]; exists {
				newItem[p.field] = m[p.field]
			}
			result = append(result, newItem)
		}
	}
	return result
}

// Cond returns the condition for this pickIf operation.
func (p *pickIfCollectionOp) Cond() Condition { return p.cond }

// Field returns the field name for this pickIf operation.
func (p *pickIfCollectionOp) Field() string { return p.field }

// eachTransformOp stores a transform function for CUE generation.
type eachTransformOp struct {
	fn func(Value) Value
}

func (e *eachTransformOp) apply(items []any) []any {
	// Transform function is applied during CUE generation, not runtime
	return items
}

// TransformFn returns the transform function.
func (e *eachTransformOp) TransformFn() func(Value) Value { return e.fn }

// --- Struct builder for Each transforms ---

// StructBuilder builds a struct value from fields.
type StructBuilder struct {
	fields []structField
}

type structField struct {
	name  string
	value Value
	cond  Condition // nil means unconditional
}

// HelperStruct creates a new struct builder with the given fields.
//
// Example:
//
//	HelperStruct(
//	    HelperField("name", v.Get("name")),
//	    HelperField("mountPath", v.Get("mountPath")),
//	    HelperFieldIf(IsSet(v.Get("subPath")), "subPath", v.Get("subPath")),
//	)
func HelperStruct(fields ...StructFieldDef) *StructBuilder {
	sb := &StructBuilder{fields: make([]structField, 0, len(fields))}
	for _, f := range fields {
		sb.fields = append(sb.fields, structField(f))
	}
	return sb
}

func (s *StructBuilder) value() {}
func (s *StructBuilder) expr()  {}

// Fields returns the struct fields for CUE generation.
func (s *StructBuilder) Fields() []StructFieldDef {
	result := make([]StructFieldDef, len(s.fields))
	for i, f := range s.fields {
		result[i] = StructFieldDef(f)
	}
	return result
}

// StructFieldDef defines a field in a struct.
type StructFieldDef struct {
	name  string
	value Value
	cond  Condition
}

// Name returns the field name.
func (f StructFieldDef) Name() string { return f.name }

// Value returns the field value.
func (f StructFieldDef) Value() Value { return f.value }

// Cond returns the condition (nil if unconditional).
func (f StructFieldDef) Cond() Condition { return f.cond }

// HelperField creates an unconditional struct field.
func HelperField(name string, value Value) StructFieldDef {
	return StructFieldDef{name: name, value: value}
}

// HelperFieldIf creates a conditional struct field.
func HelperFieldIf(cond Condition, name string, value Value) StructFieldDef {
	return StructFieldDef{name: name, value: value, cond: cond}
}

// --- ItemValue for referencing fields in Each transforms ---

// ItemValue represents a reference to the current item in an Each transform.
type ItemValue struct {
	field string // empty means the whole item, non-empty means a field
}

func (i *ItemValue) value() {}
func (i *ItemValue) expr()  {}

// Get returns a reference to a field of this item.
func (i *ItemValue) Get(field string) *ItemValue {
	return &ItemValue{field: field}
}

// Field returns the field name being accessed (empty for whole item).
func (i *ItemValue) Field() string { return i.field }

// Item returns a value representing the current item in an Each transform.
func Item() *ItemValue {
	return &ItemValue{}
}

// ItemFieldIsSet returns a condition that checks if a field is set in the current iteration item.
// This generates CUE: v.fieldName != _|_
// Used with PickIf for conditionally including fields.
//
// Example:
//
//	.Pick("name", "mountPath").
//	.PickIf(ItemFieldIsSet("subPath"), "subPath")
func ItemFieldIsSet(field string) Condition {
	return &IsSetCondition{paramName: field}
}

// StructArrayHelper represents a struct-based helper where each field is an array
// with a default empty value pattern: pvc: *[...] | []
// This pattern is used in cron-task and other components for mountsArray/volumesArray.
type StructArrayHelper struct {
	name     string
	source   Value              // e.g., parameter.volumeMounts
	fields   []StructArrayField // each field definition
	template *Template
}

// StructArrayField defines a field in a struct array helper.
type StructArrayField struct {
	Name     string   // field name (e.g., "pvc", "configMap")
	Mappings FieldMap // how to map input fields to output
}

// Ensure StructArrayHelper implements Value interface
var _ Value = (*StructArrayHelper)(nil)

func (s *StructArrayHelper) value() {}
func (s *StructArrayHelper) expr()  {}

// HelperName returns the helper name.
func (s *StructArrayHelper) HelperName() string { return s.name }

// Source returns the source parameter.
func (s *StructArrayHelper) Source() Value { return s.source }

// Fields returns all field definitions.
func (s *StructArrayHelper) Fields() []StructArrayField { return s.fields }

// StructArrayBuilder provides a fluent API for building struct-based array helpers.
type StructArrayBuilder struct {
	name     string
	source   Value
	fields   []StructArrayField
	template *Template
}

// StructArrayHelper starts building a struct-based array helper.
// This creates helpers like mountsArray or volumesArray where each source type
// is a separate field in a struct.
//
// Example:
//
//	mountsArray := tpl.StructArrayHelper("mountsArray", Param("volumeMounts")).
//	    Field("pvc", FieldMap{"name": FieldRef("name"), "mountPath": FieldRef("mountPath")}).
//	    Field("configMap", FieldMap{"name": FieldRef("name"), "mountPath": FieldRef("mountPath")}).
//	    Build()
func (t *Template) StructArrayHelper(name string, source Value) *StructArrayBuilder {
	return &StructArrayBuilder{
		name:     name,
		source:   source,
		fields:   make([]StructArrayField, 0),
		template: t,
	}
}

// Field adds a field to the struct array helper.
func (b *StructArrayBuilder) Field(name string, mappings FieldMap) *StructArrayBuilder {
	b.fields = append(b.fields, StructArrayField{
		Name:     name,
		Mappings: mappings,
	})
	return b
}

// Build finalizes the struct array helper and registers it with the template.
func (b *StructArrayBuilder) Build() *StructArrayHelper {
	helper := &StructArrayHelper{
		name:     b.name,
		source:   b.source,
		fields:   b.fields,
		template: b.template,
	}

	// Register as a special helper type
	b.template.registerStructArrayHelper(helper)

	return helper
}

// ConcatHelper represents a list.Concat helper that combines arrays from a struct.
type ConcatHelper struct {
	name      string
	source    *StructArrayHelper // the source struct helper
	fieldRefs []string           // fields to concat
	template  *Template
}

// Ensure ConcatHelper implements Value interface
var _ Value = (*ConcatHelper)(nil)

func (c *ConcatHelper) value() {}
func (c *ConcatHelper) expr()  {}

// HelperName returns the helper name.
func (c *ConcatHelper) HelperName() string { return c.name }

// Source returns the source struct helper.
func (c *ConcatHelper) Source() *StructArrayHelper { return c.source }

// FieldRefs returns the field references to concatenate.
func (c *ConcatHelper) FieldRefs() []string { return c.fieldRefs }

// RequiredImports returns the CUE imports required by ConcatHelper.
// ConcatHelper uses list.Concat which requires the "list" import.
func (c *ConcatHelper) RequiredImports() []string {
	return []string{"list"}
}

// ConcatHelperBuilder provides a fluent API for building list.Concat helpers.
type ConcatHelperBuilder struct {
	name      string
	source    *StructArrayHelper
	fieldRefs []string
	template  *Template
}

// ConcatHelper starts building a list.Concat helper.
// This creates helpers like volumesList that concatenate arrays from a struct helper.
//
// Example:
//
//	volumesList := tpl.ConcatHelper("volumesList", volumesArray).
//	    Fields("pvc", "configMap", "secret", "emptyDir", "hostPath").
//	    Build()
func (t *Template) ConcatHelper(name string, source *StructArrayHelper) *ConcatHelperBuilder {
	return &ConcatHelperBuilder{
		name:     name,
		source:   source,
		template: t,
	}
}

// Fields adds field references to concatenate.
func (b *ConcatHelperBuilder) Fields(fields ...string) *ConcatHelperBuilder {
	b.fieldRefs = append(b.fieldRefs, fields...)
	return b
}

// Build finalizes the concat helper and registers it with the template.
func (b *ConcatHelperBuilder) Build() *ConcatHelper {
	helper := &ConcatHelper{
		name:      b.name,
		source:    b.source,
		fieldRefs: b.fieldRefs,
		template:  b.template,
	}

	// Register as a special helper type
	b.template.registerConcatHelper(helper)

	return helper
}

// DedupeHelper represents a deduplication helper that removes duplicates by key.
type DedupeHelper struct {
	name     string
	source   Value  // source to dedupe (typically a ConcatHelper or HelperVar)
	keyField string // field to dedupe by (e.g., "name")
	template *Template
}

// Ensure DedupeHelper implements Value interface
var _ Value = (*DedupeHelper)(nil)

func (d *DedupeHelper) value() {}
func (d *DedupeHelper) expr()  {}

// HelperName returns the helper name.
func (d *DedupeHelper) HelperName() string { return d.name }

// Source returns the source to deduplicate.
func (d *DedupeHelper) Source() Value { return d.source }

// KeyField returns the field to dedupe by.
func (d *DedupeHelper) KeyField() string { return d.keyField }

// DedupeHelperBuilder provides a fluent API for building deduplication helpers.
type DedupeHelperBuilder struct {
	name     string
	source   Value
	keyField string
	template *Template
}

// DedupeHelper starts building a deduplication helper.
// This creates helpers like deDupVolumesArray that remove duplicates by a key field.
//
// Example:
//
//	deDupVolumes := tpl.DedupeHelper("deDupVolumesArray", volumesList).
//	    ByKey("name").
//	    Build()
func (t *Template) DedupeHelper(name string, source Value) *DedupeHelperBuilder {
	return &DedupeHelperBuilder{
		name:     name,
		source:   source,
		template: t,
	}
}

// ByKey sets the key field to deduplicate by.
func (b *DedupeHelperBuilder) ByKey(field string) *DedupeHelperBuilder {
	b.keyField = field
	return b
}

// Build finalizes the dedupe helper and registers it with the template.
func (b *DedupeHelperBuilder) Build() *DedupeHelper {
	helper := &DedupeHelper{
		name:     b.name,
		source:   b.source,
		keyField: b.keyField,
		template: b.template,
	}

	// Register with template
	b.template.registerDedupeHelper(helper)

	return helper
}
