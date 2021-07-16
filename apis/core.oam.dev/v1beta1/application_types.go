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
	xpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationTrait defines the trait of application
type ApplicationTrait struct {
	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties runtime.RawExtension `json:"properties,omitempty"`
}

// ApplicationComponent describe the component of application
type ApplicationComponent struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties runtime.RawExtension `json:"properties,omitempty"`

	// Traits define the trait of one component, the type must be array to keep the order.
	Traits []ApplicationTrait `json:"traits,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// scopes in ApplicationComponent defines the component-level scopes
	// the format is <scope-type:scope-instance-name> pairs, the key represents type of `ScopeDefinition` while the value represent the name of scope instance.
	Scopes map[string]string `json:"scopes,omitempty"`
}

// AppPolicy defines a global policy for all components in the app.
type AppPolicy struct {
	// Name is the unique name of the policy.
	Name string `json:"name"`

	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties runtime.RawExtension `json:"properties,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name string `json:"name"`

	Type string `json:"type"`

	// +kubebuilder:pruning:PreserveUnknownFields
	Properties runtime.RawExtension `json:"properties,omitempty"`

	Inputs StepInputs `json:"inputs,omitempty"`

	Outputs StepOutputs `json:"outputs,omitempty"`
}

type inputItem struct {
	ParameterKey string `json:"parameterKey"`
	From         string `json:"from"`
}

// StepInputs defines variable input of WorkflowStep
type StepInputs []inputItem

type outputItem struct {
	ExportKey string `json:"exportKey"`
	Name      string `json:"name"`
}

// StepOutputs defines output variable of WorkflowStep
type StepOutputs []outputItem

// Workflow defines workflow steps and other attributes
type Workflow struct {
	Steps []WorkflowStep `json:"steps,omitempty"`
}

// ApplicationSpec is the spec of Application
type ApplicationSpec struct {
	Components []ApplicationComponent `json:"components"`

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

	// RolloutPlan is the details on how to rollout the resources
	// The controller simply replace the old resources with the new one if there is no rollout plan involved
	// +optional
	RolloutPlan *v1alpha1.RolloutPlan `json:"rolloutPlan,omitempty"`
}

// +kubebuilder:object:root=true

// Application is the Schema for the applications API
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam},shortName=app
// +kubebuilder:printcolumn:name="COMPONENT",type=string,JSONPath=`.spec.components[*].name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.components[*].type`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="HEALTHY",type=boolean,JSONPath=`.status.services[*].healthy`
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.services[*].message`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec  `json:"spec,omitempty"`
	Status common.AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// SetConditions set condition to application
func (app *Application) SetConditions(c ...xpv1alpha1.Condition) {
	app.Status.SetConditions(c...)
}

// GetCondition get condition by given condition type
func (app *Application) GetCondition(t xpv1alpha1.ConditionType) xpv1alpha1.Condition {
	return app.Status.GetCondition(t)
}

// GetComponent get the component from the application based on its workload type
func (app *Application) GetComponent(workloadType string) *ApplicationComponent {
	for _, c := range app.Spec.Components {
		if c.Type == workloadType {
			return &c
		}
	}
	return nil
}
