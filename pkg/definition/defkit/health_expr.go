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
	"regexp"
	"strings"
)

const defaultHealthRoot = "context.output"

// resolveRoot returns root, or the default primary-output root when it is empty.
func resolveRoot(root string) string {
	if root == "" {
		return defaultHealthRoot
	}
	return root
}

// sanitizeIdent maps s to a valid CUE identifier fragment. Alphanumerics pass through; other bytes become "_"+hex.
func sanitizeIdent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "_%02x", c)
	}
	return b.String()
}

// sanitizeRoot turns a CUE root path into an identifier fragment for a preamble variable name.
func sanitizeRoot(root string) string {
	s := strings.TrimPrefix(root, "context.outputs.")
	s = strings.TrimPrefix(s, "context.")
	return sanitizeIdent(s)
}

// upperFirst upper-cases the first character, leaving the rest unchanged.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

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
	root           string
	inline         bool
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

// varName is the preamble helper name, with the root folded in so distinct scopes do not collide.
func (c *ConditionExpr) varName() string {
	if c.root == "" || c.root == defaultHealthRoot {
		return "_" + strings.ToLower(c.condType) + "Cond"
	}
	return "_" + sanitizeRoot(c.root) + upperFirst(strings.ToLower(c.condType)) + "Cond"
}

func (c *ConditionExpr) Preamble() string {
	if c.inline {
		return ""
	}
	varName := c.varName()
	// `*conditions | []` defaults a missing list to empty, so a fresh resource reads unhealthy instead of erroring.
	return fmt.Sprintf(`%s: [ for c in *%s.status.conditions | [] if c.type != _|_ if c.type == "%s" { c } ]`,
		varName, resolveRoot(c.root), c.condType)
}

func (c *ConditionExpr) ToCUE() string {
	if c.inline {
		return c.inlineCUE()
	}
	varName := c.varName()
	if c.checkExists {
		return fmt.Sprintf("len(%s) > 0", varName)
	}
	// Filter, don't index [0]: CUE's && isn't short-circuit, so indexing an empty list would error; != _|_ skips incomplete entries.
	if c.expectedReason != "" {
		return fmt.Sprintf(`len([ for c in %s if c.status != _|_ if c.status == "%s" if c.reason != _|_ if c.reason == "%s" { c } ]) > 0`,
			varName, c.expectedStatus, c.expectedReason)
	}
	return fmt.Sprintf(`len([ for c in %s if c.status != _|_ if c.status == "%s" { c } ]) > 0`,
		varName, c.expectedStatus)
}

// inlineCUE folds all filters into one comprehension so the check can nest inside Every's loop (no hoisted preamble var).
func (c *ConditionExpr) inlineCUE() string {
	src := fmt.Sprintf(`*%s.status.conditions | []`, resolveRoot(c.root))
	if c.checkExists {
		return fmt.Sprintf(`len([ for c in %s if c.type != _|_ if c.type == %q { c } ]) > 0`,
			src, c.condType)
	}
	filters := fmt.Sprintf(`if c.type != _|_ if c.type == %q if c.status != _|_ if c.status == %q`,
		c.condType, c.expectedStatus)
	if c.expectedReason != "" {
		filters += fmt.Sprintf(` if c.reason != _|_ if c.reason == %q`, c.expectedReason)
	}
	return fmt.Sprintf(`len([ for c in %s %s { c } ]) > 0`, src, filters)
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
	root string
	path string
}

// Eq checks if the field equals the given value.
func (f *HealthFieldExpr) Eq(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: "==", value: value}
}

// Ne checks if the field does not equal the given value.
func (f *HealthFieldExpr) Ne(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: "!=", value: value}
}

// Gt checks if the field is greater than the given value.
func (f *HealthFieldExpr) Gt(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: ">", value: value}
}

// Gte checks if the field is greater than or equal to the given value.
func (f *HealthFieldExpr) Gte(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: ">=", value: value}
}

// Lt checks if the field is less than the given value.
func (f *HealthFieldExpr) Lt(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: "<", value: value}
}

// Lte checks if the field is less than or equal to the given value.
func (f *HealthFieldExpr) Lte(value any) HealthExpression {
	return &fieldCompareExpr{root: f.root, path: f.path, op: "<=", value: value}
}

// In checks if the field value is one of the given values.
func (f *HealthFieldExpr) In(values ...any) HealthExpression {
	return &fieldInExpr{root: f.root, path: f.path, values: values}
}

// Contains checks if the string field contains the given substring.
func (f *HealthFieldExpr) Contains(substr string) HealthExpression {
	return &fieldContainsExpr{root: f.root, path: f.path, substr: substr}
}

// fieldCompareExpr is a field comparison expression.
type fieldCompareExpr struct {
	root  string
	path  string
	op    string
	value any
}

func (f *fieldCompareExpr) Preamble() string {
	return ""
}

func (f *fieldCompareExpr) ToCUE() string {
	fullPath := resolveRoot(f.root) + "." + f.path
	// Check if value is a HealthFieldRefExpr
	if ref, ok := f.value.(*HealthFieldRefExpr); ok {
		return fmt.Sprintf("%s %s %s", fullPath, f.op, ref.ToCUE())
	}
	return fmt.Sprintf("%s %s %s", fullPath, f.op, formatValue(f.value))
}

// fieldInExpr checks if a field is in a set of values.
type fieldInExpr struct {
	root   string
	path   string
	values []any
}

func (f *fieldInExpr) Preamble() string {
	return ""
}

func (f *fieldInExpr) ToCUE() string {
	fullPath := resolveRoot(f.root) + "." + f.path
	parts := make([]string, len(f.values))
	for i, v := range f.values {
		parts[i] = fmt.Sprintf("%s == %s", fullPath, formatValue(v))
	}
	return strings.Join(parts, " || ")
}

// fieldContainsExpr checks if a string field contains a substring.
type fieldContainsExpr struct {
	root   string
	path   string
	substr string
}

func (f *fieldContainsExpr) Preamble() string {
	return ""
}

func (f *fieldContainsExpr) ToCUE() string {
	fullPath := resolveRoot(f.root) + "." + f.path
	return fmt.Sprintf(`strings.Contains(%s, %s)`, fullPath, formatValue(f.substr))
}

// --- FieldRef for field-to-field comparisons ---

// HealthFieldRefExpr represents a reference to another field (for comparisons).
type HealthFieldRefExpr struct {
	root string
	path string
}

func (f *HealthFieldRefExpr) Preamble() string {
	return ""
}

func (f *HealthFieldRefExpr) ToCUE() string {
	return resolveRoot(f.root) + "." + f.path
}

// --- Exists / NotExists ---

// existsExpr checks if a field exists (is not bottom _|_).
type existsExpr struct {
	root   string
	path   string
	negate bool
}

func (e *existsExpr) Preamble() string {
	return ""
}

func (e *existsExpr) ToCUE() string {
	fullPath := resolveRoot(e.root) + "." + e.path
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

// --- Health scope (output-rooted expressions) ---

// HealthScope roots health expressions at a given output; the zero value targets the primary output.
type HealthScope struct {
	root   string
	inline bool // rooted at a comprehension variable (Every); suppresses ConditionExpr's hoisted preamble
}

// NewHealthScope returns a scope rooted at the given CUE path.
func NewHealthScope(root string) *HealthScope {
	return &HealthScope{root: root}
}

// Condition creates an expression to check a status condition on this scope.
func (s *HealthScope) Condition(condType string) *ConditionExpr {
	return &ConditionExpr{root: s.root, inline: s.inline, condType: condType, expectedStatus: "True"}
}

// Field creates an expression builder for a field path on this scope.
func (s *HealthScope) Field(path string) *HealthFieldExpr {
	return &HealthFieldExpr{root: s.root, path: path}
}

// FieldRef creates a reference to another field on this scope for field-to-field comparisons.
func (s *HealthScope) FieldRef(path string) *HealthFieldRefExpr {
	return &HealthFieldRefExpr{root: s.root, path: path}
}

// Exists checks if a field exists (is not _|_) on this scope.
func (s *HealthScope) Exists(path string) HealthExpression {
	return &existsExpr{root: s.root, path: path}
}

// NotExists checks if a field does not exist (is _|_) on this scope.
func (s *HealthScope) NotExists(path string) HealthExpression {
	return &existsExpr{root: s.root, path: path, negate: true}
}

// Phase checks if status.phase on this scope matches any of the given phases.
func (s *HealthScope) Phase(phases ...string) HealthExpression {
	return &phaseExpr{fieldPath: resolveRoot(s.root) + ".status.phase", phases: phases}
}

// PhaseField checks a custom phase field path on this scope.
func (s *HealthScope) PhaseField(path string, phases ...string) HealthExpression {
	return &phaseExpr{fieldPath: resolveRoot(s.root) + "." + path, phases: phases}
}

// --- Output collections and aggregation (Every) ---

// OutputCollection selects a set of auxiliary outputs by name prefix.
type OutputCollection struct {
	prefix string
}

// OutputsWithPrefix selects outputs whose name starts with prefix (output groups are numbered slots sharing a prefix).
func OutputsWithPrefix(prefix string) *OutputCollection {
	return &OutputCollection{prefix: prefix}
}

// EveryExpr requires every prefixed output to satisfy the item expression; empty is unhealthy unless AllowEmpty is set.
type EveryExpr struct {
	prefix     string
	itemVar    string
	itemExpr   HealthExpression
	allowEmpty bool
}

// AllowEmpty makes an empty match set healthy, for optional collections.
func (e *EveryExpr) AllowEmpty() *EveryExpr {
	e.allowEmpty = true
	return e
}

func (e *EveryExpr) itemsVar() string {
	return "_" + sanitizeIdent(e.prefix) + "Items"
}

func (e *EveryExpr) Preamble() string {
	// Built-in =~, not strings.HasPrefix: health CUE compiles without imports. QuoteMeta makes the anchored prefix a literal match.
	// (*context.outputs | {}) defaults a missing outputs map to empty; getTemplateContext omits it when there are no auxiliary outputs.
	return fmt.Sprintf(`%s: [ for k, v in (*context.outputs | {}) if k =~ %q { v } ]`,
		e.itemsVar(), "^"+regexp.QuoteMeta(e.prefix))
}

func (e *EveryExpr) ToCUE() string {
	items := e.itemsVar()
	all := fmt.Sprintf(`len([ for %s in %s if %s { %s } ]) == len(%s)`,
		e.itemVar, items, e.itemExpr.ToCUE(), e.itemVar, items)
	if e.allowEmpty {
		return all
	}
	return fmt.Sprintf(`len(%s) > 0 && %s`, items, all)
}

// --- Helper functions ---

// formatValue formats a Go value for CUE output.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val) // %q properly escapes quotes and special chars
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
