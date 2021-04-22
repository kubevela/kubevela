/*
Copyright 2021 The KubeVela Authors.

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

package common

import (
	v1alpha12 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// Kube defines the encapsulation in raw Kubernetes resource format
type Kube struct {
	// Template defines the raw Kubernetes resource
	// +kubebuilder:pruning:PreserveUnknownFields
	Template runtime.RawExtension `json:"template"`

	// Parameters defines configurable parameters
	Parameters []KubeParameter `json:"parameters,omitempty"`
}

// ParameterValueType refers to a data type of parameter
type ParameterValueType string

// data types of parameter value
const (
	StringType  ParameterValueType = "string"
	NumberType  ParameterValueType = "number"
	BooleanType ParameterValueType = "boolean"
)

// A KubeParameter defines a configurable parameter of a component.
type KubeParameter struct {
	// Name of this parameter
	Name string `json:"name"`

	// +kubebuilder:validation:Enum:=string;number;boolean
	// ValueType indicates the type of the parameter value, and
	// only supports basic data types: string, number, boolean.
	ValueType ParameterValueType `json:"type"`

	// FieldPaths specifies an array of fields within this workload that will be
	// overwritten by the value of this parameter. 	All fields must be of the
	// same type. Fields are specified as JSON field paths without a leading
	// dot, for example 'spec.replicas'.
	FieldPaths []string `json:"fieldPaths"`

	// +kubebuilder:default:=false
	// Required specifies whether or not a value for this parameter must be
	// supplied when authoring an Application.
	Required *bool `json:"required,omitempty"`

	// Description of this parameter.
	Description *string `json:"description,omitempty"`
}

// CUE defines the encapsulation in CUE format
type CUE struct {
	// Template defines the abstraction template data of the capability, it will replace the old CUE template in extension field.
	// Template is a required field if CUE is defined in Capability Definition.
	Template string `json:"template"`
}

// Schematic defines the encapsulation of this capability(workload/trait/scope),
// the encapsulation can be defined in different ways, e.g. CUE/HCL(terraform)/KUBE(K8s Object)/HELM, etc...
type Schematic struct {
	KUBE *Kube `json:"kube,omitempty"`

	CUE *CUE `json:"cue,omitempty"`

	HELM *Helm `json:"helm,omitempty"`

	// TODO(wonderflow): support HCL(terraform)here.
}

// A Helm represents resources used by a Helm module
type Helm struct {
	// Release records a Helm release used by a Helm module workload.
	// +kubebuilder:pruning:PreserveUnknownFields
	Release runtime.RawExtension `json:"release"`

	// HelmRelease records a Helm repository used by a Helm module workload.
	// +kubebuilder:pruning:PreserveUnknownFields
	Repository runtime.RawExtension `json:"repository"`
}

// A WorkloadTypeDescriptor refer to a Workload Type
type WorkloadTypeDescriptor struct {
	// Type ref to a WorkloadDefinition via name
	Type string `json:"type,omitempty"`
	// Definition mutually exclusive to workload.type, a embedded WorkloadDefinition
	Definition WorkloadGVK `json:"definition,omitempty"`
}

// WorkloadGVK refer to a Workload Type
type WorkloadGVK struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// A DefinitionReference refers to a CustomResourceDefinition by name.
type DefinitionReference struct {
	// Name of the referenced CustomResourceDefinition.
	Name string `json:"name"`

	// Version indicate which version should be used if CRD has multiple versions
	// by default it will use the first one if not specified
	Version string `json:"version,omitempty"`
}

// A ChildResourceKind defines a child Kubernetes resource kind with a selector
type ChildResourceKind struct {
	// APIVersion of the child resource
	APIVersion string `json:"apiVersion"`

	// Kind of the child resource
	Kind string `json:"kind"`

	// Selector to select the child resources that the workload wants to expose to traits
	Selector map[string]string `json:"selector,omitempty"`
}

// Status defines the loop back status of the abstraction by using CUE template
type Status struct {
	// CustomStatus defines the custom status message that could display to user
	// +optional
	CustomStatus string `json:"customStatus,omitempty"`
	// HealthPolicy defines the health check policy for the abstraction
	// +optional
	HealthPolicy string `json:"healthPolicy,omitempty"`
}

// ApplicationPhase is a label for the condition of a application at the current time
type ApplicationPhase string

const (
	// ApplicationRollingOut means the app is in the middle of rolling out
	ApplicationRollingOut ApplicationPhase = "rollingOut"
	// ApplicationRendering means the app is rendering
	ApplicationRendering ApplicationPhase = "rendering"
	// ApplicationRunning means the app finished rendering and applied result to the cluster
	ApplicationRunning ApplicationPhase = "running"
	// ApplicationHealthChecking means the app finished rendering and applied result to the cluster, but still unhealthy
	ApplicationHealthChecking ApplicationPhase = "healthChecking"
)

// ApplicationComponentStatus record the health status of App component
type ApplicationComponentStatus struct {
	Name string `json:"name"`
	// WorkloadDefinition is the definition of a WorkloadDefinition, such as deployments/apps.v1
	WorkloadDefinition WorkloadGVK                `json:"workloadDefinition,omitempty"`
	Healthy            bool                       `json:"healthy"`
	Message            string                     `json:"message,omitempty"`
	Traits             []ApplicationTraitStatus   `json:"traits,omitempty"`
	Scopes             []v1alpha12.TypedReference `json:"scopes,omitempty"`
}

// ApplicationTraitStatus records the trait health status
type ApplicationTraitStatus struct {
	Type    string `json:"type"`
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// Revision has name and revision number
type Revision struct {
	Name     string `json:"name"`
	Revision int64  `json:"revision"`

	// RevisionHash record the hash value of the spec of ApplicationRevision object.
	RevisionHash string `json:"revisionHash,omitempty"`
}

// RawComponent record raw component
type RawComponent struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Raw runtime.RawExtension `json:"raw"`
}

// AppStatus defines the observed state of Application
type AppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	v1alpha1.RolloutStatus `json:",inline"`

	Phase ApplicationPhase `json:"status,omitempty"`

	// Components record the related Components created by Application Controller
	Components []v1alpha12.TypedReference `json:"components,omitempty"`

	// Services record the status of the application services
	Services []ApplicationComponentStatus `json:"services,omitempty"`

	// ResourceTracker record the status of the ResourceTracker
	ResourceTracker *v1alpha12.TypedReference `json:"resourceTracker,omitempty"`

	// LatestRevision of the application configuration it generates
	// +optional
	LatestRevision *Revision `json:"latestRevision,omitempty"`
}

// DefinitionType describes the type of DefinitionRevision.
// +kubebuilder:validation:Enum=Component;Trait
type DefinitionType string

const (
	// ComponentType represents DefinitionRevision refer to type ComponentDefinition
	ComponentType DefinitionType = "Component"

	// TraitType represents DefinitionRevision refer to type TraitDefinition
	TraitType DefinitionType = "Trait"
)
