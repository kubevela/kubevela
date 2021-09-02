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

package v1alpha1

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// WorkflowStepPhase describes the phase of a workflow step.
type WorkflowStepPhase string

const (
	// WorkflowStepPhaseSucceeded will make the controller run the next step.
	WorkflowStepPhaseSucceeded WorkflowStepPhase = "succeeded"
	// WorkflowStepPhaseFailed will make the controller stop the workflow and report error in `message`.
	WorkflowStepPhaseFailed WorkflowStepPhase = "failed"
	// WorkflowStepPhaseTerminated will make the controller terminate the workflow.
	WorkflowStepPhaseTerminated WorkflowStepPhase = "terminated"
	// WorkflowStepPhaseSuspending will make the controller suspend the workflow.
	WorkflowStepPhaseSuspending WorkflowStepPhase = "suspending"
	// WorkflowStepPhaseRunning will make the controller continue the workflow.
	WorkflowStepPhaseRunning WorkflowStepPhase = "running"
)

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name string `json:"name"`
	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties runtime.RawExtension `json:"properties,omitempty"`
	Inputs     common.StepInputs           `json:"inputs,omitempty"`
	Outputs    common.StepOutputs          `json:"outputs,omitempty"`
}



// A WorkflowSpec defines the desired state of a Workflow.
type WorkflowSpec struct {
	Steps []WorkflowStep `json:"steps,omitempty"`
}

// A WorkflowStatus is the status of Workflow
type WorkflowStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	condition.ConditionedStatus `json:",inline"`
	ObservedGeneration          int64                   `json:"observedGeneration,omitempty"`
	StepIndex                   int                     `json:"stepIndex,omitempty"`
	Suspend                     bool                    `json:"suspend"`
	Terminated                  bool                    `json:"terminated"`
	ContextBackend              *corev1.ObjectReference `json:"contextBackend"`
	Steps                       []WorkflowStepStatus    `json:"steps,omitempty"`
}

// WorkflowStepStatus record the status of a workflow step
type WorkflowStepStatus struct {
	Name  string            `json:"name,omitempty"`
	Type  string            `json:"type,omitempty"`
	Phase WorkflowStepPhase `json:"phase,omitempty"`
	// A human readable message indicating details about why the workflowStep is in this state.
	Message string `json:"message,omitempty"`
	// A brief CamelCase message indicating details about why the workflowStep is in this state.
	Reason      string                 `json:"reason,omitempty"`
	ResourceRef corev1.ObjectReference `json:"resourceRef,omitempty"`
}

// Workflow is the Schema for the Workflow API
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=workflow
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflow.
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

// SetConditions set condition for Workflow
func (w *Workflow) SetConditions(c ...condition.Condition) {
	w.Status.SetConditions(c...)
}

// GetCondition gets condition from Workflow
func (w *Workflow) GetCondition(conditionType condition.ConditionType) condition.Condition {
	return w.Status.GetCondition(conditionType)
}
