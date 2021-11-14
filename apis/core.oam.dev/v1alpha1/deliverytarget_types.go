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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeliveryTargetKubernetes defines the spec of kubernetes delivery target
type DeliveryTargetKubernetes struct {
	// Cluster the name of the target cluster, empty or local indicates that the cluster is the managing cluster
	Cluster string `json:"cluster,omitempty"`
	// Namespace the name of the target namespace, if empty, it is reserved to inherit the namespace that application uses
	Namespace string `json:"namespace,omitempty"`
}

// DeliveryTargetCloudResource defines the spec of cloud resource delivery target
type DeliveryTargetCloudResource struct {
	Provider string `json:"provider"`
	Region   string `json:"region"`
	Zone     string `json:"zone,omitempty"`
	VPC      string `json:"vpc,omitempty"`
}

// DeliveryTargetSpec defines the spec of delivery target
type DeliveryTargetSpec struct {
	Kubernetes    *DeliveryTargetKubernetes    `json:"kubernetes,omitempty"`
	CloudResource *DeliveryTargetCloudResource `json:"cloud_resource,omitempty"`
}

// DeliveryTargetStatus defines the observed state of delivery target
type DeliveryTargetStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=dt
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeliveryTarget is the Schema for application delivery target (cluster, namespace)
type DeliveryTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeliveryTargetSpec   `json:"spec,omitempty"`
	Status DeliveryTargetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeliveryTargetList contains a list of DeliveryTarget
type DeliveryTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeliveryTarget `json:"items"`
}
