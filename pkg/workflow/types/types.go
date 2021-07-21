package types

import (
	"context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

type TaskRunner func(ctx wfContext.Context) (common.WorkflowStepStatus, *Operation, error)

type TaskDiscover interface {
	GetTaskGenerator(ctx context.Context, name string) (TaskGenerator, error)
}

type Operation struct {
	Suspend    bool
	Terminated bool
}

type TaskGenerator func(wfStep v1beta1.WorkflowStep) (TaskRunner, error)

type Action interface {
	Suspend(message string)
	Terminate(message string)
	Wait(message string)
}
