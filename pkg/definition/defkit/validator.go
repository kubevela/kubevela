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
// Example: FieldRef("tenantName").Matches(".*-$") generates: tenantName =~ ".*-$"
func (s *ScopedFieldRef) Matches(pattern string) Condition {
	return &ScopedFieldMatchCondition{fieldName: s.fieldName, pattern: pattern}
}

// Eq creates a condition comparing this field to a value.
// Example: FieldRef("type").Eq("aws") generates: type == "aws"
func (s *ScopedFieldRef) Eq(val any) Condition {
	return &ScopedFieldCompareCondition{fieldName: s.fieldName, op: "==", value: val}
}

// Ne creates a condition checking this field is not equal to a value.
func (s *ScopedFieldRef) Ne(val any) Condition {
	return &ScopedFieldCompareCondition{fieldName: s.fieldName, op: "!=", value: val}
}

// IsSet creates a condition that checks if this field has a value (not bottom).
// Example: FieldRef("role").IsSet() generates: role != _|_
func (s *ScopedFieldRef) IsSet() Condition {
	return &ScopedFieldIsSetCondition{fieldName: s.fieldName}
}

// NotSet creates a condition that checks if this field is not set (is bottom).
// Example: FieldRef("role").NotSet() generates: role == _|_
func (s *ScopedFieldRef) NotSet() Condition {
	return &NotCondition{inner: &ScopedFieldIsSetCondition{fieldName: s.fieldName}}
}

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
