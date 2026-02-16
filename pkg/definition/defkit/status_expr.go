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
	"strings"
)

// StatusExpression is the composable unit for building custom status messages.
// All status primitives implement this interface and can be combined
// to create dynamic status messages.
type StatusExpression interface {
	// ToCUE generates the CUE expression for this status component.
	// The expression should evaluate to a string for the message.
	ToCUE() string

	// Preamble returns any CUE definitions needed before the message expression.
	// For example, field extractions need helper variables.
	Preamble() string

	// IsStringExpr returns true if this expression produces a string value
	// that can be used directly in CUE interpolation.
	IsStringExpr() bool
}

// StatusPolicy wraps a StatusExpression and generates the complete customStatus CUE block.
func StatusPolicy(expr StatusExpression) string {
	preamble := expr.Preamble()
	if preamble != "" {
		return preamble + "\nmessage: " + expr.ToCUE()
	}
	return "message: " + expr.ToCUE()
}

// --- Status Field Expressions ---

// StatusFieldExpr extracts a field value from the output status.
type StatusFieldExpr struct {
	path         string
	isSpec       bool // true if this is from spec, false if from status
	defaultValue any
	hasDefault   bool
	varName      string // generated variable name for preamble
}

// Default sets a default value if the field doesn't exist.
func (f *StatusFieldExpr) Default(value any) *StatusFieldExpr {
	f.defaultValue = value
	f.hasDefault = true
	return f
}

// Eq checks if the field equals a value (returns a StatusConditionExpr for use in Switch).
func (f *StatusFieldExpr) Eq(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: "==", value: value}
}

// Ne checks if the field does not equal a value.
func (f *StatusFieldExpr) Ne(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: "!=", value: value}
}

// Gt checks if the field is greater than a value.
func (f *StatusFieldExpr) Gt(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: ">", value: value}
}

// Gte checks if the field is greater than or equal to a value.
func (f *StatusFieldExpr) Gte(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: ">=", value: value}
}

// Lt checks if the field is less than a value.
func (f *StatusFieldExpr) Lt(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: "<", value: value}
}

// Lte checks if the field is less than or equal to a value.
func (f *StatusFieldExpr) Lte(value any) StatusCondition {
	return &statusFieldCondition{field: f, op: "<=", value: value}
}

func (f *StatusFieldExpr) getVarName() string {
	if f.varName != "" {
		return f.varName
	}
	// Generate a variable name from the path
	// e.g., "status.readyReplicas" -> "_readyReplicas"
	parts := strings.Split(f.path, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		f.varName = "_" + strings.ToLower(lastPart[:1]) + lastPart[1:]
	} else {
		f.varName = "_field"
	}
	return f.varName
}

func (f *StatusFieldExpr) getFullPath() string {
	return "context.output." + f.path
}

func (f *StatusFieldExpr) Preamble() string {
	if !f.hasDefault {
		return ""
	}

	varName := f.getVarName()
	fullPath := f.getFullPath()

	var defaultExpr string
	switch v := f.defaultValue.(type) {
	case string:
		defaultExpr = fmt.Sprintf("*%q | string", v)
	case int, int32, int64:
		defaultExpr = fmt.Sprintf("*%v | int", v)
	case float32, float64:
		defaultExpr = fmt.Sprintf("*%v | number", v)
	case bool:
		defaultExpr = fmt.Sprintf("*%t | bool", v)
	default:
		defaultExpr = fmt.Sprintf("*%v | _", v)
	}

	return fmt.Sprintf(`%s: %s
if %s != _|_ {
	%s: %s
}`, varName, defaultExpr, fullPath, varName, fullPath)
}

func (f *StatusFieldExpr) ToCUE() string {
	if f.hasDefault {
		return fmt.Sprintf(`"\(%s)"`, f.getVarName())
	}
	return fmt.Sprintf(`"\(%s)"`, f.getFullPath())
}

func (f *StatusFieldExpr) IsStringExpr() bool {
	return true
}

// --- Status Condition Expressions (for conditions array) ---

// StatusConditionFieldExpr provides access to fields within a status condition.
type StatusConditionFieldExpr struct {
	condType string
	field    string // "status", "message", "reason"
}

func (c *StatusConditionFieldExpr) getVarName() string {
	return "_" + strings.ToLower(c.condType) + "Cond"
}

func (c *StatusConditionFieldExpr) getFieldVarName() string {
	return "_" + strings.ToLower(c.condType) + strings.ToUpper(c.field[:1]) + c.field[1:]
}

func (c *StatusConditionFieldExpr) Preamble() string {
	varName := c.getVarName()
	fieldVar := c.getFieldVarName()

	var defaultVal string
	switch c.field {
	case "status":
		defaultVal = `*"Unknown" | string`
	case "message", "reason":
		defaultVal = `*"" | string`
	default:
		defaultVal = `*"" | string`
	}

	return fmt.Sprintf(`%s: [ for cond in context.output.status.conditions if cond.type == "%s" { cond } ]
%s: %s
if len(%s) > 0 {
	%s: %s[0].%s
}`, varName, c.condType, fieldVar, defaultVal, varName, fieldVar, varName, c.field)
}

func (c *StatusConditionFieldExpr) ToCUE() string {
	return fmt.Sprintf(`"\(%s)"`, c.getFieldVarName())
}

func (c *StatusConditionFieldExpr) IsStringExpr() bool {
	return true
}

// Is checks if the condition field equals a value.
func (c *StatusConditionFieldExpr) Is(value string) StatusCondition {
	return &statusConditionCheck{condType: c.condType, field: c.field, expectedValue: value}
}

// StatusConditionAccessor provides methods to access different fields of a condition.
type StatusConditionAccessor struct {
	condType string
}

// StatusValue returns an expression for the condition's status field.
func (c *StatusConditionAccessor) StatusValue() *StatusConditionFieldExpr {
	return &StatusConditionFieldExpr{condType: c.condType, field: "status"}
}

// Message returns an expression for the condition's message field.
func (c *StatusConditionAccessor) Message() *StatusConditionFieldExpr {
	return &StatusConditionFieldExpr{condType: c.condType, field: "message"}
}

// Reason returns an expression for the condition's reason field.
func (c *StatusConditionAccessor) Reason() *StatusConditionFieldExpr {
	return &StatusConditionFieldExpr{condType: c.condType, field: "reason"}
}

// Is checks if the condition's status equals a value (for use in Switch).
func (c *StatusConditionAccessor) Is(status string) StatusCondition {
	return &statusConditionCheck{condType: c.condType, field: "status", expectedValue: status}
}

// --- Status Condition (boolean check for Switch) ---

// StatusCondition represents a boolean condition for use in Switch cases.
type StatusCondition interface {
	// ToCUECondition generates the CUE boolean expression.
	ToCUECondition() string
	// Preamble returns any CUE definitions needed.
	Preamble() string
}

// statusFieldCondition is a field comparison condition.
type statusFieldCondition struct {
	field *StatusFieldExpr
	op    string
	value any
}

func (c *statusFieldCondition) ToCUECondition() string {
	return fmt.Sprintf("%s %s %s", c.field.getFullPath(), c.op, formatValue(c.value))
}

func (c *statusFieldCondition) Preamble() string {
	return ""
}

// statusConditionCheck checks a condition field value.
type statusConditionCheck struct {
	condType      string
	field         string
	expectedValue string
}

func (c *statusConditionCheck) getVarName() string {
	return "_" + strings.ToLower(c.condType) + "Cond"
}

func (c *statusConditionCheck) ToCUECondition() string {
	varName := c.getVarName()
	return fmt.Sprintf(`len(%s) > 0 && %s[0].%s == "%s"`, varName, varName, c.field, c.expectedValue)
}

func (c *statusConditionCheck) Preamble() string {
	varName := c.getVarName()
	return fmt.Sprintf(`%s: [ for cond in context.output.status.conditions if cond.type == "%s" { cond } ]`,
		varName, c.condType)
}

// --- Exists Expression ---

// statusExistsExpr checks if a field exists.
type statusExistsExpr struct {
	path   string
	negate bool
}

func (e *statusExistsExpr) Preamble() string {
	return ""
}

func (e *statusExistsExpr) ToCUE() string {
	fullPath := "context.output." + e.path
	if e.negate {
		return fmt.Sprintf(`if %s == _|_ { "not exists" }`, fullPath)
	}
	return fmt.Sprintf(`if %s != _|_ { "exists" }`, fullPath)
}

func (e *statusExistsExpr) IsStringExpr() bool {
	return false // This is a condition, not a string value
}

// ToCUECondition returns the CUE condition for use in Switch.
func (e *statusExistsExpr) ToCUECondition() string {
	fullPath := "context.output." + e.path
	if e.negate {
		return fmt.Sprintf("%s == _|_", fullPath)
	}
	return fmt.Sprintf("%s != _|_", fullPath)
}

// --- Message Building Expressions ---

// statusLiteralExpr is a literal string value.
type statusLiteralExpr struct {
	value string
}

func (l *statusLiteralExpr) Preamble() string {
	return ""
}

func (l *statusLiteralExpr) ToCUE() string {
	return fmt.Sprintf("%q", l.value)
}

func (l *statusLiteralExpr) IsStringExpr() bool {
	return true
}

// statusConcatExpr concatenates multiple expressions.
type statusConcatExpr struct {
	parts []any // Can be StatusExpression or string literals
}

func (c *statusConcatExpr) Preamble() string {
	var preambles []string
	for _, part := range c.parts {
		if expr, ok := part.(StatusExpression); ok {
			if p := expr.Preamble(); p != "" {
				preambles = append(preambles, p)
			}
		}
	}
	return strings.Join(preambles, "\n")
}

func (c *statusConcatExpr) ToCUE() string {
	var sb strings.Builder
	sb.WriteString(`"`)
	for _, part := range c.parts {
		switch p := part.(type) {
		case string:
			sb.WriteString(p)
		case *StatusFieldExpr:
			if p.hasDefault {
				sb.WriteString(fmt.Sprintf(`\(%s)`, p.getVarName()))
			} else {
				sb.WriteString(fmt.Sprintf(`\(%s)`, p.getFullPath()))
			}
		case *StatusConditionFieldExpr:
			sb.WriteString(fmt.Sprintf(`\(%s)`, p.getFieldVarName()))
		case StatusExpression:
			// Extract the interpolation part from the expression
			cue := p.ToCUE()
			// Remove surrounding quotes if present
			cue = strings.TrimPrefix(cue, `"`)
			cue = strings.TrimSuffix(cue, `"`)
			sb.WriteString(cue)
		default:
			sb.WriteString(fmt.Sprintf("%v", p))
		}
	}
	sb.WriteString(`"`)
	return sb.String()
}

func (c *statusConcatExpr) IsStringExpr() bool {
	return true
}

// statusFormatExpr provides printf-style formatting.
type statusFormatExpr struct {
	template string
	args     []StatusExpression
}

func (f *statusFormatExpr) Preamble() string {
	var preambles []string
	for _, arg := range f.args {
		if p := arg.Preamble(); p != "" {
			preambles = append(preambles, p)
		}
	}
	return strings.Join(preambles, "\n")
}

func (f *statusFormatExpr) ToCUE() string {
	// Replace %v with CUE interpolations
	result := f.template
	for _, arg := range f.args {
		cue := arg.ToCUE()
		// Remove surrounding quotes
		cue = strings.TrimPrefix(cue, `"`)
		cue = strings.TrimSuffix(cue, `"`)
		// Replace first %v with the interpolation
		result = strings.Replace(result, "%v", cue, 1)
	}
	return fmt.Sprintf(`"%s"`, result)
}

func (f *statusFormatExpr) IsStringExpr() bool {
	return true
}

// --- Switch Expression ---

// StatusCase represents a case in a Switch expression.
type StatusCase struct {
	condition StatusCondition
	message   StatusExpression
}

// statusSwitchExpr provides conditional message selection.
type statusSwitchExpr struct {
	cases        []*StatusCase
	defaultValue StatusExpression
}

func (s *statusSwitchExpr) Preamble() string {
	var preambles []string
	seen := make(map[string]bool)

	// Collect preambles from conditions
	for _, c := range s.cases {
		if p := c.condition.Preamble(); p != "" && !seen[p] {
			preambles = append(preambles, p)
			seen[p] = true
		}
		if p := c.message.Preamble(); p != "" && !seen[p] {
			preambles = append(preambles, p)
			seen[p] = true
		}
	}
	if s.defaultValue != nil {
		if p := s.defaultValue.Preamble(); p != "" && !seen[p] {
			preambles = append(preambles, p)
		}
	}
	return strings.Join(preambles, "\n")
}

func (s *statusSwitchExpr) ToCUE() string {
	// For Switch, we generate the full message block with conditionals
	// The caller (StatusPolicy) will handle wrapping
	return s.buildMessageBlock()
}

func (s *statusSwitchExpr) buildMessageBlock() string {
	var sb strings.Builder

	// Default value first (will be overridden by conditions)
	if s.defaultValue != nil {
		sb.WriteString(fmt.Sprintf("*%s | string", s.defaultValue.ToCUE()))
	} else {
		sb.WriteString(`*"" | string`)
	}

	return sb.String()
}

func (s *statusSwitchExpr) IsStringExpr() bool {
	return true
}

// Override Preamble to include the switch logic
func (s *statusSwitchExpr) BuildFull() string {
	var sb strings.Builder

	// Collect all preambles first
	preambles := s.Preamble()
	if preambles != "" {
		sb.WriteString(preambles)
		sb.WriteString("\n")
	}

	// Write message with default
	if s.defaultValue != nil {
		sb.WriteString(fmt.Sprintf("message: *%s | string\n", s.defaultValue.ToCUE()))
	} else {
		sb.WriteString(`message: *"" | string`)
		sb.WriteString("\n")
	}

	// Write conditional overrides
	for _, c := range s.cases {
		sb.WriteString(fmt.Sprintf("if %s {\n\tmessage: %s\n}\n",
			c.condition.ToCUECondition(), c.message.ToCUE()))
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// --- Health-Aware Expression ---

// statusHealthAwareExpr provides different messages based on health status.
type statusHealthAwareExpr struct {
	healthyMsg   StatusExpression
	unhealthyMsg StatusExpression
}

func (h *statusHealthAwareExpr) Preamble() string {
	var preambles []string
	if p := h.healthyMsg.Preamble(); p != "" {
		preambles = append(preambles, p)
	}
	if p := h.unhealthyMsg.Preamble(); p != "" {
		preambles = append(preambles, p)
	}
	return strings.Join(preambles, "\n")
}

func (h *statusHealthAwareExpr) ToCUE() string {
	// Will be handled by BuildFull
	return h.healthyMsg.ToCUE()
}

func (h *statusHealthAwareExpr) IsStringExpr() bool {
	return true
}

func (h *statusHealthAwareExpr) BuildFull() string {
	var sb strings.Builder

	preamble := h.Preamble()
	if preamble != "" {
		sb.WriteString(preamble)
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("message: *%s | string\n", h.unhealthyMsg.ToCUE()))
	sb.WriteString(fmt.Sprintf("if context.status.healthy {\n\tmessage: %s\n}", h.healthyMsg.ToCUE()))

	return sb.String()
}

// --- Status Details Expression ---

// StatusDetail represents a key-value detail.
type StatusDetail struct {
	key   string
	value StatusExpression
}

// statusWithDetailsExpr adds structured details to the status.
type statusWithDetailsExpr struct {
	message StatusExpression
	details []*StatusDetail
}

func (d *statusWithDetailsExpr) Preamble() string {
	var preambles []string
	if p := d.message.Preamble(); p != "" {
		preambles = append(preambles, p)
	}
	for _, detail := range d.details {
		if p := detail.value.Preamble(); p != "" {
			preambles = append(preambles, p)
		}
	}
	return strings.Join(preambles, "\n")
}

func (d *statusWithDetailsExpr) ToCUE() string {
	return d.message.ToCUE()
}

func (d *statusWithDetailsExpr) IsStringExpr() bool {
	return true
}

// --- StatusBuilder Extension Methods ---

// Field creates an expression to extract a status field value.
// Example: Status().Field("status.readyReplicas").Default(0)
func (s *StatusBuilder) Field(path string) *StatusFieldExpr {
	return &StatusFieldExpr{path: path}
}

// SpecField creates an expression to extract a spec field value.
// Example: Status().SpecField("spec.replicas")
func (s *StatusBuilder) SpecField(path string) *StatusFieldExpr {
	return &StatusFieldExpr{path: path, isSpec: true}
}

// Condition creates an accessor for a status condition.
// Example: Status().Condition("Ready").Message()
func (s *StatusBuilder) Condition(condType string) *StatusConditionAccessor {
	return &StatusConditionAccessor{condType: condType}
}

// Exists creates an expression to check if a field exists.
// Example: Status().Exists("status.endpoint")
func (s *StatusBuilder) Exists(path string) *statusExistsExpr {
	return &statusExistsExpr{path: path, negate: false}
}

// NotExists creates an expression to check if a field does not exist.
// Example: Status().NotExists("status.error")
func (s *StatusBuilder) NotExists(path string) *statusExistsExpr {
	return &statusExistsExpr{path: path, negate: true}
}

// Literal creates a literal string expression.
// Example: Status().Literal("Service is running")
func (s *StatusBuilder) Literal(value string) StatusExpression {
	return &statusLiteralExpr{value: value}
}

// Concat creates a concatenation of multiple expressions or strings.
// Example: Status().Concat("Ready: ", s.Field("status.ready"), "/", s.Field("status.total"))
func (s *StatusBuilder) Concat(parts ...any) StatusExpression {
	return &statusConcatExpr{parts: parts}
}

// Format creates a printf-style formatted message.
// Example: Status().Format("Ready: %v/%v", readyField, totalField)
func (s *StatusBuilder) Format(template string, args ...StatusExpression) StatusExpression {
	return &statusFormatExpr{template: template, args: args}
}

// Case creates a case for use in Switch.
// Example: s.Case(s.Field("status.phase").Eq("Running"), "Service is running")
func (s *StatusBuilder) Case(condition StatusCondition, message any) *StatusCase {
	var msgExpr StatusExpression
	switch m := message.(type) {
	case string:
		msgExpr = &statusLiteralExpr{value: m}
	case StatusExpression:
		msgExpr = m
	default:
		msgExpr = &statusLiteralExpr{value: fmt.Sprintf("%v", m)}
	}
	return &StatusCase{condition: condition, message: msgExpr}
}

// Default creates a default case for Switch.
// Example: s.Default("Unknown status")
func (s *StatusBuilder) Default(message any) StatusExpression {
	switch m := message.(type) {
	case string:
		return &statusLiteralExpr{value: m}
	case StatusExpression:
		return m
	default:
		return &statusLiteralExpr{value: fmt.Sprintf("%v", m)}
	}
}

// Switch creates a conditional message selection.
// Example: Status().Switch(case1, case2, s.Default("Unknown"))
func (s *StatusBuilder) Switch(casesAndDefault ...any) StatusExpression {
	sw := &statusSwitchExpr{}
	for _, item := range casesAndDefault {
		switch v := item.(type) {
		case *StatusCase:
			sw.cases = append(sw.cases, v)
		case StatusExpression:
			sw.defaultValue = v
		}
	}
	return sw
}

// HealthAware creates a message that differs based on health status.
// Example: Status().HealthAware("All systems operational", "Service degraded")
func (s *StatusBuilder) HealthAware(healthyMsg, unhealthyMsg any) StatusExpression {
	var healthy, unhealthy StatusExpression

	switch m := healthyMsg.(type) {
	case string:
		healthy = &statusLiteralExpr{value: m}
	case StatusExpression:
		healthy = m
	}

	switch m := unhealthyMsg.(type) {
	case string:
		unhealthy = &statusLiteralExpr{value: m}
	case StatusExpression:
		unhealthy = m
	}

	return &statusHealthAwareExpr{healthyMsg: healthy, unhealthyMsg: unhealthy}
}

// Detail creates a detail entry for use with WithDetails.
// Example: s.Detail("endpoint", s.Field("status.endpoint"))
func (s *StatusBuilder) Detail(key string, value StatusExpression) *StatusDetail {
	return &StatusDetail{key: key, value: value}
}

// WithDetails creates a status expression that includes structured details alongside the message.
// The details are key-value pairs that provide additional context.
//
// Example:
//
//	s := Status()
//	CustomStatusExpr(s.WithDetails(
//	    s.Format("Ready: %v/%v", s.Field("status.readyReplicas").Default(0), s.SpecField("spec.replicas")),
//	    s.Detail("endpoint", s.Field("status.endpoint")),
//	    s.Detail("version", s.Field("status.version")),
//	))
func (s *StatusBuilder) WithDetails(message StatusExpression, details ...*StatusDetail) StatusExpression {
	return &statusWithDetailsExpr{message: message, details: details}
}

// StatusExpr sets the status expression using a composable StatusExpression.
// This generates the complete customStatus CUE block.
// Example: Status().StatusExpr(s.Concat("Ready: ", s.Field("status.ready")))
func (s *StatusBuilder) StatusExpr(expr StatusExpression) *StatusBuilder {
	// Handle special expression types that need full generation
	switch e := expr.(type) {
	case *statusSwitchExpr:
		s.rawCUE = e.BuildFull()
	case *statusHealthAwareExpr:
		s.rawCUE = e.BuildFull()
	default:
		s.rawCUE = StatusPolicy(expr)
	}
	return s
}

// CustomStatusExpr is a convenience function for component definitions.
// It creates a StatusBuilder with the given expression and returns it for use with CustomStatus().
// Example: CustomStatusExpr(Status().Concat("Ready: ", s.Field("status.ready")))
func CustomStatusExpr(expr StatusExpression) string {
	switch e := expr.(type) {
	case *statusSwitchExpr:
		return e.BuildFull()
	case *statusHealthAwareExpr:
		return e.BuildFull()
	default:
		return StatusPolicy(expr)
	}
}
