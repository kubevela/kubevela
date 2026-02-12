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
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// ComponentDefinition represents a KubeVela ComponentDefinition.
//
// ComponentDefinition embeds baseDefinition for common fields and methods shared
// with TraitDefinition and other definition types.
type ComponentDefinition struct {
	baseDefinition // embedded common fields (name, description, params, template, etc.)
	workload       WorkloadType
}

// HelperDefinition represents a CUE helper type definition like #HealthProbe.
type HelperDefinition struct {
	name   string // e.g., "HealthProbe" (without the #)
	schema string // Raw CUE schema (deprecated: prefer param)
	param  Param  // Param-based schema definition (preferred)
}

// HasParam returns true if this helper definition uses a Param.
func (h HelperDefinition) HasParam() bool { return h.param != nil }

// GetParam returns the Param for this helper definition.
func (h HelperDefinition) GetParam() Param { return h.param }

// GetSchema returns the raw CUE schema.
func (h HelperDefinition) GetSchema() string { return h.schema }

// GetName returns the helper definition name.
func (h HelperDefinition) GetName() string { return h.name }

// WorkloadType represents the workload type for a component.
type WorkloadType struct {
	apiVersion string
	kind       string
	autodetect bool // when true, uses "autodetects.core.oam.dev" without definition block
}

// NewComponent creates a new ComponentDefinition builder.
func NewComponent(name string) *ComponentDefinition {
	return &ComponentDefinition{
		baseDefinition: baseDefinition{
			name:   name,
			params: make([]Param, 0),
		},
	}
}

// Description sets the component description.
func (c *ComponentDefinition) Description(desc string) *ComponentDefinition {
	c.setDescription(desc)
	return c
}

// Workload sets the workload type for this component.
func (c *ComponentDefinition) Workload(apiVersion, kind string) *ComponentDefinition {
	c.workload = WorkloadType{apiVersion: apiVersion, kind: kind}
	return c
}

// AutodetectWorkload sets the workload type to "autodetects.core.oam.dev".
// This is used for components where the workload type is auto-detected at runtime
// rather than being statically defined.
func (c *ComponentDefinition) AutodetectWorkload() *ComponentDefinition {
	c.workload = WorkloadType{autodetect: true}
	return c
}

// Params adds multiple parameter definitions to the component.
func (c *ComponentDefinition) Params(params ...Param) *ComponentDefinition {
	c.addParams(params...)
	return c
}

// Param adds a single parameter definition to the component.
// This provides a more fluent API when adding parameters one at a time.
func (c *ComponentDefinition) Param(param Param) *ComponentDefinition {
	c.addParams(param)
	return c
}

// Template sets the template function for the component.
func (c *ComponentDefinition) Template(fn func(tpl *Template)) *ComponentDefinition {
	c.setTemplate(fn)
	return c
}

// CustomStatus sets the custom status CUE expression for the component.
// This expression is used to compute a human-readable status message.
func (c *ComponentDefinition) CustomStatus(expr string) *ComponentDefinition {
	c.setCustomStatus(expr)
	return c
}

// HealthPolicy sets the health policy CUE expression for the component.
// This expression determines whether the component is healthy.
// For raw CUE strings, use this method directly.
// For composable health expressions, use HealthPolicyExpr with Health().
func (c *ComponentDefinition) HealthPolicy(expr string) *ComponentDefinition {
	c.setHealthPolicy(expr)
	return c
}

// HealthPolicyExpr sets the health policy using a composable HealthExpression.
// This allows building health checks programmatically using primitives like
// Condition, Field, Phase, Exists, And, Or, Not via Health().
//
// Example:
//
//	h := defkit.Health()
//	def.HealthPolicyExpr(h.Condition("Ready").IsTrue())
//	def.HealthPolicyExpr(h.AllTrue("Ready", "Synced"))
//	def.HealthPolicyExpr(h.And(
//	    h.Condition("Ready").IsTrue(),
//	    h.Field("status.replicas").Gt(0),
//	))
func (c *ComponentDefinition) HealthPolicyExpr(expr HealthExpression) *ComponentDefinition {
	c.setHealthPolicyExpr(expr)
	return c
}

// DefName implements Definition.DefName - returns the definition name.
func (c *ComponentDefinition) DefName() string { return c.name }

// DefType implements Definition.DefType - returns the definition type.
func (c *ComponentDefinition) DefType() DefinitionType { return DefinitionTypeComponent }

// GetWorkload returns the workload type.
func (c *ComponentDefinition) GetWorkload() WorkloadType { return c.workload }

// Helper adds a helper type definition using fluent API.
// The param defines the schema for the helper type.
// Example:
//
//	Helper("HealthProbe", defkit.Object("probe").WithFields(...))
func (c *ComponentDefinition) Helper(name string, param Param) *ComponentDefinition {
	c.addHelper(name, param)
	return c
}

// RawCUE sets raw CUE for complex component definitions that don't fit the builder pattern.
// When set, this bypasses all other template settings and outputs the raw CUE directly.
func (c *ComponentDefinition) RawCUE(cue string) *ComponentDefinition {
	c.setRawCUE(cue)
	return c
}

// WithImports adds CUE imports to the component definition.
// Usage: component.WithImports("strconv", "strings", "list")
func (c *ComponentDefinition) WithImports(imports ...string) *ComponentDefinition {
	c.addImports(imports...)
	return c
}

// RunOn adds placement conditions specifying which clusters this definition should run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewComponent("eks-only").
//	    RunOn(placement.Label("provider").Eq("aws")).
//	    RunOn(placement.Label("cluster-type").NotEq("vcluster"))
//
// Multiple RunOn calls are combined with AND semantics (all conditions must match).
func (c *ComponentDefinition) RunOn(conditions ...placement.Condition) *ComponentDefinition {
	c.addRunOn(conditions...)
	return c
}

// NotRunOn adds placement conditions specifying which clusters this definition should NOT run on.
// Use the placement package's fluent API to build conditions.
//
// Example:
//
//	defkit.NewComponent("no-vclusters").
//	    NotRunOn(placement.Label("cluster-type").Eq("vcluster"))
//
// If any NotRunOn condition matches, the definition is ineligible for that cluster.
func (c *ComponentDefinition) NotRunOn(conditions ...placement.Condition) *ComponentDefinition {
	c.addNotRunOn(conditions...)
	return c
}

// ToCue generates the complete CUE definition string for this component.
// This is a convenience method that creates a CUEGenerator and calls GenerateFullDefinition.
func (c *ComponentDefinition) ToCue() string {
	// If raw CUE is set, use it with the name from NewComponent() taking precedence
	if c.HasRawCUE() {
		return c.GetRawCUEWithName()
	}
	gen := NewCUEGenerator()
	if len(c.GetImports()) > 0 {
		gen.WithImports(c.GetImports()...)
	}
	return gen.GenerateFullDefinition(c)
}

// ToCueWithImports generates the CUE definition with the specified imports.
// Use this when the definition requires CUE standard library imports.
// Example: component.ToCueWithImports(CUEImports.Strconv, CUEImports.List)
func (c *ComponentDefinition) ToCueWithImports(imports ...string) string {
	gen := NewCUEGenerator().WithImports(imports...)
	return gen.GenerateFullDefinition(c)
}

// ToParameterSchema generates only the parameter schema block.
// This is useful for testing or comparing parameter schemas.
func (c *ComponentDefinition) ToParameterSchema() string {
	gen := NewCUEGenerator()
	return gen.GenerateParameterSchema(c)
}

// ToYAML generates the Kubernetes YAML representation of the ComponentDefinition.
// This produces a ComponentDefinition custom resource that can be applied to a cluster.
// Note: The CUE template is embedded in the spec.schematic.cue field.
func (c *ComponentDefinition) ToYAML() ([]byte, error) {
	cueStr := c.ToCue()

	// Build the ComponentDefinition CR structure
	cr := map[string]any{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "ComponentDefinition",
		"metadata": map[string]any{
			"name": c.GetName(),
			"annotations": map[string]any{
				"definition.oam.dev/description": c.GetDescription(),
			},
		},
		"spec": map[string]any{
			"workload": map[string]any{
				"definition": map[string]any{
					"apiVersion": c.workload.apiVersion,
					"kind":       c.workload.kind,
				},
			},
			"schematic": map[string]any{
				"cue": map[string]any{
					"template": cueStr,
				},
			},
		},
	}

	// Handle autodetect workload type
	if c.workload.autodetect {
		cr["spec"].(map[string]any)["workload"] = map[string]any{
			"type": "autodetects.core.oam.dev",
		}
	}

	return yaml.Marshal(cr)
}

// APIVersion returns the workload API version.
func (w WorkloadType) APIVersion() string { return w.apiVersion }

// Kind returns the workload kind.
func (w WorkloadType) Kind() string { return w.kind }

// IsAutodetect returns true if the workload type is auto-detected at runtime.
func (w WorkloadType) IsAutodetect() bool { return w.autodetect }

// Template provides the building context for component and trait templates.
// It embeds VelaContext to provide access to runtime context values
// (Name, AppName, Namespace, Revision, etc.) directly on tpl.
//
// For components: Use Output() and Outputs() to define resources.
// For traits: Use Patch() to modify workloads or Outputs() for auxiliary resources.
type Template struct {
	*VelaContext                            // Embedded to provide tpl.Name(), tpl.AppName(), etc.
	output             *Resource            // Primary output (for components)
	outputs            map[string]*Resource // Named auxiliary outputs
	helpers            []*HelperVar         // Template-level helper definitions
	structArrayHelpers []*StructArrayHelper // Struct-based array helpers
	concatHelpers      []*ConcatHelper      // list.Concat helpers
	dedupeHelpers      []*DedupeHelper      // Deduplication helpers

	// Output groups for grouped conditional outputs
	outputGroups []*outputGroup

	// Trait-specific fields
	patch         *PatchResource // Patch operations for traits
	patchStrategy string         // Patch strategy (e.g., "retainKeys", "jsonMergePatch")

	// Advanced trait patterns
	patchContainerConfig *PatchContainerConfig // PatchContainer helper configuration
	letBindings          []*LetBinding         // Let bindings for local variables

	// Raw CUE blocks for traits that need complex logic
	rawPatchBlock     string // Raw CUE for patch: block
	rawParameterBlock string // Raw CUE for parameter: block
	rawOutputsBlock   string // Raw CUE for outputs: block (for traits that generate K8s resources)
	rawHeaderBlock    string // Raw CUE for let bindings and other pre-output declarations
}

// NewTemplate creates a new template context.
func NewTemplate() *Template {
	return &Template{
		VelaContext:        &VelaContext{},
		outputs:            make(map[string]*Resource),
		helpers:            make([]*HelperVar, 0),
		structArrayHelpers: make([]*StructArrayHelper, 0),
		concatHelpers:      make([]*ConcatHelper, 0),
		dedupeHelpers:      make([]*DedupeHelper, 0),
	}
}

// registerHelper registers a helper with the template.
func (t *Template) registerHelper(helper *HelperVar) {
	t.helpers = append(t.helpers, helper)
}

// GetHelpers returns all registered helpers.
func (t *Template) GetHelpers() []*HelperVar {
	return t.helpers
}

// GetHelpersBeforeOutput returns helpers that should appear before the output: block.
func (t *Template) GetHelpersBeforeOutput() []*HelperVar {
	result := make([]*HelperVar, 0)
	for _, h := range t.helpers {
		if !h.afterOutput {
			result = append(result, h)
		}
	}
	return result
}

// GetHelpersAfterOutput returns helpers that should appear after the output: block.
// These helpers are typically used by auxiliary outputs and should be placed
// between output: and outputs: in the generated CUE.
func (t *Template) GetHelpersAfterOutput() []*HelperVar {
	result := make([]*HelperVar, 0)
	for _, h := range t.helpers {
		if h.afterOutput {
			result = append(result, h)
		}
	}
	return result
}

// registerStructArrayHelper registers a struct-based array helper.
func (t *Template) registerStructArrayHelper(helper *StructArrayHelper) {
	t.structArrayHelpers = append(t.structArrayHelpers, helper)
}

// GetStructArrayHelpers returns all struct-based array helpers.
func (t *Template) GetStructArrayHelpers() []*StructArrayHelper {
	return t.structArrayHelpers
}

// registerConcatHelper registers a list.Concat helper.
func (t *Template) registerConcatHelper(helper *ConcatHelper) {
	t.concatHelpers = append(t.concatHelpers, helper)
}

// GetConcatHelpers returns all list.Concat helpers.
func (t *Template) GetConcatHelpers() []*ConcatHelper {
	return t.concatHelpers
}

// registerDedupeHelper registers a deduplication helper.
func (t *Template) registerDedupeHelper(helper *DedupeHelper) {
	t.dedupeHelpers = append(t.dedupeHelpers, helper)
}

// GetDedupeHelpers returns all deduplication helpers.
func (t *Template) GetDedupeHelpers() []*DedupeHelper {
	return t.dedupeHelpers
}

// Output sets or returns the primary output resource.
func (t *Template) Output(r ...*Resource) *Resource {
	if len(r) > 0 {
		t.output = r[0]
	}
	return t.output
}

// Outputs sets or returns a named auxiliary resource.
func (t *Template) Outputs(name string, r ...*Resource) *Resource {
	if len(r) > 0 {
		t.outputs[name] = r[0]
	}
	return t.outputs[name]
}

// OutputsIf conditionally sets a named auxiliary resource.
// The resource is only added if the condition evaluates to true.
func (t *Template) OutputsIf(cond Condition, name string, r *Resource) {
	r.outputCondition = cond
	t.outputs[name] = r
}

// GetOutput returns the primary output resource.
func (t *Template) GetOutput() *Resource { return t.output }

// GetOutputs returns all auxiliary resources.
func (t *Template) GetOutputs() map[string]*Resource { return t.outputs }

// outputGroup represents a group of outputs that share a common condition.
type outputGroup struct {
	cond    Condition
	outputs map[string]*Resource
}

// OutputGroup is a builder for adding outputs within a grouped condition.
type OutputGroup struct {
	tpl     *Template
	cond    Condition
	outputs map[string]*Resource
}

// Add adds a named resource to the output group.
func (g *OutputGroup) Add(name string, r *Resource) *OutputGroup {
	g.outputs[name] = r
	return g
}

// OutputsGroupIf groups multiple outputs under a single condition.
// This generates one `if cond { ... }` block containing all grouped outputs.
func (t *Template) OutputsGroupIf(cond Condition, fn func(g *OutputGroup)) {
	group := &OutputGroup{
		tpl:     t,
		cond:    cond,
		outputs: make(map[string]*Resource),
	}
	fn(group)
	if t.outputGroups == nil {
		t.outputGroups = make([]*outputGroup, 0)
	}
	t.outputGroups = append(t.outputGroups, &outputGroup{
		cond:    cond,
		outputs: group.outputs,
	})
}

// GetOutputGroups returns all output groups.
func (t *Template) GetOutputGroups() []*outputGroup {
	return t.outputGroups
}

// --- Patch methods for traits ---

// Patch returns the PatchResource builder for traits.
// If no patch has been created yet, this creates one.
// Use this to modify the workload resource in trait templates.
//
// Example:
//
//	tpl.Patch().
//	    Set("spec.replicas", replicas).
//	    SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)
func (t *Template) Patch() *PatchResource {
	if t.patch == nil {
		t.patch = NewPatchResource()
	}
	return t.patch
}

// PatchStrategy sets the patch strategy for trait patches.
// Common strategies: "retainKeys", "jsonMergePatch", "jsonPatch"
//
// Example:
//
//	tpl.PatchStrategy("retainKeys").
//	    Patch().Set("spec.replicas", replicas)
func (t *Template) PatchStrategy(strategy string) *Template {
	t.patchStrategy = strategy
	return t
}

// GetPatch returns the patch resource if set.
func (t *Template) GetPatch() *PatchResource { return t.patch }

// GetPatchStrategy returns the patch strategy.
func (t *Template) GetPatchStrategy() string { return t.patchStrategy }

// HasPatch returns true if this template has patch operations.
func (t *Template) HasPatch() bool { return t.patch != nil && len(t.patch.ops) > 0 }

// PatchResource represents a patch being built for traits.
// It uses the same fluent API as Resource but generates a patch: block.
type PatchResource struct {
	ops       []ResourceOp
	currentIf *IfBlock // tracks current If block being built
}

// NewPatchResource creates a new patch resource builder.
func NewPatchResource() *PatchResource {
	return &PatchResource{
		ops: make([]ResourceOp, 0),
	}
}

// Set records a field assignment in the patch.
// Example: p.Set("spec.replicas", replicas)
func (p *PatchResource) Set(path string, value Value) *PatchResource {
	op := &SetOp{path: path, value: value}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// SetIf records a conditional field assignment in the patch.
// Example: p.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)
func (p *PatchResource) SetIf(cond Condition, path string, value Value) *PatchResource {
	op := &SetIfOp{path: path, value: value, cond: cond}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// SpreadIf records a conditional spread operation inside a struct block in the patch.
// Example: p.SpreadIf(labels.IsSet(), "metadata.labels", labels)
func (p *PatchResource) SpreadIf(cond Condition, path string, value Value) *PatchResource {
	op := &SpreadIfOp{path: path, value: value, cond: cond}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// ForEach adds a for-each spread operation to the patch.
// This generates: for k, v in parameter { (k): v }
// Used for traits like labels that spread map keys dynamically.
//
// Example:
//
//	tpl.Patch().ForEach(labels, "metadata.labels")
//	// Generates: metadata: labels: { for k, v in parameter { (k): v } }
func (p *PatchResource) ForEach(source Value, path string) *PatchResource {
	op := &ForEachOp{path: path, source: source}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// If starts a conditional block. Operations until EndIf are conditional.
func (p *PatchResource) If(cond Condition) *PatchResource {
	p.currentIf = &IfBlock{
		cond: cond,
		ops:  make([]ResourceOp, 0),
	}
	return p
}

// EndIf ends the current conditional block.
func (p *PatchResource) EndIf() *PatchResource {
	if p.currentIf != nil {
		p.ops = append(p.ops, p.currentIf)
		p.currentIf = nil
	}
	return p
}

// PatchKey adds an array patch with a merge key annotation.
// This generates: // +patchKey=key
//
//	path: [element1, element2, ...]
//
// Used for merging arrays by key (e.g., containers by name).
//
// Example:
//
//	tpl.Patch().PatchKey("spec.template.spec.containers", "name", container)
func (p *PatchResource) PatchKey(path string, key string, elements ...Value) *PatchResource {
	op := &PatchKeyOp{path: path, key: key, elements: elements}
	if p.currentIf != nil {
		p.currentIf.ops = append(p.currentIf.ops, op)
	} else {
		p.ops = append(p.ops, op)
	}
	return p
}

// Ops returns all recorded operations.
func (p *PatchResource) Ops() []ResourceOp { return p.ops }

// Passthrough sets the patch to pass through the entire parameter.
// This generates CUE like: patch: parameter
// Used for json-patch and json-merge-patch traits where the parameter IS the patch.
func (p *PatchResource) Passthrough() *PatchResource {
	p.ops = append(p.ops, &PassthroughOp{})
	return p
}

// PassthroughOp represents a passthrough operation where the parameter becomes the patch.
type PassthroughOp struct{}

func (p *PassthroughOp) resourceOp() {}

// ForEachOp represents a for-each spread operation in a patch.
// This generates CUE like: for k, v in source { (k): v }
type ForEachOp struct {
	path   string
	source Value
}

func (f *ForEachOp) resourceOp() {}

// Path returns the parent path for the for-each operation.
func (f *ForEachOp) Path() string { return f.path }

// Source returns the source value to iterate over.
func (f *ForEachOp) Source() Value { return f.source }

// --- Context output introspection for traits ---

// ContextOutputRef represents a reference to context.output for trait introspection.
// Traits can use this to conditionally apply patches based on the workload's structure.
type ContextOutputRef struct {
	path string
}

// ContextOutput returns a reference to the component's output (context.output).
// Use this in traits to introspect the workload being modified.
//
// Example:
//
//	// Check if workload has spec.template (like Deployment)
//	hasTemplate := ContextOutput().HasPath("spec.template")
//	tpl.Patch().
//	    If(hasTemplate).
//	        Set("spec.template.metadata.labels", labels).
//	    EndIf()
func ContextOutput() *ContextOutputRef {
	return &ContextOutputRef{path: "context.output"}
}

func (c *ContextOutputRef) value() {}
func (c *ContextOutputRef) expr()  {}

// Path returns the full CUE path for this reference.
func (c *ContextOutputRef) Path() string { return c.path }

// Field returns a reference to a specific field in context.output.
// Example: ContextOutput().Field("spec.template.spec.containers")
func (c *ContextOutputRef) Field(path string) *ContextOutputRef {
	return &ContextOutputRef{path: c.path + "." + path}
}

// HasPath returns a condition that checks if a path exists in context.output.
// This generates CUE: context.output.path != _|_
//
// Example:
//
//	hasTemplate := ContextOutput().HasPath("spec.template")
func (c *ContextOutputRef) HasPath(path string) Condition {
	return &ContextPathExistsCondition{basePath: c.path, fieldPath: path}
}

// IsSet returns a condition that checks if this context path exists.
func (c *ContextOutputRef) IsSet() Condition {
	return &ContextPathExistsCondition{basePath: "", fieldPath: c.path}
}

// ContextPathExistsCondition checks if a path exists in context.output.
type ContextPathExistsCondition struct {
	baseCondition
	basePath  string
	fieldPath string
}

// BasePath returns the base path (e.g., "context.output").
func (c *ContextPathExistsCondition) BasePath() string { return c.basePath }

// FieldPath returns the field path being checked.
func (c *ContextPathExistsCondition) FieldPath() string { return c.fieldPath }

// FullPath returns the complete path.
func (c *ContextPathExistsCondition) FullPath() string {
	if c.basePath == "" {
		return c.fieldPath
	}
	return c.basePath + "." + c.fieldPath
}

// Render executes the component template with the given test context
// and returns the rendered primary output resource.
func (c *ComponentDefinition) Render(ctx *TestContextBuilder) *RenderedResource {
	// Build the runtime context
	rtCtx := ctx.Build()

	// Set up the current test context for parameter resolution
	setCurrentTestContext(rtCtx)
	defer clearCurrentTestContext()

	// Create and execute template
	tpl := NewTemplate()
	if c.template != nil {
		c.template(tpl)
	}

	// Render the output resource with resolved values
	return renderResource(tpl.output, rtCtx)
}

// RenderAll executes the component template and returns all outputs.
func (c *ComponentDefinition) RenderAll(ctx *TestContextBuilder) *RenderedOutputs {
	rtCtx := ctx.Build()
	setCurrentTestContext(rtCtx)
	defer clearCurrentTestContext()

	tpl := NewTemplate()
	if c.template != nil {
		c.template(tpl)
	}

	outputs := &RenderedOutputs{
		Primary:   renderResource(tpl.output, rtCtx),
		Auxiliary: make(map[string]*RenderedResource),
	}

	for name, res := range tpl.outputs {
		// Check if the resource has an output condition
		if res.outputCondition != nil {
			if !evaluateCondition(res.outputCondition, rtCtx) {
				// Condition is false, skip this output
				continue
			}
		}
		outputs.Auxiliary[name] = renderResource(res, rtCtx)
	}

	return outputs
}

// RenderedOutputs contains all rendered resources from a template.
type RenderedOutputs struct {
	Primary   *RenderedResource
	Auxiliary map[string]*RenderedResource
}

// RenderedResource represents a fully rendered Kubernetes resource
// with all parameter values resolved.
type RenderedResource struct {
	apiVersion string
	kind       string
	data       map[string]any
}

// APIVersion returns the resource's API version.
func (r *RenderedResource) APIVersion() string {
	if r == nil {
		return ""
	}
	return r.apiVersion
}

// Kind returns the resource's kind.
func (r *RenderedResource) Kind() string {
	if r == nil {
		return ""
	}
	return r.kind
}

// Get retrieves a value at the given path (e.g., "spec.replicas").
func (r *RenderedResource) Get(path string) any {
	if r == nil || r.data == nil {
		return nil
	}
	return getNestedValue(r.data, path)
}

// Data returns the full rendered resource data.
func (r *RenderedResource) Data() map[string]any {
	if r == nil {
		return nil
	}
	return r.data
}

// renderResource converts a Resource with operations into a RenderedResource
// with all values resolved from the test context.
func renderResource(res *Resource, ctx *TestRuntimeContext) *RenderedResource {
	if res == nil {
		return nil
	}

	rendered := &RenderedResource{
		apiVersion: res.apiVersion,
		kind:       res.kind,
		data: map[string]any{
			"apiVersion": res.apiVersion,
			"kind":       res.kind,
			"metadata": map[string]any{
				"name": ctx.Name(),
			},
		},
	}

	// Process all operations
	for _, op := range res.ops {
		processOp(rendered.data, op, ctx)
	}

	return rendered
}

// processOp processes a single resource operation.
func processOp(data map[string]any, op ResourceOp, ctx *TestRuntimeContext) {
	switch o := op.(type) {
	case *SetOp:
		value := resolveValue(o.value, ctx)
		setNestedValue(data, o.path, value)

	case *SetIfOp:
		if evaluateCondition(o.cond, ctx) {
			value := resolveValue(o.value, ctx)
			setNestedValue(data, o.path, value)
		}

	case *IfBlock:
		if evaluateCondition(o.cond, ctx) {
			for _, innerOp := range o.ops {
				processOp(data, innerOp, ctx)
			}
		}
	}
}

// resolveValue resolves a Value to its actual value using the test context.
func resolveValue(v Value, ctx *TestRuntimeContext) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case *StringParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *IntParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *BoolParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *FloatParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *ArrayParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *MapParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *StructParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *EnumParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	case *ContextRef:
		return resolveContextRef(val, ctx)
	case *Literal:
		return val.Val()
	case *TransformedValue:
		// Resolve the source value, then apply the transformation
		sourceValue := resolveValue(val.source, ctx)
		if val.transform != nil {
			return val.transform(sourceValue)
		}
		return sourceValue
	case *CollectionOp:
		// Resolve source and apply collection operations
		sourceValue := resolveValue(val.source, ctx)
		return applyCollectionOps(sourceValue, val.ops)
	case *MultiSource:
		// Combine items from multiple fields and apply operations
		return resolveMultiSource(val, ctx)
	case *StringKeyMapParam:
		return ctx.GetParamOr(val.Name(), val.GetDefault())
	default:
		// For any Param interface, use method access
		if p, ok := v.(Param); ok {
			return ctx.GetParamOr(p.Name(), p.GetDefault())
		}
		return v
	}
}

// resolveContextRef resolves a context reference to its value.
func resolveContextRef(ref *ContextRef, ctx *TestRuntimeContext) any {
	switch ref.Path() {
	case "context.name":
		return ctx.Name()
	case "context.namespace":
		return ctx.Namespace()
	case "context.appName":
		return ctx.AppName()
	case "context.appRevision":
		return ctx.AppRevision()
	default:
		return ref.String()
	}
}

// evaluateCondition evaluates a Condition using the test context.
func evaluateCondition(cond Condition, ctx *TestRuntimeContext) bool {
	if cond == nil {
		return true
	}

	switch c := cond.(type) {
	case *IsSetCondition:
		return ctx.IsParamSet(c.paramName)
	case *CompareCondition:
		left := resolveConditionValue(c.left, ctx)
		right := resolveConditionValue(c.right, ctx)
		return compareValues(left, right, c.op)
	case *Comparison:
		left := resolveConditionValue(c.Left(), ctx)
		right := resolveConditionValue(c.Right(), ctx)
		return compareValues(left, right, string(c.Op()))
	case *AndCondition:
		return evaluateCondition(c.left, ctx) && evaluateCondition(c.right, ctx)
	case *OrCondition:
		return evaluateCondition(c.left, ctx) || evaluateCondition(c.right, ctx)
	case *NotCondition:
		return !evaluateCondition(c.inner, ctx)
	case *LogicalExpr:
		if c.Op() == OpAnd {
			for _, sub := range c.Conditions() {
				if !evaluateCondition(sub, ctx) {
					return false
				}
			}
			return true
		} else { // OpOr
			for _, sub := range c.Conditions() {
				if evaluateCondition(sub, ctx) {
					return true
				}
			}
			return false
		}
	case *NotExpr:
		return !evaluateCondition(c.Cond(), ctx)
	case *HasExposedPortsCondition:
		// Resolve the ports value and check if any have expose=true
		portsValue := resolveValue(c.ports, ctx)
		return hasExposedPorts(portsValue)
	default:
		// For parameter-based conditions (param used as condition)
		if v, ok := cond.(Value); ok {
			resolved := resolveValue(v, ctx)
			return resolved != nil
		}
		return true
	}
}

// hasExposedPorts checks if a ports array has any port with expose=true.
func hasExposedPorts(ports any) bool {
	portList, ok := ports.([]any)
	if !ok {
		return false
	}
	for _, p := range portList {
		if portMap, ok := p.(map[string]any); ok {
			if expose, ok := portMap["expose"].(bool); ok && expose {
				return true
			}
		}
	}
	return false
}

// resolveConditionValue resolves a value used in a condition.
func resolveConditionValue(v any, ctx *TestRuntimeContext) any {
	if val, ok := v.(Value); ok {
		return resolveValue(val, ctx)
	}
	return v
}

// compareValues compares two values with the given operator.
func compareValues(left, right any, op string) bool {
	switch op {
	case "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return compareNumeric(left, right) < 0
	case "<=":
		return compareNumeric(left, right) <= 0
	case ">":
		return compareNumeric(left, right) > 0
	case ">=":
		return compareNumeric(left, right) >= 0
	default:
		return false
	}
}

// compareNumeric compares two numeric values.
func compareNumeric(left, right any) int {
	l := toFloat64(left)
	r := toFloat64(right)
	if l < r {
		return -1
	}
	if l > r {
		return 1
	}
	return 0
}

// toFloat64 converts a value to float64 for comparison.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// setNestedValue sets a value at a nested path in a map.
func setNestedValue(data map[string]any, path string, value any) {
	parts := splitPath(path)
	current := data

	for i, part := range parts[:len(parts)-1] {
		// Handle bracket notation: containers[0] (array) or labels[app.oam.dev/name] (map key)
		name, key, index := parseBracketAccess(part)

		switch {
		case index >= 0:
			// Array access
			arr, ok := current[name].([]any)
			if !ok {
				arr = make([]any, index+1)
				current[name] = arr
			}
			for len(arr) <= index {
				arr = append(arr, make(map[string]any)) //nolint:makezero // Only extends existing arrays; new arrays have len > index
				current[name] = arr
			}
			if m, ok := arr[index].(map[string]any); ok {
				current = m
			} else {
				m := make(map[string]any)
				arr[index] = m
				current[name] = arr
				current = m
			}
		case key != "":
			// Map key access like labels[app.oam.dev/name]
			if _, exists := current[name]; !exists {
				current[name] = make(map[string]any)
			}
			if m, ok := current[name].(map[string]any); ok {
				if _, exists := m[key]; !exists {
					m[key] = make(map[string]any)
				}
				if next, ok := m[key].(map[string]any); ok {
					current = next
				} else {
					// The key exists but is not a map - create nested structure
					newMap := make(map[string]any)
					m[key] = newMap
					current = newMap
				}
			}
		default:
			// Regular map access
			if _, exists := current[name]; !exists {
				current[name] = make(map[string]any)
			}
			if next, ok := current[name].(map[string]any); ok {
				current = next
			} else {
				// Path conflict - overwrite
				m := make(map[string]any)
				current[name] = m
				current = m
			}
		}
		_ = i // suppress unused warning
	}

	// Set the final value
	lastPart := parts[len(parts)-1]
	name, key, index := parseBracketAccess(lastPart)
	switch {
	case index >= 0:
		arr, ok := current[name].([]any)
		if !ok {
			arr = make([]any, index+1)
		}
		for len(arr) <= index {
			arr = append(arr, nil) //nolint:makezero // Only extends existing arrays; new arrays have len > index
		}
		arr[index] = value
		current[name] = arr
	case key != "":
		// Map key access like labels[app.oam.dev/name]
		if _, exists := current[name]; !exists {
			current[name] = make(map[string]any)
		}
		if m, ok := current[name].(map[string]any); ok {
			m[key] = value
		}
	default:
		current[name] = value
	}
}

// getNestedValue retrieves a value at a nested path.
func getNestedValue(data map[string]any, path string) any {
	parts := splitPath(path)
	current := any(data)

	for _, part := range parts {
		name, _, index := parseBracketAccess(part)

		switch c := current.(type) {
		case map[string]any:
			if index >= 0 {
				if arr, ok := c[name].([]any); ok && index < len(arr) {
					current = arr[index]
				} else {
					return nil
				}
			} else {
				var ok bool
				current, ok = c[name]
				if !ok {
					return nil
				}
			}
		default:
			return nil
		}
	}

	return current
}

// splitPath splits a dot-separated path.
func splitPath(path string) []string {
	var parts []string
	var current string
	bracketDepth := 0

	for _, c := range path {
		switch {
		case c == '[':
			bracketDepth++
			current += string(c)
		case c == ']':
			bracketDepth--
			current += string(c)
		case c == '.' && bracketDepth == 0:
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// parseBracketAccess parses "name[key]" or "name[index]" and returns:
// - name: the field name before the bracket
// - key: the string key if it's a map access (empty string if array)
// - index: the numeric index if it's an array access (-1 if map access or no brackets)
func parseBracketAccess(part string) (name string, key string, index int) {
	for i, c := range part {
		if c == '[' {
			if part[len(part)-1] != ']' {
				return part, "", -1
			}
			name = part[:i]
			bracketContent := part[i+1 : len(part)-1]
			// Check if the content is numeric (array index)
			isNumeric := len(bracketContent) > 0
			for _, d := range bracketContent {
				if d < '0' || d > '9' {
					isNumeric = false
					break
				}
			}
			if !isNumeric {
				// This is a map key notation
				return name, bracketContent, -1
			}
			// Parse as array index
			idx := 0
			for _, d := range bracketContent {
				idx = idx*10 + int(d-'0')
			}
			return name, "", idx
		}
	}
	return part, "", -1
}

// applyCollectionOps applies a series of collection operations to a value.
func applyCollectionOps(source any, ops []collectionOperation) any {
	// Handle both []any and []map[string]any (Go doesn't automatically convert slices)
	var items []any
	switch v := source.(type) {
	case []any:
		items = v
	case []map[string]any:
		// Convert []map[string]any to []any
		items = make([]any, len(v))
		for i, m := range v {
			items[i] = m
		}
	default:
		return source
	}
	result := items
	for _, op := range ops {
		result = op.apply(result)
	}
	return result
}

// resolveMultiSource resolves a MultiSource by combining items from multiple fields.
func resolveMultiSource(ms *MultiSource, ctx *TestRuntimeContext) any {
	sourceValue := resolveValue(ms.source, ctx)
	sourceMap, ok := sourceValue.(map[string]any)
	if !ok {
		return []any{}
	}

	// Get per-source mappings if defined
	mapBySource := ms.MapBySourceMappings()

	// Collect all items from the specified source fields
	var allItems []any
	for _, field := range ms.sources {
		// Handle both []any and []map[string]any (Go doesn't automatically convert slices)
		var items []any
		switch v := sourceMap[field].(type) {
		case []any:
			items = v
		case []map[string]any:
			// Convert []map[string]any to []any
			items = make([]any, len(v))
			for i, m := range v {
				items[i] = m
			}
		default:
			continue
		}

		// If MapBySource is defined, apply the mapping for this source type
		if mapping, hasMapping := mapBySource[field]; hasMapping {
			for _, item := range items {
				if itemMap, ok := item.(map[string]any); ok {
					mappedItem := applyFieldMap(itemMap, mapping)
					allItems = append(allItems, mappedItem)
				}
			}
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Apply operations
	result := allItems
	for _, op := range ms.ops {
		result = op.apply(result)
	}

	return result
}

// applyFieldMap applies a FieldMap to transform an item.
func applyFieldMap(item map[string]any, mapping FieldMap) map[string]any {
	result := make(map[string]any)
	for key, fieldVal := range mapping {
		resolved := fieldVal.resolve(item)
		if resolved != nil {
			result[key] = resolved
		}
	}
	return result
}
