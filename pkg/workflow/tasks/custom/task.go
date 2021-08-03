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
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// StatusReasonWait is the reason of the workflow progress condition which is Wait.
	StatusReasonWait = "Wait"
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
)

// LoadTaskTemplate gets the workflowStep definition from cluster and resolve it.
type LoadTaskTemplate func(ctx context.Context, name string) (string, error)

// TaskLoader is a client that get taskGenerator.
type TaskLoader struct {
	loadTemplate func(ctx context.Context, name string) (string, error)
	pd           *packages.PackageDiscover
	handlers     providers.Providers
}

// GetTaskGenerator get TaskGenerator by name.
func (t *TaskLoader) GetTaskGenerator(ctx context.Context, name string) (wfTypes.TaskGenerator, error) {
	templ, err := t.loadTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	return t.makeTaskGenerator(templ)
}

func (t *TaskLoader) makeTaskGenerator(templ string) (wfTypes.TaskGenerator, error) {
	return func(wfStep v1beta1.WorkflowStep) (wfTypes.TaskRunner, error) {

		exec := &executor{
			handlers: t.handlers,
			wfStatus: common.WorkflowStepStatus{
				Name:  wfStep.Name,
				Type:  wfStep.Type,
				Phase: common.WorkflowStepPhaseSucceeded,
			},
		}
		outputs := wfStep.Outputs
		inputs := wfStep.Inputs
		params := map[string]interface{}{}

		if len(wfStep.Properties.Raw) > 0 {
			bt, err := wfStep.Properties.MarshalJSON()
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(bt, &params); err != nil {
				return nil, err
			}
		}

		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {

			paramsValue, err := ctx.MakeParameter(params)
			if err != nil {
				return common.WorkflowStepStatus{}, nil, errors.WithMessage(err, "make parameter")
			}

			for _, input := range inputs {
				inputValue, err := ctx.GetVar(input.From)
				if err != nil {
					return common.WorkflowStepStatus{}, nil, errors.WithMessagef(err, "get input from [%s]", input.From)
				}
				if err := paramsValue.FillObject(inputValue, input.ParameterKey); err != nil {
					return common.WorkflowStepStatus{}, nil, err
				}
			}

			if err := paramsValue.Error(); err != nil {
				exec.err(err, StatusReasonParameter)
				return exec.status(), exec.operation(), nil
			}

			var paramFile = velacue.ParameterTag + ": {}\n"
			if params != nil {
				ps, err := paramsValue.String()
				if err != nil {
					return common.WorkflowStepStatus{}, nil, errors.WithMessage(err, "params encode")
				}
				paramFile = fmt.Sprintf(velacue.ParameterTag+": {%s}\n", ps)
			}

			taskv, err := t.makeValue(ctx, templ+"\n"+paramFile)
			if err != nil {
				exec.err(err, StatusReasonRendering)
				return exec.status(), exec.operation(), nil
			}

			if err := exec.doSteps(ctx, taskv); err != nil {
				exec.err(err, StatusReasonExecute)
				return exec.status(), exec.operation(), nil
			}

			if exec.status().Phase == common.WorkflowStepPhaseSucceeded {
				for _, output := range outputs {
					v, err := taskv.LookupValue(output.ExportKey)
					if err != nil {
						exec.err(err, StatusReasonOutput)
						return exec.status(), exec.operation(), nil
					}
					if err := ctx.SetVar(v, output.Name); err != nil {
						exec.err(err, StatusReasonOutput)
						return exec.status(), exec.operation(), nil
					}
				}
			}

			return exec.status(), exec.operation(), nil
		}, nil

	}, nil
}

func (t *TaskLoader) makeValue(ctx wfContext.Context, templ string) (*value.Value, error) {
	meta, _ := ctx.GetVar(wfTypes.ContextKeyMetadata)
	if meta != nil {
		ms, err := meta.String()
		if err != nil {
			return nil, err
		}
		templ += fmt.Sprintf("\ncontext: {%s}", ms)
	}
	return value.NewValue(templ, t.pd, value.TagFieldOrder)
}

type executor struct {
	handlers providers.Providers

	wfStatus   common.WorkflowStepStatus
	suspend    bool
	terminated bool
	wait       bool
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

func (exec *executor) err(err error, reason string) {
	exec.wfStatus.Phase = common.WorkflowStepPhaseFailed
	exec.wfStatus.Message = err.Error()
	exec.wfStatus.Reason = reason
}

func (exec *executor) operation() *wfTypes.Operation {
	return &wfTypes.Operation{
		Suspend:    exec.suspend,
		Terminated: exec.terminated,
	}
}

func (exec *executor) status() common.WorkflowStepStatus {
	return exec.wfStatus
}

// Handle process task-step value by provider and do.
func (exec *executor) Handle(ctx wfContext.Context, provider string, do string, v *value.Value) error {
	h, exist := exec.handlers.GetHandler(provider, do)
	if !exist {
		return errors.Errorf("handler not found")
	}
	return h(ctx, v, exec)
}

func (exec *executor) doSteps(ctx wfContext.Context, v *value.Value) error {
	do := opTpy(v)
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
				do := opTpy(item)
				if do == "" {
					return false, nil
				}
				return false, exec.doSteps(ctx, item)
			})
		}
		do := opTpy(in)
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

func opTpy(v *value.Value) string {
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
func NewTaskLoader(lt LoadTaskTemplate, pkgDiscover *packages.PackageDiscover, handlers providers.Providers) *TaskLoader {
	return &TaskLoader{
		loadTemplate: lt,
		pd:           pkgDiscover,
		handlers:     handlers,
	}
}
