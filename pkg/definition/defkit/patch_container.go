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
// - ParamIsSet/ParamNotSet: convenience constructors for parameter existence conditions

// PatchFieldBuilder provides a fluent API for constructing PatchContainerField values.
// Use PatchField() to start building.
//
// Example:
//
//	defkit.PatchField("exec").IsSet().Build()
//	defkit.PatchField("initialDelaySeconds").Int().IsSet().Default("0").Build()
//	defkit.PatchField("image").Strategy("retainKeys").Description("Specify the image").Build()
type PatchFieldBuilder struct {
	paramName     string
	targetField   string
	patchStrategy string
	condition     string
	paramType     string
	paramDefault  string
	description   string
}

// PatchField starts building a PatchContainerField with the given parameter name.
// The TargetField defaults to the same as the parameter name.
func PatchField(name string) *PatchFieldBuilder {
	return &PatchFieldBuilder{
		paramName:   name,
		targetField: name,
	}
}

// Target sets the container field to patch, if different from the parameter name.
func (b *PatchFieldBuilder) Target(t string) *PatchFieldBuilder {
	b.targetField = t
	return b
}

// Default sets an explicit default value for the parameter.
func (b *PatchFieldBuilder) Default(val string) *PatchFieldBuilder {
	b.paramDefault = val
	return b
}

// Type sets an explicit CUE type string (e.g., "string", "[...string]", "{...}").
func (b *PatchFieldBuilder) Type(t string) *PatchFieldBuilder {
	b.paramType = t
	return b
}

// Int is shorthand for Type("int").
func (b *PatchFieldBuilder) Int() *PatchFieldBuilder {
	return b.Type("int")
}

// Bool is shorthand for Type("bool").
func (b *PatchFieldBuilder) Bool() *PatchFieldBuilder {
	return b.Type("bool")
}

// Str is shorthand for Type("string").
func (b *PatchFieldBuilder) Str() *PatchFieldBuilder {
	return b.Type("string")
}

// StringArray is shorthand for Type("[...string]").
func (b *PatchFieldBuilder) StringArray() *PatchFieldBuilder {
	return b.Type("[...string]")
}

// Strategy sets the patch strategy (e.g., "replace", "retainKeys").
func (b *PatchFieldBuilder) Strategy(s string) *PatchFieldBuilder {
	b.patchStrategy = s
	return b
}

// --- Condition methods (following param.go / health_expr.go patterns) ---

// IsSet guards the field with an existence check (CUE: != _|_).
// Use this for optional fields that should only be patched when provided.
func (b *PatchFieldBuilder) IsSet() *PatchFieldBuilder {
	b.condition = "!= _|_"
	return b
}

// NotEmpty guards the field with a non-empty string check (CUE: != "").
// Use this for string fields that should only be patched when non-empty.
func (b *PatchFieldBuilder) NotEmpty() *PatchFieldBuilder {
	b.condition = `!= ""`
	return b
}

// Eq sets a condition that checks the field equals the given value.
func (b *PatchFieldBuilder) Eq(val string) *PatchFieldBuilder {
	b.condition = "== " + val
	return b
}

// Ne sets a condition that checks the field is not equal to the given value.
func (b *PatchFieldBuilder) Ne(val string) *PatchFieldBuilder {
	b.condition = "!= " + val
	return b
}

// Gt sets a condition that checks the field is greater than the given value.
func (b *PatchFieldBuilder) Gt(val string) *PatchFieldBuilder {
	b.condition = "> " + val
	return b
}

// Gte sets a condition that checks the field is greater than or equal to the given value.
func (b *PatchFieldBuilder) Gte(val string) *PatchFieldBuilder {
	b.condition = ">= " + val
	return b
}

// Lt sets a condition that checks the field is less than the given value.
func (b *PatchFieldBuilder) Lt(val string) *PatchFieldBuilder {
	b.condition = "< " + val
	return b
}

// Lte sets a condition that checks the field is less than or equal to the given value.
func (b *PatchFieldBuilder) Lte(val string) *PatchFieldBuilder {
	b.condition = "<= " + val
	return b
}

// RawCondition sets a raw CUE condition string.
// Use this as an escape hatch for non-standard conditions not covered by the typed API.
func (b *PatchFieldBuilder) RawCondition(c string) *PatchFieldBuilder {
	b.condition = c
	return b
}

// Description sets the +usage description for this field.
func (b *PatchFieldBuilder) Description(d string) *PatchFieldBuilder {
	b.description = d
	return b
}

// Build returns the constructed PatchContainerField.
func (b *PatchFieldBuilder) Build() PatchContainerField {
	return PatchContainerField{
		ParamName:     b.paramName,
		TargetField:   b.targetField,
		PatchStrategy: b.patchStrategy,
		Condition:     b.condition,
		ParamType:     b.paramType,
		ParamDefault:  b.paramDefault,
		Description:   b.description,
	}
}

// PatchFields builds a slice of PatchContainerField from builders.
// This eliminates the need to call .Build() on each field individually.
//
// Example:
//
//	Fields: defkit.PatchFields(
//	    defkit.PatchField("exec").IsSet(),
//	    defkit.PatchField("initialDelaySeconds").Int().IsSet().Default("0"),
//	)
func PatchFields(builders ...*PatchFieldBuilder) []PatchContainerField {
	fields := make([]PatchContainerField, len(builders))
	for i, b := range builders {
		fields[i] = b.Build()
	}
	return fields
}

// PatchContainerField defines a field to be patched in the container.
type PatchContainerField struct {
	ParamName     string // the parameter name (e.g., "command", "args")
	TargetField   string // the container field to patch (e.g., "command", "args")
	PatchStrategy string // the patch strategy (e.g., "replace", "merge")
	Condition     string // optional CUE condition (e.g., "!= null")
	ParamType     string // explicit CUE type (e.g., "string", "[...string]", "{...}")
	ParamDefault  string // explicit default value (e.g., "0", "\"\"", "false")
	Description   string // optional +usage description (auto-generated if empty)
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
	ContainersDescription     string                // +usage description for the containers param (auto-generated if empty)
	CustomParamsBlock         string                // custom CUE block for #PatchParams (for complex types)
	MultiContainerParam       string                // alternate name for multi-container param (default: "probes" for probes, "containers" for others)
	MultiContainerCheckField  string                // field name for multi-container error check (default: "containerName")
	MultiContainerErrMsg      string                // custom error message for multi-container mode (default: "container name must be set for %s")
	CustomPatchContainerBlock string                // custom CUE block for PatchContainer body (for complex merge logic)
	CustomPatchBlock          string                // custom CUE block for the patch: spec: template: spec: { ... } body
	CustomParameterBlock      string                // custom CUE block for the parameter definition
	PatchStrategy             string                // if set, emitted as // +patchStrategy=<value> before the patch block (e.g., "open")
	ParamsTypeName            string                // custom name for the #PatchParams helper (default: "PatchParams")
	NoDefaultDisjunction      bool                  // if true, omit the * default marker on parameter disjunction
}

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

// ParamIsSet creates a condition that checks if a parameter is set.
// This generates CUE: parameter.name != _|_
//
// This is a convenience wrapper around IsSetCondition.
//
// Example:
//
//	tpl.Patch().
//	    SetIf(defkit.ParamIsSet("replicas"), "spec.replicas", defkit.ParamRef("replicas"))
func ParamIsSet(name string) *IsSetCondition {
	return &IsSetCondition{paramName: name}
}

// ParamNotSet creates a condition that checks if a parameter is NOT set.
// This generates CUE: parameter.name == _|_
//
// This is a convenience wrapper around NotCondition{inner: IsSetCondition}.
//
// Example:
//
//	// Set default only if not explicitly specified
//	tpl.Patch().
//	    SetIf(defkit.ParamNotSet("replicas"), "spec.replicas", defkit.Literal(1))
func ParamNotSet(name string) *NotCondition {
	return &NotCondition{inner: &IsSetCondition{paramName: name}}
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
