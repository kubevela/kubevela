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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	monitorCtx "github.com/oam-dev/kubevela/pkg/monitor/context"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

// TaskRunner is a task runner.
type TaskRunner interface {
	Name() string
	Pending(ctx wfContext.Context) bool
	Run(ctx wfContext.Context, options *TaskRunOptions) (common.WorkflowStepStatus, *Operation, error)
}

// TaskDiscover is the interface to obtain the TaskGenerator。
type TaskDiscover interface {
	GetTaskGenerator(ctx context.Context, name string) (TaskGenerator, error)
}

// TaskRunOptions is the options for task run.
type TaskRunOptions struct {
	Data          *value.Value
	PreStartHooks []TaskPreStartHook
	PostStopHooks []TaskPostStopHook
	GetTracer     func(id string, step v1beta1.WorkflowStep) monitorCtx.Context
	RunSteps      func(isDag bool, runners ...TaskRunner) (*common.WorkflowStatus, error)
}

// TaskPreStartHook run before task execution.
type TaskPreStartHook func(ctx wfContext.Context, paramValue *value.Value, step v1beta1.WorkflowStep) error

// TaskPostStopHook  run after task execution.
type TaskPostStopHook func(ctx wfContext.Context, taskValue *value.Value, step v1beta1.WorkflowStep, phase common.WorkflowStepPhase) error

// Operation is workflow operation object.
type Operation struct {
	Suspend    bool
	Terminated bool
}

// TaskGenerator will generate taskRunner.
type TaskGenerator func(wfStep v1beta1.WorkflowStep, options *GeneratorOptions) (TaskRunner, error)

// GeneratorOptions is the options for generate task.
type GeneratorOptions struct {
	ID            string
	PrePhase      common.WorkflowStepPhase
	StepConvertor func(step v1beta1.WorkflowStep) (v1beta1.WorkflowStep, error)
}

// Action is that workflow provider can do.
type Action interface {
	Suspend(message string)
	Terminate(message string)
	Wait(message string)
}

const (
	// ContextKeyMetadata is key that refer to application metadata.
	ContextKeyMetadata = "metadata__"
)
