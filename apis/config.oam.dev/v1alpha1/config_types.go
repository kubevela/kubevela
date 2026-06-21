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

// ConfigTemplateReference references a ConfigTemplate by name and namespace. If no
// ConfigTemplate CRD with this name exists in the namespace, the controller falls
// back to a legacy `config-template-<name>` ConfigMap in the same namespace, for
// backward compatibility with addons that still install ConfigMap-based templates.
type ConfigTemplateReference struct {
	// Name of the ConfigTemplate (or the suffix of the legacy config-template-<name>
	// ConfigMap).
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace to look up the template in. Defaults to "vela-system" if empty,
	// matching the legacy CLI/factory lookup behavior.
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeySelector selects a key of a Secret in the Config's own namespace.
type SecretKeySelector struct {
	// Name of the secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the secret's data. Defaults to "properties".
	Key string `json:"key,omitempty"`
}

// PropertiesReference is the source of properties that must not be stored inline
// in the Config spec (e.g. credentials).
type PropertiesReference struct {
	// SecretRef points at a Secret containing the JSON-encoded properties.
	// +kubebuilder:validation:Required
	SecretRef SecretKeySelector `json:"secretRef"`
}

// ConfigSpec defines the desired state of Config.
type ConfigSpec struct {
	// TemplateRef is the ConfigTemplate (or legacy ConfigMap) this Config is rendered
	// from. If empty, the Config's properties are materialized as-is with no template
	// evaluation.
	TemplateRef *ConfigTemplateReference `json:"templateRef,omitempty"`

	// Properties are inline, non-sensitive properties. Mutually exclusive with
	// PropertiesFrom.
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`

	// PropertiesFrom sources properties from a Secret, for sensitive values that must
	// not be stored inline in the Config. Mutually exclusive with Properties.
	PropertiesFrom *PropertiesReference `json:"propertiesFrom,omitempty"`

	// Alias is a human-readable name for the config.
	Alias string `json:"alias,omitempty"`

	// Description documents what the config is for.
	Description string `json:"description,omitempty"`
}

// ConfigPhase describes the lifecycle phase of a Config.
type ConfigPhase string

const (
	// ConfigPhaseAvailable means the Config was rendered and its output Secret
	// materialized successfully.
	ConfigPhaseAvailable ConfigPhase = "Available"
	// ConfigPhaseError means the Config failed to resolve its template/properties or
	// render its output Secret.
	ConfigPhaseError ConfigPhase = "Error"
)

// ConfigStatus defines the observed state of Config.
type ConfigStatus struct {
	condition.ConditionedStatus `json:",inline"`

	// Phase is the current lifecycle phase of the Config.
	Phase ConfigPhase `json:"phase,omitempty"`

	// SecretRef is the materialized output Secret, owned by this Config.
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

// Config is the schema for the configs API. It is the CRD-native replacement for
// the legacy labeled-Secret config convention.
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

// SetConditions sets the supplied conditions on the status, implementing oam.Conditioned.
func (c *Config) SetConditions(cd ...condition.Condition) {
	c.Status.SetConditions(cd...)
}

// GetCondition returns the condition for the given ConditionType, implementing oam.Conditioned.
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
