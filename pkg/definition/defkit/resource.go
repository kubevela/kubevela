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

// ResourceOp represents an operation recorded during resource building.
type ResourceOp interface {
	resourceOp()
}

// SetOp represents a Set operation on a resource field.
type SetOp struct {
	path  string
	value Value
}

func (s *SetOp) resourceOp() {}

// Path returns the field path.
func (s *SetOp) Path() string { return s.path }

// Value returns the value being set.
func (s *SetOp) Value() Value { return s.value }

// SetIfOp represents a conditional Set operation.
type SetIfOp struct {
	path  string
	value Value
	cond  Condition
}

func (s *SetIfOp) resourceOp() {}

// Path returns the field path.
func (s *SetIfOp) Path() string { return s.path }

// Value returns the value being set.
func (s *SetIfOp) Value() Value { return s.value }

// Cond returns the condition for the operation.
func (s *SetIfOp) Cond() Condition { return s.cond }

// IfBlock represents a conditional block of operations.
type IfBlock struct {
	cond Condition
	ops  []ResourceOp
}

func (i *IfBlock) resourceOp() {}

// Cond returns the block condition.
func (i *IfBlock) Cond() Condition { return i.cond }

// Ops returns the operations within the block.
func (i *IfBlock) Ops() []ResourceOp { return i.ops }

// SpreadIfOp represents a conditional spread operation inside a block.
// Instead of conditionally setting a whole field, it spreads a value
// inside a block when a condition is met.
// Example: labels: { if param.labels != _|_ { parameter.labels } ... }
type SpreadIfOp struct {
	path  string    // parent path (e.g., "spec.template.metadata.labels")
	value Value     // value to spread
	cond  Condition // condition for spreading
}

func (s *SpreadIfOp) resourceOp() {}

// Path returns the parent field path.
func (s *SpreadIfOp) Path() string { return s.path }

// Value returns the value being spread.
func (s *SpreadIfOp) Value() Value { return s.value }

// Cond returns the condition for the spread.
func (s *SpreadIfOp) Cond() Condition { return s.cond }

// VersionConditional represents a conditional apiVersion setting.
type VersionConditional struct {
	Condition  Condition
	ApiVersion string
}

// Resource represents a Kubernetes resource being built.
type Resource struct {
	apiVersion          string
	kind                string
	ops                 []ResourceOp
	currentIf           *IfBlock  // tracks current If block being built
	outputCondition     Condition // condition for conditional output (used by OutputsIf)
	versionConditionals []VersionConditional
}

// NewResource creates a new resource builder with API version and kind.
func NewResource(apiVersion, kind string) *Resource {
	return &Resource{
		apiVersion: apiVersion,
		kind:       kind,
		ops:        make([]ResourceOp, 0),
	}
}

// NewResourceWithConditionalVersion creates a resource with conditional apiVersion.
// This is used when the apiVersion depends on runtime conditions like cluster version.
//
// Example:
//
//	vela := defkit.VelaCtx()
//	cronjob := defkit.NewResourceWithConditionalVersion("CronJob").
//	    VersionIf(Lt(vela.ClusterVersion().Minor(), 25), "batch/v1beta1").
//	    VersionIf(Gte(vela.ClusterVersion().Minor(), 25), "batch/v1")
func NewResourceWithConditionalVersion(kind string) *Resource {
	return &Resource{
		kind:                kind,
		ops:                 make([]ResourceOp, 0),
		versionConditionals: make([]VersionConditional, 0),
	}
}

// VersionIf adds a conditional apiVersion to the resource.
// When the condition is true, this apiVersion will be used.
func (r *Resource) VersionIf(cond Condition, apiVersion string) *Resource {
	r.versionConditionals = append(r.versionConditionals, VersionConditional{
		Condition:  cond,
		ApiVersion: apiVersion,
	})
	return r
}

// HasVersionConditionals returns true if the resource has conditional apiVersion.
func (r *Resource) HasVersionConditionals() bool {
	return len(r.versionConditionals) > 0
}

// VersionConditionals returns all conditional apiVersion settings.
func (r *Resource) VersionConditionals() []VersionConditional {
	return r.versionConditionals
}

// APIVersion returns the resource's API version.
func (r *Resource) APIVersion() string { return r.apiVersion }

// Kind returns the resource's kind.
func (r *Resource) Kind() string { return r.kind }

// Ops returns all recorded operations.
func (r *Resource) Ops() []ResourceOp { return r.ops }

// Set records a field assignment operation.
func (r *Resource) Set(path string, value Value) *Resource {
	op := &SetOp{path: path, value: value}
	if r.currentIf != nil {
		r.currentIf.ops = append(r.currentIf.ops, op)
	} else {
		r.ops = append(r.ops, op)
	}
	return r
}

// SetIf records a conditional field assignment.
func (r *Resource) SetIf(cond Condition, path string, value Value) *Resource {
	op := &SetIfOp{path: path, value: value, cond: cond}
	if r.currentIf != nil {
		r.currentIf.ops = append(r.currentIf.ops, op)
	} else {
		r.ops = append(r.ops, op)
	}
	return r
}

// SpreadIf records a conditional spread operation inside a struct block.
// Unlike SetIf which wraps the entire field in a conditional, SpreadIf spreads
// the value inside the block only when the condition is true, while the block
// itself always exists with its other fields.
//
// Example usage:
//
//	Set("spec.template.metadata.labels[app.oam.dev/name]", vela.AppName()).
//	SpreadIf(labels.IsSet(), "spec.template.metadata.labels", labels)
//
// Generates:
//
//	labels: {
//	    "app.oam.dev/name": context.appName
//	    if parameter.labels != _|_ {
//	        parameter.labels
//	    }
//	}
func (r *Resource) SpreadIf(cond Condition, path string, value Value) *Resource {
	op := &SpreadIfOp{path: path, value: value, cond: cond}
	if r.currentIf != nil {
		r.currentIf.ops = append(r.currentIf.ops, op)
	} else {
		r.ops = append(r.ops, op)
	}
	return r
}

// If starts a conditional block. Operations until EndIf are conditional.
func (r *Resource) If(cond Condition) *Resource {
	r.currentIf = &IfBlock{
		cond: cond,
		ops:  make([]ResourceOp, 0),
	}
	return r
}

// EndIf ends the current conditional block.
func (r *Resource) EndIf() *Resource {
	if r.currentIf != nil {
		r.ops = append(r.ops, r.currentIf)
		r.currentIf = nil
	}
	return r
}

// Directive records a CUE directive annotation on a field path.
// The directive string should be like "patchKey=ip" and will be rendered as // +patchKey=ip.
func (r *Resource) Directive(path string, directive string) *Resource {
	op := &DirectiveOp{path: path, directive: directive}
	if r.currentIf != nil {
		r.currentIf.ops = append(r.currentIf.ops, op)
	} else {
		r.ops = append(r.ops, op)
	}
	return r
}

// DirectiveOp records a CUE directive annotation on a field path.
// The directive string (e.g. "patchKey=ip") is rendered as // +patchKey=ip.
type DirectiveOp struct {
	path      string
	directive string
}

func (d *DirectiveOp) resourceOp() {}

// Path returns the field path.
func (d *DirectiveOp) Path() string { return d.path }

// GetDirective returns the directive string.
func (d *DirectiveOp) GetDirective() string { return d.directive }
