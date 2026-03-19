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
