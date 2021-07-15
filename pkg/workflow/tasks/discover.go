package tasks

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/workflow"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/workspace"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/remote"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TaskGenerator func(params map[string]interface{},inputs workflow.StepInput,output workflow.StepOutput) (workflow.TaskRunner, error)

type taskDiscover struct {
	builtins           map[string]TaskGenerator
	remoteTaskDiscover TaskDiscover
}

func (td *taskDiscover) GetTaskGenerator(name string) (TaskGenerator, error) {
	tg, ok := td.builtins[name]
	if ok {
		return tg, nil
	}
	if td.remoteTaskDiscover != nil {
		var err error
		tg, err = td.remoteTaskDiscover.GetTaskGenerator(name)
		if err != nil {
			return nil, err
		}
		return tg, nil

	}
	return nil, errors.Errorf("can't find task generator: %s", name)
}

type TaskDiscover interface {
	GetTaskGenerator(name string) (TaskGenerator, error)
}

func suspend(_ map[string]interface{},_ workflow.StepInput,_ workflow.StepOutput) (workflow.TaskRunner, error) {
	return func(ctx wfContext.Context) (common.WorkflowStepStatus, *workflow.Operation, error) {
		return common.WorkflowStepStatus{
			Phase: common.WorkflowStepPhaseSucceeded,
		}, &workflow.Operation{Suspend: true}, nil
	}, nil
}

func NewTaskDiscover(dm discoverymapper.DiscoveryMapper, cli client.Client, pd *packages.PackageDiscover) TaskDiscover {

	providerHandlers := providers.NewProviders()
	kube.Install(providerHandlers, cli)
	workspace.Install(providerHandlers)

	return &taskDiscover{
		builtins: map[string]TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: remote.NewTaskLoader(dm, cli, pd, providerHandlers),
	}
}
