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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Rollout is the Schema for the Rollout API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam},shortName=rollout
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.status.rolloutTargetSize`
// +kubebuilder:printcolumn:name="UPGRADED",type=string,JSONPath=`.status.upgradedReplicas`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.upgradedReadyReplicas`
// +kubebuilder:printcolumn:name="BATCH-STATE",type=string,JSONPath=`.status.batchRollingState`
// +kubebuilder:printcolumn:name="ROLLING-STATE",type=string,JSONPath=`.status.rollingState`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type Rollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutSpec       `json:"spec,omitempty"`
	Status CompRolloutStatus `json:"status,omitempty"`
}

// RolloutSpec defines how to describe an update between different compRevision
type RolloutSpec struct {
	// TargetRevisionName contains the name of the componentRevisionName that we need to upgrade to.
	TargetRevisionName string `json:"targetRevisionName"`

	// SourceRevisionName contains the name of the componentRevisionName  that we need to upgrade from.
	// it can be empty only when it's the first time to deploy the application
	SourceRevisionName string `json:"sourceRevisionName,omitempty"`

	// ComponentName specify the component name
	ComponentName string `json:"componentName"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan RolloutPlan `json:"rolloutPlan"`
}

// CompRolloutStatus defines the observed state of rollout
type CompRolloutStatus struct {
	RolloutStatus `json:",inline"`

	// LastUpgradedTargetRevision contains the name of the componentRevisionName that we upgraded to
	// We will restart the rollout if this is not the same as the spec
	LastUpgradedTargetRevision string `json:"lastTargetRevision"`

	// LastSourceRevision contains the name of the componentRevisionName that we need to upgrade from.
	// We will restart the rollout if this is not the same as the spec
	LastSourceRevision string `json:"LastSourceRevision,omitempty"`
}

// RolloutList contains a list of Rollout
// +kubebuilder:object:root=true
type RolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rollout `json:"items"`
}
