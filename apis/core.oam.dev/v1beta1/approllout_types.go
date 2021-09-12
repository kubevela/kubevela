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

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// AppRolloutSpec defines how to describe an upgrade between different apps
type AppRolloutSpec struct {
	// TargetAppRevisionName contains the name of the applicationConfiguration that we need to upgrade to.
	// Here we use an applicationConfiguration as a revision of an application, thus the name alone is suffice
	TargetAppRevisionName string `json:"targetAppRevisionName"`

	// SourceAppRevisionName contains the name of the applicationConfiguration that we need to upgrade from.
	// it can be empty only when it's the first time to deploy the application
	SourceAppRevisionName string `json:"sourceAppRevisionName,omitempty"`

	// The list of component to upgrade in the application.
	// We only support single component application so far
	// TODO: (RZ) Support multiple components in an application
	// +optional
	ComponentList []string `json:"componentList,omitempty"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan v1alpha1.RolloutPlan `json:"rolloutPlan"`

	// RevertOnDelete revert the failed rollout when the rollout CR is deleted
	// It will revert the change back to the source version at once (not in batches)
	// Default is false
	// +optional
	RevertOnDelete bool `json:"revertOnDelete,omitempty"`
}

// AppRollout is the Schema for the AppRollout API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam},shortName=approllout
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.status.rolloutTargetSize`
// +kubebuilder:printcolumn:name="UPGRADED",type=string,JSONPath=`.status.upgradedReplicas`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.upgradedReadyReplicas`
// +kubebuilder:printcolumn:name="BATCH-STATE",type=string,JSONPath=`.status.batchRollingState`
// +kubebuilder:printcolumn:name="ROLLING-STATE",type=string,JSONPath=`.status.rollingState`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AppRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRolloutSpec          `json:"spec,omitempty"`
	Status common.AppRolloutStatus `json:"status,omitempty"`
}

// AppRolloutList contains a list of AppRollout
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AppRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppRollout `json:"items"`
}
