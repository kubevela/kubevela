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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// SourceDefinitionSpec defines the desired state of SourceDefinition.
type SourceDefinitionSpec struct {
	// Schematic defines source resolution logic.
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Version is the semantic version of this definition. Setting it makes each
	// version a distinct DefinitionRevision, so a binding can pin one with
	// `type: my-source@v1` and stop tracking whatever is newest.
	// +optional
	Version string `json:"version,omitempty"`
}

// SourceDefinitionStatus defines the observed state of SourceDefinition.
type SourceDefinitionStatus struct {
	condition.ConditionedStatus `json:",inline"`
	// ConfigTemplateRef references the ConfigTemplate generated from this SourceDefinition schema.
	// +optional
	ConfigTemplateRef *SourceDefinitionConfigTemplateRef `json:"configTemplateRef,omitempty"`

	// LatestRevision of this source definition.
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`
}

// SourceDefinitionConfigTemplateRef references an auto-generated config template for source schema validation.
type SourceDefinitionConfigTemplateRef struct {
	// Name is the deterministic template name.
	Name string `json:"name,omitempty"`
	// SchemaHash is the sha256 hash (hex) of the canonical schema cue snippet.
	SchemaHash string `json:"schemaHash,omitempty"`
}

// SetConditions sets conditions for SourceDefinition.
func (d *SourceDefinition) SetConditions(c ...condition.Condition) {
	d.Status.SetConditions(c...)
}

// GetCondition gets a condition from SourceDefinition.
func (d *SourceDefinition) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return d.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// SourceDefinition is the Schema for sourcedefinitions API.
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=def-source
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SourceDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SourceDefinitionSpec   `json:"spec,omitempty"`
	Status SourceDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SourceDefinitionList contains a list of SourceDefinition.
type SourceDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SourceDefinition `json:"items"`
}
