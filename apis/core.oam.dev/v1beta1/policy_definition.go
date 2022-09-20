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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// PolicyDefinitionSpec defines the desired state of PolicyDefinition
type PolicyDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this trait kind.
	Reference common.DefinitionReference `json:"definitionRef,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the policy definition.
	// Only CUE schematic is supported for now.
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// ManageHealthCheck means the policy will handle health checking and skip application controller
	// built-in health checking.
	ManageHealthCheck bool `json:"manageHealthCheck,omitempty"`
}

// PolicyDefinitionStatus is the status of PolicyDefinition
type PolicyDefinitionStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	condition.ConditionedStatus `json:",inline"`

	// ConfigMapRef refer to a ConfigMap which contains OpenAPI V3 JSON schema of Component parameters.
	ConfigMapRef string `json:"configMapRef,omitempty"`

	// LatestRevision of the component definition
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`
}

// SetConditions set condition for PolicyDefinition
func (d *PolicyDefinition) SetConditions(c ...condition.Condition) {
	d.Status.SetConditions(c...)
}

// GetCondition gets condition from PolicyDefinition
func (d *PolicyDefinition) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return d.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// PolicyDefinition is the Schema for the policydefinitions API
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=def-policy
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PolicyDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicyDefinitionSpec   `json:"spec,omitempty"`
	Status PolicyDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PolicyDefinitionList contains a list of PolicyDefinition
type PolicyDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyDefinition `json:"items"`
}
