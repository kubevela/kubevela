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

// StatusBuilder provides a fluent API for building component status expressions.
type StatusBuilder struct {
	fields  []*StatusField
	message string
	rawCUE  string // Raw CUE for complex status expressions that don't fit the builder pattern
}

// StatusField represents a status field derived from the output.
type StatusField struct {
	name         string
	sourcePath   string
	defaultValue any
	fieldType    string // "int", "string", "bool"
}

// Status creates a new status builder.
func Status() *StatusBuilder {
	return &StatusBuilder{
		fields: make([]*StatusField, 0),
	}
}

// IntField adds an integer field derived from output status.
// Usage: .IntField("ready.replicas", "status.numberReady", 0)
func (s *StatusBuilder) IntField(name, sourcePath string, defaultVal int) *StatusBuilder {
	s.fields = append(s.fields, &StatusField{
		name:         name,
		sourcePath:   sourcePath,
		defaultValue: defaultVal,
		fieldType:    "int",
	})
	return s
}

// StringField adds a string field derived from output status.
func (s *StatusBuilder) StringField(name, sourcePath string, defaultVal string) *StatusBuilder {
	s.fields = append(s.fields, &StatusField{
		name:         name,
		sourcePath:   sourcePath,
		defaultValue: defaultVal,
		fieldType:    string(ParamTypeString),
	})
	return s
}

// Message sets the status message template.
// Use \(fieldName) for field interpolation (CUE syntax).
// Usage: .Message("Ready:\\(ready.replicas)/\\(desired.replicas)")
func (s *StatusBuilder) Message(msg string) *StatusBuilder {
	s.message = msg
	return s
}

// Build generates the CUE expression for customStatus.
func (s *StatusBuilder) Build() string {
	// If raw CUE is set, use it directly (for complex status expressions that don't fit builder pattern)
	if s.rawCUE != "" {
		return s.rawCUE
	}

	var parts []string

	for _, f := range s.fields {
		parts = append(parts, s.buildField(f))
	}

	if s.message != "" {
		// Use simple quotes around message - don't use %q which escapes backslashes,
		// preserving CUE interpolation syntax like \(field)
		parts = append(parts, fmt.Sprintf(`message: "%s"`, s.message))
	}

	return strings.Join(parts, "\n")
}

// RawCUE sets raw CUE for complex status expressions that don't fit the builder pattern.
func (s *StatusBuilder) RawCUE(cue string) *StatusBuilder {
	s.rawCUE = cue
	return s
}

func (s *StatusBuilder) buildField(f *StatusField) string {
	// Split nested path like "ready.replicas" into ["ready", "replicas"]
	pathParts := strings.Split(f.name, ".")

	// Build the nested structure
	var defaultExpr string
	switch f.fieldType {
	case string(ParamTypeInt):
		defaultExpr = fmt.Sprintf("*%d | int", f.defaultValue)
	case string(ParamTypeString):
		defaultExpr = fmt.Sprintf("*%q | string", f.defaultValue)
	case string(ParamTypeBool):
		defaultExpr = fmt.Sprintf("*%v | bool", f.defaultValue)
	}

	// For nested paths, build the structure
	if len(pathParts) == 2 {
		return fmt.Sprintf(`%s: {
	%s: %s
} & {
	if context.output.%s != _|_ {
		%s: context.output.%s
	}
}`, pathParts[0], pathParts[1], defaultExpr, f.sourcePath, pathParts[1], f.sourcePath)
	}

	// Simple field
	return fmt.Sprintf(`%s: %s & {
	if context.output.%s != _|_ {
		%s: context.output.%s
	}
}`, f.name, defaultExpr, f.sourcePath, f.name, f.sourcePath)
}

// HealthBuilder provides a fluent API for building health policy expressions.
type HealthBuilder struct {
	*StatusBuilder
	conditions []string
	rawCUE     string // Raw CUE string for complex health policies that don't fit the builder pattern
}

// Health creates a new health policy builder.
// It embeds StatusBuilder so you can add fields the same way.
func Health() *HealthBuilder {
	return &HealthBuilder{
		StatusBuilder: Status(),
		conditions:    make([]string, 0),
	}
}

// IntField adds an integer field (delegates to StatusBuilder).
func (h *HealthBuilder) IntField(name, sourcePath string, defaultVal int) *HealthBuilder {
	h.StatusBuilder.IntField(name, sourcePath, defaultVal)
	return h
}

// StringField adds a string field (delegates to StatusBuilder).
func (h *HealthBuilder) StringField(name, sourcePath string, defaultVal string) *HealthBuilder {
	h.StatusBuilder.StringField(name, sourcePath, defaultVal)
	return h
}

// MetadataField adds a field from metadata (e.g., generation).
func (h *HealthBuilder) MetadataField(name, sourcePath string) *HealthBuilder {
	h.fields = append(h.fields, &StatusField{
		name:       name,
		sourcePath: sourcePath,
		fieldType:  "metadata",
	})
	return h
}

// HealthyWhen adds health conditions.
// Usage: .HealthyWhen("desired.replicas == ready.replicas", "updated.replicas == desired.replicas")
func (h *HealthBuilder) HealthyWhen(conditions ...string) *HealthBuilder {
	h.conditions = append(h.conditions, conditions...)
	return h
}

// StatusEq is a helper for equality conditions in status expressions.
// Usage: .HealthyWhen(StatusEq("desired.replicas", "ready.replicas"))
func StatusEq(left, right string) string {
	return fmt.Sprintf("%s == %s", left, right)
}

// StatusGte is a helper for >= conditions in status expressions.
func StatusGte(left, right string) string {
	return fmt.Sprintf("%s >= %s", left, right)
}

// StatusOr combines conditions with ||.
func StatusOr(conditions ...string) string {
	return "(" + strings.Join(conditions, " || ") + ")"
}

// StatusAnd combines conditions with &&.
func StatusAnd(conditions ...string) string {
	return "(" + strings.Join(conditions, " && ") + ")"
}

// Build generates the CUE expression for healthPolicy.
func (h *HealthBuilder) Build() string {
	// If raw CUE is set, use it directly (for complex policies that don't fit builder pattern)
	if h.rawCUE != "" {
		return h.rawCUE
	}

	var parts []string

	for _, f := range h.fields {
		if f.fieldType == "metadata" {
			parts = append(parts, h.buildMetadataField(f))
		} else {
			parts = append(parts, h.StatusBuilder.buildField(f))
		}
	}

	if len(h.conditions) > 0 {
		healthExpr := strings.Join(h.conditions, " && ")
		parts = append(parts, fmt.Sprintf("isHealth: %s", healthExpr))
	}

	return strings.Join(parts, "\n")
}

// RawCUE sets raw CUE for complex health policies that don't fit the builder pattern.
func (h *HealthBuilder) RawCUE(cue string) *HealthBuilder {
	h.rawCUE = cue
	return h
}

func (h *HealthBuilder) buildMetadataField(f *StatusField) string {
	pathParts := strings.Split(f.name, ".")
	if len(pathParts) == 2 {
		return fmt.Sprintf(`%s: {
	%s: context.output.%s
}`, pathParts[0], pathParts[1], f.sourcePath)
	}
	return fmt.Sprintf("%s: context.output.%s", f.name, f.sourcePath)
}

// --- Predefined status/health builders for common workloads ---

// DaemonSetStatus returns a pre-configured status builder for DaemonSet.
func DaemonSetStatus() *StatusBuilder {
	return Status().
		IntField("ready.replicas", "status.numberReady", 0).
		IntField("desired.replicas", "status.desiredNumberScheduled", 0).
		Message(`Ready:\(ready.replicas)/\(desired.replicas)`)
}

// DaemonSetHealth returns a pre-configured health builder for DaemonSet.
func DaemonSetHealth() *HealthBuilder {
	return Health().
		IntField("ready.replicas", "status.numberReady", 0).
		IntField("desired.replicas", "status.desiredNumberScheduled", 0).
		IntField("current.replicas", "status.currentNumberScheduled", 0).
		IntField("updated.replicas", "status.updatedNumberScheduled", 0).
		MetadataField("generation.metadata", "metadata.generation").
		IntField("generation.observed", "status.observedGeneration", 0).
		HealthyWhen(
			StatusEq("desired.replicas", "ready.replicas"),
			StatusEq("desired.replicas", "updated.replicas"),
			StatusEq("desired.replicas", "current.replicas"),
			StatusOr(StatusEq("generation.observed", "generation.metadata"), "generation.observed > generation.metadata"),
		)
}

// DeploymentStatus returns a pre-configured status builder for Deployment.
// Matches the original CUE structure which uses spec.replicas for desired count.
func DeploymentStatus() *StatusBuilder {
	return Status().
		IntField("ready.readyReplicas", "status.readyReplicas", 0).
		Message(`Ready:\(ready.readyReplicas)/\(context.output.spec.replicas)`)
}

// DeploymentHealth returns a pre-configured health builder for Deployment.
// Uses flat structure matching the original CUE healthPolicy with all fields in single block.
func DeploymentHealth() *HealthBuilder {
	h := &HealthBuilder{
		StatusBuilder: Status(),
		conditions:    make([]string, 0),
	}
	// Use raw CUE that matches original exactly
	h.rawCUE = `ready: {
	updatedReplicas:    *0 | int
	readyReplicas:      *0 | int
	replicas:           *0 | int
	observedGeneration: *0 | int
} & {
	if context.output.status.updatedReplicas != _|_ {
		updatedReplicas: context.output.status.updatedReplicas
	}
	if context.output.status.readyReplicas != _|_ {
		readyReplicas: context.output.status.readyReplicas
	}
	if context.output.status.replicas != _|_ {
		replicas: context.output.status.replicas
	}
	if context.output.status.observedGeneration != _|_ {
		observedGeneration: context.output.status.observedGeneration
	}
}
_isHealth: (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas) && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)
isHealth: *_isHealth | bool
if context.output.metadata.annotations != _|_ {
	if context.output.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
		isHealth: true
	}
}`
	return h
}

// StatefulSetStatus returns a pre-configured status builder for StatefulSet.
func StatefulSetStatus() *StatusBuilder {
	return Status().
		IntField("ready.replicas", "status.readyReplicas", 0).
		IntField("desired.replicas", "status.replicas", 0).
		Message(`Ready:\(ready.replicas)/\(desired.replicas)`)
}

// StatefulSetHealth returns a pre-configured health builder for StatefulSet.
func StatefulSetHealth() *HealthBuilder {
	return Health().
		IntField("ready.replicas", "status.readyReplicas", 0).
		IntField("updated.replicas", "status.updatedReplicas", 0).
		IntField("desired.replicas", "status.replicas", 0).
		MetadataField("generation.metadata", "metadata.generation").
		IntField("generation.observed", "status.observedGeneration", 0).
		HealthyWhen(
			StatusEq("desired.replicas", "ready.replicas"),
			StatusEq("desired.replicas", "updated.replicas"),
			StatusOr(StatusEq("generation.observed", "generation.metadata"), "generation.observed > generation.metadata"),
		)
}

// JobHealth returns a pre-configured health builder for Job.
func JobHealth() *HealthBuilder {
	return Health().
		IntField("succeeded", "status.succeeded", 0).
		IntField("failed", "status.failed", 0).
		HealthyWhen("succeeded >= 1 || failed >= 1")
}

// CronJobHealth returns a pre-configured health builder for CronJob.
func CronJobHealth() *HealthBuilder {
	return Health().
		HealthyWhen("true") // CronJob is always considered healthy if it exists
}

// --- Composable Health Expression Methods on HealthBuilder ---
// These methods allow building health checks using composable expressions
// that are then converted to CUE via HealthyWhenExpr().

// Condition creates an expression to check a status condition.
// Example: Health().Condition("Ready").IsTrue()
func (h *HealthBuilder) Condition(condType string) *ConditionExpr {
	return &ConditionExpr{
		condType:       condType,
		expectedStatus: "True", // default
	}
}

// Field creates an expression builder for a field path.
// Example: Health().Field("status.replicas").Gt(0)
func (h *HealthBuilder) Field(path string) *HealthFieldExpr {
	return &HealthFieldExpr{path: path}
}

// FieldRef creates a reference to another field for field-to-field comparisons.
// Example: Health().Field("status.readyReplicas").Eq(Health().FieldRef("spec.replicas"))
func (h *HealthBuilder) FieldRef(path string) *HealthFieldRefExpr {
	return &HealthFieldRefExpr{path: path}
}

// Phase creates an expression to check if status.phase matches any of the given phases.
// Example: Health().Phase("Running", "Succeeded")
func (h *HealthBuilder) Phase(phases ...string) HealthExpression {
	return &phaseExpr{
		fieldPath: "context.output.status.phase",
		phases:    phases,
	}
}

// PhaseField creates an expression to check a custom phase field path.
// Example: Health().PhaseField("status.currentPhase", "Active", "Ready")
func (h *HealthBuilder) PhaseField(path string, phases ...string) HealthExpression {
	return &phaseExpr{
		fieldPath: "context.output." + path,
		phases:    phases,
	}
}

// Exists checks if a field exists (is not _|_).
// Example: Health().Exists("status.loadBalancer.ingress")
func (h *HealthBuilder) Exists(path string) HealthExpression {
	return &existsExpr{path: path, negate: false}
}

// NotExists checks if a field does not exist (is _|_).
// Example: Health().NotExists("status.error")
func (h *HealthBuilder) NotExists(path string) HealthExpression {
	return &existsExpr{path: path, negate: true}
}

// And combines multiple health expressions with AND.
// All expressions must be true for the health check to pass.
// Example: Health().And(expr1, expr2, expr3)
func (h *HealthBuilder) And(exprs ...HealthExpression) HealthExpression {
	return &andExpr{exprs: exprs}
}

// Or combines multiple health expressions with OR.
// Any expression being true makes the health check pass.
// Example: Health().Or(expr1, expr2)
func (h *HealthBuilder) Or(exprs ...HealthExpression) HealthExpression {
	return &orExpr{exprs: exprs}
}

// Not negates a health expression.
// Example: Health().Not(Health().Condition("Stalled").IsTrue())
func (h *HealthBuilder) Not(expr HealthExpression) HealthExpression {
	return &notExpr{expr: expr}
}

// Always returns a health expression that is always true.
// Use this when resource existence is the only health criteria.
// Example: Health().Always()
func (h *HealthBuilder) Always() HealthExpression {
	return &alwaysExpr{}
}

// AllTrue checks if all specified conditions have status "True".
// Example: Health().AllTrue("Ready", "Synced")
func (h *HealthBuilder) AllTrue(condTypes ...string) HealthExpression {
	exprs := make([]HealthExpression, len(condTypes))
	for i, ct := range condTypes {
		exprs[i] = h.Condition(ct).IsTrue()
	}
	return h.And(exprs...)
}

// AnyTrue checks if any of the specified conditions have status "True".
// Example: Health().AnyTrue("Ready", "Available")
func (h *HealthBuilder) AnyTrue(condTypes ...string) HealthExpression {
	exprs := make([]HealthExpression, len(condTypes))
	for i, ct := range condTypes {
		exprs[i] = h.Condition(ct).IsTrue()
	}
	return h.Or(exprs...)
}

// HealthyWhenExpr sets the health condition using a composable HealthExpression.
// This generates the appropriate preamble and isHealth expression.
// Example: Health().HealthyWhenExpr(Health().Condition("Ready").IsTrue())
func (h *HealthBuilder) HealthyWhenExpr(expr HealthExpression) *HealthBuilder {
	h.rawCUE = HealthPolicy(expr)
	return h
}

// Policy generates a complete healthPolicy CUE block from a HealthExpression.
// This is useful for generating standalone health policies without setting them on the builder.
// Example: Health().Policy(Health().Condition("Ready").IsTrue())
func (h *HealthBuilder) Policy(expr HealthExpression) string {
	return HealthPolicy(expr)
}
