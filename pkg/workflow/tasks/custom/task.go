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

package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	"k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/hooks"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

var (
	// MaxWorkflowStepErrorRetryTimes is the max retry times of the failed workflow step.
	MaxWorkflowStepErrorRetryTimes = 10
)

const (
	// StatusReasonWait is the reason of the workflow progress condition which is Wait.
	StatusReasonWait = "Wait"
	// StatusReasonSkip is the reason of the workflow progress condition which is Skip.
	StatusReasonSkip = "Skip"
	// StatusReasonRendering is the reason of the workflow progress condition which is Rendering.
	StatusReasonRendering = "Rendering"
	// StatusReasonExecute is the reason of the workflow progress condition which is Execute.
	StatusReasonExecute = "Execute"
	// StatusReasonSuspend is the reason of the workflow progress condition which is Suspend.
	StatusReasonSuspend = "Suspend"
	// StatusReasonTerminate is the reason of the workflow progress condition which is Terminate.
	StatusReasonTerminate = "Terminate"
	// StatusReasonParameter is the reason of the workflow progress condition which is ProcessParameter.
	StatusReasonParameter = "ProcessParameter"
	// StatusReasonOutput is the reason of the workflow progress condition which is Output.
	StatusReasonOutput = "Output"
	// StatusReasonFailedAfterRetries is the reason of the workflow progress condition which is FailedAfterRetries.
	StatusReasonFailedAfterRetries = "FailedAfterRetries"
	// StatusReasonTimeout is the reason of the workflow progress condition which is Timeout.
	StatusReasonTimeout = "Timeout"
)

// LoadTaskTemplate gets the workflowStep definition from cluster and resolve it.
type LoadTaskTemplate func(ctx context.Context, name string) (string, error)

// TaskLoader is a client that get taskGenerator.
type TaskLoader struct {
	loadTemplate      func(ctx context.Context, name string) (string, error)
	pd                *packages.PackageDiscover
	handlers          providers.Providers
	runOptionsProcess func(*wfTypes.TaskRunOptions)
	logLevel          int
}

// GetTaskGenerator get TaskGenerator by name.
func (t *TaskLoader) GetTaskGenerator(ctx context.Context, name string) (wfTypes.TaskGenerator, error) {
	templ, err := t.loadTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	return t.makeTaskGenerator(templ)
}

type taskRunner struct {
	name         string
	run          func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.StepStatus, *wfTypes.Operation, error)
	checkPending func(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool
}

// Name return step name.
func (tr *taskRunner) Name() string {
	return tr.name
}

// Run execute task.
func (tr *taskRunner) Run(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.StepStatus, *wfTypes.Operation, error) {
	return tr.run(ctx, options)
}

// Pending check task should be executed or not.
func (tr *taskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
	return tr.checkPending(ctx, stepStatus)
}

// nolint:gocyclo
func (t *TaskLoader) makeTaskGenerator(templ string) (wfTypes.TaskGenerator, error) {
	return func(wfStep v1beta1.WorkflowStep, genOpt *wfTypes.GeneratorOptions) (wfTypes.TaskRunner, error) {

		exec := &executor{
			handlers: t.handlers,
			wfStatus: common.StepStatus{
				Name:  wfStep.Name,
				Type:  wfStep.Type,
				Phase: common.WorkflowStepPhaseSucceeded,
			},
		}

		var err error

		if genOpt != nil {
			exec.wfStatus.ID = genOpt.ID
			if genOpt.StepConvertor != nil {
				wfStep, err = genOpt.StepConvertor(wfStep)
				if err != nil {
					return nil, errors.WithMessage(err, "convert step")
				}
			}
		}

		params := map[string]interface{}{}

		if wfStep.Properties != nil && len(wfStep.Properties.Raw) > 0 {
			bt, err := common.RawExtensionPointer{RawExtension: wfStep.Properties}.MarshalJSON()
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(bt, &params); err != nil {
				return nil, err
			}
		}

		tRunner := new(taskRunner)
		tRunner.name = wfStep.Name
		tRunner.checkPending = func(ctx wfContext.Context, stepStatus map[string]common.StepStatus) bool {
			return CheckPending(ctx, wfStep, stepStatus)
		}
		tRunner.run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (common.StepStatus, *wfTypes.Operation, error) {
			if options.GetTracer == nil {
				options.GetTracer = func(id string, step v1beta1.WorkflowStep) monitorContext.Context {
					return monitorContext.NewTraceContext(context.Background(), "")
				}
			}
			tracer := options.GetTracer(exec.wfStatus.ID, wfStep).AddTag("step_name", wfStep.Name, "step_type", wfStep.Type)
			tracer.V(t.logLevel)
			defer func() {
				tracer.Commit(string(exec.status().Phase))
			}()

			for _, hook := range options.PreCheckHooks {
				result, err := hook(wfStep)
				if err != nil {
					tracer.Error(err, "do preCheckHook")
					return common.StepStatus{}, nil, errors.WithMessage(err, "do preCheckHook")
				}
				if result.Skip {
					exec.Skip("")
					return exec.status(), exec.operation(), nil
				}
				if result.Timeout {
					exec.err(ctx, errors.New("timeout"), StatusReasonTimeout)
					exec.terminated = true
					return exec.status(), exec.operation(), nil
				}
			}

			if exec.operation().FailedAfterRetries {
				tracer.Info("failed after retries, skip this step")
				return exec.status(), exec.operation(), nil
			}

			if t.runOptionsProcess != nil {
				t.runOptionsProcess(options)
			}
			paramsValue, err := ctx.MakeParameter(params)
			if err != nil {
				tracer.Error(err, "make parameter")
				return common.StepStatus{}, nil, errors.WithMessage(err, "make parameter")
			}

			for _, hook := range options.PreStartHooks {
				if err := hook(ctx, paramsValue, wfStep); err != nil {
					tracer.Error(err, "do preStartHook")
					return common.StepStatus{}, nil, errors.WithMessage(err, "do preStartHook")
				}
			}

			if err := paramsValue.Error(); err != nil {
				exec.err(ctx, err, StatusReasonParameter)
				return exec.status(), exec.operation(), nil
			}

			var paramFile = model.ParameterFieldName + ": {}\n"
			if params != nil {
				ps, err := paramsValue.String()
				if err != nil {
					return common.StepStatus{}, nil, errors.WithMessage(err, "params encode")
				}
				paramFile = fmt.Sprintf(model.ParameterFieldName+": {%s}\n", ps)
			}

			taskv, err := t.makeValue(ctx, strings.Join([]string{templ, paramFile}, "\n"), exec.wfStatus.ID, options.PCtx)
			if err != nil {
				exec.err(ctx, err, StatusReasonRendering)
				return exec.status(), exec.operation(), nil
			}

			exec.tracer = tracer
			if debugLog(taskv) {
				exec.printStep("workflowStepStart", "workflow", "", taskv)
				defer exec.printStep("workflowStepEnd", "workflow", "", taskv)
			}
			if options.Debug != nil {
				defer func() {
					if err := options.Debug(exec.wfStatus.Name, taskv); err != nil {
						tracer.Error(err, "failed to debug")
					}
				}()
			}
			if err := exec.doSteps(ctx, taskv); err != nil {
				tracer.Error(err, "do steps")
				exec.err(ctx, err, StatusReasonExecute)
				return exec.status(), exec.operation(), nil
			}

			for _, hook := range options.PostStopHooks {
				if err := hook(ctx, taskv, wfStep, exec.status().Phase); err != nil {
					exec.err(ctx, err, StatusReasonOutput)
					return exec.status(), exec.operation(), nil
				}
			}

			return exec.status(), exec.operation(), nil
		}
		return tRunner, nil
	}, nil
}

func (t *TaskLoader) makeValue(ctx wfContext.Context, templ string, id string, pCtx process.Context) (*value.Value, error) {
	var contextTempl string
	meta, _ := ctx.GetVar(wfTypes.ContextKeyMetadata)
	if meta != nil {
		ms, err := meta.String()
		if err != nil {
			return nil, err
		}
		contextTempl = fmt.Sprintf("\ncontext: {%s}\ncontext: stepSessionID: \"%s\"", ms, id)
	}
	c, err := pCtx.ExtendedContextFile()
	if err != nil {
		return nil, err
	}
	contextTempl += "\n" + c

	return value.NewValue(templ+contextTempl, t.pd, contextTempl, value.ProcessScript, value.TagFieldOrder)
}

type executor struct {
	handlers providers.Providers

	wfStatus           common.StepStatus
	suspend            bool
	terminated         bool
	failedAfterRetries bool
	wait               bool
	skip               bool

	tracer monitorContext.Context
}

// Suspend let workflow pause.
func (exec *executor) Suspend(message string) {
	exec.suspend = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseSucceeded
	exec.wfStatus.Message = message
	exec.wfStatus.Reason = StatusReasonSuspend
}

// Terminate let workflow terminate.
func (exec *executor) Terminate(message string) {
	exec.terminated = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseSucceeded
	exec.wfStatus.Message = message
	exec.wfStatus.Reason = StatusReasonTerminate
}

// Wait let workflow wait.
func (exec *executor) Wait(message string) {
	exec.wait = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseRunning
	exec.wfStatus.Reason = StatusReasonWait
	exec.wfStatus.Message = message
}

func (exec *executor) Skip(message string) {
	exec.skip = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseSkipped
	exec.wfStatus.Reason = StatusReasonSkip
	exec.wfStatus.Message = message
}

func (exec *executor) err(ctx wfContext.Context, err error, reason string) {
	exec.wait = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseFailed
	exec.wfStatus.Message = err.Error()
	exec.wfStatus.Reason = reason
	exec.checkErrorTimes(ctx)
}

func (exec *executor) checkErrorTimes(ctx wfContext.Context) {
	times := ctx.IncreaseCountValueInMemory(wfTypes.ContextPrefixFailedTimes, exec.wfStatus.ID)
	if times >= MaxWorkflowStepErrorRetryTimes {
		exec.wait = false
		exec.failedAfterRetries = true
		exec.wfStatus.Reason = StatusReasonFailedAfterRetries
	}
}

func (exec *executor) operation() *wfTypes.Operation {
	return &wfTypes.Operation{
		Suspend:            exec.suspend,
		Terminated:         exec.terminated,
		Waiting:            exec.wait,
		Skip:               exec.skip,
		FailedAfterRetries: exec.failedAfterRetries,
	}
}

func (exec *executor) status() common.StepStatus {
	return exec.wfStatus
}

func (exec *executor) printStep(phase string, provider string, do string, v *value.Value) {
	msg, _ := v.String()
	exec.tracer.Info("cue eval: "+msg, "phase", phase, "provider", provider, "do", do)
}

// Handle process task-step value by provider and do.
func (exec *executor) Handle(ctx wfContext.Context, provider string, do string, v *value.Value) error {
	if debugLog(v) {
		exec.printStep("stepStart", provider, do, v)
		defer exec.printStep("stepEnd", provider, do, v)
	}
	h, exist := exec.handlers.GetHandler(provider, do)
	if !exist {
		return errors.Errorf("handler not found")
	}
	return h(ctx, v, exec)
}

func (exec *executor) doSteps(ctx wfContext.Context, v *value.Value) error {
	do := OpTpy(v)
	if do != "" && do != "steps" {
		provider := opProvider(v)
		if err := exec.Handle(ctx, provider, do, v); err != nil {
			return errors.WithMessagef(err, "run step(provider=%s,do=%s)", provider, do)
		}
		return nil
	}
	return v.StepByFields(func(fieldName string, in *value.Value) (bool, error) {
		if in.CueValue().IncompleteKind() == cue.BottomKind {
			errInfo, err := sets.ToString(in.CueValue())
			if err != nil {
				errInfo = "value is _|_"
			}
			return true, errors.New(errInfo + "(bottom kind)")
		}
		if retErr := in.CueValue().Err(); retErr != nil {
			errInfo, err := sets.ToString(in.CueValue())
			if err == nil {
				retErr = errors.WithMessage(retErr, errInfo)
			}
			return false, retErr
		}

		if isStepList(fieldName) {
			return false, in.StepByList(func(name string, item *value.Value) (bool, error) {
				do := OpTpy(item)
				if do == "" {
					return false, nil
				}
				return false, exec.doSteps(ctx, item)
			})
		}
		do := OpTpy(in)
		if do == "" {
			return false, nil
		}
		if do == "steps" {
			if err := exec.doSteps(ctx, in); err != nil {
				return false, err
			}
		} else {
			provider := opProvider(in)
			if err := exec.Handle(ctx, provider, do, in); err != nil {
				return false, errors.WithMessagef(err, "run step(provider=%s,do=%s)", provider, do)
			}
		}

		if exec.suspend || exec.terminated || exec.wait {
			return true, nil
		}
		return false, nil
	})
}

func isStepList(fieldName string) bool {
	if fieldName == "#up" {
		return true
	}
	return strings.HasPrefix(fieldName, "#up_")
}

func debugLog(v *value.Value) bool {
	debug, _ := v.CueValue().LookupDef("#debug").Bool()
	return debug
}

// OpTpy get label do
func OpTpy(v *value.Value) string {
	return getLabel(v, "#do")
}

func opProvider(v *value.Value) string {
	provider := getLabel(v, "#provider")
	if provider == "" {
		provider = "builtin"
	}
	return provider
}

func getLabel(v *value.Value, label string) string {
	do, err := v.Field(label)
	if err == nil && do.Exists() {
		if str, err := do.String(); err == nil {
			return str
		}
	}
	return ""
}

// NewTaskLoader create a tasks loader.
func NewTaskLoader(lt LoadTaskTemplate, pkgDiscover *packages.PackageDiscover, handlers providers.Providers, logLevel int, pCtx process.Context) *TaskLoader {
	return &TaskLoader{
		loadTemplate: lt,
		pd:           pkgDiscover,
		handlers:     handlers,
		runOptionsProcess: func(options *wfTypes.TaskRunOptions) {
			options.PreStartHooks = append(options.PreStartHooks, hooks.Input)
			options.PostStopHooks = append(options.PostStopHooks, hooks.Output)
			options.PCtx = pCtx
		},
		logLevel: logLevel,
	}
}

// SkipOptions is the options of skip task runner
type SkipOptions struct {
	If             string
	DependsOnPhase common.WorkflowStepPhase
}

// SkipTaskRunner will decide whether to skip task runner.
func SkipTaskRunner(options *SkipOptions) bool {
	switch options.If {
	case "always":
		return false
	case "":
		return options.DependsOnPhase != common.WorkflowStepPhaseSucceeded
	default:
		// TODO:(fog) support more if cases
		return false
	}
}

// CheckPending checks whether to pending task run
func CheckPending(ctx wfContext.Context, step v1beta1.WorkflowStep, stepStatus map[string]common.StepStatus) bool {
	for _, depend := range step.DependsOn {
		if status, ok := stepStatus[depend]; ok {
			if !IsStepFinish(status.Phase, status.Reason) {
				return true
			}
		} else {
			return true
		}
	}
	for _, input := range step.Inputs {
		if _, err := ctx.GetVar(strings.Split(input.From, ".")...); err != nil {
			return true
		}
	}
	return false
}

// IsStepFinish will decide whether step is finish.
func IsStepFinish(phase common.WorkflowStepPhase, reason string) bool {
	if feature.DefaultMutableFeatureGate.Enabled(features.EnableSuspendOnFailure) {
		return phase == common.WorkflowStepPhaseSucceeded
	}
	switch phase {
	case common.WorkflowStepPhaseFailed:
		return reason == StatusReasonTerminate || reason == StatusReasonFailedAfterRetries || reason == StatusReasonTimeout
	case common.WorkflowStepPhaseSkipped:
		return true
	case common.WorkflowStepPhaseSucceeded:
		return true
	default:
		return false
	}
}
