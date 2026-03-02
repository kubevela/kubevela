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

// AndCondition represents a binary logical AND of two conditions.
// This is an internal IR type used by cuegen to combine conditions during code generation.
// For the user-facing API, use And() which accepts variadic conditions via LogicalExpr.
type AndCondition struct {
	baseCondition
	left  Condition
	right Condition
}

// OrCondition represents a binary logical OR of two conditions.
// This is an internal IR type used by cuegen to combine conditions during code generation.
// For the user-facing API, use Or() which accepts variadic conditions via LogicalExpr.
type OrCondition struct {
	baseCondition
	left  Condition
	right Condition
}

// NotCondition represents a logical NOT of a condition.
// Used both internally (e.g., by ParamNotSet) and as part of the user-facing Not() API.
type NotCondition struct {
	baseCondition
	inner Condition
}

// Inner returns the negated condition.
func (n *NotCondition) Inner() Condition { return n.inner }

// --- Parameter Runtime Condition Types ---

// FalsyCondition represents a falsy check on a boolean parameter.
// In CUE, this generates `if !parameter.name`.
type FalsyCondition struct {
	baseCondition
	paramName string
}

// ParamName returns the parameter name being checked.
func (f *FalsyCondition) ParamName() string { return f.paramName }

// InCondition represents a check if a parameter value is in a set of values.
// Generates: parameter.name == val1 || parameter.name == val2 || ...
type InCondition struct {
	baseCondition
	paramName string
	values    []any
}

// ParamName returns the parameter name being checked.
func (c *InCondition) ParamName() string { return c.paramName }

// Values returns the set of values to check against.
func (c *InCondition) Values() []any { return c.values }

// StringContainsCondition checks if a string parameter contains a substring.
// Generates: strings.Contains(parameter.name, "substr")
type StringContainsCondition struct {
	baseCondition
	paramName string
	substr    string
}

// ParamName returns the parameter name being checked.
func (c *StringContainsCondition) ParamName() string { return c.paramName }

// Substr returns the substring to check for.
func (c *StringContainsCondition) Substr() string { return c.substr }

// StringMatchesCondition checks if a string parameter matches a regex pattern.
// Generates: parameter.name =~ "pattern"
type StringMatchesCondition struct {
	baseCondition
	paramName string
	pattern   string
}

// ParamName returns the parameter name being checked.
func (c *StringMatchesCondition) ParamName() string { return c.paramName }

// Pattern returns the regex pattern to match against.
func (c *StringMatchesCondition) Pattern() string { return c.pattern }

// StringStartsWithCondition checks if a string parameter starts with a prefix.
// Generates: strings.HasPrefix(parameter.name, "prefix")
type StringStartsWithCondition struct {
	baseCondition
	paramName string
	prefix    string
}

// ParamName returns the parameter name being checked.
func (c *StringStartsWithCondition) ParamName() string { return c.paramName }

// Prefix returns the prefix to check for.
func (c *StringStartsWithCondition) Prefix() string { return c.prefix }

// StringEndsWithCondition checks if a string parameter ends with a suffix.
// Generates: strings.HasSuffix(parameter.name, "suffix")
type StringEndsWithCondition struct {
	baseCondition
	paramName string
	suffix    string
}

// ParamName returns the parameter name being checked.
func (c *StringEndsWithCondition) ParamName() string { return c.paramName }

// Suffix returns the suffix to check for.
func (c *StringEndsWithCondition) Suffix() string { return c.suffix }

// LenCondition checks the length of a parameter (string, array, or map).
// Generates: len(parameter.name) op n
type LenCondition struct {
	baseCondition
	paramName string
	op        string // ==, !=, <, <=, >, >=
	length    int
}

// ParamName returns the parameter name being checked.
func (c *LenCondition) ParamName() string { return c.paramName }

// Op returns the comparison operator.
func (c *LenCondition) Op() string { return c.op }

// Length returns the length to compare against.
func (c *LenCondition) Length() int { return c.length }

// ArrayContainsCondition checks if an array parameter contains a specific value.
// Generates: list.Contains(parameter.name, value)
type ArrayContainsCondition struct {
	baseCondition
	paramName string
	value     any
}

// ParamName returns the parameter name being checked.
func (c *ArrayContainsCondition) ParamName() string { return c.paramName }

// Value returns the value to check for.
func (c *ArrayContainsCondition) Value() any { return c.value }

// MapHasKeyCondition checks if a map parameter has a specific key.
// Generates: parameter.name.key != _|_
type MapHasKeyCondition struct {
	baseCondition
	paramName string
	key       string
}

// ParamName returns the parameter name being checked.
func (c *MapHasKeyCondition) ParamName() string { return c.paramName }

// Key returns the key to check for.
func (c *MapHasKeyCondition) Key() string { return c.key }

// --- CUE Stdlib Function Wrappers ---
// These functions generate CUE expressions that call standard library functions.

// CUEFunc represents a call to a CUE standard library function.
type CUEFunc struct {
	pkg  string // e.g., "strconv", "strings", "list"
	fn   string // e.g., "FormatInt", "ToLower"
	args []Value
}

func (c *CUEFunc) expr()  {}
func (c *CUEFunc) value() {}

// RequiredImports returns the CUE imports required by this function call.
func (c *CUEFunc) RequiredImports() []string {
	if c.pkg != "" {
		return []string{c.pkg}
	}
	return nil
}

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

// SpreadAllOp represents a patch operation that constrains all array elements.
// This generates: path: [...{element}]
// Used for applying the same patch to every element in an array (e.g., all containers).
type SpreadAllOp struct {
	path     string
	elements []Value
}

func (s *SpreadAllOp) resourceOp() {}

// Path returns the path being patched.
func (s *SpreadAllOp) Path() string { return s.path }

// Elements returns the array elements to constrain.
func (s *SpreadAllOp) Elements() []Value { return s.elements }

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

// patchKeyField represents a field within an ArrayElement that has a patchKey annotation.
// This is used for nested patchKey annotations inside array elements,
// e.g., volumeMounts inside a containers element.
type patchKeyField struct {
	field string // field name (e.g., "volumeMounts")
	key   string // patchKey value (e.g., "name")
	value Value  // the array value
}

// ArrayElement represents a single element in an array patch.
// Used for building array values with struct elements.
type ArrayElement struct {
	fields         map[string]Value
	ops            []ResourceOp    // nested operations for complex structs
	patchKeyFields []patchKeyField // nested patchKey-annotated array fields
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

// PatchKeyField adds a patchKey-annotated array field to the array element.
// This generates within the element:
//
//	// +patchKey=key
//	field: value
//
// Used for nested patchKey annotations inside array elements, e.g.,
// volumeMounts with patchKey=name inside a containers element.
func (a *ArrayElement) PatchKeyField(field string, key string, value Value) *ArrayElement {
	a.patchKeyFields = append(a.patchKeyFields, patchKeyField{field: field, key: key, value: value})
	return a
}

// Fields returns all fields set on this element.
func (a *ArrayElement) Fields() map[string]Value { return a.fields }

// Ops returns any conditional operations.
func (a *ArrayElement) Ops() []ResourceOp { return a.ops }

// PatchKeyFields returns the patchKey-annotated fields.
func (a *ArrayElement) PatchKeyFields() []patchKeyField { return a.patchKeyFields }

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
	source  string       // The source to iterate over (e.g., "parameter")
	keyVar  string       // Variable name for key (e.g., "k")
	valVar  string       // Variable name for value (e.g., "v")
	keyExpr string       // Expression for key output (empty means "(keyVar)")
	valExpr string       // Expression for value output (empty means valVar)
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

// --- CUE String Interpolation ---

// InterpolatedString represents a CUE string interpolation expression.
// Literal string parts are inlined, Value parts are wrapped in \(...).
//
// Example:
//
//	Interpolation(vela.Namespace(), Lit(":"), name)
//	// Generates: "\(context.namespace):\(parameter.name)"
type InterpolatedString struct {
	parts []Value
}

func (i *InterpolatedString) value() {}
func (i *InterpolatedString) expr()  {}

// Parts returns the interpolation parts.
func (i *InterpolatedString) Parts() []Value { return i.parts }

// Interpolation creates a CUE string interpolation expression.
// Literal string values are inlined directly. All other values are
// wrapped in \(...) interpolation syntax.
func Interpolation(parts ...Value) *InterpolatedString {
	return &InterpolatedString{parts: parts}
}

// --- LenValueCondition ---

// LenValueCondition checks the length of an arbitrary Value (not just a parameter).
// This extends LenCondition to work with let variables and other expressions.
// Generates: len(source) op n
type LenValueCondition struct {
	baseCondition
	source Value
	op     string // ==, !=, <, <=, >, >=
	length int
}

// Source returns the source value being measured.
func (c *LenValueCondition) Source() Value { return c.source }

// Op returns the comparison operator.
func (c *LenValueCondition) Op() string { return c.op }

// Length returns the length to compare against.
func (c *LenValueCondition) Length() int { return c.length }

// LenGt creates a condition: len(source) > n.
func LenGt(source Value, n int) *LenValueCondition {
	return &LenValueCondition{source: source, op: ">", length: n}
}

// LenGe creates a condition: len(source) >= n.
func LenGe(source Value, n int) *LenValueCondition {
	return &LenValueCondition{source: source, op: ">=", length: n}
}

// LenEq creates a condition: len(source) == n.
func LenEq(source Value, n int) *LenValueCondition {
	return &LenValueCondition{source: source, op: "==", length: n}
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

// PlusExpr represents a + operator between multiple values.
// Generates CUE: a + b + c
// Works for string concatenation, array concatenation, etc.
type PlusExpr struct {
	parts []Value
}

func (p *PlusExpr) value() {}
func (p *PlusExpr) expr()  {}

// Parts returns the operands.
func (p *PlusExpr) Parts() []Value { return p.parts }

// Plus creates a + expression between values.
// Generates CUE: parts[0] + parts[1] + ...
func Plus(parts ...Value) *PlusExpr {
	return &PlusExpr{parts: parts}
}

// IterFieldRef references a field on the iteration variable.
// Generates CUE: v.fieldName (where v is the iteration variable).
type IterFieldRef struct {
	varName string
	field   string
}

func (r *IterFieldRef) value() {}
func (r *IterFieldRef) expr()  {}

// VarName returns the iteration variable name.
func (r *IterFieldRef) VarName() string { return r.varName }

// FieldName returns the field name.
func (r *IterFieldRef) FieldName() string { return r.field }

// IterVarRef references the iteration variable itself (not a field on it).
// Generates CUE: v (where v is the iteration variable).
type IterVarRef struct {
	varName string
}

func (r *IterVarRef) value() {}
func (r *IterVarRef) expr()  {}

// VarName returns the iteration variable name.
func (r *IterVarRef) VarName() string { return r.varName }

// IterLetRef references a let binding defined inside an iteration body.
// Generates CUE: _name (a private CUE identifier).
type IterLetRef struct {
	name string
}

func (r *IterLetRef) value() {}
func (r *IterLetRef) expr()  {}

// RefName returns the binding name.
func (r *IterLetRef) RefName() string { return r.name }

// IterFieldExistsCondition checks if an iteration variable field exists.
// Generates CUE: v.field != _|_ (or v.field == _|_ when negated).
type IterFieldExistsCondition struct {
	baseCondition
	varName string
	field   string
	negate  bool
}

// VarName returns the iteration variable name.
func (c *IterFieldExistsCondition) VarName() string { return c.varName }

// FieldName returns the field name.
func (c *IterFieldExistsCondition) FieldName() string { return c.field }

// IsNegated returns true if this is a "not exists" check.
func (c *IterFieldExistsCondition) IsNegated() bool { return c.negate }

// InlineArrayValue represents an inline array literal containing struct elements.
// This generates CUE like: [{field1: value1, field2: value2}]
// Used for deprecated parameter fallbacks that create a single-element array.
type InlineArrayValue struct {
	fields map[string]Value
}

func (a *InlineArrayValue) expr()  {}
func (a *InlineArrayValue) value() {}

// Fields returns the field mappings.
func (a *InlineArrayValue) Fields() map[string]Value { return a.fields }

// InlineArray creates an inline array value with a single struct element.
// Example: InlineArray(map[string]Value{"containerPort": port})
// Generates: [{containerPort: parameter.port}]
func InlineArray(fields map[string]Value) *InlineArrayValue {
	return &InlineArrayValue{fields: fields}
}
