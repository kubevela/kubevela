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

// This file extends the fluent API with patterns commonly used in traits:
// - PatchContainerConfig: for traits that patch containers by name
// - LetBinding: for CUE let bindings (local variables)
// - ListComprehension: for CUE list comprehensions with conditional fields
// - ParamIsSet: condition checking if a parameter is set

// PatchContainerHelper generates the PatchContainer CUE helper pattern.
// This helper is used by traits that need to patch a specific container by name
// within context.output.spec.template.spec.containers.
//
// The PatchContainer pattern:
// 1. Looks up the container by name in context.output.spec.template.spec.containers
// 2. Reports an error if the container is not found
// 3. Applies the patch fields to the matching container
//
// Usage in trait:
//
//	defkit.NewTrait("command").
//	    Template(func(tpl *defkit.Template) {
//	        tpl.UsePatchContainer(defkit.PatchContainerConfig{
//	            ContainerNameParam: "containerName",
//	            DefaultToContextName: true,
//	            PatchFields: []defkit.PatchContainerField{
//	                {ParamName: "command", TargetField: "command", PatchStrategy: "replace"},
//	                {ParamName: "args", TargetField: "args", PatchStrategy: "replace"},
//	            },
//	        })
//	    })
type PatchContainerHelper struct {
	name               string                // helper name (default: "PatchContainer")
	containerNameParam string                // parameter name for container name (e.g., "containerName")
	defaultToContext   bool                  // if true, defaults container name to context.name
	fields             []PatchContainerField // fields to patch
	customLogic        string                // optional custom CUE logic
}

// PatchContainerField defines a field to be patched in the container.
type PatchContainerField struct {
	ParamName     string // the parameter name (e.g., "command", "args")
	TargetField   string // the container field to patch (e.g., "command", "args")
	PatchStrategy string // the patch strategy (e.g., "replace", "merge")
	Condition     string // optional CUE condition (e.g., "!= null")
	ParamType     string // explicit CUE type (e.g., "string", "[...string]", "{...}")
	ParamDefault  string // explicit default value (e.g., "0", "\"\"", "false")
}

// PatchContainerGroup defines a group of fields under a common parent field.
// This generates CUE like:
//
//	startupProbe: {
//	    if _params.exec != _|_ { exec: _params.exec }
//	    if _params.httpGet != _|_ { httpGet: _params.httpGet }
//	}
type PatchContainerGroup struct {
	TargetField string                // the parent field name (e.g., "startupProbe", "securityContext")
	Fields      []PatchContainerField // fields within this group
	SubGroups   []PatchContainerGroup // nested groups (e.g., securityContext.capabilities)
}

// PatchContainerConfig configures the PatchContainer helper.
type PatchContainerConfig struct {
	ContainerNameParam        string                // parameter for container name
	DefaultToContextName      bool                  // default container name to context.name
	PatchFields               []PatchContainerField // flat fields to patch directly on container
	Groups                    []PatchContainerGroup // grouped fields (e.g., startupProbe: { ... })
	AllowMultiple             bool                  // if true, allow patching multiple containers
	ContainersParam           string                // for multi-container mode, the array param name
	CustomParamsBlock         string                // custom CUE block for #PatchParams (for complex types)
	MultiContainerParam       string                // alternate name for multi-container param (default: "probes" for probes, "containers" for others)
	CustomPatchContainerBlock string                // custom CUE block for PatchContainer body (for complex merge logic)
	CustomPatchBlock          string                // custom CUE block for the patch: spec: template: spec: { ... } body
	CustomParameterBlock      string                // custom CUE block for the parameter definition
}

// NewPatchContainerHelper creates a new PatchContainer helper.
func NewPatchContainerHelper(config PatchContainerConfig) *PatchContainerHelper {
	return &PatchContainerHelper{
		name:               "PatchContainer",
		containerNameParam: config.ContainerNameParam,
		defaultToContext:   config.DefaultToContextName,
		fields:             config.PatchFields,
	}
}

// WithName sets a custom name for the helper.
func (h *PatchContainerHelper) WithName(name string) *PatchContainerHelper {
	h.name = name
	return h
}

// WithCustomLogic adds custom CUE logic to the helper.
func (h *PatchContainerHelper) WithCustomLogic(logic string) *PatchContainerHelper {
	h.customLogic = logic
	return h
}

// Name returns the helper name.
func (h *PatchContainerHelper) Name() string { return h.name }

// ContainerNameParam returns the container name parameter.
func (h *PatchContainerHelper) ContainerNameParam() string { return h.containerNameParam }

// DefaultToContext returns whether to default to context.name.
func (h *PatchContainerHelper) DefaultToContext() bool { return h.defaultToContext }

// Fields returns the patch fields.
func (h *PatchContainerHelper) Fields() []PatchContainerField { return h.fields }

// CustomLogic returns any custom CUE logic.
func (h *PatchContainerHelper) CustomLogic() string { return h.customLogic }

// --- Let Binding Support ---

// LetBinding represents a CUE let binding for local variables.
// This generates: let varName = expression
//
// Usage:
//
//	tpl.AddLetBinding("resourceContent", defkit.Struct(...))
//	tpl.AddLetBinding("_baseContainers", defkit.ContextOutput().Field("spec.template.spec.containers"))
type LetBinding struct {
	name string
	expr Value
}

// NewLetBinding creates a new let binding.
func NewLetBinding(name string, expr Value) *LetBinding {
	return &LetBinding{name: name, expr: expr}
}

// Name returns the binding name.
func (l *LetBinding) Name() string { return l.name }

// Expr returns the bound expression.
func (l *LetBinding) Expr() Value { return l.expr }

// LetRef references a let binding by name.
// This generates: varName in CUE expressions.
type LetRef struct {
	name string
}

func (l *LetRef) value() {}
func (l *LetRef) expr()  {}

// LetVariable creates a reference to a let binding.
// Use this to reference a variable created with AddLetBinding.
//
// Example:
//
//	tpl.AddLetBinding("resourceContent", defkit.Struct(...))
//	// Later in template:
//	defkit.LetVariable("resourceContent")
func LetVariable(name string) *LetRef {
	return &LetRef{name: name}
}

// Name returns the variable name.
func (l *LetRef) Name() string { return l.name }

// --- List Comprehension with Conditionals ---

// ListComprehension represents a CUE list comprehension.
// This generates: [for v in source { ... if v.field != _|_ { field: v.field } }]
//
// Usage:
//
//	defkit.ForEachIn(defkit.Param("constraints")).
//	    MapFields(defkit.FieldMap{
//	        "maxSkew":     defkit.FieldRef("maxSkew"),         // always included
//	        "minDomains":  defkit.Optional("minDomains"),      // only if set
//	    })
type ListComprehension struct {
	source            Value
	filterCondition   ListPredicate
	mappings          FieldMap
	conditionalFields []string // fields that should only appear if set
}

func (l *ListComprehension) value() {}
func (l *ListComprehension) expr()  {}

// ForEachIn creates a list comprehension from a source collection.
// This is the starting point for building list comprehensions.
//
// Example:
//
//	defkit.ForEachIn(defkit.ParamRef("constraints")).
//	    MapFields(defkit.FieldMap{...})
func ForEachIn(source Value) *ListComprehension {
	return &ListComprehension{
		source:            source,
		conditionalFields: make([]string, 0),
	}
}

// WithFilter adds a filter condition to the comprehension.
// Only items matching the filter will be included in the output.
func (l *ListComprehension) WithFilter(pred ListPredicate) *ListComprehension {
	l.filterCondition = pred
	return l
}

// MapFields sets the field mappings for each item in the comprehension.
// Use FieldRef for always-included fields and Optional for conditional fields.
func (l *ListComprehension) MapFields(mappings FieldMap) *ListComprehension {
	l.mappings = mappings
	return l
}

// WithOptionalFields marks additional fields that should only appear if set.
// This is an alternative to using Optional in the FieldMap.
func (l *ListComprehension) WithOptionalFields(fields ...string) *ListComprehension {
	l.conditionalFields = append(l.conditionalFields, fields...)
	return l
}

// Source returns the source value.
func (l *ListComprehension) Source() Value { return l.source }

// FilterCondition returns the filter condition.
func (l *ListComprehension) FilterCondition() ListPredicate { return l.filterCondition }

// Mappings returns the field mappings.
func (l *ListComprehension) Mappings() FieldMap { return l.mappings }

// ConditionalFields returns fields that should be conditionally included.
func (l *ListComprehension) ConditionalFields() []string { return l.conditionalFields }

// --- ListPredicate for List Comprehension Filters ---

// ListPredicate represents a condition used to filter list comprehension items.
// Unlike Predicate in collections.go (which is for runtime filtering),
// ListPredicate generates CUE conditional expressions.
type ListPredicate interface {
	listPredicate()
}

// ListFieldExistsPredicate checks if a field exists (is not bottom) in list items.
type ListFieldExistsPredicate struct {
	field string
}

func (p *ListFieldExistsPredicate) listPredicate() {}

// GetField returns the field name being checked.
func (p *ListFieldExistsPredicate) GetField() string { return p.field }

// ListFieldExists creates a predicate that checks if a field exists in list items.
// This generates CUE: if v.field != _|_
//
// Example:
//
//	defkit.ForEachIn(source).
//	    WithFilter(defkit.ListFieldExists("optionalField"))
func ListFieldExists(field string) *ListFieldExistsPredicate {
	return &ListFieldExistsPredicate{field: field}
}

// --- Template Extensions for Traits ---

// UsePatchContainer configures the template to use the PatchContainer pattern.
func (t *Template) UsePatchContainer(config PatchContainerConfig) *Template {
	t.patchContainerConfig = &config
	return t
}

// AddLetBinding adds a let binding to the template.
// Let bindings create local variables in the CUE template.
//
// Example:
//
//	tpl.AddLetBinding("resourceContent", defkit.Struct(
//	    defkit.Field("cpu", defkit.ParamTypeString),
//	    defkit.Field("memory", defkit.ParamTypeString),
//	))
func (t *Template) AddLetBinding(name string, expr Value) *Template {
	if t.letBindings == nil {
		t.letBindings = make([]*LetBinding, 0)
	}
	t.letBindings = append(t.letBindings, NewLetBinding(name, expr))
	return t
}

// GetLetBindings returns all let bindings.
func (t *Template) GetLetBindings() []*LetBinding {
	return t.letBindings
}

// GetPatchContainerConfig returns the PatchContainer configuration.
func (t *Template) GetPatchContainerConfig() *PatchContainerConfig {
	return t.patchContainerConfig
}

// SetRawPatchBlock sets a raw CUE block for the patch section.
// This allows traits to define complex patch structures directly in CUE.
func (t *Template) SetRawPatchBlock(block string) *Template {
	t.rawPatchBlock = block
	return t
}

// GetRawPatchBlock returns the raw patch block.
func (t *Template) GetRawPatchBlock() string {
	return t.rawPatchBlock
}

// SetRawParameterBlock sets a raw CUE block for the parameter section.
// This allows traits to define complex parameter schemas directly in CUE.
func (t *Template) SetRawParameterBlock(block string) *Template {
	t.rawParameterBlock = block
	return t
}

// GetRawParameterBlock returns the raw parameter block.
func (t *Template) GetRawParameterBlock() string {
	return t.rawParameterBlock
}

// SetRawOutputsBlock sets a raw CUE block for the outputs section.
// This allows traits to define K8s resources to create directly in CUE.
// Use this for traits that generate Services, Ingresses, HPAs, PVCs, etc.
func (t *Template) SetRawOutputsBlock(block string) *Template {
	t.rawOutputsBlock = block
	return t
}

// GetRawOutputsBlock returns the raw outputs block.
func (t *Template) GetRawOutputsBlock() string {
	return t.rawOutputsBlock
}

// SetRawHeaderBlock sets a raw CUE block for let bindings and pre-output declarations.
// This allows traits to define local variables and helper expressions in CUE.
// Use this for let bindings, helper lists, and other declarations that come before outputs.
func (t *Template) SetRawHeaderBlock(block string) *Template {
	t.rawHeaderBlock = block
	return t
}

// GetRawHeaderBlock returns the raw header block.
func (t *Template) GetRawHeaderBlock() string {
	return t.rawHeaderBlock
}

// --- ParamIsSet condition ---

// ParamIsSetCondition checks if a parameter is set (not bottom).
type ParamIsSetCondition struct {
	baseCondition
	param string
}

// Param returns the parameter name being checked.
func (p *ParamIsSetCondition) Param() string { return p.param }

// ParamIsSet creates a condition that checks if a parameter is set.
// This generates CUE: parameter.name != _|_
//
// Example:
//
//	tpl.Patch().
//	    SetIf(defkit.ParamIsSet("replicas"), "spec.replicas", defkit.ParamRef("replicas"))
func ParamIsSet(name string) *ParamIsSetCondition {
	return &ParamIsSetCondition{param: name}
}

// --- ParamNotSet condition ---

// ParamNotSetCondition checks if a parameter is NOT set (is bottom).
type ParamNotSetCondition struct {
	baseCondition
	param string
}

// Param returns the parameter name being checked.
func (p *ParamNotSetCondition) Param() string { return p.param }

// ParamNotSet creates a condition that checks if a parameter is NOT set.
// This generates CUE: parameter.name == _|_
//
// Example:
//
//	// Set default only if not explicitly specified
//	tpl.Patch().
//	    SetIf(defkit.ParamNotSet("replicas"), "spec.replicas", defkit.Literal(1))
func ParamNotSet(name string) *ParamNotSetCondition {
	return &ParamNotSetCondition{param: name}
}

// --- ContextOutputExists condition ---

// ContextOutputExistsCondition checks if a path exists in context.output.
type ContextOutputExistsCondition struct {
	baseCondition
	path string
}

// Path returns the path being checked.
func (c *ContextOutputExistsCondition) Path() string { return c.path }

// ContextOutputExists creates a condition that checks if a path exists in context.output.
// This generates CUE: context.output.path != _|_
//
// Example:
//
//	// Only patch if workload has spec.template (Deployment/StatefulSet)
//	tpl.Patch().
//	    SetIf(defkit.ContextOutputExists("spec.template"), "spec.template.spec.replicas", defkit.ParamRef("replicas"))
func ContextOutputExists(path string) *ContextOutputExistsCondition {
	return &ContextOutputExistsCondition{path: path}
}

// --- AllConditions compound condition ---

// AllConditionsCondition is a compound condition that requires all sub-conditions to be true.
type AllConditionsCondition struct {
	baseCondition
	conditions []Condition
}

// Conditions returns the list of sub-conditions.
func (a *AllConditionsCondition) Conditions() []Condition { return a.conditions }

// AllConditions creates a compound condition that requires all sub-conditions to be true.
// This generates CUE: if cond1 if cond2 if cond3 { ... }
//
// Example:
//
//	// Apply only if cpu AND memory are set AND requests AND limits are NOT set
//	tpl.Patch().
//	    SetIf(defkit.AllConditions(
//	        defkit.ParamIsSet("cpu"),
//	        defkit.ParamIsSet("memory"),
//	        defkit.ParamNotSet("requests"),
//	        defkit.ParamNotSet("limits"),
//	    ), "spec.resources", resourceStruct)
func AllConditions(conditions ...Condition) *AllConditionsCondition {
	return &AllConditionsCondition{conditions: conditions}
}
