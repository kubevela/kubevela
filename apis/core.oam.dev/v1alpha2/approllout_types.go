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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// AppRolloutSpec defines how to describe an upgrade between different apps
type AppRolloutSpec struct {
	// TargetAppRevisionName contains the name of the applicationRevision that we need to upgrade to.
	TargetAppRevisionName string `json:"targetAppRevisionName"`

	// SourceAppRevisionName contains the name of the applicationRevision that we need to upgrade from.
	// it can be empty only when the rolling is only a scale event
	SourceAppRevisionName string `json:"sourceAppRevisionName,omitempty"`

	// The list of component to upgrade in the application.
	// We only support single component application so far
	// TODO: (RZ) Support multiple components in an application
	// +optional
	ComponentList []string `json:"componentList,omitempty"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan v1alpha1.RolloutPlan `json:"rolloutPlan"`

	// RevertOnDelete revert the rollout when the rollout CR is deleted
	// It will remove the target app from the kubernetes if it's set to true
	// +optional
	RevertOnDelete *bool `json:"revertOnDelete,omitempty"`
}

// AppRolloutStatus defines the observed state of AppRollout
type AppRolloutStatus struct {
	v1alpha1.RolloutStatus `json:",inline"`

	// LastUpgradedTargetAppRevision contains the name of the app that we upgraded to
	// We will restart the rollout if this is not the same as the spec
	LastUpgradedTargetAppRevision string `json:"lastTargetAppRevision"`

	// LastSourceAppRevision contains the name of the app that we need to upgrade from.
	// We will restart the rollout if this is not the same as the spec
	LastSourceAppRevision string `json:"LastSourceAppRevision,omitempty"`
}

// AppRollout is the Schema for the AppRollout API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam},shortName=approllout
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.status.rolloutStatus.rolloutTargetSize`
// +kubebuilder:printcolumn:name="UPGRADED",type=string,JSONPath=`.status.rolloutStatus.upgradedReplicas`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.rolloutStatus.upgradedReadyReplicas`
// +kubebuilder:printcolumn:name="BATCH-STATE",type=string,JSONPath=`.status.rolloutStatus.batchRollingState`
// +kubebuilder:printcolumn:name="ROLLING-STATE",type=string,JSONPath=`.status.rolloutStatus.rollingState`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type AppRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRolloutSpec   `json:"spec,omitempty"`
	Status AppRolloutStatus `json:"status,omitempty"`
}

// AppRolloutList contains a list of AppRollout
// +kubebuilder:object:root=true
type AppRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppRollout `json:"items"`
}
