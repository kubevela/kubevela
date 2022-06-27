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
	builtintime "time"

	"github.com/pkg/errors"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/email"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/http"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/time"
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
		wait: false,
	}

	doDelay, _, err := GetSuspendStepDurationWaiting(step)
	if err != nil {
		return nil, err
	}

	tr.wait = doDelay

	return tr, nil
}

// StepGroup is the step group runner
func StepGroup(step v1beta1.WorkflowStep, opt *types.GeneratorOptions) (types.TaskRunner, error) {
	return &stepGroupTaskRunner{
		id:             opt.ID,
		name:           step.Name,
		step:           step,
		subTaskRunners: opt.SubTaskRunners,
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
	id    string
	step  v1beta1.WorkflowStep
	wait  bool
	phase common.WorkflowStepPhase
}

// Name return suspend step name.
func (tr *suspendTaskRunner) Name() string {
	return tr.step.Name
}

// Run make workflow suspend.
func (tr *suspendTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (common.StepStatus, *types.Operation, error) {
	if tr.wait {
		tr.phase = common.WorkflowStepPhaseRunning
	} else {
		tr.phase = common.WorkflowStepPhaseSucceeded
	}
	stepStatus := common.StepStatus{
		ID:    tr.id,
		Name:  tr.step.Name,
		Type:  types.WorkflowStepTypeSuspend,
		Phase: tr.phase,
	}

	return stepStatus, &types.Operation{Suspend: true}, nil
}

// Pending check task should be executed or not.
func (tr *suspendTaskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
	return custom.CheckPending(ctx, tr.step, stepStatus)
}

func (tr *suspendTaskRunner) Skip(dependsOnPhase common.WorkflowStepPhase, stepStatus map[string]common.StepStatus) (common.StepStatus, bool) {
	status := common.StepStatus{
		ID:    tr.id,
		Name:  tr.step.Name,
		Type:  types.WorkflowStepTypeSuspend,
		Phase: tr.phase,
	}
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		return status, false
	}
	skip := custom.SkipTaskRunner(&custom.SkipOptions{
		If:             tr.step.If,
		DependsOnPhase: dependsOnPhase,
	})
	if skip {
		status.Phase = common.WorkflowStepPhaseSkipped
		status.Reason = custom.StatusReasonSkip
	}
	return status, skip
}

type stepGroupTaskRunner struct {
	id             string
	name           string
	step           v1beta1.WorkflowStep
	subTaskRunners []types.TaskRunner
}

// Name return suspend step name.
func (tr *stepGroupTaskRunner) Name() string {
	return tr.name
}

// Pending check task should be executed or not.
func (tr *stepGroupTaskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
	return custom.CheckPending(ctx, tr.step, stepStatus)
}

func (tr *stepGroupTaskRunner) Skip(dependsOnPhase common.WorkflowStepPhase, stepStatus map[string]common.StepStatus) (common.StepStatus, bool) {
	status := common.StepStatus{
		ID:   tr.id,
		Name: tr.step.Name,
		Type: types.WorkflowStepTypeStepGroup,
	}
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		return status, false
	}
	skip := custom.SkipTaskRunner(&custom.SkipOptions{
		If:             tr.step.If,
		DependsOnPhase: dependsOnPhase,
	})
	if skip {
		status.Phase = common.WorkflowStepPhaseSkipped
		status.Reason = custom.StatusReasonSkip
		stepStatus[tr.step.Name] = common.StepStatus{
			ID:    tr.id,
			Phase: status.Phase,
		}
		// return false here to set all the sub steps to skipped
		return status, false
	}
	return status, skip
}

// Run make workflow step group.
func (tr *stepGroupTaskRunner) Run(ctx wfContext.Context, options *types.TaskRunOptions) (common.StepStatus, *types.Operation, error) {
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
	status := common.StepStatus{
		ID:   tr.id,
		Name: tr.name,
		Type: types.WorkflowStepTypeStepGroup,
	}

	subStepCounts := make(map[string]int)
	for _, subStepsStatus := range stepStatus.SubStepsStatus {
		subStepCounts[string(subStepsStatus.Phase)]++
		subStepCounts[subStepsStatus.Reason]++
	}
	switch {
	case len(stepStatus.SubStepsStatus) < len(tr.subTaskRunners):
		status.Phase = common.WorkflowStepPhaseRunning
	case subStepCounts[string(common.WorkflowStepPhaseRunning)] > 0:
		status.Phase = common.WorkflowStepPhaseRunning
	case subStepCounts[string(common.WorkflowStepPhaseStopped)] > 0:
		status.Phase = common.WorkflowStepPhaseStopped
	case subStepCounts[string(common.WorkflowStepPhaseFailed)] > 0:
		status.Phase = common.WorkflowStepPhaseFailed
		switch {
		case subStepCounts[custom.StatusReasonFailedAfterRetries] > 0:
			status.Reason = custom.StatusReasonFailedAfterRetries
		case subStepCounts[custom.StatusReasonTerminate] > 0:
			status.Reason = custom.StatusReasonTerminate
		}
	case subStepCounts[string(common.WorkflowStepPhaseSkipped)] > 0:
		status.Phase = common.WorkflowStepPhaseSkipped
		status.Reason = custom.StatusReasonSkip
	default:
		status.Phase = common.WorkflowStepPhaseSucceeded
	}
	return status, e.GetOperation(), nil
}

// NewViewTaskDiscover will create a client for load task generator.
func NewViewTaskDiscover(pd *packages.PackageDiscover, cli client.Client, cfg *rest.Config, apply kube.Dispatcher, delete kube.Deleter, viewNs string, logLevel int, pCtx process.Context, loader template.Loader) types.TaskDiscover {
	handlerProviders := providers.NewProviders()

	// install builtin provider
	query.Install(handlerProviders, cli, cfg)
	time.Install(handlerProviders)
	kube.Install(handlerProviders, nil, cli, apply, delete)
	http.Install(handlerProviders, cli, viewNs)
	email.Install(handlerProviders)

	return &taskDiscover{
		remoteTaskDiscover: custom.NewTaskLoader(loader.LoadTaskTemplate, pd, handlerProviders, logLevel, pCtx),
		templateLoader:     loader,
	}
}

// GetSuspendStepDurationWaiting get suspend step wait duration
func GetSuspendStepDurationWaiting(step v1beta1.WorkflowStep) (bool, builtintime.Duration, error) {
	if step.Properties.Size() > 0 {
		o := struct {
			Duration string `json:"duration"`
		}{}
		js, err := common.RawExtensionPointer{RawExtension: step.Properties}.MarshalJSON()
		if err != nil {
			return false, 0, err
		}

		if err := json.Unmarshal(js, &o); err != nil {
			return false, 0, err
		}

		if o.Duration != "" {
			waitDuration, err := builtintime.ParseDuration(o.Duration)
			return true, waitDuration, err
		}
	}

	return false, 0, nil
}
