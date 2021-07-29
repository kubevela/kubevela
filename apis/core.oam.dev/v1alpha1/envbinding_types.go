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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// ClusterManagementEngine represents a multi-cluster management solution
type ClusterManagementEngine string

const (
	// OCMEngine represents Open-Cluster-Management multi-cluster management solution
	OCMEngine ClusterManagementEngine = "ocm"
)

// EnvBindingPhase is a label for the condition of a EnvBinding at the current time
type EnvBindingPhase string

const (
	// EnvBindingPrepare means EnvBinding is preparing the pre-work for cluster scheduling
	EnvBindingPrepare EnvBindingPhase = "preparing"

	// EnvBindingRendering means EnvBinding is rendering the apps in different envs
	EnvBindingRendering EnvBindingPhase = "rendering"

	// EnvBindingScheduling means EnvBinding is deciding which cluster the apps is scheduled to.
	EnvBindingScheduling EnvBindingPhase = "scheduling"

	// EnvBindingFinished means EnvBinding finished env binding
	EnvBindingFinished EnvBindingPhase = "finished"
)

// EnvPatch specify the parameter configuration for different environments
type EnvPatch struct {
	Components []common.ApplicationComponent `json:"components"`
}

// EnvConfig is the configuration for different environments.
type EnvConfig struct {
	Name      string                  `json:"name"`
	Placement common.ClusterPlacement `json:"placement"`
	Patch     EnvPatch                `json:"patch"`
}

// AppTemplate represents a application to be configured.
type AppTemplate struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	runtime.RawExtension `json:",inline"`
}

// ClusterDecision recorded the mapping of environment and cluster
type ClusterDecision struct {
	EnvName     string `json:"env_name"`
	ClusterName string `json:"cluster_name"`
}

// A EnvBindingSpec defines the desired state of a EnvBinding.
type EnvBindingSpec struct {
	Engine ClusterManagementEngine `json:"engine,omitempty"`

	// AppTemplate indicates the application template.
	AppTemplate AppTemplate `json:"appTemplate"`

	Envs []EnvConfig `json:"envs"`
}

// A EnvBindingStatus is the status of EnvBinding
type EnvBindingStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	condition.ConditionedStatus `json:",inline"`

	Phase EnvBindingPhase `json:"phase,omitempty"`

	ClusterDecisions []ClusterDecision `json:"cluster_decisions,omitempty"`
}

// EnvBinding is the Schema for the EnvBinding API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=envbind
// +kubebuilder:printcolumn:name="ENGINE",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type EnvBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvBindingSpec   `json:"spec,omitempty"`
	Status EnvBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvBindingList contains a list of EnvBinding.
type EnvBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvBinding `json:"items"`
}

// SetConditions set condition for EnvBinding
func (e *EnvBinding) SetConditions(c ...condition.Condition) {
	e.Status.SetConditions(c...)
}

// GetCondition gets condition from EnvBinding
func (e *EnvBinding) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return e.Status.GetCondition(conditionType)
}
