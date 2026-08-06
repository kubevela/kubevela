/*
Copyright 2026 The KubeVela Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// ConfigTemplateScope defines where Configs created from this template default to.
type ConfigTemplateScope string

const (
	ConfigTemplateScopeSystem    ConfigTemplateScope = "system"
	ConfigTemplateScopeNamespace ConfigTemplateScope = "namespace"
)

// ConfigTemplateSpec defines the desired state of ConfigTemplate.
type ConfigTemplateSpec struct {
	// Template is the CUE template. It must define a `template: {parameter: {...}, output: {...}}`
	// block, matching the format used by the legacy config-template-* ConfigMaps.
	// +kubebuilder:validation:Required
	Template string `json:"template"`

	// +kubebuilder:validation:Enum=system;namespace
	// +kubebuilder:default=namespace
	Scope ConfigTemplateScope `json:"scope,omitempty"`

	// Sensitive marks Configs created from this template as not safe to read back
	// through the API (e.g. CLI/workflow read operations).
	Sensitive bool `json:"sensitive,omitempty"`

	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
}

// ConfigTemplatePhase describes the lifecycle phase of a ConfigTemplate.
type ConfigTemplatePhase string

const (
	ConfigTemplatePhaseAvailable ConfigTemplatePhase = "Available"
	ConfigTemplatePhaseError     ConfigTemplatePhase = "Error"
)

// ConfigTemplateStatus defines the observed state of ConfigTemplate.
type ConfigTemplateStatus struct {
	condition.ConditionedStatus `json:",inline"`

	Phase ConfigTemplatePhase `json:"phase,omitempty"`

	// Schema is the OpenAPI v3 schema extracted from template.parameter.
	// +kubebuilder:pruning:PreserveUnknownFields
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=ct
// +kubebuilder:printcolumn:name="SCOPE",type=string,JSONPath=`.spec.scope`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigTemplate is the CRD-native replacement for the legacy `config-template-*` ConfigMap convention.
type ConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigTemplateSpec   `json:"spec,omitempty"`
	Status ConfigTemplateStatus `json:"status,omitempty"`
}

// SetConditions implements oam.Conditioned.
func (c *ConfigTemplate) SetConditions(cd ...condition.Condition) {
	c.Status.SetConditions(cd...)
}

// GetCondition implements oam.Conditioned.
func (c *ConfigTemplate) GetCondition(t condition.ConditionType) condition.Condition {
	return c.Status.GetCondition(t)
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigTemplateList contains a list of ConfigTemplate.
type ConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigTemplate `json:"items"`
}
