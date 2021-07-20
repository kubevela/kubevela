package tasks

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/workspace"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

type taskDiscover struct {
	builtins           map[string]types.TaskGenerator
	remoteTaskDiscover types.TaskDiscover
}

// GetTaskGenerator get task generator by name.
func (td *taskDiscover) GetTaskGenerator(name string) (types.TaskGenerator, error) {
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

func suspend(step v1beta1.WorkflowStep) (types.TaskRunner, error) {
	return func(ctx wfContext.Context) (common.WorkflowStepStatus, *types.Operation, error) {
		return common.WorkflowStepStatus{
			Name:  step.Name,
			Type:  step.Type,
			Phase: common.WorkflowStepPhaseSucceeded,
		}, &types.Operation{Suspend: true}, nil
	}, nil
}

// NewTaskDiscover will create a client for load task generator.
func NewTaskDiscover(cli client.Client, pd *packages.PackageDiscover, loadTemplate custom.LoadTaskTemplate) types.TaskDiscover {

	providerHandlers := providers.NewProviders()
	kube.Install(providerHandlers, cli)
	workspace.Install(providerHandlers)

	return &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, pd, providerHandlers),
	}
}
