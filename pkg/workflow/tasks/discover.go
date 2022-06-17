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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/email"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/http"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	timeprovider "github.com/oam-dev/kubevela/pkg/workflow/providers/time"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/util"
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
	tr := &suspendTaskRunner{
		id:   opt.ID,
		step: step,
		pd:   opt.PackageDiscover,
		pCtx: opt.ProcessContext,
	}

	return tr, nil
}

// StepGroup is the step group runner
func StepGroup(step v1beta1.WorkflowStep, opt *types.GeneratorOptions) (types.TaskRunner, error) {
	return &stepGroupTaskRunner{
		id:             opt.ID,
		name:           step.Name,
		step:           step,
		subTaskRunners: opt.SubTaskRunners,
		pd:             opt.PackageDiscover,
		pCtx:           opt.ProcessContext,
	}, nil
}

func newTaskDiscover(ctx monitorContext.Context, providerHandlers providers.Providers, pd *packages.PackageDiscover, pCtx process.Context, templateLoader template.Loader) types.TaskDiscover {
	// install builtin provider
	workspace.Install(providerHandlers)
	email.Install(providerHandlers)
	util.Install(ctx, providerHandlers)

	return &taskDiscover{
		builtins: map[string]types.TaskGenerator{
			types.WorkflowStepTypeSuspend:   suspend,
			types.WorkflowStepTypeStepGroup: StepGroup,
		},
		remoteTaskDiscover: custom.NewTaskLoader(templateLoader.LoadTaskTemplate, pd, providerHandlers, 0, pCtx),
		templateLoader:     templateLoader,
	}
}

// NewTaskDiscoverFromRevision will create a client for load task generator from ApplicationRevision.
func NewTaskDiscoverFromRevision(ctx monitorContext.Context, providerHandlers providers.Providers, pd *packages.PackageDiscover, rev *v1beta1.ApplicationRevision, dm discoverymapper.DiscoveryMapper, pCtx process.Context) types.TaskDiscover {
	templateLoader := template.NewWorkflowStepTemplateRevisionLoader(rev, dm)
	return newTaskDiscover(ctx, providerHandlers, pd, pCtx, templateLoader)
}

type suspendTaskRunner struct {
	id   string
	step v1beta1.WorkflowStep
	pd   *packages.PackageDiscover
	pCtx process.Context
}

// Name return suspend step name.
func (tr *suspendTaskRunner) Name() string {
	return tr.step.Name
}

// Run make workflow suspend.
func (tr *suspendTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (stepStatus common.StepStatus, operations *types.Operation, rErr error) {
	stepStatus = common.StepStatus{
		ID:    tr.id,
		Name:  tr.step.Name,
		Type:  types.WorkflowStepTypeSuspend,
		Phase: common.WorkflowStepPhaseRunning,
	}
	operations = &types.Operation{Suspend: true}

	status := &stepStatus
	defer handleOutput(ctx, status, operations, tr.step, options.PostStopHooks, tr.pd, tr.id, tr.pCtx)

	for _, hook := range options.PreCheckHooks {
		result, err := hook(tr.step, &types.PreCheckOptions{
			PackageDiscover: tr.pd,
			ProcessContext:  tr.pCtx,
		})
		if err != nil {
			stepStatus.Phase = common.WorkflowStepPhaseSkipped
			stepStatus.Reason = types.StatusReasonSkip
			stepStatus.Message = fmt.Sprintf("pre check error: %s", err.Error())
			operations.Suspend = false
			operations.Skip = true
			continue
		}
		switch {
		case result.Skip:
			stepStatus.Phase = common.WorkflowStepPhaseSkipped
			stepStatus.Reason = types.StatusReasonSkip
			operations.Suspend = false
			operations.Skip = true
		case result.Timeout:
			stepStatus.Phase = common.WorkflowStepPhaseFailed
			stepStatus.Reason = types.StatusReasonTimeout
			operations.Suspend = false
			operations.Terminated = true
		default:
			continue
		}
		return stepStatus, operations, nil
	}

	for _, input := range tr.step.Inputs {
		if input.ParameterKey == "duration" {
			inputValue, err := ctx.GetVar(strings.Split(input.From, ".")...)
			if err != nil {
				return common.StepStatus{}, nil, errors.WithMessagef(err, "do preStartHook: get input from [%s]", input.From)
			}
			d, err := inputValue.String()
			if err != nil {
				return common.StepStatus{}, nil, errors.WithMessagef(err, "do preStartHook: input value from [%s] is not a valid string", input.From)
			}
			tr.step.Properties = &runtime.RawExtension{Raw: []byte(`{"duration":` + d + `}`)}
		}
	}
	d, err := GetSuspendStepDurationWaiting(tr.step)
	if err != nil {
		stepStatus.Message = fmt.Sprintf("invalid suspend duration: %s", err.Error())
		return stepStatus, operations, nil
	}
	if d != 0 {
		e := options.Engine
		firstExecuteTime := time.Now()
		if ss := e.GetCommonStepStatus(tr.step.Name); !ss.FirstExecuteTime.IsZero() {
			firstExecuteTime = ss.FirstExecuteTime.Time
		}
		if time.Now().After(firstExecuteTime.Add(d)) {
			stepStatus.Phase = common.WorkflowStepPhaseSucceeded
			operations.Suspend = false
		}
	}
	return stepStatus, operations, nil
}

// Pending check task should be executed or not.
func (tr *suspendTaskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
	return custom.CheckPending(ctx, tr.step, stepStatus)
}

type stepGroupTaskRunner struct {
	id             string
	name           string
	step           v1beta1.WorkflowStep
	subTaskRunners []types.TaskRunner
	pd             *packages.PackageDiscover
	pCtx           process.Context
}

// Name return suspend step name.
func (tr *stepGroupTaskRunner) Name() string {
	return tr.name
}

// Pending check task should be executed or not.
func (tr *stepGroupTaskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
	return custom.CheckPending(ctx, tr.step, stepStatus)
}

// Run make workflow step group.
func (tr *stepGroupTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (status common.StepStatus, operations *types.Operation, rErr error) {
	status = common.StepStatus{
		ID:   tr.id,
		Name: tr.name,
		Type: types.WorkflowStepTypeStepGroup,
	}

	pStatus := &status
	defer handleOutput(ctx, pStatus, operations, tr.step, options.PostStopHooks, tr.pd, tr.id, tr.pCtx)
	for _, hook := range options.PreCheckHooks {
		result, err := hook(tr.step, &types.PreCheckOptions{
			PackageDiscover: tr.pd,
			ProcessContext:  options.PCtx,
		})
		if err != nil {
			status.Phase = common.WorkflowStepPhaseSkipped
			status.Reason = types.StatusReasonSkip
			status.Message = fmt.Sprintf("pre check error: %s", err.Error())
			continue
		}
		if result.Skip {
			status.Phase = common.WorkflowStepPhaseSkipped
			status.Reason = types.StatusReasonSkip
			options.StepStatus[tr.step.Name] = status
			break
		}
		if result.Timeout {
			status.Phase = common.WorkflowStepPhaseFailed
			status.Reason = types.StatusReasonTimeout
			options.StepStatus[tr.step.Name] = status
		}
	}
	// step-group has no properties so there is no need to fill in the properties with the input values
	// skip input handle here
	e := options.Engine
	if len(tr.subTaskRunners) > 0 {
		e.SetParentRunner(tr.name)
		// set sub steps to dag mode for now
		if err := e.Run(tr.subTaskRunners, true); err != nil {
			return common.StepStatus{
				ID:    tr.id,
				Name:  tr.name,
				Type:  types.WorkflowStepTypeStepGroup,
				Phase: common.WorkflowStepPhaseRunning,
			}, e.GetOperation(), err
		}
		e.SetParentRunner("")
	}

	stepStatus := e.GetStepStatus(tr.name)
	status, operations = getStepGroupStatus(status, stepStatus, e.GetOperation(), len(tr.subTaskRunners))

	return status, operations, nil
}

func getStepGroupStatus(status common.StepStatus, stepStatus common.WorkflowStepStatus, operation *types.Operation, subTaskRunners int) (common.StepStatus, *types.Operation) {
	subStepCounts := make(map[string]int)
	for _, subStepsStatus := range stepStatus.SubStepsStatus {
		subStepCounts[string(subStepsStatus.Phase)]++
		subStepCounts[subStepsStatus.Reason]++
	}
	switch {
	case status.Phase == common.WorkflowStepPhaseSkipped:
		return status, &types.Operation{Skip: true}
	case status.Phase == common.WorkflowStepPhaseFailed && status.Reason == types.StatusReasonTimeout:
		return status, &types.Operation{Terminated: true}
	case len(stepStatus.SubStepsStatus) < subTaskRunners:
		status.Phase = common.WorkflowStepPhaseRunning
	case subStepCounts[string(common.WorkflowStepPhaseRunning)] > 0:
		status.Phase = common.WorkflowStepPhaseRunning
	case subStepCounts[string(common.WorkflowStepPhaseStopped)] > 0:
		status.Phase = common.WorkflowStepPhaseStopped
	case subStepCounts[string(common.WorkflowStepPhaseFailed)] > 0:
		status.Phase = common.WorkflowStepPhaseFailed
		switch {
		case subStepCounts[types.StatusReasonFailedAfterRetries] > 0:
			status.Reason = types.StatusReasonFailedAfterRetries
		case subStepCounts[types.StatusReasonTimeout] > 0:
			status.Reason = types.StatusReasonTimeout
		case subStepCounts[types.StatusReasonAction] > 0:
			status.Reason = types.StatusReasonAction
		case subStepCounts[types.StatusReasonTerminate] > 0:
			status.Reason = types.StatusReasonTerminate
		}
	case subStepCounts[string(common.WorkflowStepPhaseSkipped)] > 0 && subStepCounts[string(common.WorkflowStepPhaseSkipped)] == subTaskRunners:
		status.Phase = common.WorkflowStepPhaseSkipped
		status.Reason = types.StatusReasonSkip
	default:
		status.Phase = common.WorkflowStepPhaseSucceeded
	}
	return status, operation
}

// NewViewTaskDiscover will create a client for load task generator.
func NewViewTaskDiscover(pd *packages.PackageDiscover, cli client.Client, cfg *rest.Config, apply kube.Dispatcher, delete kube.Deleter, viewNs string, logLevel int, pCtx process.Context, loader template.Loader) types.TaskDiscover {
	handlerProviders := providers.NewProviders()

	// install builtin provider
	query.Install(handlerProviders, cli, cfg)
	timeprovider.Install(handlerProviders)
	kube.Install(handlerProviders, nil, cli, apply, delete)
	http.Install(handlerProviders, cli, viewNs)
	email.Install(handlerProviders)

	return &taskDiscover{
		remoteTaskDiscover: custom.NewTaskLoader(loader.LoadTaskTemplate, pd, handlerProviders, logLevel, pCtx),
		templateLoader:     loader,
	}
}

// GetSuspendStepDurationWaiting get suspend step wait duration
func GetSuspendStepDurationWaiting(step v1beta1.WorkflowStep) (time.Duration, error) {
	if step.Properties.Size() > 0 {
		o := struct {
			Duration string `json:"duration"`
		}{}
		js, err := common.RawExtensionPointer{RawExtension: step.Properties}.MarshalJSON()
		if err != nil {
			return 0, err
		}

		if err := json.Unmarshal(js, &o); err != nil {
			return 0, err
		}

		if o.Duration != "" {
			waitDuration, err := time.ParseDuration(o.Duration)
			return waitDuration, err
		}
	}

	return 0, nil
}

func handleOutput(ctx wfContext.Context, stepStatus *common.StepStatus, operations *types.Operation, step v1beta1.WorkflowStep, postStopHooks []types.TaskPostStopHook, pd *packages.PackageDiscover, id string, pCtx process.Context) {
	status := *stepStatus
	if status.Phase != common.WorkflowStepPhaseSkipped && len(step.Outputs) > 0 {
		contextValue, err := custom.MakeContextValue(ctx, pd, id, pCtx)
		if err != nil {
			status.Phase = common.WorkflowStepPhaseFailed
			if status.Reason == "" {
				status.Reason = types.StatusReasonOutput
			}
			operations.Terminated = true
			status.Message = fmt.Sprintf("make context value error: %s", err.Error())
			return
		}

		for _, hook := range postStopHooks {
			if err := hook(ctx, contextValue, step, status); err != nil {
				status.Phase = common.WorkflowStepPhaseFailed
				if status.Reason == "" {
					status.Reason = types.StatusReasonOutput
				}
				operations.Terminated = true
				status.Message = fmt.Sprintf("output error: %s", err.Error())
				return
			}
		}
	}
}
