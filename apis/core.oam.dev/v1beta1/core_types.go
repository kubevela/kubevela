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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

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
	// applies to. Workload kinds are specified in resource.group/version format,
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

	// Schematic defines the data format and template of the encapsulation of the trait.
	// Only CUE and Kube schematic are supported for now.
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Status defines the custom health policy and status message for trait
	// +optional
	Status *common.Status `json:"status,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`

	// ManageWorkload defines the trait would be responsible for creating the workload
	// +optional
	ManageWorkload bool `json:"manageWorkload,omitempty"`
	// ControlPlaneOnly defines which cluster is dispatched to
	// +optional
	ControlPlaneOnly bool `json:"controlPlaneOnly,omitempty"`

	// Stage defines the stage information to which this trait resource processing belongs.
	// Currently, PreDispatch and PostDispatch are provided, which are used to control resource
	// pre-process and post-process respectively.
	// +optional
	Stage StageType `json:"stage,omitempty"`
}

// StageType describes how the manifests should be dispatched.
// Only one of the following stage types may be specified.
// If none of the following types is specified, the default one
// is DefaultDispatch.
type StageType string

const (
	// PreDispatch means that pre dispatch for manifests
	PreDispatch StageType = "PreDispatch"
	// DefaultDispatch means that default dispatch for manifests
	DefaultDispatch StageType = "DefaultDispatch"
	// PostDispatch means that post dispatch for manifests
	PostDispatch StageType = "PostDispatch"
)

// TraitDefinitionStatus is the status of TraitDefinition
type TraitDefinitionStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	condition.ConditionedStatus `json:",inline"`
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
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type TraitDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TraitDefinitionSpec   `json:"spec,omitempty"`
	Status TraitDefinitionStatus `json:"status,omitempty"`
}

// SetConditions set condition for TraitDefinition
func (td *TraitDefinition) SetConditions(c ...condition.Condition) {
	td.Status.SetConditions(c...)
}

// GetCondition gets condition from TraitDefinition
func (td *TraitDefinition) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return td.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TraitDefinitionList contains a list of TraitDefinition.
type TraitDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TraitDefinition `json:"items"`
}
