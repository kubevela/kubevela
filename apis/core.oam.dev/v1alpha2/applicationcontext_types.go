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
)

// ApplicationContextSpec is the spec of ApplicationContext
type ApplicationContextSpec struct {
	// ApplicationRevisionName points to the snapshot of an Application with all its closure
	ApplicationRevisionName string `json:"applicationRevisionName"`
}

// ApplicationContext is the Schema for the ApplicationContext API
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=appcontext;context,categories={oam}
// +kubebuilder:subresource:status
type ApplicationContext struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ApplicationContextSpec `json:"spec,omitempty"`
	// we need to reuse the AC status
	Status ApplicationConfigurationStatus `json:"status,omitempty"`
}

// ApplicationContextList contains a list of ApplicationContext
// +kubebuilder:object:root=true
type ApplicationContextList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationContext `json:"items"`
}
