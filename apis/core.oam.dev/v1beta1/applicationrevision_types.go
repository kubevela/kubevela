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
	"encoding/json"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/utils/compression"
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

	// Policies records the external policies
	Policies map[string]v1alpha1.Policy `json:"policies,omitempty"`

	// Workflow records the external workflow
	Workflow *workflowv1alpha1.Workflow `json:"workflow,omitempty"`

	// ReferredObjects records the referred objects used in the ref-object typed components
	// +kubebuilder:pruning:PreserveUnknownFields
	ReferredObjects []common.ReferredObject `json:"referredObjects,omitempty"`

	Compression ApplicationRevisionCompression `json:"compression,omitempty"`
}

// ApplicationRevisionCompression represents the compressed components in apprev.
type ApplicationRevisionCompression struct {
	compression.CompressedText `json:",inline"`
}

// compressedObject is a temporary object used to store the fields that need
// compression in apprev.
type compressedObjects struct {
	Application             Application                       `json:"application"`
	ComponentDefinitions    map[string]ComponentDefinition    `json:"componentDefinitions,omitempty"`
	WorkloadDefinitions     map[string]WorkloadDefinition     `json:"workloadDefinitions,omitempty"`
	TraitDefinitions        map[string]TraitDefinition        `json:"traitDefinitions,omitempty"`
	ScopeDefinitions        map[string]ScopeDefinition        `json:"scopeDefinitions,omitempty"`
	PolicyDefinitions       map[string]PolicyDefinition       `json:"policyDefinitions,omitempty"`
	WorkflowStepDefinitions map[string]WorkflowStepDefinition `json:"workflowStepDefinitions,omitempty"`
	Policies                map[string]v1alpha1.Policy        `json:"policies,omitempty"`
	Workflow                *workflowv1alpha1.Workflow        `json:"workflow,omitempty"`
	ReferredObjects         []common.ReferredObject           `json:"referredObjects,omitempty"`
}

// copyFromApprev copies the fields need compression from apprev.
func (c *compressedObjects) copyFromApprev(apprev *ApplicationRevisionSpec) {
	c.Application = apprev.Application
	c.ComponentDefinitions = apprev.ComponentDefinitions
	c.WorkloadDefinitions = apprev.WorkloadDefinitions
	c.TraitDefinitions = apprev.TraitDefinitions
	c.ScopeDefinitions = apprev.ScopeDefinitions
	c.PolicyDefinitions = apprev.PolicyDefinitions
	c.WorkflowStepDefinitions = apprev.WorkflowStepDefinitions
	c.Policies = apprev.Policies
	c.Workflow = apprev.Workflow
	c.ReferredObjects = apprev.ReferredObjects
}

// copyToApprev copies the fields in compressedObjects to apprev.
func (c *compressedObjects) copyToApprev(apprev *ApplicationRevisionSpec) {
	apprev.Application = c.Application
	apprev.ComponentDefinitions = c.ComponentDefinitions
	apprev.WorkloadDefinitions = c.WorkloadDefinitions
	apprev.TraitDefinitions = c.TraitDefinitions
	apprev.ScopeDefinitions = c.ScopeDefinitions
	apprev.PolicyDefinitions = c.PolicyDefinitions
	apprev.WorkflowStepDefinitions = c.WorkflowStepDefinitions
	apprev.Policies = c.Policies
	apprev.Workflow = c.Workflow
	apprev.ReferredObjects = c.ReferredObjects
}

// cleanCompressedApprev cleans all the fields that need compression from apprev
// so that the compressed fields is not stored again.
func cleanCompressedApprev(apprev *ApplicationRevisionSpec) {
	// Application Components are required.
	apprev.Application = Application{Spec: ApplicationSpec{Components: []common.ApplicationComponent{}}}
	apprev.ComponentDefinitions = nil
	apprev.WorkloadDefinitions = nil
	apprev.TraitDefinitions = nil
	apprev.ScopeDefinitions = nil
	apprev.PolicyDefinitions = nil
	apprev.WorkflowStepDefinitions = nil
	apprev.Policies = nil
	apprev.Workflow = nil
	apprev.ReferredObjects = nil
}

// MarshalJSON serves the same purpose as the one in ResourceTrackerSpec.
func (apprev *ApplicationRevisionSpec) MarshalJSON() ([]byte, error) {
	type Alias ApplicationRevisionSpec
	tmp := &struct {
		*Alias
	}{}

	if apprev.Compression.Type == compression.Uncompressed {
		tmp.Alias = (*Alias)(apprev)
	} else {
		objs := compressedObjects{}
		cpy := apprev.DeepCopy()
		objs.copyFromApprev(cpy)
		cleanCompressedApprev(cpy)
		err := cpy.Compression.EncodeFrom(objs)
		if err != nil {
			return nil, err
		}
		tmp.Alias = (*Alias)(cpy)
	}

	return json.Marshal(tmp.Alias)
}

// UnmarshalJSON serves the same purpose as the one in ResourceTrackerSpec.
func (apprev *ApplicationRevisionSpec) UnmarshalJSON(data []byte) error {
	type Alias ApplicationRevisionSpec
	tmp := &struct {
		*Alias
	}{}

	if err := json.Unmarshal(data, tmp); err != nil {
		return err
	}

	if tmp.Compression.Type != compression.Uncompressed {
		objs := compressedObjects{}
		err := tmp.Compression.DecodeTo(&objs)
		if err != nil {
			return err
		}
		objs.copyToApprev((*ApplicationRevisionSpec)(tmp.Alias))
		tmp.Compression.Clean()
	}

	(*ApplicationRevisionSpec)(tmp.Alias).DeepCopyInto(apprev)
	return nil
}

// ApplicationRevisionStatus is the status of ApplicationRevision
type ApplicationRevisionStatus struct {
	// Succeeded records if the workflow finished running with success
	Succeeded bool `json:"succeeded"`
	// Workflow the running status of the workflow
	Workflow *common.WorkflowStatus `json:"workflow,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationRevision is the Schema for the ApplicationRevision API
// +kubebuilder:storageversion
// +kubebuilder:resource:categories={oam},shortName=apprev
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="PUBLISH_VERSION",type=string,JSONPath=`.metadata.annotations['app\.oam\.dev\/publishVersion']`
// +kubebuilder:printcolumn:name="SUCCEEDED",type=string,JSONPath=`.status.succeeded`
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
