/*
 Copyright 2021. The KubeVela Authors.

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

package v1beta1

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// A WorkloadDefinitionSpec defines the desired state of a WorkloadDefinition.
type WorkloadDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this workload kind.
	Reference common.DefinitionReference `json:"definitionRef"`

	// ChildResourceKinds are the list of GVK of the child resources this workload generates
	ChildResourceKinds []common.ChildResourceKind `json:"childResourceKinds,omitempty"`

	// RevisionLabel indicates which label for underlying resources(e.g. pods) of this workload
	// can be used by trait to create resource selectors(e.g. label selector for pods).
	// +optional
	RevisionLabel string `json:"revisionLabel,omitempty"`

	// PodSpecPath indicates where/if this workload has K8s podSpec field
	// if one workload has podSpec, trait can do lot's of assumption such as port, env, volume fields.
	// +optional
	PodSpecPath string `json:"podSpecPath,omitempty"`

	// Status defines the custom health policy and status message for workload
	// +optional
	Status *common.Status `json:"status,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the workload
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// WorkloadDefinitionStatus is the status of WorkloadDefinition
type WorkloadDefinitionStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`
}

// +kubebuilder:object:root=true

// A WorkloadDefinition registers a kind of Kubernetes custom resource as a
// valid OAM workload kind by referencing its CustomResourceDefinition. The CRD
// is used to validate the schema of the workload when it is embedded in an OAM
// Component.
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=workload
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="DEFINITION-NAME",type=string,JSONPath=".spec.definitionRef.name"
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=".metadata.annotations.definition\\.oam\\.dev/description"
type WorkloadDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadDefinitionSpec   `json:"spec,omitempty"`
	Status WorkloadDefinitionStatus `json:"status,omitempty"`
}

// SetConditions set condition for WorkloadDefinition
func (wd *WorkloadDefinition) SetConditions(c ...runtimev1alpha1.Condition) {
	wd.Status.SetConditions(c...)
}

// GetCondition gets condition from WorkloadDefinition
func (wd *WorkloadDefinition) GetCondition(conditionType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return wd.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// WorkloadDefinitionList contains a list of WorkloadDefinition.
type WorkloadDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadDefinition `json:"items"`
}

// A TraitDefinitionSpec defines the desired state of a TraitDefinition.
type TraitDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this trait kind.
	Reference common.DefinitionReference `json:"definitionRef,omitempty"`

	// Revision indicates whether a trait is aware of component revision
	// +optional
	RevisionEnabled bool `json:"revisionEnabled,omitempty"`

	// WorkloadRefPath indicates where/if a trait accepts a workloadRef object
	// +optional
	WorkloadRefPath string `json:"workloadRefPath,omitempty"`

	// PodDisruptive specifies whether using the trait will cause the pod to restart or not.
	// +optional
	PodDisruptive bool `json:"podDisruptive,omitempty"`

	// AppliesToWorkloads specifies the list of workload kinds this trait
	// applies to. Workload kinds are specified in kind.group/version format,
	// e.g. server.core.oam.dev/v1alpha2. Traits that omit this field apply to
	// all workload kinds.
	// +optional
	AppliesToWorkloads []string `json:"appliesToWorkloads,omitempty"`

	// ConflictsWith specifies the list of traits(CRD name, Definition name, CRD group)
	// which could not apply to the same workloads with this trait.
	// Traits that omit this field can work with any other traits.
	// Example rules:
	// "service" # Trait definition name
	// "services.k8s.io" # API resource/crd name
	// "*.networking.k8s.io" # API group
	// "labelSelector:foo=bar" # label selector
	// labelSelector format: https://pkg.go.dev/k8s.io/apimachinery/pkg/labels#Parse
	// +optional
	ConflictsWith []string `json:"conflictsWith,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the trait
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Status defines the custom health policy and status message for trait
	// +optional
	Status *common.Status `json:"status,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// TraitDefinitionStatus is the status of TraitDefinition
type TraitDefinitionStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	runtimev1alpha1.ConditionedStatus `json:",inline"`
	// ConfigMapRef refer to a ConfigMap which contains OpenAPI V3 JSON schema of Component parameters.
	ConfigMapRef string `json:"configMapRef,omitempty"`
	// LatestRevision of the component definition
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`
}

// +kubebuilder:object:root=true

// A TraitDefinition registers a kind of Kubernetes custom resource as a valid
// OAM trait kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the trait when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=trait
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="APPLIES-TO",type=string,JSONPath=".spec.appliesToWorkloads"
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=".metadata.annotations.definition\\.oam\\.dev/description"
type TraitDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TraitDefinitionSpec   `json:"spec,omitempty"`
	Status TraitDefinitionStatus `json:"status,omitempty"`
}

// SetConditions set condition for TraitDefinition
func (td *TraitDefinition) SetConditions(c ...runtimev1alpha1.Condition) {
	td.Status.SetConditions(c...)
}

// GetCondition gets condition from TraitDefinition
func (td *TraitDefinition) GetCondition(conditionType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return td.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// TraitDefinitionList contains a list of TraitDefinition.
type TraitDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TraitDefinition `json:"items"`
}

// A ScopeDefinitionSpec defines the desired state of a ScopeDefinition.
type ScopeDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this scope kind.
	Reference common.DefinitionReference `json:"definitionRef"`

	// WorkloadRefsPath indicates if/where a scope accepts workloadRef objects
	WorkloadRefsPath string `json:"workloadRefsPath,omitempty"`

	// AllowComponentOverlap specifies whether an OAM component may exist in
	// multiple instances of this kind of scope.
	AllowComponentOverlap bool `json:"allowComponentOverlap"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// +kubebuilder:object:root=true

// A ScopeDefinition registers a kind of Kubernetes custom resource as a valid
// OAM scope kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the scope when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:printcolumn:JSONPath=".spec.definitionRef.name",name=DEFINITION-NAME,type=string
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=scope
// +kubebuilder:storageversion
type ScopeDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScopeDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ScopeDefinitionList contains a list of ScopeDefinition.
type ScopeDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeDefinition `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// An ResourceTracker represents a tracker for track cross namespace resources
// +kubebuilder:resource:scope=Cluster,categories={oam},shortName=tracker
type ResourceTracker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status ResourceTrackerStatus `json:"status,omitempty"`
}

// ResourceTrackerStatus define the status of resourceTracker
type ResourceTrackerStatus struct {
	TrackedResources []TypedReference `json:"trackedResources,omitempty"`
}

// A TypedReference refers to an object by Name, Kind, and APIVersion. It is
// commonly used to reference across-namespace objects
type TypedReference struct {
	// APIVersion of the referenced object.
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced object.
	Kind string `json:"kind"`

	// Name of the referenced object.
	Name string `json:"name"`

	// Namespace of the objects outside the application namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// UID of the referenced object.
	// +optional
	UID types.UID `json:"uid,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceTrackerList contains a list of ResourceTracker
type ResourceTrackerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceTracker `json:"items"`
}
