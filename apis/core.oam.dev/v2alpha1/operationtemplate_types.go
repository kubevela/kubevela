/*
 Copyright 2026. The KubeVela Authors.

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

package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
)

// OperationAttachScope is the kind of source an OperationTemplate can be
// invoked against.
type OperationAttachScope string

const (
	// OperationAttachScopeComponent means the template is invoked against a
	// single Component.
	OperationAttachScopeComponent OperationAttachScope = "Component"
	// OperationAttachScopeApplication means the template is invoked against
	// an Application as a whole, selected by label.
	OperationAttachScopeApplication OperationAttachScope = "Application"
	// OperationAttachScopeNone means the template carries no OAM source at
	// all -- no placement resolution, no OAM context. Parity with
	// WorkflowRun inside the Operation primitive.
	OperationAttachScopeNone OperationAttachScope = "None"
)

// OperationApplicationSelector restricts which Applications an
// Application-scoped OperationTemplate may be invoked against.
type OperationApplicationSelector struct {
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// CEL rule enforces valid operator values at admission (the vendored
	// LabelSelectorRequirement type has no such constraint of its own).
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(e, e.operator in ['In','NotIn','Exists','DoesNotExist'])",message="operator must be one of In, NotIn, Exists, DoesNotExist"
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
	// RequiredComponentTypes: every listed ComponentDefinition type must be
	// present among the Application's components for a match.
	// +optional
	RequiredComponentTypes []string `json:"requiredComponentTypes,omitempty"`
}

// OperationAttach describes what an OperationTemplate can be invoked against.
type OperationAttach struct {
	// Scope is the kind of source this template attaches to.
	// +kubebuilder:validation:Enum=Component;Application;None
	// +kubebuilder:default=Component
	Scope OperationAttachScope `json:"scope,omitempty"`

	// AllowedComponentTypes restricts which ComponentDefinition types this
	// template may be invoked against. Component scope only. Empty means
	// any type is allowed.
	// +optional
	AllowedComponentTypes []string `json:"allowedComponentTypes,omitempty"`

	// Selector restricts which Applications this template may be invoked
	// against. Application scope only.
	// +optional
	Selector *OperationApplicationSelector `json:"selector,omitempty"`
}

// OperationTemplateParameters declares the input parameters an Operation may
// supply via spec.parameters, as a `parameter: {...}` CUE definition -- the
// same shape used by ComponentDefinition/WorkflowStepDefinition schematics
// (see common.Schematic). The OpenAPI schema shown by `vela operation list`
// is derived from this via pkg/utils/common.GenOpenAPI; there is no separate
// schema format to author.
type OperationTemplateParameters struct {
	// CUE is a `parameter: {...}` definition.
	CUE string `json:"cue,omitempty"`
}

// OperationTemplateSpec is the spec of OperationTemplate.
type OperationTemplateSpec struct {
	// Description is a human-readable summary shown by `vela operation list`.
	// +optional
	Description string `json:"description,omitempty"`

	// Attach describes what this template can be invoked against.
	Attach OperationAttach `json:"attach"`

	// Parameters declares the parameters accepted by this template.
	// +optional
	Parameters *OperationTemplateParameters `json:"parameters,omitempty"`

	// Workflow is the set of steps run when this template is invoked.
	Workflow wfv1alpha1.WorkflowSpec `json:"workflow"`
}

// +kubebuilder:object:root=true

// OperationTemplate is the Schema for the OperationTemplate API.
//
// KEP 2.15 permission model: not implemented yet. Any RBAC principal able
// to create an Operation can invoke any OperationTemplate against any
// source in its namespace. Do not release or promote this code path
// until the permission model lands.
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam},shortName={opt,optemplate}
// +kubebuilder:printcolumn:name="SCOPE",type=string,JSONPath=`.spec.attach.scope`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OperationTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// OperationTemplateList contains a list of OperationTemplate.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperationTemplate `json:"items"`
}
