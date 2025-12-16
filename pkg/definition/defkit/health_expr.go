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

// HealthExpression is the composable unit for building health policies.
// All health primitives implement this interface and can be combined
// using Health().And(), Health().Or(), and Health().Not().
type HealthExpression interface {
	// ToCUE generates the CUE expression for this health check.
	// The expression should evaluate to a boolean.
	ToCUE() string

	// Preamble returns any CUE definitions needed before the isHealth expression.
	// For example, condition checks need helper variables to extract conditions.
	Preamble() string
}

// HealthPolicy wraps a HealthExpression and generates the complete healthPolicy CUE block.
func HealthPolicy(expr HealthExpression) string {
	preamble := expr.Preamble()
	if preamble != "" {
		return preamble + "\nisHealth: " + expr.ToCUE()
	}
	return "isHealth: " + expr.ToCUE()
}

// --- Condition Expressions ---

// ConditionExpr checks a condition in status.conditions[] array.
type ConditionExpr struct {
	condType       string
	expectedStatus string
	expectedReason string
	checkExists    bool
}

// IsTrue checks if the condition status is "True".
func (c *ConditionExpr) IsTrue() HealthExpression {
	c.expectedStatus = "True"
	return c
}

// IsFalse checks if the condition status is "False".
func (c *ConditionExpr) IsFalse() HealthExpression {
	c.expectedStatus = "False"
	return c
}

// Is checks if the condition status matches the expected value.
func (c *ConditionExpr) Is(status string) HealthExpression {
	c.expectedStatus = status
	return c
}

// Exists checks if the condition exists (regardless of status).
func (c *ConditionExpr) Exists() HealthExpression {
	c.checkExists = true
	return c
}

// ReasonIs checks if the condition has a specific reason.
func (c *ConditionExpr) ReasonIs(reason string) HealthExpression {
	c.expectedReason = reason
	return c
}

func (c *ConditionExpr) varName() string {
	return "_" + strings.ToLower(c.condType) + "Cond"
}

func (c *ConditionExpr) Preamble() string {
	varName := c.varName()
	return fmt.Sprintf(`%s: [ for c in context.output.status.conditions if c.type == "%s" { c } ]`,
		varName, c.condType)
}

func (c *ConditionExpr) ToCUE() string {
	varName := c.varName()
	if c.checkExists {
		return fmt.Sprintf("len(%s) > 0", varName)
	}
	if c.expectedReason != "" {
		return fmt.Sprintf(`len(%s) > 0 && %s[0].status == "%s" && %s[0].reason == "%s"`,
			varName, varName, c.expectedStatus, varName, c.expectedReason)
	}
	return fmt.Sprintf(`len(%s) > 0 && %s[0].status == "%s"`,
		varName, varName, c.expectedStatus)
}

// --- Phase Expressions ---

// phaseExpr checks the status.phase field.
type phaseExpr struct {
	fieldPath string
	phases    []string
}

func (p *phaseExpr) Preamble() string {
	return ""
}

func (p *phaseExpr) ToCUE() string {
	if len(p.phases) == 1 {
		return fmt.Sprintf(`%s == "%s"`, p.fieldPath, p.phases[0])
	}
	parts := make([]string, len(p.phases))
	for i, phase := range p.phases {
		parts[i] = fmt.Sprintf(`%s == "%s"`, p.fieldPath, phase)
	}
	return strings.Join(parts, " || ")
}

// --- Field Expressions ---

// HealthFieldExpr provides comparison operations on a status field.
type HealthFieldExpr struct {
	path string
}

// Eq checks if the field equals the given value.
func (f *HealthFieldExpr) Eq(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: "==", value: value}
}

// Ne checks if the field does not equal the given value.
func (f *HealthFieldExpr) Ne(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: "!=", value: value}
}

// Gt checks if the field is greater than the given value.
func (f *HealthFieldExpr) Gt(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: ">", value: value}
}

// Gte checks if the field is greater than or equal to the given value.
func (f *HealthFieldExpr) Gte(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: ">=", value: value}
}

// Lt checks if the field is less than the given value.
func (f *HealthFieldExpr) Lt(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: "<", value: value}
}

// Lte checks if the field is less than or equal to the given value.
func (f *HealthFieldExpr) Lte(value any) HealthExpression {
	return &fieldCompareExpr{path: f.path, op: "<=", value: value}
}

// In checks if the field value is one of the given values.
func (f *HealthFieldExpr) In(values ...any) HealthExpression {
	return &fieldInExpr{path: f.path, values: values}
}

// Contains checks if the string field contains the given substring.
func (f *HealthFieldExpr) Contains(substr string) HealthExpression {
	return &fieldContainsExpr{path: f.path, substr: substr}
}

// fieldCompareExpr is a field comparison expression.
type fieldCompareExpr struct {
	path  string
	op    string
	value any
}

func (f *fieldCompareExpr) Preamble() string {
	return ""
}

func (f *fieldCompareExpr) ToCUE() string {
	fullPath := "context.output." + f.path
	// Check if value is a HealthFieldRefExpr
	if ref, ok := f.value.(*HealthFieldRefExpr); ok {
		return fmt.Sprintf("%s %s %s", fullPath, f.op, ref.ToCUE())
	}
	return fmt.Sprintf("%s %s %s", fullPath, f.op, formatValue(f.value))
}

// fieldInExpr checks if a field is in a set of values.
type fieldInExpr struct {
	path   string
	values []any
}

func (f *fieldInExpr) Preamble() string {
	return ""
}

func (f *fieldInExpr) ToCUE() string {
	fullPath := "context.output." + f.path
	parts := make([]string, len(f.values))
	for i, v := range f.values {
		parts[i] = fmt.Sprintf("%s == %s", fullPath, formatValue(v))
	}
	return strings.Join(parts, " || ")
}

// fieldContainsExpr checks if a string field contains a substring.
type fieldContainsExpr struct {
	path   string
	substr string
}

func (f *fieldContainsExpr) Preamble() string {
	return ""
}

func (f *fieldContainsExpr) ToCUE() string {
	fullPath := "context.output." + f.path
	return fmt.Sprintf(`strings.Contains(%s, "%s")`, fullPath, f.substr)
}

// --- FieldRef for field-to-field comparisons ---

// HealthFieldRefExpr represents a reference to another field (for comparisons).
type HealthFieldRefExpr struct {
	path string
}

func (f *HealthFieldRefExpr) Preamble() string {
	return ""
}

func (f *HealthFieldRefExpr) ToCUE() string {
	return "context.output." + f.path
}

// --- Exists / NotExists ---

// existsExpr checks if a field exists (is not bottom _|_).
type existsExpr struct {
	path   string
	negate bool
}

func (e *existsExpr) Preamble() string {
	return ""
}

func (e *existsExpr) ToCUE() string {
	fullPath := "context.output." + e.path
	if e.negate {
		return fmt.Sprintf("%s == _|_", fullPath)
	}
	return fmt.Sprintf("%s != _|_", fullPath)
}

// --- Combinators: And, Or, Not ---

// andExpr combines multiple expressions with AND.
type andExpr struct {
	exprs []HealthExpression
}

func (a *andExpr) Preamble() string {
	var preambles []string
	for _, expr := range a.exprs {
		if p := expr.Preamble(); p != "" {
			preambles = append(preambles, p)
		}
	}
	return strings.Join(preambles, "\n")
}

func (a *andExpr) ToCUE() string {
	parts := make([]string, len(a.exprs))
	for i, expr := range a.exprs {
		parts[i] = "(" + expr.ToCUE() + ")"
	}
	return strings.Join(parts, " && ")
}

// orExpr combines multiple expressions with OR.
type orExpr struct {
	exprs []HealthExpression
}

func (o *orExpr) Preamble() string {
	var preambles []string
	for _, expr := range o.exprs {
		if p := expr.Preamble(); p != "" {
			preambles = append(preambles, p)
		}
	}
	return strings.Join(preambles, "\n")
}

func (o *orExpr) ToCUE() string {
	parts := make([]string, len(o.exprs))
	for i, expr := range o.exprs {
		parts[i] = "(" + expr.ToCUE() + ")"
	}
	return strings.Join(parts, " || ")
}

// notExpr negates an expression.
type notExpr struct {
	expr HealthExpression
}

func (n *notExpr) Preamble() string {
	return n.expr.Preamble()
}

func (n *notExpr) ToCUE() string {
	return "!(" + n.expr.ToCUE() + ")"
}

// --- Always (existence-based health) ---

// alwaysExpr always returns true (resource existence = healthy).
type alwaysExpr struct{}

func (a *alwaysExpr) Preamble() string {
	return ""
}

func (a *alwaysExpr) ToCUE() string {
	return "true"
}

// --- Helper functions ---

// formatValue formats a Go value for CUE output.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, val)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
