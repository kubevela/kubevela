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
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppDeploymentPhase defines the phase that the AppDeployment is undergoing.
type AppDeploymentPhase string

const (
	// PhaseRolling is the phase when the AppDeployment is rolling live instances from old revisions to new ones.
	PhaseRolling AppDeploymentPhase = "Rolling"

	// PhaseCompleted is the phase when the AppDeployment is done with reconciliation.
	PhaseCompleted AppDeploymentPhase = "Completed"

	// PhaseFailed is the phase when the AppDeployment has failed in reconciliation due to unexpected conditions.
	PhaseFailed AppDeploymentPhase = "Failed"
)

// HTTPMatchRequest specifies a set of criterion to be met in order for the
// rule to be applied to the HTTP request. For example, the following
// restricts the rule to match only requests where the URL path
// starts with /ratings/v2/ and the request contains a custom `end-user` header
// with value `jason`.
type HTTPMatchRequest struct {
	// URI defines how to match with an URI.
	URI *URIMatch `json:"uri,omitempty"`
}

// URIMatch defines the rules to match with an URI.
type URIMatch struct {
	Prefix string `json:"prefix,omitempty"`
}

// HTTPRule defines the rules to match and split http traffic across revisions.
type HTTPRule struct {

	// Match defines the conditions to be satisfied for the rule to be
	// activated. All conditions inside a single match block have AND
	// semantics, while the list of match blocks have OR semantics. The rule
	// is matched if any one of the match blocks succeed.
	Match []*HTTPMatchRequest `json:"match,omitempty"`

	// WeightedTargets defines the revision targets to select and route traffic to.
	WeightedTargets []WeightedTarget `json:"weightedTargets,omitempty"`
}

// WeightedTarget defines the revision target to select and route traffic to.
type WeightedTarget struct {

	// RevisionName is the name of the app revision.
	RevisionName string `json:"revisionName,omitempty"`

	// ComponentName is the name of the component.
	// Note that it is the original component name in the Application. No need to append revision.
	ComponentName string `json:"componentName,omitempty"`

	// Port is the port to route traffic towards.
	Port int `json:"port,omitempty"`

	// Weight defines the proportion of traffic to be forwarded to the service
	// version. (0-100). Sum of weights across destinations SHOULD BE == 100.
	// If there is only one destination in a rule, the weight value is assumed to
	// be 100.
	Weight int `json:"weight,omitempty"`
}

// Traffic defines the traffic rules to apply across revisions.
type Traffic struct {
	// Hosts are the destination hosts to which traffic is being sent. Could
	// be a DNS name with wildcard prefix or an IP address.
	Hosts []string `json:"hosts,omitempty"`

	// Gateways specifies the names of gateways that should apply these rules.
	// Gateways in other namespaces may be referred to by
	// `<gateway namespace>/<gateway name>`; specifying a gateway with no
	// namespace qualifier is the same as specifying the AppDeployment's namespace.
	Gateways []string `json:"gateways,omitempty"`

	// HTTP defines the rules to match and split http traffoc across revisions.
	HTTP []HTTPRule `json:"http,omitempty"`
}

// ClusterSelector defines the rules to select a Cluster resource.
// Either name or labels is needed.
type ClusterSelector struct {
	// Name is the name of the cluster.
	Name string `json:"name,omitempty"`

	// Labels defines the label selector to select the cluster.
	Labels map[string]string `json:"labels,omitempty"`
}

// Distribution defines the replica distribution of an AppRevision to a cluster.
type Distribution struct {
	// Replicas is the replica number.
	Replicas int `json:"replicas,omitempty"`
}

// ClusterPlacement defines the cluster placement rules for an app revision.
type ClusterPlacement struct {
	// ClusterSelector selects the cluster to  deploy apps to.
	// If not specified, it indicates the host cluster per se.
	ClusterSelector *ClusterSelector `json:"clusterSelector,omitempty"`

	// Distribution defines the replica distribution of an AppRevision to a cluster.
	Distribution Distribution `json:"distribution,omitempty"`
}

// AppRevision specifies an AppRevision resource to and the rules to apply to it.
type AppRevision struct {
	// RevisionName is the name of the AppRevision.
	RevisionName string `json:"revisionName,omitempty"`

	// Placement defines the cluster placement rules for an app revision.
	Placement []ClusterPlacement `json:"placement,omitempty"`
}

// ClusterPlacementStatus shows the placement results of a cluster.
type ClusterPlacementStatus struct {
	// ClusterName indicates the name of the cluster to deploy apps to.
	// If empty, it indicates the host cluster per se.
	ClusterName string `json:"clusterName,omitempty"`

	// Replicas indicates the replica number of an app revision to deploy to a cluster.
	Replicas int `json:"replicas,omitempty"`
}

// PlacementStatus shows the cluster placement results of an app revision.
type PlacementStatus struct {
	// RevisionName is the name of the AppRevision.
	RevisionName string `json:"revisionName,omitempty"`

	// Clusters shows cluster placement results.
	Clusters []ClusterPlacementStatus `json:"clusters,omitempty"`
}

// AppDeploymentSpec defines how to describe an upgrade between different apps
type AppDeploymentSpec struct {

	// Traffic defines the traffic rules to apply across revisions.
	Traffic *Traffic `json:"traffic,omitempty"`

	// AppRevision specifies  AppRevision resources to and the rules to apply to them.
	AppRevisions []AppRevision `json:"appRevisions,omitempty"`
}

// AppDeploymentStatus defines the observed state of AppDeployment
type AppDeploymentStatus struct {
	// Conditions represents the latest available observations of a CloneSet's current state.
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// Phase shows the phase that the AppDeployment is undergoing.
	// If Phase is Rolling, no update should be made to the spec.
	Phase AppDeploymentPhase `json:"phase,omitempty"`

	// Placement shows the cluster placement results of the app revisions.
	Placement []PlacementStatus `json:"placement,omitempty"`
}

// AppDeployment is the Schema for the AppDeployment API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam},shortName=appdeploy
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
