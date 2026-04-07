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

// LocalField creates a reference to a field in the current CUE scope (no "parameter." prefix).
// Used inside validators for condition-building on sibling fields.
// The name is emitted verbatim — supports dot-paths ("Principal.AWS")
// and array indexing ("expiration[0].date").
//
// Compare with defkit.String("name") / defkit.Bool("name") which reference
// top-level parameters and emit "parameter.name" in CUE.
//
// Example: LocalField("tenantName").Matches(".*-$")
func LocalField(name string) *LocalFieldRef {
	return &LocalFieldRef{fieldName: name}
}

// LocalFieldRef represents a reference to a field within the current scope.
// It implements Value so it can be used with upstream Comparison, LenValueCondition, etc.
// It provides ergonomic condition builder methods: Matches(), Eq(), IsSet(), LenEq(), Gte(), etc.
type LocalFieldRef struct {
	fieldName string
}

// Value/Expr interface implementation — allows LocalFieldRef to be used
// with upstream types like Comparison, LenValueCondition, etc.
func (s *LocalFieldRef) expr()  {}
func (s *LocalFieldRef) value() {}

// Name returns the field name.
func (s *LocalFieldRef) Name() string { return s.fieldName }

// Matches creates a condition that checks if this field matches a regex pattern.
// Example: LocalField("tenantName").Matches(".*-$") generates: tenantName =~ ".*-$"
// Reuses upstream RegexMatchCondition.
func (s *LocalFieldRef) Matches(pattern string) Condition {
	return RegexMatch(s, pattern)
}

// Eq creates a condition comparing this field to a value.
// Example: LocalField("type").Eq("aws") generates: type == "aws"
// Reuses upstream Comparison type.
func (s *LocalFieldRef) Eq(val any) Condition {
	return Eq(s, Lit(val))
}

// Ne creates a condition checking this field is not equal to a value.
// Reuses upstream Comparison type.
func (s *LocalFieldRef) Ne(val any) Condition {
	return Ne(s, Lit(val))
}

// IsSet creates a condition that checks if this field has a value (not bottom).
// Example: LocalField("role").IsSet() generates: role != _|_
// Reuses upstream PathExistsCondition.
func (s *LocalFieldRef) IsSet() Condition {
	return PathExists(s.fieldName)
}

// NotSet creates a condition that checks if this field is not set (is bottom).
// Example: LocalField("role").NotSet() generates: role == _|_
func (s *LocalFieldRef) NotSet() Condition {
	return Not(PathExists(s.fieldName))
}

// LenEq creates a condition that checks if this field's length equals n.
// Example: LocalField("Principal.AWS").LenEq(0) generates: len(Principal.AWS) == 0
// Reuses upstream LenValueCondition via LenEq().
func (s *LocalFieldRef) LenEq(n int) Condition {
	return LenEq(s, n)
}

// LenGt creates a condition that checks if this field's length is greater than n.
// Reuses upstream LenValueCondition via LenGt().
func (s *LocalFieldRef) LenGt(n int) Condition {
	return LenGt(s, n)
}

// IsEmpty creates a condition that checks if this field has length 0.
func (s *LocalFieldRef) IsEmpty() Condition {
	return s.LenEq(0)
}

// Gte creates a condition comparing this field >= another local field.
// Example: LocalField("days").Gte(LocalField("expiration[0].days"))
// generates: days >= expiration[0].days
// Reuses upstream Comparison type via Ge().
func (s *LocalFieldRef) Gte(other *LocalFieldRef) Condition {
	return Ge(s, other)
}

// LenOf wraps a Value expression and provides comparison methods that produce Conditions.
// It emits len(<value>) in CUE. Reuses upstream LenValueCondition.
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

// Gt creates a condition: len(inner) > n. Returns upstream *LenValueCondition.
func (l *LenOfExpr) Gt(n int) Condition {
	return LenGt(l.inner, n)
}

// Gte creates a condition: len(inner) >= n. Returns upstream *LenValueCondition.
func (l *LenOfExpr) Gte(n int) Condition {
	return LenGe(l.inner, n)
}

// Eq creates a condition: len(inner) == n. Returns upstream *LenValueCondition.
func (l *LenOfExpr) Eq(n int) Condition {
	return LenEq(l.inner, n)
}

// TimeParse creates a Value representing time.Parse(layout, expr) in CUE.
// Used for date comparison validators.
//
// Example: TimeParse("2006-01-02T15:04:05Z", LocalField("date"))
// generates: time.Parse("2006-01-02T15:04:05Z", date)
func TimeParse(layout string, field *LocalFieldRef) *TimeParseExpr {
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
