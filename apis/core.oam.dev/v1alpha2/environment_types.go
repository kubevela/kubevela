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
)

// EnvironmentSpec defines vela workload of Environment
type EnvironmentSpec struct {

	// FIXME vela Namespace duplicate from k8s namesapce

	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// +kubebuilder:validation:Type=string
	// +optional
	Email string `json:"email,omitempty"`

	// +kubebuilder:validation:Type=string
	// +optional
	Domain string `json:"domain,omitempty"`

	// +kubebuilder:validation:Type=string
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// +optional
	CapabilityList []string `json:"capabilityList,omitempty"`
}

// EnvironmentStatus defines the observed state of Environment
type EnvironmentStatus struct {
	// TODO Support aggregation workload status?
}

// +kubebuilder:object:root=true

// Environment is vela Managed environments
// +kubebuilder:resource:scope=Cluster
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}
