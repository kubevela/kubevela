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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// ConfigTemplateReference references a ConfigTemplate, falling back to a legacy
// config-template-<name> ConfigMap if no CRD with this name exists.
type ConfigTemplateReference struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace defaults to "vela-system" if empty.
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeySelector selects a key of a Secret in the Config's own namespace.
type SecretKeySelector struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key defaults to "properties".
	Key string `json:"key,omitempty"`
}

// PropertiesReference sources properties from a Secret instead of storing them inline.
type PropertiesReference struct {
	// +kubebuilder:validation:Required
	SecretRef SecretKeySelector `json:"secretRef"`
}

// ConfigSpec defines the desired state of Config.
type ConfigSpec struct {
	// TemplateRef is the ConfigTemplate (or legacy ConfigMap) to render against. If
	// empty, Properties are materialized as-is with no template evaluation.
	TemplateRef *ConfigTemplateReference `json:"templateRef,omitempty"`

	// Properties and PropertiesFrom are mutually exclusive.
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`

	PropertiesFrom *PropertiesReference `json:"propertiesFrom,omitempty"`

	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
}

// ConfigPhase describes the lifecycle phase of a Config.
type ConfigPhase string

const (
	ConfigPhaseAvailable ConfigPhase = "Available"
	ConfigPhaseError     ConfigPhase = "Error"
)

// ConfigStatus defines the observed state of Config.
type ConfigStatus struct {
	condition.ConditionedStatus `json:",inline"`

	Phase     ConfigPhase                  `json:"phase,omitempty"`
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=cfg
// +kubebuilder:printcolumn:name="TEMPLATE",type=string,JSONPath=`.spec.templateRef.name`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Config is the CRD-native replacement for the legacy labeled-Secret config convention.
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

// SetConditions implements oam.Conditioned.
func (c *Config) SetConditions(cd ...condition.Condition) {
	c.Status.SetConditions(cd...)
}

// GetCondition implements oam.Conditioned.
func (c *Config) GetCondition(t condition.ConditionType) condition.Condition {
	return c.Status.GetCondition(t)
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigList contains a list of Config.
type ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Config `json:"items"`
}
