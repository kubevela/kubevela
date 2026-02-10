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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// PolicyScope defines the scope at which a policy operates
type PolicyScope string

const (
	// DefaultScope (empty string) means standard output-based policy (topology, override, etc.)
	// These policies generate Kubernetes resources from their CUE templates
	DefaultScope PolicyScope = ""

	// ApplicationScope means the policy transforms the Application CR before parsing
	// These policies use the transforms pattern and don't generate resources
	ApplicationScope PolicyScope = "Application"
)

// PolicyDefinitionSpec defines the desired state of PolicyDefinition
type PolicyDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this trait kind.
	Reference common.DefinitionReference `json:"definitionRef,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the policy definition.
	// Only CUE schematic is supported for now.
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// ManageHealthCheck means the policy will handle health checking and skip application controller
	// built-in health checking.
	ManageHealthCheck bool `json:"manageHealthCheck,omitempty"`

	//+optional
	Version string `json:"version,omitempty"`

	// Scope defines the scope at which this policy operates.
	// - DefaultScope (empty/omitted): Standard output-based policies (topology, override, etc.)
	//   These generate Kubernetes resources from CUE templates with an 'output' field.
	// - ApplicationScope: Transform-based policies that modify the Application CR before parsing.
	//   These use 'transforms' instead of 'output' and don't generate resources.
	// +optional
	Scope PolicyScope `json:"scope,omitempty"`

	// Global indicates this policy should automatically apply to all Applications
	// in this namespace (or all namespaces if in vela-system).
	// Global policies cannot be explicitly referenced in Application specs.
	// Requires EnableGlobalPolicies feature gate.
	// +optional
	Global bool `json:"global,omitempty"`

	// Priority defines the order in which global policies are applied.
	// Higher priority policies run first. Policies with the same priority
	// are applied in alphabetical order by name.
	// If not specified, defaults to 0.
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// CacheTTLSeconds defines how long the rendered policy output should be cached
	// before re-rendering. This is stored per-policy in the ConfigMap.
	// - -1 (default): Never refresh, always reuse cached result (deterministic)
	// - 0: Never cache, always re-render (useful for policies with external dependencies)
	// - >0: Cache for this many seconds before re-rendering
	// The prior cached result is available to the policy template as context.prior
	// +optional
	// +kubebuilder:default=-1
	CacheTTLSeconds int32 `json:"cacheTTLSeconds,omitempty"`
}

// PolicyDefinitionStatus is the status of PolicyDefinition
type PolicyDefinitionStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	condition.ConditionedStatus `json:",inline"`

	// ConfigMapRef refer to a ConfigMap which contains OpenAPI V3 JSON schema of Component parameters.
	ConfigMapRef string `json:"configMapRef,omitempty"`

	// LatestRevision of the component definition
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`
}

// SetConditions set condition for PolicyDefinition
func (d *PolicyDefinition) SetConditions(c ...condition.Condition) {
	d.Status.SetConditions(c...)
}

// GetCondition gets condition from PolicyDefinition
func (d *PolicyDefinition) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return d.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// PolicyDefinition is the Schema for the policydefinitions API
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=def-policy
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PolicyDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicyDefinitionSpec   `json:"spec,omitempty"`
	Status PolicyDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PolicyDefinitionList contains a list of PolicyDefinition
type PolicyDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyDefinition `json:"items"`
}
