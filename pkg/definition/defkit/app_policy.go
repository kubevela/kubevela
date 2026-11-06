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
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ApplicationScoped makes the policy transform the Application before it is
// rendered, through the template's Output, instead of producing resources.
func (p *PolicyDefinition) ApplicationScoped() *PolicyDefinition {
	p.scope = v1beta1.ApplicationScope
	return p
}

// Global applies the policy to every Application in the namespace it is
// installed in, or to every Application when installed in vela-system, without
// being listed in spec.policies. A global policy is always Application-scoped.
func (p *PolicyDefinition) Global() *PolicyDefinition {
	p.scope = v1beta1.ApplicationScope
	p.global = true
	return p
}

// Priority orders the policy among the Application-scoped policies that apply
// to an Application: lower runs first, and ties run in name order.
func (p *PolicyDefinition) Priority(priority int32) *PolicyDefinition {
	p.priority = &priority
	return p
}

// IsApplicationScoped reports whether the policy transforms the Application.
func (p *PolicyDefinition) IsApplicationScoped() bool {
	return p.scope == v1beta1.ApplicationScope
}

// IsGlobal reports whether the policy applies without being listed.
func (p *PolicyDefinition) IsGlobal() bool { return p.global }

// GetPriority returns the priority, or nil when unset.
func (p *PolicyDefinition) GetPriority() *int32 { return p.priority }

// CtxSchema declares the shape of what the policy puts in ctx, which
// components and traits read as context.custom: admission validates them
// against it, and the controller refuses an output that does not match. A
// field is required, the policy certain to set it, unless it is Optional().
// The schema is open: the policy may set keys it does not declare.
func (p *PolicyDefinition) CtxSchema(fields ...Param) *PolicyDefinition {
	p.ctxSchema = append(p.ctxSchema, fields...)
	return p
}

// ClosedCtxSchema declares the schema as CtxSchema does, closed: the policy
// sets no key it does not declare, so admission can refuse a component reading
// one, and Register refuses an Output().Ctx it does not declare.
func (p *PolicyDefinition) ClosedCtxSchema(fields ...Param) *PolicyDefinition {
	p.ctxSchemaClosed = true
	return p.CtxSchema(fields...)
}

// writeCtxSchema writes the schema.ctx block, the fields as a parameter block
// writes them.
func (g *PolicyCUEGenerator) writeCtxSchema(sb *strings.Builder, gen *CUEGenerator, p *PolicyDefinition) {
	if len(p.ctxSchema) == 0 && !p.ctxSchemaClosed {
		return
	}
	open, end := "{", "}"
	if p.ctxSchemaClosed {
		open, end = "close({", "})"
	}
	sb.WriteString(fmt.Sprintf("%sschema: ctx: %s\n", g.indent, open))
	for _, f := range p.ctxSchema {
		gen.writeParam(sb, f, 2)
	}
	sb.WriteString(fmt.Sprintf("%s%s\n", g.indent, end))
}

// validateDefinition refuses settings the controller only honours on an
// Application-scoped policy, and an output a closed context schema refuses.
func (p *PolicyDefinition) validateDefinition() error {
	if p.IsApplicationScoped() {
		return p.validateCtxOutput()
	}
	var used []string
	if p.priority != nil {
		used = append(used, "Priority")
	}
	if len(p.ctxSchema) > 0 {
		used = append(used, "CtxSchema")
	}
	if p.policyTemplate != nil {
		pt := NewPolicyTemplate()
		p.policyTemplate(pt)
		if pt.output != nil {
			used = append(used, "Output")
		}
		if pt.enabled != nil {
			used = append(used, "Enabled")
		}
	}
	if len(used) == 0 {
		return nil
	}
	return fmt.Errorf("%s only apply to an Application-scoped policy; call ApplicationScoped()", strings.Join(used, ", "))
}

// validateCtxOutput refuses an Output().Ctx whose top-level key a closed
// schema does not declare: the controller would refuse every render.
func (p *PolicyDefinition) validateCtxOutput() error {
	if !p.ctxSchemaClosed || p.policyTemplate == nil {
		return nil
	}
	declared := map[string]bool{}
	for _, f := range p.ctxSchema {
		declared[f.Name()] = true
	}
	pt := NewPolicyTemplate()
	p.policyTemplate(pt)
	if pt.output == nil {
		return nil
	}
	for _, op := range pt.output.res.Ops() {
		set, ok := op.(*SetOp)
		if !ok || !strings.HasPrefix(set.path, "ctx.") {
			continue
		}
		path := strings.TrimPrefix(set.path, "ctx.")
		if !declared[topKey(path)] {
			return fmt.Errorf("Output().Ctx(%q) is not declared by the closed CtxSchema", path)
		}
	}
	return nil
}

// topKey is the first key of an output path, which ends at a "." or a "["
// opening a map key.
func topKey(path string) string {
	if i := strings.IndexAny(path, ".["); i >= 0 {
		return path[:i]
	}
	return path
}

// writeAppScopeAttributes writes scope, global and priority as attributes.
func writeAppScopeAttributes(sb *strings.Builder, p *PolicyDefinition, indent string) {
	if p.scope != "" {
		sb.WriteString(fmt.Sprintf("%sscope: %q\n", indent, p.scope))
	}
	if p.global {
		sb.WriteString(fmt.Sprintf("%sglobal: true\n", indent))
	}
	if p.priority != nil {
		sb.WriteString(fmt.Sprintf("%spriority: %d\n", indent, *p.priority))
	}
}

// --- Template ---

// Enabled turns the policy on only when the condition holds. It is written as
// config.enabled: the PolicyDefinition webhook accepts only a literal for a
// root-level enabled.
func (pt *PolicyTemplate) Enabled(cond Condition) *PolicyTemplate {
	pt.enabled = cond
	return pt
}

// Output returns the builder for what the policy changes on the Application.
func (pt *PolicyTemplate) Output() *PolicyOutput {
	if pt.output == nil {
		pt.output = &PolicyOutput{res: &Resource{}}
	}
	return pt.output
}

// PolicyOutput builds an Application-scoped policy's output. It offers only the
// fields the controller accepts there.
type PolicyOutput struct {
	res *Resource
}

// Components replaces the Application's components. An empty list leaves them
// as they are: the controller applies output.components only when it has entries.
func (o *PolicyOutput) Components(v Value) *PolicyOutput {
	o.res.Set("components", v)
	return o
}

// Workflow replaces the Application's workflow.
func (o *PolicyOutput) Workflow(v Value) *PolicyOutput {
	o.res.Set("workflow", v)
	return o
}

// Policies replaces the Application's policies. As with Components, an empty
// list leaves them as they are.
func (o *PolicyOutput) Policies(v Value) *PolicyOutput {
	o.res.Set("policies", v)
	return o
}

// Label sets a label on the Application.
func (o *PolicyOutput) Label(key string, v Value) *PolicyOutput {
	o.res.Set("labels["+key+"]", v)
	return o
}

// Annotation sets an annotation on the Application.
func (o *PolicyOutput) Annotation(key string, v Value) *PolicyOutput {
	o.res.Set("annotations["+key+"]", v)
	return o
}

// Ctx sets a dotted path under ctx, which components and traits read as
// context.custom.<path>.
func (o *PolicyOutput) Ctx(path string, v Value) *PolicyOutput {
	o.res.Set("ctx."+path, v)
	return o
}

// writePolicyOutput writes the output block from its field operations.
func (g *PolicyCUEGenerator) writePolicyOutput(sb *strings.Builder, gen *CUEGenerator, o *PolicyOutput) {
	sb.WriteString(fmt.Sprintf("%soutput: {\n", g.indent))
	gen.writeFieldTree(sb, gen.buildFieldTree(o.res.Ops()), 2)
	sb.WriteString(fmt.Sprintf("%s}\n", g.indent))
}

// --- Components transform ---

// AppComponentsTransform rewrites every component of the Application, for use
// with PolicyOutput.Components.
type AppComponentsTransform struct {
	traits []appendedTrait
}

type appendedTrait struct {
	typ        string
	properties map[string]Value
}

func (t *AppComponentsTransform) value() {}
func (t *AppComponentsTransform) expr()  {}

// AppComponents starts a transform of context.appComponents that keeps each
// component as written.
func AppComponents() *AppComponentsTransform {
	return &AppComponentsTransform{}
}

// AddTrait appends a trait to every component, after the traits it already has.
func (t *AppComponentsTransform) AddTrait(traitType string, properties map[string]Value) *AppComponentsTransform {
	t.traits = append(t.traits, appendedTrait{typ: traitType, properties: properties})
	return t
}

// RenderCUE implements CUERenderer.
func (t *AppComponentsTransform) RenderCUE(rv func(Value) string) string {
	if len(t.traits) == 0 {
		return "context.appComponents"
	}
	traits := []string{"for t in *c.traits | [] {t}"}
	for _, tr := range t.traits {
		keys := make([]string, 0, len(tr.properties))
		for k := range tr.properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props := make([]string, 0, len(keys))
		for _, k := range keys {
			props = append(props, fmt.Sprintf("%s: %s", propertyLabel(k), rv(tr.properties[k])))
		}
		traits = append(traits, fmt.Sprintf("{type: %q, properties: {%s}}", tr.typ, strings.Join(props, ", ")))
	}
	return fmt.Sprintf(`[for c in context.appComponents {{for k, v in c if k != "traits" {(k): v}}, traits: [%s]}]`,
		strings.Join(traits, ", "))
}

// propertyLabel writes a trait property key as a CUE label. Any key that is not
// a plain identifier is quoted, since "#" and "_" would make a definition or a
// hidden field of it.
func propertyLabel(k string) string {
	if ast.IsValidIdent(k) && !strings.HasPrefix(k, "#") && !strings.HasPrefix(k, "_") {
		return k
	}
	return strconv.Quote(k)
}
