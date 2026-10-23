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

	// PropertiesFrom reads the properties from a Secret. A Config that uses it is always
	// sensitive: its Secret is marked config.oam.dev/sensitive, whatever the template says.
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

	// ObservedGeneration is the spec generation the rest of the status describes, so a
	// reader can tell an Available from the last spec apart from one for the current
	// spec; the create-config workflow step waits on it.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Phase     ConfigPhase                  `json:"phase,omitempty"`
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// ExpandedWriter records the last confirmed sync of the template's expanded writer
	// block (today: nacos), published or found current. Unset when the template has no
	// such block.
	ExpandedWriter *ExpandedWriterStatus `json:"expandedWriter,omitempty"`
}

// ExpandedWriterStatus holds one entry per expanded writer the template declares.
type ExpandedWriterStatus struct {
	Nacos *NacosSyncStatus `json:"nacos,omitempty"`
}

// NacosSyncStatus is the last confirmed sync of the template's nacos block: where the
// content lives in Nacos and a hash of the content and its target, recorded both when
// the controller published and when it found Nacos already current (a config adopted
// from the CLI, say), so the next reconcile publishes again only when the rendered
// content or its target changes.
type NacosSyncStatus struct {
	DataID      string `json:"dataId"`
	Group       string `json:"group"`
	ContentHash string `json:"contentHash"`
	// Endpoint is the endpoint config (the config made from the nacos-server template) the
	// template names, recorded on every sync attempt whether it succeeded or not, so a
	// change to that config re-triggers exactly the Configs that use it, including the
	// ones still waiting for it to exist.
	Endpoint *NacosEndpointReference `json:"endpoint,omitempty"`
	// EndpointHash is a digest of that endpoint config's server part (addresses, ports,
	// discovery endpoint, region), never its credentials. A changed endpoint, another
	// server say, is re-synced like changed content: read first, published only when the
	// server does not hold it. Rotated credentials change nothing in Nacos and are not
	// part of it; the status is readable by whoever can read Configs.
	EndpointHash string `json:"endpointHash,omitempty"`
	// LastSyncTime is when the content was last confirmed current in Nacos, by a
	// publish or by a read that found it already there.
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// NacosEndpointReference names an endpoint config Secret.
type NacosEndpointReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ConditionNacosSynced reports whether the content rendered from the template's nacos
// block is what Nacos holds. False carries the reason and the error of the last attempt;
// the controller keeps retrying with backoff, and the Config's phase stays Available
// because its Secret and outputs are already in place.
const ConditionNacosSynced condition.ConditionType = "NacosSynced"

// Reasons of the NacosSynced condition.
const (
	// ReasonNacosAlreadyCurrent: the first sync found the content already in Nacos, so
	// nothing was published (a config the CLI published before the controller took over).
	ReasonNacosAlreadyCurrent condition.ConditionReason = "AlreadyCurrent"
	// ReasonNacosPublished: the controller published the rendered content.
	ReasonNacosPublished condition.ConditionReason = "Published"
	// ReasonNacosEndpointNotFound: the endpoint config named by the template does not exist.
	ReasonNacosEndpointNotFound condition.ConditionReason = "EndpointNotFound"
	// ReasonNacosEndpointUnreadable: the endpoint config exists but cannot be used (no
	// properties, undecodable, the API refused the read, or no Nacos client can be built from
	// what it holds). A sensitive endpoint config is read like any other.
	ReasonNacosEndpointUnreadable condition.ConditionReason = "EndpointUnreadable"
	// ReasonNacosReadFailed: the first-sync read from Nacos failed.
	ReasonNacosReadFailed condition.ConditionReason = "ReadFailed"
	// ReasonNacosPublishFailed: the publish to Nacos failed.
	ReasonNacosPublishFailed condition.ConditionReason = "PublishFailed"
)

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
