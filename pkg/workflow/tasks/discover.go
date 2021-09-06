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
	"strings"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/hooks"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/http"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/workspace"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/template"
	"github.com/oam-dev/kubevela/pkg/workflow/types"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type taskDiscover struct {
	builtins           map[string]types.TaskGenerator
	remoteTaskDiscover *custom.TaskLoader
	templateLoader     *template.Loader
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

// RegisterGenerator register generator to taskDiscover.
func (td *taskDiscover) RegisterGenerator(name string, p types.BuiltinTaskRunner) {
	td.builtins[name] = makeGenerator(name, p)
}

func suspend(step v1beta1.WorkflowStep, _ *types.GeneratorOptions) (types.TaskRunner, error) {
	return &suspendTaskRunner{
		name: step.Name,
	}, nil
}

// NewTaskDiscover will create a client for load task generator.
func NewTaskDiscover(providerHandlers providers.Providers, pd *packages.PackageDiscover, cli client.Client, dm discoverymapper.DiscoveryMapper) types.TaskDiscover {
	// install builtin provider
	workspace.Install(providerHandlers)
	http.Install(providerHandlers)
	templateLoader := template.NewTemplateLoader(cli, dm)
	return &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			"suspend": suspend,
		},
		remoteTaskDiscover: custom.NewTaskLoader(templateLoader.LoadTaskTemplate, pd, providerHandlers),
		templateLoader:     templateLoader,
	}
}

type suspendTaskRunner struct {
	name string
}

// Name return suspend step name.
func (tr *suspendTaskRunner) Name() string {
	return tr.name
}

// Run make workflow suspend.
func (tr *suspendTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (common.WorkflowStepStatus, *types.Operation, error) {
	return common.WorkflowStepStatus{
		Name:  tr.name,
		Type:  "suspend",
		Phase: common.WorkflowStepPhaseSucceeded,
	}, &types.Operation{Suspend: true}, nil
}

// Pending check task should be executed or not.
func (tr *suspendTaskRunner) Pending(ctx wfContext.Context) bool {
	return false
}

type commonTaskRunner struct {
	tpy          string
	step         v1beta1.WorkflowStep
	generatorOpt *types.GeneratorOptions
	up           types.BuiltinTaskRunner
}

// Name return step name.
func (ct *commonTaskRunner) Name() string {
	return ct.step.Name
}

// Run workflow step.
func (ct *commonTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (common.WorkflowStepStatus, *types.Operation, error) {
	paramsValue, err := ctx.MakeParameter(ct.step.Properties)
	if err != nil {
		return ct.setMeta(common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed}), nil, errors.WithMessage(err, "make parameter")
	}

	if err := hooks.Input(ctx, paramsValue, ct.step); err != nil {
		return ct.setMeta(common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed, Reason: "Input", Message: err.Error()}), nil, nil
	}
	status, operation, taskValue := ct.up(ctx, options, paramsValue)

	if status.Phase != common.WorkflowStepPhaseSucceeded {
		return ct.setMeta(status), nil, nil
	}
	if err := hooks.Output(ctx, taskValue, ct.step, status.Phase); err != nil {
		return ct.setMeta(common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed, Reason: "Output", Message: err.Error()}), nil, nil
	}
	return ct.setMeta(status), operation, nil
}

func (ct *commonTaskRunner) setMeta(status common.WorkflowStepStatus) common.WorkflowStepStatus {
	if ct.generatorOpt != nil {
		status.ID = ct.generatorOpt.ID
	}
	status.Name = ct.step.Name
	status.Type = ct.tpy
	return status
}

// Pending check task should be executed or not.
func (ct *commonTaskRunner) Pending(ctx wfContext.Context) bool {
	for _, input := range ct.step.Inputs {
		if _, err := ctx.GetVar(strings.Split(input.From, ".")...); err != nil {
			return true
		}
	}
	return false
}

func makeGenerator(tpy string, p types.BuiltinTaskRunner) types.TaskGenerator {
	return func(wfStep v1beta1.WorkflowStep, opt *types.GeneratorOptions) (types.TaskRunner, error) {
		return &commonTaskRunner{
			tpy:          tpy,
			step:         wfStep,
			generatorOpt: opt,
			up:           p,
		}, nil
	}
}
