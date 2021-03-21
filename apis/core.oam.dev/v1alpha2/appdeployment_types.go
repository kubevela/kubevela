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
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AppDeploymentPhase string

const (
	PhaseRolling   AppDeploymentPhase = "Rolling"
	PhaseCompleted AppDeploymentPhase = "Completed"
)

type HttpRule struct {
	WeightedTargets []WeightedTarget `json:"weightedTargets,omitempty"`
}

type WeightedTarget struct {
	RevisionName string `json:"revisionName,omitempty"`

	ComponentName string `json:"componentName,omitempty"`

	Port int `json:"port,omitempty"`

	Weight int `json:"weight,omitempty"`
}

type Traffic struct {
	Hosts []string `json:"hosts,omitempty"`

	Gateways []string `json:"gateways,omitempty"`

	Http []HttpRule `json:"http,omitempty"`
}

type ClusterSelector struct {
	Name string `json:"name,omitempty"`

	Labels map[string]string `json:"labels,omitempty"`
}

type Distribution struct {
	Replicas int `json:"replicas,omitempty"`
}

type ClusterPlacement struct {
	// ClusterSelector selects the cluster to  deploy apps to.
	// If not specified, it indicates the host cluster per se.
	ClusterSelector *ClusterSelector `json:"clusterSelector,omitempty"`

	Distribution Distribution `json:"distribution,omitempty"`
}

type AppRevision struct {
	RevisionName string `json:"revisionName,omitempty"`

	Placement []ClusterPlacement `json:"placement,omitempty"`
}

type ClusterPlacementStatus struct {
	// ClusterName indicates the name of the cluster to  deploy apps to.
	// If empty, it indicates the host cluster per se.
	ClusterName string `json:"clusterName,omitempty"`

	Replicas int `json:"replicas,omitempty"`
}

type PlacementStatus struct {
	RevisionName string `json:"revisionName,omitempty"`

	Clusters []ClusterPlacementStatus `json:"clusters,omitempty"`
}

// AppDeploymentSpec defines how to describe an upgrade between different apps
type AppDeploymentSpec struct {
	Traffic Traffic `json:"traffic,omitempty"`

	AppRevisions []AppRevision `json:"appRevisions,omitempty"`
}

// AppDeploymentStatus defines the observed state of AppDeployment
type AppDeploymentStatus struct {
	// Conditions represents the latest available observations of a CloneSet's current state.
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// If Phase is Rolling, no update should be made to the spec.
	Phase AppDeploymentPhase `json:"phase,omitempty"`

	Placement []PlacementStatus `json:"placement,omitempty"`
}

// AppDeployment is the Schema for the AppDeployment API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type AppDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppDeploymentSpec   `json:"spec,omitempty"`
	Status AppDeploymentStatus `json:"status,omitempty"`
}

// AppDeploymentList contains a list of AppDeployment
// +kubebuilder:object:root=true
type AppDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppDeployment `json:"items"`
}
