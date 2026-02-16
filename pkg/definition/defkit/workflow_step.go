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

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// WorkflowStepDefinition represents a KubeVela WorkflowStepDefinition.
// Workflow steps define operations in an application's deployment workflow,
// such as deploy, suspend, notification, approval, etc.
type WorkflowStepDefinition struct {
	baseDefinition                                 // embedded common fields and methods
	category       string                          // e.g., "Application Delivery", "Notification"
	scope          string                          // e.g., "Application", "Workflow"
	stepTemplate   func(tpl *WorkflowStepTemplate) // template function for step logic (type-specific)
}

// WorkflowStepTemplate provides the building context for workflow step templates.
// Workflow steps typically use vela builtins and may have conditional logic.
type WorkflowStepTemplate struct {
	actions    []WorkflowAction
	suspendMsg string // for suspend steps
}

// WorkflowAction represents an action in a workflow step.
type WorkflowAction interface {
	isWorkflowAction()
}

// BuiltinAction represents a call to a vela builtin.
type BuiltinAction struct {
	name   string           // e.g., "multicluster.#Deploy", "builtin.#Suspend"
	params map[string]Value // parameters to pass
}

func (b *BuiltinAction) isWorkflowAction() {}

// ConditionalAction represents a conditional workflow action.
type ConditionalAction struct {
	cond   Condition
	action WorkflowAction
}

func (c *ConditionalAction) isWorkflowAction() {}

// NewWorkflowStep creates a new WorkflowStepDefinition builder.
func NewWorkflowStep(name string) *WorkflowStepDefinition {
	return &WorkflowStepDefinition{
		baseDefinition: baseDefinition{
			name:   name,
			params: make([]Param, 0),
		},
	}
}

// Description sets the workflow step description.
func (w *WorkflowStepDefinition) Description(desc string) *WorkflowStepDefinition {
	w.setDescription(desc)
	return w
}

// Category sets the workflow step category (shown in annotations).
// Common values: "Application Delivery", "Notification", "Approval"
func (w *WorkflowStepDefinition) Category(category string) *WorkflowStepDefinition {
	w.category = category
	return w
}

// Scope sets the workflow step scope (shown in labels).
// Common values: "Application", "Workflow"
func (w *WorkflowStepDefinition) Scope(scope string) *WorkflowStepDefinition {
	w.scope = scope
	return w
}

// Params adds multiple parameter definitions to the workflow step.
func (w *WorkflowStepDefinition) Params(params ...Param) *WorkflowStepDefinition {
	w.addParams(params...)
	return w
}

// Param adds a single parameter definition to the workflow step.
// This provides a more fluent API when adding parameters one at a time.
func (w *WorkflowStepDefinition) Param(param Param) *WorkflowStepDefinition {
	w.addParams(param)
	return w
}

// Template sets the template function for the workflow step.
func (w *WorkflowStepDefinition) Template(fn func(tpl *WorkflowStepTemplate)) *WorkflowStepDefinition {
	w.stepTemplate = fn
	return w
}

// RawCUE sets raw CUE for complex workflow step definitions that don't fit the builder pattern.
func (w *WorkflowStepDefinition) RawCUE(cue string) *WorkflowStepDefinition {
	w.setRawCUE(cue)
	return w
}

// Helper adds a helper type definition using fluent API.
// The param defines the schema for the helper type.
// Example:
//
//	Helper("Placement", defkit.Struct("placement").Fields(...))
func (w *WorkflowStepDefinition) Helper(name string, param Param) *WorkflowStepDefinition {
	w.addHelper(name, param)
	return w
}

// Note: GetHelperDefinitions() is inherited from baseDefinition

// WithImports adds CUE imports to the workflow step definition.
// Common imports: "vela/multicluster", "vela/builtin"
func (w *WorkflowStepDefinition) WithImports(imports ...string) *WorkflowStepDefinition {
	w.addImports(imports...)
	return w
}

// CustomStatus sets the custom status CUE expression for the workflow step.
// This provides status visibility in the workflow execution.
func (w *WorkflowStepDefinition) CustomStatus(expr string) *WorkflowStepDefinition {
	w.setCustomStatus(expr)
	return w
}

// HealthPolicy sets the health policy CUE expression for the workflow step.
// This defines how the step's health is determined.
func (w *WorkflowStepDefinition) HealthPolicy(expr string) *WorkflowStepDefinition {
	w.setHealthPolicy(expr)
	return w
}

// HealthPolicyExpr sets the health policy using a composable HealthExpression.
func (w *WorkflowStepDefinition) HealthPolicyExpr(expr HealthExpression) *WorkflowStepDefinition {
	w.setHealthPolicyExpr(expr)
	return w
}

// RunOn adds placement conditions specifying which clusters this workflow step should run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewWorkflowStep("eks-deploy").
//	    RunOn(placement.Label("provider").Eq("aws"))
//
// Multiple RunOn calls are combined with AND semantics (all conditions must match).
func (w *WorkflowStepDefinition) RunOn(conditions ...placement.Condition) *WorkflowStepDefinition {
	w.addRunOn(conditions...)
	return w
}

// NotRunOn adds placement conditions specifying which clusters this workflow step should NOT run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewWorkflowStep("no-vclusters").
//	    NotRunOn(placement.Label("cluster-type").Eq("vcluster"))
//
// If any NotRunOn condition matches, the workflow step is ineligible for that cluster.
func (w *WorkflowStepDefinition) NotRunOn(conditions ...placement.Condition) *WorkflowStepDefinition {
	w.addNotRunOn(conditions...)
	return w
}

// DefName implements Definition.DefName.
func (w *WorkflowStepDefinition) DefName() string { return w.GetName() }

// DefType implements Definition.DefType.
func (w *WorkflowStepDefinition) DefType() DefinitionType { return DefinitionTypeWorkflowStep }

// Note: GetName(), GetDescription(), GetParams(), GetHelperDefinitions(),
// GetRawCUE(), GetImports(), GetCustomStatus(), GetHealthPolicy()
// are all inherited from baseDefinition

// GetCategory returns the workflow step category.
func (w *WorkflowStepDefinition) GetCategory() string { return w.category }

// GetScope returns the workflow step scope.
func (w *WorkflowStepDefinition) GetScope() string { return w.scope }

// ToCue generates the complete CUE definition string for this workflow step.
func (w *WorkflowStepDefinition) ToCue() string {
	// If raw CUE is set, use it with the name from NewWorkflowStep() taking precedence
	if w.HasRawCUE() {
		return w.GetRawCUEWithName()
	}

	gen := NewWorkflowStepCUEGenerator()
	if len(w.GetImports()) > 0 {
		gen.WithImports(w.GetImports()...)
	}
	return gen.GenerateFullDefinition(w)
}

// ToYAML generates the Kubernetes YAML representation of the WorkflowStepDefinition.
func (w *WorkflowStepDefinition) ToYAML() ([]byte, error) {
	cueStr := w.ToCue()

	// Build the WorkflowStepDefinition CR structure
	cr := map[string]any{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "WorkflowStepDefinition",
		"metadata": map[string]any{
			"name": w.GetName(),
			"annotations": map[string]any{
				"definition.oam.dev/description": w.GetDescription(),
			},
		},
		"spec": map[string]any{
			"schematic": map[string]any{
				"cue": map[string]any{
					"template": cueStr,
				},
			},
		},
	}

	return yaml.Marshal(cr)
}

// --- WorkflowStepTemplate methods ---

// NewWorkflowStepTemplate creates a new workflow step template.
func NewWorkflowStepTemplate() *WorkflowStepTemplate {
	return &WorkflowStepTemplate{
		actions: make([]WorkflowAction, 0),
	}
}

// Builtin adds a call to a vela builtin.
// Example: tpl.Builtin("deploy", "multicluster.#Deploy").WithParams(...)
func (wt *WorkflowStepTemplate) Builtin(name, builtinRef string) *BuiltinActionBuilder {
	action := &BuiltinAction{
		name:   builtinRef,
		params: make(map[string]Value),
	}
	return &BuiltinActionBuilder{
		template: wt,
		action:   action,
		varName:  name,
	}
}

// Suspend adds a suspend action.
// Example: tpl.Suspend("Waiting for approval")
func (wt *WorkflowStepTemplate) Suspend(message string) *WorkflowStepTemplate {
	wt.suspendMsg = message
	return wt
}

// SuspendIf adds a conditional suspend action.
// Example: tpl.SuspendIf(param.Eq(false), "Waiting for approval")
func (wt *WorkflowStepTemplate) SuspendIf(cond Condition, message string) *WorkflowStepTemplate {
	wt.actions = append(wt.actions, &ConditionalAction{
		cond: cond,
		action: &BuiltinAction{
			name: "builtin.#Suspend",
			params: map[string]Value{
				"message": Lit(message),
			},
		},
	})
	return wt
}

// GetActions returns all actions.
func (wt *WorkflowStepTemplate) GetActions() []WorkflowAction { return wt.actions }

// GetSuspendMessage returns the suspend message.
func (wt *WorkflowStepTemplate) GetSuspendMessage() string { return wt.suspendMsg }

// BuiltinActionBuilder builds a builtin action.
type BuiltinActionBuilder struct {
	template *WorkflowStepTemplate
	action   *BuiltinAction
	varName  string
	cond     Condition // optional condition set by If()
}

// WithParams sets parameters for the builtin.
func (b *BuiltinActionBuilder) WithParams(params map[string]Value) *BuiltinActionBuilder {
	for k, v := range params {
		b.action.params[k] = v
	}
	return b
}

// Build finalizes the action and adds it to the template.
func (b *BuiltinActionBuilder) Build() *WorkflowStepTemplate {
	if b.cond != nil {
		b.template.actions = append(b.template.actions, &ConditionalAction{
			cond:   b.cond,
			action: b.action,
		})
	} else {
		b.template.actions = append(b.template.actions, b.action)
	}
	return b.template
}

// If makes this action conditional.
// The condition is applied when Build() is called.
func (b *BuiltinActionBuilder) If(cond Condition) *BuiltinActionBuilder {
	b.cond = cond
	return b
}

// --- WorkflowStepCUEGenerator ---

// WorkflowStepCUEGenerator generates CUE definitions for workflow steps.
type WorkflowStepCUEGenerator struct {
	indent  string
	imports []string
}

// NewWorkflowStepCUEGenerator creates a new workflow step CUE generator.
func NewWorkflowStepCUEGenerator() *WorkflowStepCUEGenerator {
	return &WorkflowStepCUEGenerator{
		indent:  "\t",
		imports: []string{},
	}
}

// WithImports adds CUE imports.
func (g *WorkflowStepCUEGenerator) WithImports(imports ...string) *WorkflowStepCUEGenerator {
	g.imports = append(g.imports, imports...)
	return g
}

// GenerateFullDefinition generates the complete CUE definition for a workflow step.
func (g *WorkflowStepCUEGenerator) GenerateFullDefinition(w *WorkflowStepDefinition) string {
	var sb strings.Builder

	// Write imports if any
	if len(g.imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range g.imports {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Write workflow step header - quote names with special characters
	name := w.GetName()
	if strings.ContainsAny(name, "-./") {
		name = fmt.Sprintf("%q", name)
	}
	sb.WriteString(fmt.Sprintf("%s: {\n", name))
	sb.WriteString(fmt.Sprintf("%stype: \"workflow-step\"\n", g.indent))

	// Write annotations (category)
	sb.WriteString(fmt.Sprintf("%sannotations: {\n", g.indent))
	if w.GetCategory() != "" {
		sb.WriteString(fmt.Sprintf("%s\t\"category\": %q\n", g.indent, w.GetCategory()))
	}
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))

	// Write labels (scope)
	sb.WriteString(fmt.Sprintf("%slabels: {\n", g.indent))
	if w.GetScope() != "" {
		sb.WriteString(fmt.Sprintf("%s\t\"scope\": %q\n", g.indent, w.GetScope()))
	}
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))

	sb.WriteString(fmt.Sprintf("%sdescription: %q\n", g.indent, w.GetDescription()))
	sb.WriteString("}\n")

	// Write template section
	sb.WriteString(g.GenerateTemplate(w))

	return sb.String()
}

// GenerateTemplate generates the template block for a workflow step.
func (g *WorkflowStepCUEGenerator) GenerateTemplate(w *WorkflowStepDefinition) string {
	var sb strings.Builder
	sb.WriteString("template: {\n")

	gen := NewCUEGenerator()

	// Generate helper type definitions
	for _, helperDef := range w.GetHelperDefinitions() {
		gen.WriteHelperDefinition(&sb, helperDef, 1)
	}

	// Execute template function if provided
	if w.stepTemplate != nil {
		wt := NewWorkflowStepTemplate()
		w.stepTemplate(wt)

		// Write actions
		g.writeActions(&sb, wt, 1)
	}

	// Generate parameter section
	sb.WriteString(g.generateParameterBlock(w, 1))

	sb.WriteString("}\n")
	return sb.String()
}

// writeActions writes the workflow actions.
func (g *WorkflowStepCUEGenerator) writeActions(sb *strings.Builder, wt *WorkflowStepTemplate, depth int) {
	indent := strings.Repeat(g.indent, depth)
	gen := NewCUEGenerator()

	for _, action := range wt.GetActions() {
		switch a := action.(type) {
		case *BuiltinAction:
			g.writeBuiltinAction(sb, a, "", indent, gen)
		case *ConditionalAction:
			condStr := gen.conditionToCUE(a.cond)
			sb.WriteString(fmt.Sprintf("%sif %s {\n", indent, condStr))
			if builtin, ok := a.action.(*BuiltinAction); ok {
				g.writeBuiltinAction(sb, builtin, "\t", indent, gen)
			}
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	}
}

// writeBuiltinAction writes a builtin action.
func (g *WorkflowStepCUEGenerator) writeBuiltinAction(sb *strings.Builder, a *BuiltinAction, extraIndent, indent string, gen *CUEGenerator) {
	// Extract the action name from the builtin reference
	// e.g., "multicluster.#Deploy" -> "deploy"
	actionName := extractActionName(a.name)

	sb.WriteString(fmt.Sprintf("%s%s%s: %s & {\n", indent, extraIndent, actionName, a.name))
	if len(a.params) > 0 {
		sb.WriteString(fmt.Sprintf("%s%s\t$params: {\n", indent, extraIndent))
		for paramName, paramVal := range a.params {
			sb.WriteString(fmt.Sprintf("%s%s\t\t%s: %s\n", indent, extraIndent, paramName, gen.valueToCUE(paramVal)))
		}
		sb.WriteString(fmt.Sprintf("%s%s\t}\n", indent, extraIndent))
	}
	sb.WriteString(fmt.Sprintf("%s%s}\n", indent, extraIndent))
}

// extractActionName extracts a simple action name from a builtin reference.
func extractActionName(builtinRef string) string {
	// "multicluster.#Deploy" -> "deploy"
	// "builtin.#Suspend" -> "suspend"
	parts := strings.Split(builtinRef, "#")
	if len(parts) == 2 {
		return strings.ToLower(parts[1])
	}
	return strings.ToLower(builtinRef)
}

// generateParameterBlock generates the parameter schema for the workflow step.
func (g *WorkflowStepCUEGenerator) generateParameterBlock(w *WorkflowStepDefinition, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)

	sb.WriteString(fmt.Sprintf("%sparameter: {\n", indent))

	gen := NewCUEGenerator()
	for _, param := range w.GetParams() {
		gen.writeParam(&sb, param, depth+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return sb.String()
}
