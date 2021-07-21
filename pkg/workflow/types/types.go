package types

import (
	"context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

// TaskRunner is a task runner.
type TaskRunner func(ctx wfContext.Context) (common.WorkflowStepStatus, *Operation, error)

// TaskDiscover is the interface to obtain the TaskGenerator。
type TaskDiscover interface {
	GetTaskGenerator(ctx context.Context, name string) (TaskGenerator, error)
}

// Operation is workflow operation object.
type Operation struct {
	Suspend    bool
	Terminated bool
}

// TaskGenerator will generate taskRunner.
type TaskGenerator func(wfStep v1beta1.WorkflowStep) (TaskRunner, error)

// Action is that workflow provider can do.
type Action interface {
	Suspend(message string)
	Terminate(message string)
	Wait(message string)
}
