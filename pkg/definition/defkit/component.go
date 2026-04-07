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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// ComponentDefinition represents a KubeVela ComponentDefinition.
//
// ComponentDefinition embeds baseDefinition for common fields and methods shared
// with TraitDefinition and other definition types.
type ComponentDefinition struct {
	baseDefinition      // embedded common fields (name, description, params, template, etc.)
	workload            WorkloadType
	omitWorkloadType    bool              // when true, suppresses the auto-generated workload.type field in CUE
	labels              map[string]string  // metadata labels for the component definition
	childResourceKinds  []common.ChildResourceKind
	podSpecPath         string
}

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

// OmitWorkloadType suppresses the auto-generated workload.type field in the CUE output.
// Use this when the vela source CUE does not include a workload type field.
func (c *ComponentDefinition) OmitWorkloadType() *ComponentDefinition {
	c.omitWorkloadType = true
	return c
}

// IsOmitWorkloadType returns whether workload type should be suppressed in CUE output.
func (c *ComponentDefinition) IsOmitWorkloadType() bool {
	return c.omitWorkloadType
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

// StatusDetails sets the status details CUE expression for the component.
func (c *ComponentDefinition) StatusDetails(details string) *ComponentDefinition {
	c.setStatusDetails(details)
	return c
}

// Annotations sets metadata annotations on the component definition.
func (c *ComponentDefinition) Annotations(annotations map[string]string) *ComponentDefinition {
	c.setAnnotations(annotations)
	return c
}

// Version sets the version string for the component definition.
func (c *ComponentDefinition) Version(v string) *ComponentDefinition {
	c.setVersion(v)
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

// Labels sets metadata labels on the component definition.
// Usage: component.Labels(map[string]string{"ui-hidden": "true"})
func (c *ComponentDefinition) Labels(labels map[string]string) *ComponentDefinition {
	c.labels = labels
	return c
}

// GetLabels returns the component's metadata labels.
func (c *ComponentDefinition) GetLabels() map[string]string { return c.labels }

// ChildResourceKind adds a child resource kind entry to the component definition.
// Multiple calls accumulate entries.
func (c *ComponentDefinition) ChildResourceKind(apiVersion, kind string, selector map[string]string) *ComponentDefinition {
	c.childResourceKinds = append(c.childResourceKinds, common.ChildResourceKind{
		APIVersion: apiVersion,
		Kind:       kind,
		Selector:   selector,
	})
	return c
}

// GetChildResourceKinds returns all accumulated child resource kind entries.
func (c *ComponentDefinition) GetChildResourceKinds() []common.ChildResourceKind {
	return c.childResourceKinds
}

// PodSpecPath sets the pod spec path for the component definition.
func (c *ComponentDefinition) PodSpecPath(path string) *ComponentDefinition {
	c.podSpecPath = path
	return c
}

// GetPodSpecPath returns the pod spec path.
func (c *ComponentDefinition) GetPodSpecPath() string { return c.podSpecPath }

// RawCUE sets raw CUE for complex component definitions that don't fit the builder pattern.
// When set, this bypasses all other template settings and outputs the raw CUE directly.
func (c *ComponentDefinition) RawCUE(cue string) *ComponentDefinition {
	c.setRawCUE(cue)
	return c
}

// AppendRawParameterCUE appends a raw CUE block inside the parameter: { ... } section.
// The block is appended after the fluent-generated parameters, indented appropriately.
// Use this for complex CUE patterns that cannot be expressed with the fluent API,
// such as conditional parameter blocks, _validate* blocks, or inline error messages.
func (c *ComponentDefinition) AppendRawParameterCUE(block string) *ComponentDefinition {
	c.addRawParameterBlock(block)
	return c
}

// AppendRawOutputCUE appends a raw CUE block inside the output: { ... } section.
// The block is appended after the fluent-generated output tree, indented appropriately.
// Use this for complex CUE patterns in the output that cannot be expressed with the
// fluent API, such as conditionally emitting entire struct blocks.
func (c *ComponentDefinition) AppendRawOutputCUE(block string) *ComponentDefinition {
	c.addRawOutputBlock(block)
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
			"annotations": func() map[string]any {
				a := map[string]any{}
				for k, v := range c.GetAnnotations() {
					a[k] = v
				}
				a["definition.oam.dev/description"] = c.GetDescription()
				return a
			}(),
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

	if len(c.childResourceKinds) > 0 {
		cr["spec"].(map[string]any)["childResourceKinds"] = c.childResourceKinds
	}

	if c.podSpecPath != "" {
		cr["spec"].(map[string]any)["podSpecPath"] = c.podSpecPath
	}

	if c.GetVersion() != "" {
		cr["spec"].(map[string]any)["version"] = c.GetVersion()
	}

	return yaml.Marshal(cr)
}

// APIVersion returns the workload API version.
func (w WorkloadType) APIVersion() string { return w.apiVersion }

// Kind returns the workload kind.
func (w WorkloadType) Kind() string { return w.kind }

// IsAutodetect returns true if the workload type is auto-detected at runtime.
func (w WorkloadType) IsAutodetect() bool { return w.autodetect }
