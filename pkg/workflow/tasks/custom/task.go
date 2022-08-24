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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/hooks"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
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
	checkPending func(ctx wfContext.Context, stepStatus map[string]common.StepStatus) (bool, common.StepStatus)
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
func (tr *taskRunner) Pending(ctx wfContext.Context, stepStatus map[string]common.StepStatus) (bool, common.StepStatus) {
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
		tRunner.checkPending = func(ctx wfContext.Context, stepStatus map[string]common.StepStatus) (bool, common.StepStatus) {
			return CheckPending(ctx, wfStep, exec.wfStatus.ID, stepStatus)
		}
		tRunner.run = func(ctx wfContext.Context, options *wfTypes.TaskRunOptions) (stepStatus common.StepStatus, operations *wfTypes.Operation, rErr error) {
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

			if t.runOptionsProcess != nil {
				t.runOptionsProcess(options)
			}

			exec.wfStatus.Message = ""
			var taskv *value.Value
			var err error
			var paramFile string

			defer func() {
				if r := recover(); r != nil {
					exec.err(ctx, false, fmt.Errorf("invalid cue task for evaluation: %v", r), wfTypes.StatusReasonRendering)
					stepStatus = exec.status()
					operations = exec.operation()
					return
				}
				if taskv == nil {
					taskv, err = convertTemplate(ctx, t.pd, strings.Join([]string{templ, paramFile}, "\n"), exec.wfStatus.ID, options.PCtx)
					if err != nil {
						return
					}
				}
				for _, hook := range options.PostStopHooks {
					if err := hook(ctx, taskv, wfStep, exec.status(), options.StepStatus); err != nil {
						exec.wfStatus.Message = err.Error()
						stepStatus = exec.status()
						operations = exec.operation()
						return
					}
				}
			}()

			for _, hook := range options.PreCheckHooks {
				result, err := hook(wfStep, &wfTypes.PreCheckOptions{
					PackageDiscover: t.pd,
					ProcessContext:  options.PCtx,
				})
				if err != nil {
					tracer.Error(err, "do preCheckHook")
					exec.Skip(fmt.Sprintf("pre check error: %s", err.Error()))
					return exec.status(), exec.operation(), nil
				}
				if result.Skip {
					exec.Skip("")
					return exec.status(), exec.operation(), nil
				}
				if result.Timeout {
					exec.timeout("")
				}
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
				exec.err(ctx, false, err, wfTypes.StatusReasonParameter)
				return exec.status(), exec.operation(), nil
			}

			paramFile = model.ParameterFieldName + ": {}\n"
			if params != nil {
				ps, err := paramsValue.String()
				if err != nil {
					return common.StepStatus{}, nil, errors.WithMessage(err, "params encode")
				}
				paramFile = fmt.Sprintf(model.ParameterFieldName+": {%s}\n", ps)
			}

			taskv, err = convertTemplate(ctx, t.pd, strings.Join([]string{templ, paramFile}, "\n"), exec.wfStatus.ID, options.PCtx)
			if err != nil {
				exec.err(ctx, false, err, wfTypes.StatusReasonRendering)
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
				exec.err(ctx, true, err, wfTypes.StatusReasonExecute)
				return exec.status(), exec.operation(), nil
			}

			return exec.status(), exec.operation(), nil
		}
		return tRunner, nil
	}, nil
}

// ValidateIfValue validates the if value
func ValidateIfValue(ctx wfContext.Context, step v1beta1.WorkflowStep, stepStatus map[string]common.StepStatus, options *wfTypes.PreCheckOptions) (bool, error) {
	var pd *packages.PackageDiscover
	var pCtx process.Context
	if options != nil {
		pd = options.PackageDiscover
		pCtx = options.ProcessContext
	}

	template := fmt.Sprintf("if: %s", step.If)
	value, err := buildValueForStatus(ctx, step, pd, template, stepStatus, pCtx)
	if err != nil {
		return false, errors.WithMessage(err, "invalid if value")
	}
	check, err := value.GetBool("if")
	if err != nil {
		return false, err
	}
	return check, nil
}

func buildValueForStatus(ctx wfContext.Context, step v1beta1.WorkflowStep, pd *packages.PackageDiscover, template string, stepStatus map[string]common.StepStatus, pCtx process.Context) (*value.Value, error) {
	contextTempl := getContextTemplate(ctx, "", pCtx)
	inputsTempl := getInputsTemplate(ctx, step)
	statusTemplate := "\n"
	statusMap := make(map[string]interface{})
	for name, ss := range stepStatus {
		abbrStatus := struct {
			common.StepStatus  `json:",inline"`
			Failed             bool `json:"failed"`
			Succeeded          bool `json:"succeeded"`
			Skipped            bool `json:"skipped"`
			Timeout            bool `json:"timeout"`
			FailedAfterRetries bool `json:"failedAfterRetries"`
			Terminate          bool `json:"terminate"`
		}{
			StepStatus:         ss,
			Failed:             ss.Phase == common.WorkflowStepPhaseFailed,
			Succeeded:          ss.Phase == common.WorkflowStepPhaseSucceeded,
			Skipped:            ss.Phase == common.WorkflowStepPhaseSkipped,
			Timeout:            ss.Reason == wfTypes.StatusReasonTimeout,
			FailedAfterRetries: ss.Reason == wfTypes.StatusReasonFailedAfterRetries,
			Terminate:          ss.Reason == wfTypes.StatusReasonTerminate,
		}
		statusMap[name] = abbrStatus
	}
	status, err := json.Marshal(statusMap)
	if err != nil {
		return nil, err
	}
	statusTemplate += fmt.Sprintf("status: %s\n", status)
	statusTemplate += contextTempl
	statusTemplate += "\n" + inputsTempl
	v, err := value.NewValue(template+"\n"+statusTemplate, pd, "")
	if err != nil {
		return nil, err
	}
	if v.Error() != nil {
		return nil, v.Error()
	}
	return v, nil
}

func convertTemplate(ctx wfContext.Context, pd *packages.PackageDiscover, templ, id string, pCtx process.Context) (*value.Value, error) {
	contextTempl := getContextTemplate(ctx, id, pCtx)
	return value.NewValue(templ+contextTempl, pd, "", value.ProcessScript, value.TagFieldOrder)
}

// MakeValueForContext makes context value
func MakeValueForContext(ctx wfContext.Context, pd *packages.PackageDiscover, id string, pCtx process.Context) (*value.Value, error) {
	contextTempl := getContextTemplate(ctx, id, pCtx)
	return value.NewValue(contextTempl, pd, "")
}

func getContextTemplate(ctx wfContext.Context, id string, pCtx process.Context) string {
	var contextTempl string
	meta, _ := ctx.GetVar(wfTypes.ContextKeyMetadata)
	if meta != nil {
		ms, err := meta.String()
		if err != nil {
			return ""
		}
		contextTempl = fmt.Sprintf("\ncontext: {%s}\ncontext: stepSessionID: \"%s\"", ms, id)
	}
	if pCtx == nil {
		return ""
	}
	c, err := pCtx.ExtendedContextFile()
	if err != nil {
		return ""
	}
	contextTempl += "\n" + c
	return contextTempl
}

func getInputsTemplate(ctx wfContext.Context, step v1beta1.WorkflowStep) string {
	var inputsTempl string
	for _, input := range step.Inputs {
		inputValue, err := ctx.GetVar(strings.Split(input.From, ".")...)
		if err != nil {
			continue
		}
		s, err := inputValue.String()
		if err != nil {
			continue
		}
		inputsTempl += fmt.Sprintf("\ninputs: \"%s\": %s", input.From, s)
	}
	return inputsTempl
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
	exec.wfStatus.Reason = wfTypes.StatusReasonSuspend
}

// Terminate let workflow terminate.
func (exec *executor) Terminate(message string) {
	exec.terminated = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseSucceeded
	exec.wfStatus.Message = message
	exec.wfStatus.Reason = wfTypes.StatusReasonTerminate
}

// Wait let workflow wait.
func (exec *executor) Wait(message string) {
	exec.wait = true
	if exec.wfStatus.Phase != common.WorkflowStepPhaseFailed {
		exec.wfStatus.Phase = common.WorkflowStepPhaseRunning
		exec.wfStatus.Reason = wfTypes.StatusReasonWait
		exec.wfStatus.Message = message
	}
}

// Fail let the step fail, its status is failed and reason is Action
func (exec *executor) Fail(message string) {
	exec.terminated = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseFailed
	exec.wfStatus.Reason = wfTypes.StatusReasonAction
	exec.wfStatus.Message = message
}

func (exec *executor) Skip(message string) {
	exec.skip = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseSkipped
	exec.wfStatus.Reason = wfTypes.StatusReasonSkip
	exec.wfStatus.Message = message
}

func (exec *executor) timeout(message string) {
	exec.terminated = true
	exec.wfStatus.Phase = common.WorkflowStepPhaseFailed
	exec.wfStatus.Reason = wfTypes.StatusReasonTimeout
	exec.wfStatus.Message = message
}

func (exec *executor) err(ctx wfContext.Context, wait bool, err error, reason string) {
	exec.wait = wait
	exec.wfStatus.Phase = common.WorkflowStepPhaseFailed
	exec.wfStatus.Message = err.Error()
	if exec.wfStatus.Reason == "" {
		exec.wfStatus.Reason = reason
	}
	exec.checkErrorTimes(ctx)
}

func (exec *executor) checkErrorTimes(ctx wfContext.Context) {
	times := ctx.IncreaseCountValueInMemory(wfTypes.ContextPrefixFailedTimes, exec.wfStatus.ID)
	if times >= wfTypes.MaxWorkflowStepErrorRetryTimes {
		exec.wait = false
		exec.failedAfterRetries = true
		exec.wfStatus.Reason = wfTypes.StatusReasonFailedAfterRetries
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
			// continue if the field is incomplete
			return false, nil
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
	debug, _ := v.CueValue().LookupPath(value.FieldPath("#debug")).Bool()
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
			if len(options.PreStartHooks) == 0 {
				options.PreStartHooks = append(options.PreStartHooks, hooks.Input)
			}
			if len(options.PostStopHooks) == 0 {
				options.PostStopHooks = append(options.PostStopHooks, hooks.Output)
			}
			options.PCtx = pCtx
		},
		logLevel: logLevel,
	}
}

// CheckPending checks whether to pending task run
func CheckPending(ctx wfContext.Context, step v1beta1.WorkflowStep, id string, stepStatus map[string]common.StepStatus) (bool, common.StepStatus) {
	pStatus := common.StepStatus{
		Phase: common.WorkflowStepPhasePending,
		Type:  step.Type,
		ID:    id,
		Name:  step.Name,
	}
	for _, depend := range step.DependsOn {
		pStatus.Message = fmt.Sprintf("Pending on DependsOn: %s", depend)
		if status, ok := stepStatus[depend]; ok {
			if !wfTypes.IsStepFinish(status.Phase, status.Reason) {
				return true, pStatus
			}
		} else {
			return true, pStatus
		}
	}
	for _, input := range step.Inputs {
		pStatus.Message = fmt.Sprintf("Pending on Input: %s", input.From)
		if _, err := ctx.GetVar(strings.Split(input.From, ".")...); err != nil {
			return true, pStatus
		}
	}
	return false, common.StepStatus{}
}
