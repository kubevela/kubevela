/*
Copyright 2020 The KubeVela Authors.

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

	v1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// ApplicationDeploymentSpec defines how to describe an upgrade between different application
type ApplicationDeploymentSpec struct {
	// TargetApplicationName contains the name of the application that we need to upgrade to.
	// We assume that an application is immutable, thus the name alone is suffice
	TargetApplicationName string `json:"targetApplicationName"`

	// SourceApplicationName contains the name of the application that we need to upgrade from.
	SourceApplicationName string `json:"sourceApplicationName"`

	// The list of component to upgrade in the application.
	// We only support single component application so far
	// TODO: (RZ) Support multiple components in an application
	// +optional
	ComponentList []string `json:"componentList,omitempty"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan v1alpha1.RolloutPlan `json:"rolloutPlan"`

	// RevertOnDelete revert the rollout when the rollout CR is deleted, default is false
	// It will remove the target application from the kubernetes
	// +optional
	RevertOnDelete bool `json:"revertOnDelete,omitempty"`
}

// ApplicationDeployment is the Schema for the ApplicationDeployment API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type ApplicationDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationDeploymentSpec `json:"spec,omitempty"`
	Status v1alpha1.RolloutStatus    `json:"status,omitempty"`
}

// ApplicationDeploymentList contains a list of ApplicationDeployment
// +kubebuilder:object:root=true
type ApplicationDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationDeployment `json:"items"`
}
