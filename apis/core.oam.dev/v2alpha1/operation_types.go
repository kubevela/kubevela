/*
 Copyright 2026. The KubeVela Authors.

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

package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	oamv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/kubevela/workflow/api/condition"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
)

// OperationSource identifies what an Operation's workflow steps read
// through `context` -- the same target a healthPolicy already evaluates
// against. A Component never exists apart from its owning Application, so
// it's expressed as App+Component rather than as a peer "kind" of App
// alone: App alone sources from the Application; App+Component sources
// from the Component within it. None scope is expressed by
// OperationSpec.Source being nil.
type OperationSource struct {
	// App is the name of the source Application.
	// +kubebuilder:validation:MinLength=1
	App string `json:"app"`

	// Component is the name of the source Component within App. Omit to
	// source from the Application as a whole; an explicit empty string is
	// rejected at admission rather than silently treated as omitted.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Component *string `json:"component,omitempty"`
}

// OperationSpec is the spec of Operation.
type OperationSpec struct {
	// Template is the name of the OperationTemplate to invoke. Resolved in
	// the Operation's own namespace first, then "vela-system".
	// +kubebuilder:validation:MinLength=1
	Template string `json:"template"`

	// Source is what the workflow steps read through `context`. Required
	// for every attach.scope except None, which must omit it.
	// +optional
	Source *OperationSource `json:"source,omitempty"`

	// Clusters is reserved for multi-cluster dispatch (KEP 2.15). Only a
	// single (local) cluster is resolved so far; a non-empty value beyond
	// that is rejected at reconcile time rather than silently ignored, so
	// the field can be adopted later without a behavior change for
	// existing single-cluster Operations.
	// +optional
	Clusters []string `json:"clusters,omitempty"`

	// Parameters are literal values merged into `context.operationParams`.
	// TODO(KEP 2.15): no schema validation/defaulting is performed yet --
	// this is the raw JSON the caller sent, unified against the
	// OperationTemplate's parameter schema once admission lands.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// TTLSecondsAfterFinished bounds how long a terminal Operation is kept
	// before the controller deletes it. Unset uses the cluster default
	// (--default-operation-ttl-seconds); explicit 0 disables TTL for this
	// Operation. Negative values are rejected -- 0 is the only way to
	// disable it.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}

// OperationPhase is the terminal/non-terminal phase of an Operation run.
type OperationPhase string

const (
	// OperationPhasePending means the Operation has not started running yet.
	OperationPhasePending OperationPhase = "Pending"
	// OperationPhaseRunning means the Operation's workflow is executing.
	OperationPhaseRunning OperationPhase = "Running"
	// OperationPhaseSucceeded means the Operation's workflow finished
	// successfully. Terminal -- re-execution isn't supported yet.
	OperationPhaseSucceeded OperationPhase = "Succeeded"
	// OperationPhaseFailed means the Operation's workflow finished with an
	// error. Terminal -- re-execution isn't supported yet.
	OperationPhaseFailed OperationPhase = "Failed"
	// OperationPhaseSuspended means the workflow is paused (a `suspend` step,
	// or a manual `vela operation suspend`). Non-terminal: a
	// `vela operation resume` (or a resolved suspend step) is what moves it
	// forward.
	OperationPhaseSuspended OperationPhase = "Suspended"
	// OperationPhaseCancelled means a human stopped the operation before it
	// reached a natural terminal phase. Terminal.
	OperationPhaseCancelled OperationPhase = "Cancelled"
)

// OperationStepAttempt records one completed execution of a single workflow
// step -- logged the moment that execution reaches a terminal phase
// (succeeded, failed, or skipped), whether or not it's ever superseded by a
// restart. A step that succeeds on its first try, with no restart ever
// happening, still gets one recorded here. Nested under the step's own
// entry in OperationWorkflowStatus.Steps (OperationWorkflowStepStatus.
// Attempts) -- kept as its own type, rather than folded into
// OperationWorkflowStepStatus directly, only because it's also the payload
// type OperationWorkflowStatus.StepAttempts moves briefly during a restart
// (see that field's doc).
type OperationStepAttempt struct {
	// AttemptNumber is the 1-based ordinal of this attempt for the step.
	AttemptNumber int64 `json:"attemptNumber"`
	// ID is the embedded engine's own execution ID for this attempt
	// (WorkflowStepStatus.ID). Used to detect whether a given execution has
	// already been recorded, so a step sitting in a terminal phase across
	// many reconciles doesn't get logged more than once; also a
	// correlation key back to whatever the step's own type named using
	// context.stepSessionID (e.g. a Job name).
	// +optional
	ID string `json:"id,omitempty"`
	// Phase is this attempt's terminal WorkflowStepPhase.
	// +optional
	Phase workflowv1alpha1.WorkflowStepPhase `json:"phase,omitempty"`
	// Message carries this attempt's message, if any.
	// +optional
	Message string `json:"message,omitempty"`
	// Reason carries this attempt's reason, if any.
	// +optional
	Reason string `json:"reason,omitempty"`
	// StartTime is when this attempt of the step first executed.
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`
	// TriggeredBy names how this attempt started, e.g.
	// "vela operation restart --step backup". Empty for the original run.
	// +optional
	TriggeredBy string `json:"triggeredBy,omitempty"`
}

// OperationWorkflowStepStatus is one workflow step's live status, wrapping
// the embedded engine's own per-step status (from github.com/kubevela/workflow)
// with this step's attempt history. The controller appends to Attempts the
// moment this step's execution reaches a terminal phase (see
// recordStepCompletion in pkg/controller/.../operation/generator.go) --
// every completed run gets logged, not only ones later superseded by a
// restart. A restart of this step reuses the same step name but generates a
// fresh embedded WorkflowStepStatus.ID (the engine looks up the ID by name
// and only reuses it if a live entry with that name still exists -- see
// generateStepID upstream -- which is also exactly why a restart must
// remove, not reset in place, the target step's entry: reusing the ID would
// reuse the prior attempt's Job name too). Attempts already recorded on the
// entry a restart is about to remove get preserved via
// OperationWorkflowStatus.StepAttempts until the step reappears.
type OperationWorkflowStepStatus struct {
	// WorkflowStepStatus is this step's live status, reused verbatim from
	// github.com/kubevela/workflow.
	workflowv1alpha1.WorkflowStepStatus `json:",inline"`

	// Attempts is prior-attempt history for this step, most recent last.
	// +optional
	Attempts []OperationStepAttempt `json:"attempts,omitempty"`
}

// OperationWorkflowStatus is the workflow status for one cluster. Only a
// single entry is populated so far, but the shape already matches
// multi-cluster dispatch (status.workflows[], KEP 2.15) so a later
// multi-cluster Operation doesn't need a status-shape migration.
//
// This does not inline workflowv1alpha1.WorkflowRunStatus verbatim the way
// an early draft did -- Steps needs to be OperationWorkflowStepStatus
// (attempts nested per-step) instead of the vendored WorkflowStepStatus, and
// a Go JSON tag can only route to one field at a given nesting depth: had
// Steps stayed inlined from the embed while also being redeclared here to
// change its element type, the *embedded* copy (what
// pkg/controller/.../operation.buildWorkflowInstance actually feeds back to
// the engine to resume mid-workflow) would silently unmarshal as empty on
// every read, and the engine would re-run every step from scratch on every
// reconcile. So every other field WorkflowRunStatus carries is declared
// explicitly below instead, mirroring its JSON shape field-for-field.
type OperationWorkflowStatus struct {
	// ConditionedStatus carries this workflow's conditions, mirroring the
	// same embed upstream's WorkflowRunStatus carries. Nothing currently
	// populates it (the embedded engine's own executor never calls
	// SetConditions on the status it returns), but it's kept here for API
	// parity with upstream rather than silently dropped -- an external
	// consumer reading this type shouldn't lose a field the un-reshaped
	// version exposed.
	condition.ConditionedStatus `json:",inline"`

	// Cluster is the resolved cluster this workflow ran against. Always
	// populated, even though only one is ever resolved so far.
	Cluster string `json:"cluster,omitempty"`

	// Mode is the embedded engine's own step/sub-step execution mode. Same
	// JSON tag as upstream (no omitempty -- WorkflowExecuteMode is a
	// struct, its own fields are what carry omitempty).
	Mode oamv1alpha1.WorkflowExecuteMode `json:"mode"`
	// Phase is the embedded engine's own workflow-run phase (its JSON tag
	// is "status" upstream; kept identical here).
	Phase workflowv1alpha1.WorkflowRunPhase `json:"status"`
	// Message carries the embedded engine's own message, if any.
	// +optional
	Message string `json:"message,omitempty"`
	// Suspend mirrors the embedded engine's own suspend flag.
	Suspend bool `json:"suspend"`
	// +optional
	SuspendState string `json:"suspendState,omitempty"`
	// Terminated mirrors the embedded engine's own terminated flag.
	Terminated bool `json:"terminated"`
	// Finished mirrors the embedded engine's own finished flag.
	Finished bool `json:"finished"`
	// +optional
	ContextBackend *corev1.ObjectReference `json:"contextBackend,omitempty"`

	// Steps is the per-step status. Unlike the embedded engine's own
	// WorkflowStepStatus, each entry carries its own Attempts history
	// alongside its live status -- see OperationWorkflowStepStatus.
	// +optional
	Steps []OperationWorkflowStepStatus `json:"steps,omitempty"`

	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`
	// +optional
	EndTime metav1.Time `json:"endTime,omitempty"`

	// StepAttempts holds a step's attempt history only transiently, for the
	// single reconcile between a restart removing that step's live entry
	// (from Steps, above) and the engine recreating it: with nowhere on a
	// (currently nonexistent) entry to nest Attempts, the history has to
	// wait here until the step reappears, at which point it's merged into
	// that entry's own Attempts and cleared from here. In steady state
	// (nothing mid-restart) this is empty.
	// +optional
	StepAttempts map[string][]OperationStepAttempt `json:"stepAttempts,omitempty"`
}

// OperationStatus is the status of Operation.
type OperationStatus struct {
	// Phase is the terminal/non-terminal phase of this Operation.
	Phase OperationPhase `json:"phase,omitempty"`

	// Message carries a human-readable explanation, mainly used when Phase
	// is Failed before a workflow could even start (e.g. template/source
	// resolution failure).
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime is when the Operation began running.
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the Operation reached a terminal phase.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Template is a snapshot of the resolved OperationTemplateSpec at the
	// time this Operation started, so a later edit to the template doesn't
	// change what an already-running (or completed) Operation did.
	// +optional
	Template *OperationTemplateSpec `json:"template,omitempty"`

	// Workflows holds one entry per cluster the Operation's workflow ran
	// against. Only a single cluster is resolved so far.
	// +optional
	Workflows []OperationWorkflowStatus `json:"workflows,omitempty"`

	// Attempts is how many times this Operation's workflow has been
	// (re)started, including the original run (starts at 1).
	// +optional
	Attempts int64 `json:"attempts,omitempty"`
}

// +kubebuilder:object:root=true

// Operation is the Schema for the Operation API: one run-to-completion
// invocation of an OperationTemplate against a source.
//
// KEP 2.15 permission model: not implemented yet. Any RBAC principal able
// to create an Operation can invoke any OperationTemplate against any
// source in its namespace. Do not release or promote this code path
// until the permission model lands.
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam},shortName={op,vop}
// +kubebuilder:printcolumn:name="TEMPLATE",type=string,JSONPath=`.spec.template`
// +kubebuilder:printcolumn:name="SOURCE-APP",type=string,JSONPath=`.spec.source.app`
// +kubebuilder:printcolumn:name="SOURCE-COMPONENT",type=string,JSONPath=`.spec.source.component`
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Operation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperationSpec   `json:"spec,omitempty"`
	Status OperationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OperationList contains a list of Operation.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Operation `json:"items"`
}

// IsTerminal reports whether the Operation has reached a phase from which
// it will not progress further without an explicit restart.
func (o *Operation) IsTerminal() bool {
	switch o.Status.Phase {
	case OperationPhaseSucceeded, OperationPhaseFailed, OperationPhaseCancelled:
		return true
	default:
		return false
	}
}
