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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// DefinitionRevisionSpec is the spec of DefinitionRevision
type DefinitionRevisionSpec struct {
	// Revision record revision number of DefinitionRevision
	Revision int64 `json:"revision"`

	// RevisionHash record the hash value of the spec of DefinitionRevision object.
	RevisionHash string `json:"revisionHash"`

	// DefinitionType
	DefinitionType common.DefinitionType `json:"definitionType"`

	// ComponentDefinition records the snapshot of the created/modified ComponentDefinition
	ComponentDefinition ComponentDefinition `json:"componentDefinition,omitempty"`

	// TraitDefinition records the snapshot of the created/modified TraitDefinition
	TraitDefinition TraitDefinition `json:"traitDefinition,omitempty"`
}

// +kubebuilder:object:root=true

// DefinitionRevision is the Schema for the DefinitionRevision API
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=comprev
// +kubebuilder:printcolumn:name="REVISION",type=integer,JSONPath=".spec.revision"
// +kubebuilder:printcolumn:name="HASH",type=string,JSONPath=".spec.revisionHash"
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=".spec.definitionType"
type DefinitionRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DefinitionRevisionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// DefinitionRevisionList contains a list of DefinitionRevision
type DefinitionRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DefinitionRevision `json:"items"`
}
