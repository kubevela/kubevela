package types

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

type TaskRunner func(ctx wfContext.Context) (common.WorkflowStepStatus, *Operation, error)

type TaskDiscover interface {
	GetTaskGenerator(name string) (TaskGenerator, error)
}

type Operation struct {
	Suspend    bool
	Terminated bool
}

type TaskGenerator func(wfStep v1beta1.WorkflowStep) (TaskRunner, error)

type Action interface {
	Suspend()
	Terminated()
	Wait()
	Message(msg string)
	Reason(reason string)
}
