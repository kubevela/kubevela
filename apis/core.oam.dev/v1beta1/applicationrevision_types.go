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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationRevisionSpec is the spec of ApplicationRevision
type ApplicationRevisionSpec struct {
	// Application records the snapshot of the created/modified Application
	Application Application `json:"application"`

	// ComponentDefinitions records the snapshot of the componentDefinitions related with the created/modified Application
	ComponentDefinitions map[string]ComponentDefinition `json:"componentDefinitions,omitempty"`

	// WorkloadDefinitions records the snapshot of the workloadDefinitions related with the created/modified Application
	WorkloadDefinitions map[string]WorkloadDefinition `json:"workloadDefinitions,omitempty"`

	// TraitDefinitions records the snapshot of the traitDefinitions related with the created/modified Application
	TraitDefinitions map[string]TraitDefinition `json:"traitDefinitions,omitempty"`

	// ScopeDefinitions records the snapshot of the scopeDefinitions related with the created/modified Application
	ScopeDefinitions map[string]ScopeDefinition `json:"scopeDefinitions,omitempty"`

	// PolicyDefinitions records the snapshot of the PolicyDefinitions related with the created/modified Application
	PolicyDefinitions map[string]PolicyDefinition `json:"policyDefinitions,omitempty"`

	// WorkflowStepDefinitions records the snapshot of the WorkflowStepDefinitions related with the created/modified Application
	WorkflowStepDefinitions map[string]WorkflowStepDefinition `json:"workflowStepDefinitions,omitempty"`

	// ScopeGVK records the apiVersion to GVK mapping
	ScopeGVK map[string]metav1.GroupVersionKind `json:"scopeGVK,omitempty"`

	// AppfileCache records the cache for appfile
	AppfileCache *ApplicationRevisionAppfileCache `json:"appfileCache,omitempty"`
}

// ApplicationRevisionAppfileCache cache appfile parsed information which depends on external Kubernetes objects
type ApplicationRevisionAppfileCache struct {
	// Policies records the polices use in the current application revision
	Policies []AppPolicy `json:"policies,omitempty"`

	// WorkflowSteps records the steps to run in the current application revision
	WorkflowSteps []WorkflowStep `json:"workflowSteps,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	ExternalPolicies []*runtime.RawExtension `json:"externalPolicies,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	ExternalWorkflow *runtime.RawExtension `json:"externalWorkflow,omitempty"`
}

// ApplicationRevisionStatus is the status of ApplicationRevision
type ApplicationRevisionStatus struct {
	// Workflow the running status of the workflow
	Workflow ApplicationRevisionWorkflowStatus `json:"workflow,omitempty"`
}

// ApplicationRevisionWorkflowStatus is the status of the workflow running for ApplicationRevision
type ApplicationRevisionWorkflowStatus struct {
	// StartTime workflow start time
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// EndTime workflow finish time
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// PublishVersion the version of workflow
	PublishVersion string `json:"publishVersion,omitempty"`
	// Status workflow status
	Status string `json:"status"`
}

// +kubebuilder:object:root=true

// ApplicationRevision is the Schema for the ApplicationRevision API
// +kubebuilder:storageversion
// +kubebuilder:resource:categories={oam},shortName=apprev
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="PUBLISH_VERSION",type=string,JSONPath=`.status.workflow.publishVersion`
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.workflow.status`
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationRevisionSpec   `json:"spec,omitempty"`
	Status ApplicationRevisionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationRevisionList contains a list of ApplicationRevision
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationRevision `json:"items"`
}
