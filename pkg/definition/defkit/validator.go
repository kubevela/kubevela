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

// Validator represents a CUE _validate* block pattern used for cross-field validation.
// Validators emit blocks like:
//
//	_validateTenantName: {
//	    "tenantName must not end with a hyphen": true
//	    if tenantName =~ ".*-$" {
//	        "tenantName must not end with a hyphen": false
//	    }
//	}
//
// Validators can be guarded (only active when a condition is true) and can be attached
// at different levels: top-level parameter, inside map/struct params, or inside array elements.
type Validator struct {
	message   string    // the validation message (used as CUE field key)
	failCond  Condition // when this condition is true, validation fails
	guardCond Condition // optional: validator only active when guard is true
	name      string    // optional: override the CUE variable name (default: derived from message)
}

// Validate creates a new Validator with the given error message.
// The message is used both as the CUE field key and as the error shown on failure.
func Validate(message string) *Validator {
	return &Validator{message: message}
}

// FailWhen sets the condition that causes validation to fail.
// When this condition evaluates to true, the validator emits false for the message key.
func (v *Validator) FailWhen(cond Condition) *Validator {
	v.failCond = cond
	return v
}

// OnlyWhen sets a guard condition — the validator is only active when this condition is true.
// If the guard condition is false, the entire validator block is skipped.
func (v *Validator) OnlyWhen(guard Condition) *Validator {
	v.guardCond = guard
	return v
}

// WithName overrides the CUE variable name for the validator block.
// By default, the name is derived from the message (e.g., "_validateTenantName").
func (v *Validator) WithName(name string) *Validator {
	v.name = name
	return v
}

// Message returns the validation message.
func (v *Validator) Message() string { return v.message }

// FailCondition returns the fail condition.
func (v *Validator) FailCondition() Condition { return v.failCond }

// GuardCondition returns the guard condition, or nil if not set.
func (v *Validator) GuardCondition() Condition { return v.guardCond }

// CUEName returns the CUE variable name for this validator.
func (v *Validator) CUEName() string { return v.name }

// ScopedField creates a reference to a sibling field within a struct, without the "parameter." prefix.
// This is used inside validators attached to MapParam or ArrayParam where
// fields are siblings, not top-level parameters.
//
// Example: ScopedField("tenantName") generates just "tenantName" in CUE
func ScopedField(name string) *ScopedFieldRef {
	return &ScopedFieldRef{fieldName: name}
}

// ScopedFieldRef represents a reference to a field within the current scope (no parameter. prefix).
// It provides condition builder methods like Matches(), Eq(), IsSet() etc.
type ScopedFieldRef struct {
	fieldName string
}

// Name returns the field name.
func (s *ScopedFieldRef) Name() string { return s.fieldName }

// Matches creates a condition that checks if this field matches a regex pattern.
// Example: ScopedField("tenantName").Matches(".*-$") generates: tenantName =~ ".*-$"
func (s *ScopedFieldRef) Matches(pattern string) Condition {
	return &ScopedFieldMatchCondition{fieldName: s.fieldName, pattern: pattern}
}

// Eq creates a condition comparing this field to a value.
// Example: ScopedField("type").Eq("aws") generates: type == "aws"
func (s *ScopedFieldRef) Eq(val any) Condition {
	return &ScopedFieldCompareCondition{fieldName: s.fieldName, op: "==", value: val}
}

// Ne creates a condition checking this field is not equal to a value.
func (s *ScopedFieldRef) Ne(val any) Condition {
	return &ScopedFieldCompareCondition{fieldName: s.fieldName, op: "!=", value: val}
}

// IsSet creates a condition that checks if this field has a value (not bottom).
// Example: ScopedField("role").IsSet() generates: role != _|_
func (s *ScopedFieldRef) IsSet() Condition {
	return &ScopedFieldIsSetCondition{fieldName: s.fieldName}
}

// NotSet creates a condition that checks if this field is not set (is bottom).
// Example: ScopedField("role").NotSet() generates: role == _|_
func (s *ScopedFieldRef) NotSet() Condition {
	return &NotCondition{inner: &ScopedFieldIsSetCondition{fieldName: s.fieldName}}
}

// LenEq creates a condition that checks if this field's length equals n.
// Example: ScopedField("Principal.AWS").LenEq(0) generates: len(Principal.AWS) == 0
func (s *ScopedFieldRef) LenEq(n int) Condition {
	return &ScopedFieldLenCondition{fieldName: s.fieldName, op: "==", value: n}
}

// LenGt creates a condition that checks if this field's length is greater than n.
func (s *ScopedFieldRef) LenGt(n int) Condition {
	return &ScopedFieldLenCondition{fieldName: s.fieldName, op: ">", value: n}
}

// IsEmpty creates a condition that checks if this field has length 0.
func (s *ScopedFieldRef) IsEmpty() Condition {
	return s.LenEq(0)
}

// Gte creates a condition comparing this field >= another scoped field.
// Example: ScopedField("days").Gte(ScopedField("expiration[0].days"))
// generates: days >= expiration[0].days
func (s *ScopedFieldRef) Gte(other *ScopedFieldRef) Condition {
	return &ScopedFieldFieldCompareCondition{fieldName: s.fieldName, op: ">=", otherField: other.fieldName}
}

// ScopedFieldLenCondition checks the length of a scoped field.
type ScopedFieldLenCondition struct {
	baseCondition
	fieldName string
	op        string
	value     int
}

// FieldName returns the field name.
func (c *ScopedFieldLenCondition) FieldName() string { return c.fieldName }

// Op returns the comparison operator.
func (c *ScopedFieldLenCondition) Op() string { return c.op }

// LenValue returns the length comparison value.
func (c *ScopedFieldLenCondition) LenValue() int { return c.value }

// ScopedFieldFieldCompareCondition compares two scoped fields.
type ScopedFieldFieldCompareCondition struct {
	baseCondition
	fieldName  string
	op         string
	otherField string
}

// FieldName returns the left field name.
func (c *ScopedFieldFieldCompareCondition) FieldName() string { return c.fieldName }

// Op returns the comparison operator.
func (c *ScopedFieldFieldCompareCondition) Op() string { return c.op }

// OtherField returns the right field name.
func (c *ScopedFieldFieldCompareCondition) OtherField() string { return c.otherField }

// ScopedFieldMatchCondition checks if a scoped field matches a regex.
type ScopedFieldMatchCondition struct {
	baseCondition
	fieldName string
	pattern   string
}

// FieldName returns the field name.
func (c *ScopedFieldMatchCondition) FieldName() string { return c.fieldName }

// Pattern returns the regex pattern.
func (c *ScopedFieldMatchCondition) Pattern() string { return c.pattern }

// ScopedFieldCompareCondition compares a scoped field to a value.
type ScopedFieldCompareCondition struct {
	baseCondition
	fieldName string
	op        string
	value     any
}

// FieldName returns the field name.
func (c *ScopedFieldCompareCondition) FieldName() string { return c.fieldName }

// Op returns the comparison operator.
func (c *ScopedFieldCompareCondition) Op() string { return c.op }

// CompareValue returns the comparison value.
func (c *ScopedFieldCompareCondition) CompareValue() any { return c.value }

// ScopedFieldIsSetCondition checks if a scoped field is set.
type ScopedFieldIsSetCondition struct {
	baseCondition
	fieldName string
}

// FieldName returns the field name.
func (c *ScopedFieldIsSetCondition) FieldName() string { return c.fieldName }

// LenOf wraps a Value expression and provides comparison methods that produce Conditions.
// It emits len(<value>) in CUE.
//
// Example: LenOf(Plus(Lit("tenant-"), Reference("parameter.governance.tenantName"), Lit("-"), name)).Gt(63)
// generates: len("tenant-" + parameter.governance.tenantName + "-" + parameter.name) > 63
type LenOfExpr struct {
	inner Value
}

// LenOf creates a length expression wrapper around any Value.
func LenOf(v Value) *LenOfExpr {
	return &LenOfExpr{inner: v}
}

// Gt creates a condition: len(inner) > n
func (l *LenOfExpr) Gt(n int) Condition {
	return &LenOfCondition{inner: l.inner, op: ">", value: n}
}

// Gte creates a condition: len(inner) >= n
func (l *LenOfExpr) Gte(n int) Condition {
	return &LenOfCondition{inner: l.inner, op: ">=", value: n}
}

// Eq creates a condition: len(inner) == n
func (l *LenOfExpr) Eq(n int) Condition {
	return &LenOfCondition{inner: l.inner, op: "==", value: n}
}

// Inner returns the wrapped value.
func (l *LenOfExpr) Inner() Value { return l.inner }

// LenOfCondition checks the length of a Value expression against an integer.
type LenOfCondition struct {
	baseCondition
	inner Value
	op    string
	value int
}

// InnerValue returns the Value whose length is being checked.
func (c *LenOfCondition) InnerValue() Value { return c.inner }

// Op returns the comparison operator.
func (c *LenOfCondition) Op() string { return c.op }

// CompareValue returns the integer comparison value.
func (c *LenOfCondition) CompareValue() int { return c.value }

// TimeParse creates a Value representing time.Parse(layout, expr) in CUE.
// Used for date comparison validators.
//
// Example: TimeParse("2006-01-02T15:04:05Z", ScopedField("date"))
// generates: time.Parse("2006-01-02T15:04:05Z", date)
func TimeParse(layout string, field *ScopedFieldRef) *TimeParseExpr {
	return &TimeParseExpr{layout: layout, fieldName: field.fieldName}
}

// TimeParseExpr represents a time.Parse() call in CUE.
type TimeParseExpr struct {
	layout    string
	fieldName string
}

func (t *TimeParseExpr) expr()  {}
func (t *TimeParseExpr) value() {}

// Layout returns the time layout string.
func (t *TimeParseExpr) Layout() string { return t.layout }

// FieldName returns the field name passed to time.Parse.
func (t *TimeParseExpr) FieldName() string { return t.fieldName }

// Gte creates a condition: time.Parse(layout, fieldA) >= time.Parse(layout, fieldB)
func (t *TimeParseExpr) Gte(other *TimeParseExpr) Condition {
	return &TimeParseCompareCondition{left: t, right: other, op: ">="}
}

// TimeParseCompareCondition compares two time.Parse expressions.
type TimeParseCompareCondition struct {
	baseCondition
	left  *TimeParseExpr
	right *TimeParseExpr
	op    string
}

// Left returns the left-hand time.Parse expression.
func (c *TimeParseCompareCondition) Left() *TimeParseExpr { return c.left }

// Right returns the right-hand time.Parse expression.
func (c *TimeParseCompareCondition) Right() *TimeParseExpr { return c.right }

// Op returns the comparison operator.
func (c *TimeParseCompareCondition) Op() string { return c.op }

// RawCUECondition wraps a raw CUE expression string as a Condition.
// Use this for expressions too complex to model with the fluent API.
//
// Example: CUEExpr(`len("tenant-"+parameter.governance.tenantName+"-"+name) > 63`)
type RawCUECondition struct {
	baseCondition
	rawExpr string
}

// CUEExpr creates a condition from a raw CUE expression string.
// The expression is emitted verbatim in the generated CUE.
func CUEExpr(rawExpr string) *RawCUECondition {
	return &RawCUECondition{rawExpr: rawExpr}
}

// Expr returns the raw CUE expression.
func (c *RawCUECondition) Expr() string { return c.rawExpr }
