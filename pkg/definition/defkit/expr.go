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

// Literal represents a literal value in an expression.
type Literal struct {
	val any
}

func (l *Literal) expr()  {}
func (l *Literal) value() {}

// Val returns the underlying value.
func (l *Literal) Val() any { return l.val }

// Lit creates a literal value from any Go value.
func Lit(v any) *Literal {
	return &Literal{val: v}
}

// CompOp represents a comparison operator.
type CompOp string

const (
	// OpEq represents equality (==)
	OpEq CompOp = "=="
	// OpNe represents inequality (!=)
	OpNe CompOp = "!="
	// OpLt represents less than (<)
	OpLt CompOp = "<"
	// OpLe represents less than or equal (<=)
	OpLe CompOp = "<="
	// OpGt represents greater than (>)
	OpGt CompOp = ">"
	// OpGe represents greater than or equal (>=)
	OpGe CompOp = ">="
)

// Comparison represents a comparison between two expressions.
type Comparison struct {
	baseCondition
	left  Expr
	op    CompOp
	right Expr
}

// Left returns the left-hand side expression.
func (c *Comparison) Left() Expr { return c.left }

// Op returns the comparison operator.
func (c *Comparison) Op() CompOp { return c.op }

// Right returns the right-hand side expression.
func (c *Comparison) Right() Expr { return c.right }

// Eq creates an equality comparison.
func Eq(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpEq, right: right}
}

// Ne creates an inequality comparison.
func Ne(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpNe, right: right}
}

// Lt creates a less-than comparison.
func Lt(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpLt, right: right}
}

// Le creates a less-than-or-equal comparison.
func Le(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpLe, right: right}
}

// Gt creates a greater-than comparison.
func Gt(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpGt, right: right}
}

// Ge creates a greater-than-or-equal comparison.
func Ge(left, right Expr) *Comparison {
	return &Comparison{left: left, op: OpGe, right: right}
}

// LogicalOp represents a logical operator.
type LogicalOp string

const (
	// OpAnd represents logical AND (&&)
	OpAnd LogicalOp = "&&"
	// OpOr represents logical OR (||)
	OpOr LogicalOp = "||"
)

// LogicalExpr represents a logical combination of conditions.
type LogicalExpr struct {
	baseCondition
	op         LogicalOp
	conditions []Condition
}

// Op returns the logical operator.
func (l *LogicalExpr) Op() LogicalOp { return l.op }

// Conditions returns the list of combined conditions.
func (l *LogicalExpr) Conditions() []Condition { return l.conditions }

// And creates a logical AND of multiple conditions.
func And(conditions ...Condition) *LogicalExpr {
	return &LogicalExpr{
		op:         OpAnd,
		conditions: conditions,
	}
}

// Or creates a logical OR of multiple conditions.
func Or(conditions ...Condition) *LogicalExpr {
	return &LogicalExpr{
		op:         OpOr,
		conditions: conditions,
	}
}

// NotExpr represents a logical negation of a condition.
type NotExpr struct {
	baseCondition
	cond Condition
}

// Cond returns the negated condition.
func (n *NotExpr) Cond() Condition { return n.cond }

// Not creates a logical NOT of a condition.
func Not(cond Condition) *NotExpr {
	return &NotExpr{cond: cond}
}

// IsSetCondition represents a check for whether a parameter is set.
type IsSetCondition struct {
	baseCondition
	paramName string
}

// ParamName returns the parameter name being checked.
func (i *IsSetCondition) ParamName() string { return i.paramName }

// TruthyCondition represents a truthy check on a boolean parameter.
// In CUE, this generates `if parameter.name` instead of `if parameter.name == true`.
type TruthyCondition struct {
	baseCondition
	paramName string
}

// ParamName returns the parameter name being checked.
func (t *TruthyCondition) ParamName() string { return t.paramName }

// CompareCondition represents a comparison between two values.
type CompareCondition struct {
	baseCondition
	left  any
	right any
	op    string
}

// Left returns the left operand.
func (c *CompareCondition) Left() any { return c.left }

// Right returns the right operand.
func (c *CompareCondition) Right() any { return c.right }

// Operator returns the comparison operator.
func (c *CompareCondition) Operator() string { return c.op }

// AndCondition represents a logical AND of two conditions.
type AndCondition struct {
	baseCondition
	left  Condition
	right Condition
}

// OrCondition represents a logical OR of two conditions.
type OrCondition struct {
	baseCondition
	left  Condition
	right Condition
}

// NotCondition represents a logical NOT of a condition.
type NotCondition struct {
	baseCondition
	inner Condition
}

// Inner returns the negated condition.
func (n *NotCondition) Inner() Condition { return n.inner }

// --- CUE Stdlib Function Wrappers ---
// These functions generate CUE expressions that call standard library functions.

// CUEFunc represents a call to a CUE standard library function.
type CUEFunc struct {
	pkg    string // e.g., "strconv", "strings", "list"
	fn     string // e.g., "FormatInt", "ToLower"
	args   []Value
}

func (c *CUEFunc) expr()  {}
func (c *CUEFunc) value() {}

// Package returns the CUE package name.
func (c *CUEFunc) Package() string { return c.pkg }

// Function returns the function name.
func (c *CUEFunc) Function() string { return c.fn }

// Args returns the function arguments.
func (c *CUEFunc) Args() []Value { return c.args }

// StrconvFormatInt creates a strconv.FormatInt(v, base) expression.
// In CUE: strconv.FormatInt(v, 10)
func StrconvFormatInt(v Value, base int) *CUEFunc {
	return &CUEFunc{
		pkg:  "strconv",
		fn:   "FormatInt",
		args: []Value{v, Lit(base)},
	}
}

// StringsToLower creates a strings.ToLower(v) expression.
// In CUE: strings.ToLower(v)
func StringsToLower(v Value) *CUEFunc {
	return &CUEFunc{
		pkg:  "strings",
		fn:   "ToLower",
		args: []Value{v},
	}
}

// StringsToUpper creates a strings.ToUpper(v) expression.
// In CUE: strings.ToUpper(v)
func StringsToUpper(v Value) *CUEFunc {
	return &CUEFunc{
		pkg:  "strings",
		fn:   "ToUpper",
		args: []Value{v},
	}
}

// StringsHasPrefix creates a strings.HasPrefix(s, prefix) expression.
// In CUE: strings.HasPrefix(s, prefix)
func StringsHasPrefix(s Value, prefix string) *CUEFunc {
	return &CUEFunc{
		pkg:  "strings",
		fn:   "HasPrefix",
		args: []Value{s, Lit(prefix)},
	}
}

// StringsHasSuffix creates a strings.HasSuffix(s, suffix) expression.
// In CUE: strings.HasSuffix(s, suffix)
func StringsHasSuffix(s Value, suffix string) *CUEFunc {
	return &CUEFunc{
		pkg:  "strings",
		fn:   "HasSuffix",
		args: []Value{s, Lit(suffix)},
	}
}

// ListConcat creates a list.Concat(lists...) expression.
// In CUE: list.Concat([list1, list2, ...])
func ListConcat(lists ...Value) *CUEFunc {
	return &CUEFunc{
		pkg:  "list",
		fn:   "Concat",
		args: lists,
	}
}

// --- Patch Key Annotation Support ---

// PatchKeyOp represents a patch operation with a // +patchKey=name annotation.
// This is used for array merging strategies in Kubernetes strategic merge patch.
type PatchKeyOp struct {
	path     string
	key      string // e.g., "name" for containers
	elements []Value
}

func (p *PatchKeyOp) resourceOp() {}

// Path returns the path being patched.
func (p *PatchKeyOp) Path() string { return p.path }

// Key returns the patch key for array merging.
func (p *PatchKeyOp) Key() string { return p.key }

// Elements returns the array elements to patch.
func (p *PatchKeyOp) Elements() []Value { return p.elements }

// --- Context Path Exists Check ---

// PathExistsCondition checks if a path exists in CUE (path != _|_).
type PathExistsCondition struct {
	baseCondition
	path string
}

// Path returns the path being checked.
func (p *PathExistsCondition) Path() string { return p.path }

// PathExists creates a condition that checks if a path exists.
// In CUE: path != _|_
func PathExists(path string) *PathExistsCondition {
	return &PathExistsCondition{path: path}
}

// --- Array Element Struct ---

// ArrayElement represents a single element in an array patch.
// Used for building array values with struct elements.
type ArrayElement struct {
	fields map[string]Value
	ops    []ResourceOp // nested operations for complex structs
}

func (a *ArrayElement) expr()  {}
func (a *ArrayElement) value() {}

// NewArrayElement creates a new array element builder.
func NewArrayElement() *ArrayElement {
	return &ArrayElement{
		fields: make(map[string]Value),
		ops:    make([]ResourceOp, 0),
	}
}

// Set sets a field on the array element.
func (a *ArrayElement) Set(key string, value Value) *ArrayElement {
	a.fields[key] = value
	return a
}

// SetIf conditionally sets a field on the array element.
func (a *ArrayElement) SetIf(cond Condition, key string, value Value) *ArrayElement {
	a.ops = append(a.ops, &SetIfOp{path: key, value: value, cond: cond})
	return a
}

// Fields returns all fields set on this element.
func (a *ArrayElement) Fields() map[string]Value { return a.fields }

// Ops returns any conditional operations.
func (a *ArrayElement) Ops() []ResourceOp { return a.ops }

// --- Reference Expressions ---

// Ref creates a raw reference expression.
// Use this for referencing CUE variables like "v.protocol" or "context.output".
type Ref struct {
	path string
}

func (r *Ref) expr()  {}
func (r *Ref) value() {}

// Path returns the reference path.
func (r *Ref) Path() string { return r.path }

// Reference creates a raw reference to a CUE path.
// Example: Reference("v.protocol") for use in comprehensions
func Reference(path string) *Ref {
	return &Ref{path: path}
}

// --- Parameter Reference ---

// Parameter creates a reference to a parameter value.
// This generates "parameter" or "parameter.fieldName" in CUE.
func Parameter() *Ref {
	return &Ref{path: "parameter"}
}

// ParameterField creates a reference to a field within parameter.
// Example: ParameterField("replicas") generates "parameter.replicas"
func ParameterField(field string) *Ref {
	return &Ref{path: "parameter." + field}
}

// ParamRef creates a reference to a parameter field for use in expressions.
// This is more explicit than ParameterField and is intended for use with
// list comprehensions and other value expressions.
// Example: ParamRef("constraints") generates "parameter.constraints"
func ParamRef(field string) *Ref {
	return ParameterField(field)
}

// --- ForEach Map Iteration ---

// ForEachMapOp represents a for comprehension over a map.
// Generates: for k, v in source { (k): v } or custom expressions
type ForEachMapOp struct {
	source  string      // The source to iterate over (e.g., "parameter")
	keyVar  string      // Variable name for key (e.g., "k")
	valVar  string      // Variable name for value (e.g., "v")
	keyExpr string      // Expression for key output (empty means "(keyVar)")
	valExpr string      // Expression for value output (empty means valVar)
	body    []ResourceOp // Optional nested operations in the body
}

func (f *ForEachMapOp) resourceOp() {}
func (f *ForEachMapOp) expr()       {}
func (f *ForEachMapOp) value()      {}

// Source returns the iteration source.
func (f *ForEachMapOp) Source() string { return f.source }

// KeyVar returns the key variable name.
func (f *ForEachMapOp) KeyVar() string { return f.keyVar }

// ValVar returns the value variable name.
func (f *ForEachMapOp) ValVar() string { return f.valVar }

// KeyExpr returns the key expression.
func (f *ForEachMapOp) KeyExpr() string { return f.keyExpr }

// ValExpr returns the value expression.
func (f *ForEachMapOp) ValExpr() string { return f.valExpr }

// Body returns nested operations.
func (f *ForEachMapOp) Body() []ResourceOp { return f.body }

// ForEachMap creates a for comprehension over the parameter map.
// This generates: for k, v in parameter { (k): v }
func ForEachMap() *ForEachMapOp {
	return &ForEachMapOp{
		source: "parameter",
		keyVar: "k",
		valVar: "v",
	}
}

// Over specifies the source to iterate over.
func (f *ForEachMapOp) Over(source string) *ForEachMapOp {
	f.source = source
	return f
}

// WithVars specifies the key and value variable names.
func (f *ForEachMapOp) WithVars(keyVar, valVar string) *ForEachMapOp {
	f.keyVar = keyVar
	f.valVar = valVar
	return f
}

// WithKeyExpr specifies a custom key expression.
func (f *ForEachMapOp) WithKeyExpr(expr string) *ForEachMapOp {
	f.keyExpr = expr
	return f
}

// WithValExpr specifies a custom value expression.
func (f *ForEachMapOp) WithValExpr(expr string) *ForEachMapOp {
	f.valExpr = expr
	return f
}

// WithBody adds nested operations to the for body.
func (f *ForEachMapOp) WithBody(ops ...ResourceOp) *ForEachMapOp {
	f.body = append(f.body, ops...)
	return f
}
