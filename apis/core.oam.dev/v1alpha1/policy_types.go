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

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	// TopologyPolicyType refers to the type of topology policy
	TopologyPolicyType = "topology"
	// OverridePolicyType refers to the type of override policy
	OverridePolicyType = "override"
	// DebugPolicyType refers to the type of debug policy
	DebugPolicyType = "debug"
	// SharedResourcePolicyType refers to the type of shared resource policy
	SharedResourcePolicyType = "shared-resource"
	// ReplicationPolicyType refers to the type of replication policy
	ReplicationPolicyType = "replication"
)

// TopologyPolicySpec defines the spec of topology policy
type TopologyPolicySpec struct {
	// Placement embeds the selectors for choosing cluster
	Placement `json:",inline"`
	// Namespace is the target namespace to deploy in the selected clusters.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// Placement describes which clusters to be selected in this topology
type Placement struct {
	// Clusters is the names of the clusters to select.
	Clusters []string `json:"clusters,omitempty"`

	// ClusterLabelSelector is the label selector for clusters.
	// Exclusive to "clusters"
	ClusterLabelSelector map[string]string `json:"clusterLabelSelector,omitempty"`

	// DeprecatedClusterSelector is a depreciated alias for ClusterLabelSelector.
	// Deprecated: Use clusterLabelSelector instead.
	DeprecatedClusterSelector map[string]string `json:"clusterSelector,omitempty"`
}

// OverridePolicySpec defines the spec of override policy
type OverridePolicySpec struct {
	Components []EnvComponentPatch `json:"components,omitempty"`
	Selector   []string            `json:"selector,omitempty"`
}

// SharedResourcePolicySpec defines the spec of shared-resource policy
type SharedResourcePolicySpec struct {
	Rules []SharedResourcePolicyRule `json:"rules"`
}

// SharedResourcePolicyRule defines the rule for sharing resources
type SharedResourcePolicyRule struct {
	Selector ResourcePolicyRuleSelector `json:"selector"`
}

// FindStrategy return if the target resource should be shared
func (in SharedResourcePolicySpec) FindStrategy(manifest *unstructured.Unstructured) bool {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return true
		}
	}
	return false
}

// ReplicationPolicySpec defines the spec of replication policy
// Override policy should be used together with replication policy to select the deploy target components
// ReplicationPolicySpec.Selector is the subset of selected components which will be replicated.
type ReplicationPolicySpec struct {
	Keys     []string `json:"keys,omitempty"`
	Selector []string `json:"selector,omitempty"`
}
