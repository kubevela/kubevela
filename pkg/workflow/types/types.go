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

package types

import (
	"context"

	"k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	monitorCtx "github.com/oam-dev/kubevela/pkg/monitor/context"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

// TaskRunner is a task runner.
type TaskRunner interface {
	Name() string
	Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool
	Run(ctx wfContext.Context, options *TaskRunOptions) (common.StepStatus, *Operation, error)
}

// TaskDiscover is the interface to obtain the TaskGenerator。
type TaskDiscover interface {
	GetTaskGenerator(ctx context.Context, name string) (TaskGenerator, error)
}

// Engine is the engine to run workflow
type Engine interface {
	Run(taskRunners []TaskRunner, dag bool) error
	GetStepStatus(stepName string) common.WorkflowStepStatus
	GetCommonStepStatus(stepName string) common.StepStatus
	SetParentRunner(name string)
	GetOperation() *Operation
}

// TaskRunOptions is the options for task run.
type TaskRunOptions struct {
	Data          *value.Value
	PCtx          process.Context
	PreCheckHooks []TaskPreCheckHook
	PreStartHooks []TaskPreStartHook
	PostStopHooks []TaskPostStopHook
	GetTracer     func(id string, step v1beta1.WorkflowStep) monitorCtx.Context
	RunSteps      func(isDag bool, runners ...TaskRunner) (*common.WorkflowStatus, error)
	Debug         func(step string, v *value.Value) error
	StepStatus    map[string]common.StepStatus
	Engine        Engine
}

// PreCheckResult is the result of pre check.
type PreCheckResult struct {
	Skip    bool
	Timeout bool
}

// PreCheckOptions is the options for pre check.
type PreCheckOptions struct {
	PackageDiscover *packages.PackageDiscover
	ProcessContext  process.Context
}

// TaskPreCheckHook is the hook for pre check.
type TaskPreCheckHook func(step v1beta1.WorkflowStep, options *PreCheckOptions) (*PreCheckResult, error)

// TaskPreStartHook run before task execution.
type TaskPreStartHook func(ctx wfContext.Context, paramValue *value.Value, step v1beta1.WorkflowStep) error

// TaskPostStopHook  run after task execution.
type TaskPostStopHook func(ctx wfContext.Context, taskValue *value.Value, step v1beta1.WorkflowStep, status common.StepStatus) error

// Operation is workflow operation object.
type Operation struct {
	Suspend            bool
	Terminated         bool
	Waiting            bool
	Skip               bool
	FailedAfterRetries bool
}

// TaskGenerator will generate taskRunner.
type TaskGenerator func(wfStep v1beta1.WorkflowStep, options *GeneratorOptions) (TaskRunner, error)

// GeneratorOptions is the options for generate task.
type GeneratorOptions struct {
	ID              string
	PrePhase        common.WorkflowStepPhase
	StepConvertor   func(step v1beta1.WorkflowStep) (v1beta1.WorkflowStep, error)
	SubTaskRunners  []TaskRunner
	PackageDiscover *packages.PackageDiscover
	ProcessContext  process.Context
}

// Action is that workflow provider can do.
type Action interface {
	Suspend(message string)
	Terminate(message string)
	Wait(message string)
	Fail(message string)
}

const (
	// ContextKeyMetadata is key that refer to application metadata.
	ContextKeyMetadata = "metadata__"
	// ContextPrefixFailedTimes is the prefix that refer to the failed times of the step in workflow context config map.
	ContextPrefixFailedTimes = "failed_times"
	// ContextPrefixBackoffTimes is the prefix that refer to the backoff times in workflow context config map.
	ContextPrefixBackoffTimes = "backoff_times"
	// ContextPrefixBackoffReason is the prefix that refer to the current backoff reason in workflow context config map
	ContextPrefixBackoffReason = "backoff_reason"
	// ContextKeyLastExecuteTime is the key that refer to the last execute time in workflow context config map.
	ContextKeyLastExecuteTime = "last_execute_time"
	// ContextKeyNextExecuteTime is the key that refer to the next execute time in workflow context config map.
	ContextKeyNextExecuteTime = "next_execute_time"
)

const (
	// WorkflowStepTypeSuspend type suspend
	WorkflowStepTypeSuspend = "suspend"
	// WorkflowStepTypeApplyComponent type apply-component
	WorkflowStepTypeApplyComponent = "apply-component"
	// WorkflowStepTypeBuiltinApplyComponent type builtin-apply-component
	WorkflowStepTypeBuiltinApplyComponent = "builtin-apply-component"
	// WorkflowStepTypeStepGroup type step-group
	WorkflowStepTypeStepGroup = "step-group"
)

var (
	// MaxWorkflowStepErrorRetryTimes is the max retry times of the failed workflow step.
	MaxWorkflowStepErrorRetryTimes = 10
	// MaxWorkflowWaitBackoffTime is the max time to wait before reconcile wait workflow again
	MaxWorkflowWaitBackoffTime = 60
	// MaxWorkflowFailedBackoffTime is the max time to wait before reconcile failed workflow again
	MaxWorkflowFailedBackoffTime = 300
)

const (
	// StatusReasonWait is the reason of the workflow progress condition which is Wait.
	StatusReasonWait = "Wait"
	// StatusReasonSkip is the reason of the workflow progress condition which is Skip.
	StatusReasonSkip = "Skip"
	// StatusReasonRendering is the reason of the workflow progress condition which is Rendering.
	StatusReasonRendering = "Rendering"
	// StatusReasonExecute is the reason of the workflow progress condition which is Execute.
	StatusReasonExecute = "Execute"
	// StatusReasonSuspend is the reason of the workflow progress condition which is Suspend.
	StatusReasonSuspend = "Suspend"
	// StatusReasonTerminate is the reason of the workflow progress condition which is Terminate.
	StatusReasonTerminate = "Terminate"
	// StatusReasonParameter is the reason of the workflow progress condition which is ProcessParameter.
	StatusReasonParameter = "ProcessParameter"
	// StatusReasonOutput is the reason of the workflow progress condition which is Output.
	StatusReasonOutput = "Output"
	// StatusReasonFailedAfterRetries is the reason of the workflow progress condition which is FailedAfterRetries.
	StatusReasonFailedAfterRetries = "FailedAfterRetries"
	// StatusReasonTimeout is the reason of the workflow progress condition which is Timeout.
	StatusReasonTimeout = "Timeout"
	// StatusReasonAction is the reason of the workflow progress condition which is Action.
	StatusReasonAction = "Action"
)

// IsStepFinish will decide whether step is finish.
func IsStepFinish(phase common.WorkflowStepPhase, reason string) bool {
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		return phase == common.WorkflowStepPhaseSucceeded
	}
	switch phase {
	case common.WorkflowStepPhaseFailed:
		return reason != "" && reason != StatusReasonExecute
	case common.WorkflowStepPhaseSkipped:
		return true
	case common.WorkflowStepPhaseSucceeded:
		return true
	default:
		return false
	}
}
