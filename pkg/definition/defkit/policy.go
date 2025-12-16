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
)

// PolicyDefinition represents a KubeVela PolicyDefinition.
// Policies define application-level behaviors such as topology (where to deploy),
// override (how to customize components), and health (how to check application health).
type PolicyDefinition struct {
	baseDefinition                             // embedded common fields and methods
	policyTemplate func(tpl *PolicyTemplate)  // template function for policy logic (type-specific)
}

// PolicyTemplate provides the building context for policy templates.
// Policies typically just define parameters and let the vela runtime handle the logic.
type PolicyTemplate struct {
	// Policies usually don't have output resources, but may have computed values
	computedFields map[string]Value
}

// NewPolicy creates a new PolicyDefinition builder.
func NewPolicy(name string) *PolicyDefinition {
	return &PolicyDefinition{
		baseDefinition: baseDefinition{
			name:   name,
			params: make([]Param, 0),
		},
	}
}

// Description sets the policy description.
func (p *PolicyDefinition) Description(desc string) *PolicyDefinition {
	p.setDescription(desc)
	return p
}

// Params adds multiple parameter definitions to the policy.
func (p *PolicyDefinition) Params(params ...Param) *PolicyDefinition {
	p.addParams(params...)
	return p
}

// Param adds a single parameter definition to the policy.
// This provides a more fluent API when adding parameters one at a time.
func (p *PolicyDefinition) Param(param Param) *PolicyDefinition {
	p.addParams(param)
	return p
}

// Template sets the template function for the policy.
// Most policies only need parameters, but this allows for computed values.
func (p *PolicyDefinition) Template(fn func(tpl *PolicyTemplate)) *PolicyDefinition {
	p.policyTemplate = fn
	return p
}

// RawCUE sets raw CUE for complex policy definitions that don't fit the builder pattern.
func (p *PolicyDefinition) RawCUE(cue string) *PolicyDefinition {
	p.setRawCUE(cue)
	return p
}

// Helper adds a helper type definition using fluent API.
// The param defines the schema for the helper type.
// Example:
//
//	Helper("RuleSelector", defkit.Struct("selector").Fields(...))
func (p *PolicyDefinition) Helper(name string, param Param) *PolicyDefinition {
	p.addHelper(name, param)
	return p
}

// Note: GetHelperDefinitions() is inherited from baseDefinition

// WithImports adds CUE imports to the policy definition.
func (p *PolicyDefinition) WithImports(imports ...string) *PolicyDefinition {
	p.addImports(imports...)
	return p
}

// CustomStatus sets the custom status CUE expression for the policy.
// This provides status visibility in the application status.
func (p *PolicyDefinition) CustomStatus(expr string) *PolicyDefinition {
	p.setCustomStatus(expr)
	return p
}

// HealthPolicy sets the health policy CUE expression for the policy.
// This defines how the policy's health is determined.
func (p *PolicyDefinition) HealthPolicy(expr string) *PolicyDefinition {
	p.setHealthPolicy(expr)
	return p
}

// HealthPolicyExpr sets the health policy using a composable HealthExpression.
func (p *PolicyDefinition) HealthPolicyExpr(expr HealthExpression) *PolicyDefinition {
	p.setHealthPolicyExpr(expr)
	return p
}

// DefName implements Definition.DefName.
func (p *PolicyDefinition) DefName() string { return p.GetName() }

// DefType implements Definition.DefType.
func (p *PolicyDefinition) DefType() DefinitionType { return DefinitionTypePolicy }

// Note: GetName(), GetDescription(), GetParams(), GetHelperDefinitions(),
// GetRawCUE(), GetImports(), GetCustomStatus(), GetHealthPolicy()
// are all inherited from baseDefinition

// ToCue generates the complete CUE definition string for this policy.
func (p *PolicyDefinition) ToCue() string {
	// If raw CUE is set, use it with the name from NewPolicy() taking precedence
	if p.HasRawCUE() {
		return p.GetRawCUEWithName()
	}

	gen := NewPolicyCUEGenerator()
	if len(p.GetImports()) > 0 {
		gen.WithImports(p.GetImports()...)
	}
	return gen.GenerateFullDefinition(p)
}

// ToYAML generates the Kubernetes YAML representation of the PolicyDefinition.
func (p *PolicyDefinition) ToYAML() ([]byte, error) {
	cueStr := p.ToCue()

	// Build the PolicyDefinition CR structure
	cr := map[string]any{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "PolicyDefinition",
		"metadata": map[string]any{
			"name": p.GetName(),
			"annotations": map[string]any{
				"definition.oam.dev/description": p.GetDescription(),
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

// --- PolicyTemplate methods ---

// NewPolicyTemplate creates a new policy template.
func NewPolicyTemplate() *PolicyTemplate {
	return &PolicyTemplate{
		computedFields: make(map[string]Value),
	}
}

// SetField sets a computed field value.
func (pt *PolicyTemplate) SetField(name string, value Value) *PolicyTemplate {
	pt.computedFields[name] = value
	return pt
}

// GetComputedFields returns all computed fields.
func (pt *PolicyTemplate) GetComputedFields() map[string]Value {
	return pt.computedFields
}

// --- PolicyCUEGenerator ---

// PolicyCUEGenerator generates CUE definitions for policies.
type PolicyCUEGenerator struct {
	indent  string
	imports []string
}

// NewPolicyCUEGenerator creates a new policy CUE generator.
func NewPolicyCUEGenerator() *PolicyCUEGenerator {
	return &PolicyCUEGenerator{
		indent:  "\t",
		imports: []string{},
	}
}

// WithImports adds CUE imports.
func (g *PolicyCUEGenerator) WithImports(imports ...string) *PolicyCUEGenerator {
	g.imports = append(g.imports, imports...)
	return g
}

// GenerateFullDefinition generates the complete CUE definition for a policy.
func (g *PolicyCUEGenerator) GenerateFullDefinition(p *PolicyDefinition) string {
	var sb strings.Builder

	// Write imports if any
	if len(g.imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range g.imports {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Write policy header - quote names with special characters
	name := p.GetName()
	if strings.ContainsAny(name, "-./") {
		name = fmt.Sprintf("%q", name)
	}
	sb.WriteString(fmt.Sprintf("%s: {\n", name))
	sb.WriteString(fmt.Sprintf("%sannotations: {}\n", g.indent))
	sb.WriteString(fmt.Sprintf("%sdescription: %q\n", g.indent, p.GetDescription()))
	sb.WriteString(fmt.Sprintf("%slabels: {}\n", g.indent))
	sb.WriteString(fmt.Sprintf("%sattributes: {}\n", g.indent))
	sb.WriteString(fmt.Sprintf("%stype: \"policy\"\n", g.indent))
	sb.WriteString("}\n\n")

	// Write template section
	sb.WriteString(g.GenerateTemplate(p))

	return sb.String()
}

// GenerateTemplate generates the template block for a policy.
func (g *PolicyCUEGenerator) GenerateTemplate(p *PolicyDefinition) string {
	var sb strings.Builder
	sb.WriteString("template: {\n")

	gen := NewCUEGenerator()

	// Generate helper type definitions (like #RuleSelector, #ApplyOnceStrategy)
	for _, helperDef := range p.GetHelperDefinitions() {
		gen.WriteHelperDefinition(&sb, helperDef, 1)
	}

	// Execute template function if provided
	if p.policyTemplate != nil {
		pt := NewPolicyTemplate()
		p.policyTemplate(pt)

		// Write computed fields
		for name, val := range pt.GetComputedFields() {
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", g.indent, name, gen.valueToCUE(val)))
		}
	}

	// Generate parameter section
	sb.WriteString(g.generateParameterBlock(p, 1))

	sb.WriteString("}\n")
	return sb.String()
}

// generateParameterBlock generates the parameter schema for the policy.
func (g *PolicyCUEGenerator) generateParameterBlock(p *PolicyDefinition, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat(g.indent, depth)

	sb.WriteString(fmt.Sprintf("%sparameter: {\n", indent))

	gen := NewCUEGenerator()
	for _, param := range p.GetParams() {
		gen.writeParam(&sb, param, depth+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return sb.String()
}
