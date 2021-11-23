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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/convert"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/email"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/http"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/time"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/workspace"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/template"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

type taskDiscover struct {
	builtins           map[string]types.TaskGenerator
	remoteTaskDiscover *custom.TaskLoader
	templateLoader     template.Loader
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

func suspend(step v1beta1.WorkflowStep, opt *types.GeneratorOptions) (types.TaskRunner, error) {
	return &suspendTaskRunner{
		id:   opt.ID,
		name: step.Name,
	}, nil
}

// NewTaskDiscover will create a client for load task generator.
func NewTaskDiscover(providerHandlers providers.Providers, pd *packages.PackageDiscover, cli client.Client, dm discoverymapper.DiscoveryMapper) types.TaskDiscover {
	// install builtin provider
	workspace.Install(providerHandlers)
	convert.Install(providerHandlers)
	email.Install(providerHandlers)
	templateLoader := template.NewWorkflowStepTemplateLoader(cli, dm)
	return &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(templateLoader.LoadTaskTemplate, pd, providerHandlers),
		templateLoader:     templateLoader,
	}
}

type suspendTaskRunner struct {
	id   string
	name string
}

// Name return suspend step name.
func (tr *suspendTaskRunner) Name() string {
	return tr.name
}

// Run make workflow suspend.
func (tr *suspendTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (common.WorkflowStepStatus, *types.Operation, error) {
	return common.WorkflowStepStatus{
		ID:    tr.id,
		Name:  tr.name,
		Type:  "suspend",
		Phase: common.WorkflowStepPhaseSucceeded,
	}, &types.Operation{Suspend: true}, nil
}

// Pending check task should be executed or not.
func (tr *suspendTaskRunner) Pending(ctx wfContext.Context) bool {
	return false
}

// NewViewTaskDiscover will create a client for load task generator.
func NewViewTaskDiscover(pd *packages.PackageDiscover, cli client.Client, apply kube.Dispatcher, delete kube.Deleter, viewNs string) types.TaskDiscover {
	handlerProviders := providers.NewProviders()

	// install builtin provider
	query.Install(handlerProviders, cli)
	time.Install(handlerProviders)
	kube.Install(handlerProviders, cli, apply, delete)
	http.Install(handlerProviders, cli, viewNs)
	convert.Install(handlerProviders)
	email.Install(handlerProviders)

	templateLoader := template.NewViewTemplateLoader(cli, viewNs)
	return &taskDiscover{
		remoteTaskDiscover: custom.NewTaskLoader(templateLoader.LoadTaskTemplate, pd, handlerProviders),
		templateLoader:     templateLoader,
	}
}
