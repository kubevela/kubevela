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

// entryKind describes what kind of array entry this is.
type entryKind int

const (
	entryStatic      entryKind = iota // always-present item
	entryConditional                  // conditional item (if cond { item })
	entryForEach                      // iterated item (for v in source { item })
)

// arrayEntry is a single entry in an ArrayBuilder.
type arrayEntry struct {
	kind        entryKind
	element     *ArrayElement // the item fields (for static, conditional, forEach)
	cond        Condition     // for conditional entries
	source      Value         // for forEach entries (iteration source)
	guard       Condition     // for forEach entries (optional guard: if source != _|_)
	filter      Predicate     // for forEach entries (optional filter: if v.field == value)
	itemBuilder *ItemBuilder  // for forEachWith entries (complex per-item logic)
}

// ArrayBuilder builds CUE arrays with static items, conditional items, and for-each items.
// This is the core type for building arrays where some items are always present,
// some are conditional, and some come from iteration.
//
// Example:
//
//	NewArray().
//	    Item(cpuMetric).
//	    ItemIf(mem.IsSet(), memMetric).
//	    ForEachGuarded(podCustomMetrics.IsSet(), podCustomMetrics, customMetric)
type ArrayBuilder struct {
	entries []arrayEntry
}

func (a *ArrayBuilder) value() {}
func (a *ArrayBuilder) expr()  {}

// NewArray creates a new ArrayBuilder.
func NewArray() *ArrayBuilder {
	return &ArrayBuilder{
		entries: make([]arrayEntry, 0),
	}
}

// Item adds an always-present item to the array.
func (a *ArrayBuilder) Item(elem *ArrayElement) *ArrayBuilder {
	a.entries = append(a.entries, arrayEntry{
		kind:    entryStatic,
		element: elem,
	})
	return a
}

// ItemIf adds a conditional item to the array.
// The item is only included when the condition is true.
func (a *ArrayBuilder) ItemIf(cond Condition, elem *ArrayElement) *ArrayBuilder {
	a.entries = append(a.entries, arrayEntry{
		kind:    entryConditional,
		element: elem,
		cond:    cond,
	})
	return a
}

// ForEach adds an iterated item to the array.
// For each element in source, an item is created using the element template.
// In the element template, use Reference("m.field") to reference iteration variable fields.
func (a *ArrayBuilder) ForEach(source Value, elem *ArrayElement) *ArrayBuilder {
	a.entries = append(a.entries, arrayEntry{
		kind:    entryForEach,
		element: elem,
		source:  source,
	})
	return a
}

// ForEachGuarded adds a guarded iterated item to the array.
// The guard condition (typically source.IsSet()) wraps the for loop.
// Generates: if guard for m in source { item }
func (a *ArrayBuilder) ForEachGuarded(guard Condition, source Value, elem *ArrayElement) *ArrayBuilder {
	a.entries = append(a.entries, arrayEntry{
		kind:    entryForEach,
		element: elem,
		source:  source,
		guard:   guard,
	})
	return a
}

// Entries returns all entries in the array builder.
func (a *ArrayBuilder) Entries() []arrayEntry { return a.entries }

// entryForEachWith indicates a complex iterated item using an ItemBuilder.
const entryForEachWith entryKind = 3

// ForEachWith adds a complex iterated item to the array, using an ItemBuilder
// callback for per-item operations like conditionals, let bindings, and defaults.
// Uses "v" as the default iteration variable name.
func (a *ArrayBuilder) ForEachWith(source Value, fn func(item *ItemBuilder)) *ArrayBuilder {
	return a.ForEachWithVar("v", source, fn)
}

// ForEachWithVar is like ForEachWith but allows specifying the iteration variable name.
func (a *ArrayBuilder) ForEachWithVar(varName string, source Value, fn func(item *ItemBuilder)) *ArrayBuilder {
	ib := &ItemBuilder{varName: varName, ops: make([]itemOp, 0)}
	fn(ib)
	a.entries = append(a.entries, arrayEntry{
		kind:        entryForEachWith,
		source:      source,
		itemBuilder: ib,
	})
	return a
}

// ForEachWithGuardedFiltered adds a guarded and filtered complex iterated item to the array.
// The guard condition wraps the for loop, and the filter predicate filters iteration items.
// Generates: if guard for v in source if filter { ... }
func (a *ArrayBuilder) ForEachWithGuardedFiltered(guard Condition, filter Predicate, source Value, fn func(item *ItemBuilder)) *ArrayBuilder {
	return a.ForEachWithGuardedFilteredVar("v", guard, filter, source, fn)
}

// ForEachWithGuardedFilteredVar is like ForEachWithGuardedFiltered but allows specifying the iteration variable name.
func (a *ArrayBuilder) ForEachWithGuardedFilteredVar(varName string, guard Condition, filter Predicate, source Value, fn func(item *ItemBuilder)) *ArrayBuilder {
	ib := &ItemBuilder{varName: varName, ops: make([]itemOp, 0)}
	fn(ib)
	a.entries = append(a.entries, arrayEntry{
		kind:        entryForEachWith,
		source:      source,
		guard:       guard,
		filter:      filter,
		itemBuilder: ib,
	})
	return a
}

// --- ItemBuilder ---

// itemOp is a single operation recorded by the ItemBuilder.
type itemOp interface {
	isItemOp()
}

// setOp records an unconditional field assignment.
type setOp struct {
	field string
	value Value
}

func (setOp) isItemOp() {}

// ifBlockOp records a conditional block of nested operations.
type ifBlockOp struct {
	cond Condition
	body []itemOp
}

func (ifBlockOp) isItemOp() {}

// letOp records a private field binding (_name: value).
type letOp struct {
	name  string
	value Value
}

func (letOp) isItemOp() {}

// setDefaultOp records a CUE default value: field: *defValue | typeName.
type setDefaultOp struct {
	field    string
	defValue Value
	typeName string
}

func (setDefaultOp) isItemOp() {}

// ItemBuilder records per-item operations for complex ForEach iterations.
// It supports field assignment, conditionals, let bindings, and CUE default values.
type ItemBuilder struct {
	varName string
	ops     []itemOp
}

// Var returns a reference builder for the iteration variable.
// Use v.Field("port") to reference v.port in CUE.
func (b *ItemBuilder) Var() *IterVarBuilder {
	return &IterVarBuilder{varName: b.varName}
}

// Set records an unconditional field assignment.
func (b *ItemBuilder) Set(field string, value Value) {
	b.ops = append(b.ops, setOp{field: field, value: value})
}

// If records a conditional block of operations.
func (b *ItemBuilder) If(cond Condition, fn func()) {
	outer := b.ops
	b.ops = make([]itemOp, 0)
	fn()
	inner := b.ops
	b.ops = outer
	b.ops = append(b.ops, ifBlockOp{cond: cond, body: inner})
}

// IfSet records a conditional block that executes when the iteration variable's field is set.
// Generates CUE: if v.field != _|_ { ... }
func (b *ItemBuilder) IfSet(field string, fn func()) {
	cond := &IterFieldExistsCondition{varName: b.varName, field: field}
	b.If(cond, fn)
}

// IfNotSet records a conditional block that executes when the iteration variable's field is NOT set.
// Generates CUE: if v.field == _|_ { ... }
func (b *ItemBuilder) IfNotSet(field string, fn func()) {
	cond := &IterFieldExistsCondition{varName: b.varName, field: field, negate: true}
	b.If(cond, fn)
}

// Let records a private field binding and returns a reference to it.
// Generates CUE: _name: value
func (b *ItemBuilder) Let(name string, value Value) Value {
	b.ops = append(b.ops, letOp{name: name, value: value})
	return &IterLetRef{name: name}
}

// SetDefault records a CUE default value assignment.
// Generates CUE: field: *defValue | typeName
func (b *ItemBuilder) SetDefault(field string, defValue Value, typeName string) {
	b.ops = append(b.ops, setDefaultOp{field: field, defValue: defValue, typeName: typeName})
}

// FieldExists returns a Condition that checks if the iteration variable's field is set.
// Generates CUE: v.field != _|_
func (b *ItemBuilder) FieldExists(field string) Condition {
	return &IterFieldExistsCondition{varName: b.varName, field: field}
}

// FieldNotExists returns a Condition that checks if the iteration variable's field is NOT set.
// Generates CUE: v.field == _|_
func (b *ItemBuilder) FieldNotExists(field string) Condition {
	return &IterFieldExistsCondition{varName: b.varName, field: field, negate: true}
}

// Ops returns the recorded operations.
func (b *ItemBuilder) Ops() []itemOp { return b.ops }

// VarName returns the iteration variable name.
func (b *ItemBuilder) VarName() string { return b.varName }

// IterVarBuilder provides access to iteration variable fields.
type IterVarBuilder struct {
	varName string
}

// Ref returns a Value referencing the iteration variable itself.
// Generates CUE: v (or whatever the variable name is).
// Useful when iterating over primitive arrays (e.g., [...int]).
func (v *IterVarBuilder) Ref() *IterVarRef {
	return &IterVarRef{varName: v.varName}
}

// Field returns a Value referencing the iteration variable's field.
// v.Field("port") generates CUE: v.port
func (v *IterVarBuilder) Field(name string) *IterFieldRef {
	return &IterFieldRef{varName: v.varName, field: name}
}

// ArrayConcatValue represents an array concatenation: left + right.
// Used for expressions like [items] + parameter.extraVolumeMounts.
type ArrayConcatValue struct {
	left  Value
	right Value
}

func (a *ArrayConcatValue) value() {}
func (a *ArrayConcatValue) expr()  {}

// Left returns the left operand.
func (a *ArrayConcatValue) Left() Value { return a.left }

// Right returns the right operand.
func (a *ArrayConcatValue) Right() Value { return a.right }

// ArrayConcat creates an array concatenation expression.
// Generates CUE: left + right
func ArrayConcat(left, right Value) *ArrayConcatValue {
	return &ArrayConcatValue{left: left, right: right}
}
