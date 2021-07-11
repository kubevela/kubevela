package tasks

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/workflow"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TaskGenerator func(params workflow.Value, td TaskDiscovery, pds workflow.Providers) (workflow.TaskRunner, error)

type taskDiscovery struct {
	builtins map[string]TaskGenerator
	remoteTaskDiscovery TaskDiscovery
}

func (td *taskDiscovery) GetTaskGenerator(name string) TaskGenerator {
	if td.remoteTaskDiscovery != nil {
		tg:=td.remoteTaskDiscovery.GetTaskGenerator(name)
		if tg!=nil{
			return tg
		}
	}
	return td.builtins[name]
}

func (td *taskDiscovery) builtin() TaskDiscovery {
	return &taskDiscovery{builtins: td.builtins}
}

type TaskDiscovery interface {
	GetTaskGenerator(name string) TaskGenerator
}

func suspend(params workflow.Value, td TaskDiscovery, pds workflow.Providers) (workflow.TaskRunner, error){
	return func(ctx interface{}) (common.WorkflowStepStatus, *workflow.Operation, error) {
		return common.WorkflowStepStatus{
			Phase: common.WorkflowStepPhaseSucceeded,
		},&workflow.Operation{Suspend: true},nil
	},nil
}

func NewTaskDiscovery(dm discoverymapper.DiscoveryMapper, cli client.Reader)(TaskDiscovery,error){
	td:=&taskDiscovery{
		builtins: map[string]TaskGenerator{
			"suspend": suspend,
		},
	}

}