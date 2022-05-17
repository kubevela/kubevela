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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

const (
	// TopologyPolicyType refers to the type of topology policy
	TopologyPolicyType = "topology"
	// OverridePolicyType refers to the type of override policy
	OverridePolicyType = "override"
	// DebugPolicyType refers to the type of debug policy
	DebugPolicyType = "debug"
	// PostStopHookPolicyType refers to the type of post stop hook policy
	PostStopHookPolicyType = "postStopHook"
)

// TopologyPolicySpec defines the spec of topology policy
type TopologyPolicySpec struct {
	// Placement embeds the selectors for choosing cluster
	Placement `json:",inline"`
	// Namespace is the target namespace to deploy in the selected clusters.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PostStopHookPolicySpec defines the spec of post stop hook policy
type PostStopHookPolicySpec struct {
	Phases []HookPolicyPhase `json:"phases"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Notification *runtime.RawExtension `json:"notification,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Webhook *runtime.RawExtension `json:"webhook,omitempty"`
}

// PostStopHookPolicyStatus defines the status of post stop hook policy
type PostStopHookPolicyStatus struct {
	Webhook      *common.WorkflowStepStatus `json:"webhook,omitempty"`
	Notification *common.WorkflowStepStatus `json:"notification,omitempty"`
}

// HookPolicyPhase is the phase of post stop hook policy
type HookPolicyPhase string

const (
	// OnErrorPhase is the phase of application on error
	OnErrorPhase HookPolicyPhase = "onError"
	// OnGarbageCollectFinishPhase is the phase of application on garbage collect finish
	OnGarbageCollectFinishPhase HookPolicyPhase = "onGarbageCollectFinish"
)

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
