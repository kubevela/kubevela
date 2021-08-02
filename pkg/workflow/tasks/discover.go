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

package tasks

import (
	"context"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/workspace"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

type taskDiscover struct {
	builtins           map[string]types.TaskGenerator
	remoteTaskDiscover types.TaskDiscover
}

// GetTaskGenerator get task generator by name.
func (td *taskDiscover) GetTaskGenerator(ctx context.Context, name string) (types.TaskGenerator, error) {

	tg, ok := td.builtins[name]
	if ok {
		return tg, nil
	}
	if td.remoteTaskDiscover != nil {
		var err error
		tg, err = td.remoteTaskDiscover.GetTaskGenerator(ctx, name)
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
func NewTaskDiscover(providerHandlers providers.Providers, pd *packages.PackageDiscover, loadTemplate custom.LoadTaskTemplate) types.TaskDiscover {
	// install builtin provider
	workspace.Install(providerHandlers)

	return &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(loadTemplate, pd, providerHandlers),
	}
}
