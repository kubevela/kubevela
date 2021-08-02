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

package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// APIs copied from open-cluster-management-io/api/cluster/v1alpha1/

// Placement defines a rule to select a set of ManagedClusters from the ManagedClusterSets bound
// to the placement namespace.
type Placement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the attributes of Placement.
	// +kubebuilder:validation:Required
	// +required
	Spec PlacementSpec `json:"spec"`

	// Status represents the current status of the Placement
	// +optional
	Status PlacementStatus `json:"status,omitempty"`
}

// PlacementSpec defines the attributes of Placement.
// An empty PlacementSpec selects all ManagedClusters from the ManagedClusterSets bound to
// the placement namespace. The containing fields are ANDed.
type PlacementSpec struct {
	// ClusterSets represent the ManagedClusterSets from which the ManagedClusters are selected.
	// +optional
	ClusterSets []string `json:"clusterSets,omitempty"`

	// NumberOfClusters represents the desired number of ManagedClusters to be selected which meet the
	// placement requirements.
	// +optional
	NumberOfClusters *int32 `json:"numberOfClusters,omitempty"`

	// Predicates represent a slice of predicates to select ManagedClusters. The predicates are ORed.
	// +optional
	Predicates []ClusterPredicate `json:"predicates,omitempty"`
}

// PlacementStatus represents the current status of the Placement.
type PlacementStatus struct {
	// NumberOfSelectedClusters represents the number of selected ManagedClusters
	// +optional
	NumberOfSelectedClusters int32 `json:"numberOfSelectedClusters"`

	// Conditions contains the different condition statuses for this Placement.
	// +optional
	// Conditions []metav1.Condition `json:"conditions"`
}

// ClusterPredicate represents a predicate to select ManagedClusters.
type ClusterPredicate struct {
	// RequiredClusterSelector represents a selector of ManagedClusters by label and claim.
	// +optional
	RequiredClusterSelector ClusterSelector `json:"requiredClusterSelector,omitempty"`
}

// ClusterSelector represents the AND of the containing selectors. An empty cluster selector matches all objects.
// A null cluster selector matches no objects.
type ClusterSelector struct {
	// LabelSelector represents a selector of ManagedClusters by label
	// +optional
	LabelSelector metav1.LabelSelector `json:"labelSelector,omitempty"`

	// ClaimSelector represents a selector of ManagedClusters by clusterClaims in status
	// +optional
	ClaimSelector ClusterClaimSelector `json:"claimSelector,omitempty"`
}

// ClusterClaimSelector is a claim query over a set of ManagedClusters. An empty cluster claim
// selector matches all objects. A null cluster claim selector matches no objects.
type ClusterClaimSelector struct {
	// matchExpressions is a list of cluster claim selector requirements. The requirements are ANDed.
	// +optional
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// PlacementDecision indicates a decision from a placement
// PlacementDecision should has a label cluster.open-cluster-management.io/placement={placement name}
// to reference a certain placement.
type PlacementDecision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Status represents the current status of the PlacementDecision
	// +optional
	Status PlacementDecisionStatus `json:"status,omitempty"`
}

// PlacementDecisionStatus represents the current status of the PlacementDecision.
type PlacementDecisionStatus struct {
	// Decisions is a slice of decisions according to a placement
	// The number of decisions should not be larger than 100
	// +kubebuilder:validation:Required
	// +required
	Decisions []ClusterDecision `json:"decisions"`
}

// ClusterDecision represents a decision from a placement
// An empty ClusterDecision indicates it is not scheduled yet.
type ClusterDecision struct {
	// ClusterName is the name of the ManagedCluster. If it is not empty, its value should be unique cross all
	// placement decisions for the Placement.
	// +kubebuilder:validation:Required
	// +required
	ClusterName string `json:"clusterName"`

	// Reason represents the reason why the ManagedCluster is selected.
	// +kubebuilder:validation:Required
	// +required
	Reason string `json:"reason"`
}

// PlacementDecisionList is a collection of PlacementDecision.
type PlacementDecisionList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of PlacementDecision.
	Items []PlacementDecision `json:"items"`
}

// APIs copied from open-cluster-management-io/api/work/v1

// ManifestWork represents a manifests workload that hub wants to deploy on the managed cluster.
// A manifest workload is defined as a set of Kubernetes resources.
// ManifestWork must be created in the cluster namespace on the hub, so that agent on the
// corresponding managed cluster can access this resource and deploy on the managed
// cluster.
type ManifestWork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec represents a desired configuration of work to be deployed on the managed cluster.
	Spec ManifestWorkSpec `json:"spec"`

	// Status represents the current status of work.
	// +optional
	Status ManifestWorkStatus `json:"status,omitempty"`
}

// ManifestWorkSpec represents a desired configuration of manifests to be deployed on the managed cluster.
type ManifestWorkSpec struct {
	// Workload represents the manifest workload to be deployed on a managed cluster.
	Workload ManifestsTemplate `json:"workload,omitempty"`
}

// ManifestWorkStatus represents the current status of managed cluster ManifestWork.
type ManifestWorkStatus struct {
	// Conditions contains the different condition statuses for this work.
	// Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResourceStatus represents the status of each resource in manifestwork deployed on a
	// managed cluster. The Klusterlet agent on managed cluster syncs the condition from the managed cluster to the hub.
	// +optional
	ResourceStatus ManifestResourceStatus `json:"resourceStatus,omitempty"`
}

// ManifestResourceStatus represents the status of each resource in manifest work deployed on
// managed cluster
type ManifestResourceStatus struct {
	// Manifests represents the condition of manifests deployed on managed cluster.
	// Valid condition types are:
	Manifests []ManifestCondition `json:"manifests,omitempty"`
}

// ManifestCondition represents the conditions of the resources deployed on a
// managed cluster.
type ManifestCondition struct {
	// ResourceMeta represents the group, version, kind, name and namespace of a resoure.
	// +required
	ResourceMeta ManifestResourceMeta `json:"resourceMeta"`

	// Conditions represents the conditions of this resource on a managed cluster.
	// +required
	// Conditions []metav1.Condition `json:"conditions"`
}

// ManifestResourceMeta represents the group, version, kind, as well as the group, version, resource, name and namespace of a resoure.
type ManifestResourceMeta struct {
	// Ordinal represents the index of the manifest on spec.
	// +required
	Ordinal int32 `json:"ordinal"`

	// Group is the API Group of the Kubernetes resource.
	// +optional
	Group string `json:"group"`

	// Version is the version of the Kubernetes resource.
	// +optional
	Version string `json:"version"`

	// Kind is the kind of the Kubernetes resource.
	// +optional
	Kind string `json:"kind"`

	// Resource is the resource name of the Kubernetes resource.
	// +optional
	Resource string `json:"resource"`

	// Name is the name of the Kubernetes resource.
	// +optional
	Name string `json:"name"`

	// Name is the namespace of the Kubernetes resource.
	// +optional
	Namespace string `json:"namespace"`
}

// ManifestsTemplate represents the manifest workload to be deployed on a managed cluster.
type ManifestsTemplate struct {
	// Manifests represents a list of kuberenetes resources to be deployed on a managed cluster.
	// +optional
	Manifests []Manifest `json:"manifests,omitempty"`
}

// Manifest represents a resource to be deployed on managed cluster.
type Manifest struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	runtime.RawExtension `json:",inline"`
}

var (
	// PlacementGVK refers to GVK of ocm/placement
	PlacementGVK = schema.GroupVersionKind{
		Group:   "cluster.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "Placement",
	}

	// PlacementDecisionListGVK refers to GVK of ocm/placementDecisionList
	PlacementDecisionListGVK = schema.GroupVersionKind{
		Group:   "cluster.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "PlacementDecisionList",
	}

	// ManifestWorkGVK refers to GVK of ocm/manifestWork
	ManifestWorkGVK = schema.GroupVersionKind{
		Group:   "work.open-cluster-management.io",
		Version: "v1",
		Kind:    "ManifestWork",
	}
)
