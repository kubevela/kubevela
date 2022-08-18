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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

const (
	// TypeHealthy application are believed to be determined as healthy by a health scope.
	TypeHealthy condition.ConditionType = "Healthy"
)

// Reasons an application is or is not healthy
const (
	ReasonHealthy        condition.ConditionReason = "AllComponentsHealthy"
	ReasonUnhealthy      condition.ConditionReason = "UnhealthyOrUnknownComponents"
	ReasonHealthCheckErr condition.ConditionReason = "HealthCheckeError"
)

// AppPolicy defines a global policy for all components in the app.
type AppPolicy struct {
	// Name is the unique name of the policy.
	Name string `json:"name"`

	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

// Workflow defines workflow steps and other attributes
type Workflow struct {
	Ref   string                                `json:"ref,omitempty"`
	Mode  *workflowv1alpha1.WorkflowExecuteMode `json:"mode,omitempty"`
	Steps []workflowv1alpha1.WorkflowStep       `json:"steps,omitempty"`
}

// ApplicationSpec is the spec of Application
type ApplicationSpec struct {
	Components []common.ApplicationComponent `json:"components"`

	// Policies defines the global policies for all components in the app, e.g. security, metrics, gitops,
	// multi-cluster placement rules, etc.
	// Policies are applied after components are rendered and before workflow steps are executed.
	Policies []AppPolicy `json:"policies,omitempty"`

	// Workflow defines how to customize the control logic.
	// If workflow is specified, Vela won't apply any resource, but provide rendered output in AppRevision.
	// Workflow steps are executed in array order, and each step:
	// - will have a context in annotation.
	// - should mark "finish" phase in status.conditions.
	Workflow *Workflow `json:"workflow,omitempty"`

	// TODO(wonderflow): we should have application level scopes supported here
}

// +kubebuilder:object:root=true

// Application is the Schema for the applications API
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam},shortName={app,velaapp}
// +kubebuilder:printcolumn:name="COMPONENT",type=string,JSONPath=`.spec.components[*].name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.components[*].type`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="HEALTHY",type=boolean,JSONPath=`.status.services[*].healthy`
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.services[*].message`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec  `json:"spec,omitempty"`
	Status common.AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// SetConditions set condition to application
func (app *Application) SetConditions(c ...condition.Condition) {
	app.Status.SetConditions(c...)
}

// GetCondition get condition by given condition type
func (app *Application) GetCondition(t condition.ConditionType) condition.Condition {
	return app.Status.GetCondition(t)
}

// GetComponent get the component from the application based on its workload type
func (app *Application) GetComponent(workloadType string) *common.ApplicationComponent {
	for _, c := range app.Spec.Components {
		if c.Type == workloadType {
			return &c
		}
	}
	return nil
}

// Unstructured convert application to unstructured.Unstructured.
func (app *Application) Unstructured() (*unstructured.Unstructured, error) {
	var obj = &unstructured.Unstructured{}
	app.SetGroupVersionKind(ApplicationKindVersionKind)
	bt, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}
	if err := obj.UnmarshalJSON(bt); err != nil {
		return nil, err
	}

	if app.Status.Services == nil {
		if err := unstructured.SetNestedSlice(obj.Object, []interface{}{}, "status", "services"); err != nil {
			return nil, err
		}
	}

	if app.Status.AppliedResources == nil {
		if err := unstructured.SetNestedSlice(obj.Object, []interface{}{}, "status", "appliedResources"); err != nil {
			return nil, err
		}
	}

	if wfStatus := app.Status.Workflow; wfStatus != nil && wfStatus.Steps == nil {
		if err := unstructured.SetNestedSlice(obj.Object, []interface{}{}, "status", "workflow", "steps"); err != nil {
			return nil, err
		}
	}

	return obj, nil
}
