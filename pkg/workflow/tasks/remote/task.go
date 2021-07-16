package remote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	types2 "github.com/oam-dev/kubevela/pkg/workflow/types"
)

type LoadTaskTemplate func(ctx context.Context, name string) (string, error)

type taskLoader struct {
	loadTemplate func(ctx context.Context, name string) (string, error)
	pd           *packages.PackageDiscover
	handlers     providers.Providers
}

func (t *taskLoader) GetTaskGenerator(name string) (types2.TaskGenerator, error) {
	templ, err := t.loadTemplate(context.Background(), name)
	if err != nil {
		return nil, err
	}
	return t.makeTaskGenerator(templ)
}

func (t *taskLoader) makeTaskGenerator(templ string) (types2.TaskGenerator, error) {
	return func(wfStep v1beta1.WorkflowStep) (types2.TaskRunner, error) {

		exec := &executor{
			handlers: t.handlers,
		}
		outputs := wfStep.Outputs
		inputs := wfStep.Inputs
		params := map[string]interface{}{}

		bt, err := wfStep.Properties.MarshalJSON()
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bt, &params); err != nil {
			return nil, err
		}
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *types2.Operation, error) {

			if wfStep.Outputs != nil {
				for _, output := range outputs {
					params[output.ExportKey] = output.Name
				}
			}

			paramsValue, err := ctx.MakeParameter(params)
			if err != nil {
				return common.WorkflowStepStatus{}, nil, err
			}

			if inputs != nil {
				for _, input := range inputs {
					inputValue, err := ctx.GetVar(input.From)
					if err != nil {
						return common.WorkflowStepStatus{}, nil, errors.WithMessagef(err, "get input from [%s]", input.From)
					}
					paramsValue.FillObject(inputValue, input.ParameterKey)
				}
			}

			var paramFile = velacue.ParameterTag + ": {}\n"
			if params != nil {
				ps, err := paramsValue.String()
				if err != nil {
					return common.WorkflowStepStatus{}, nil, errors.WithMessage(err, "params encode")
				}
				paramFile = fmt.Sprintf(velacue.ParameterTag + ": {%s}\n" + ps)
			}
			status := common.WorkflowStepStatus{
				Name: wfStep.Name,
				Type: wfStep.Type,
			}
			taskv, err := value.NewValue(paramFile+templ, t.pd)
			if err != nil {
				status.Phase = common.WorkflowStepPhaseFailed
				status.Message = err.Error()
				status.Reason = "Rendering"
				return status, nil, nil
			}

			if err := exec.doSteps(ctx, taskv); err != nil {
				status.Phase = common.WorkflowStepPhaseFailed
				status.Message = err.Error()
				status.Reason = "Execute"
				return status, nil, nil
			}

			status.Phase = common.WorkflowStepPhaseSucceeded
			status.Message = exec.message
			status.Reason = exec.reason
			if exec.wait {
				status.Reason = "Wait"
				status.Phase = common.WorkflowStepPhaseRunning
			}

			operation := &types2.Operation{
				Terminated: exec.terminated,
				Suspend:    exec.suspend,
			}

			return status, operation, nil
		}, nil

	}, nil
}

type executor struct {
	handlers providers.Providers

	wfStatus   common.WorkflowStepStatus
	suspend    bool
	terminated bool
	wait       bool
	message    string
	reason     string
}

func (exec *executor) Suspend() {
	exec.suspend = true
}
func (exec *executor) Terminated() {
	exec.terminated = true
}

func (exec *executor) Wait() {
	exec.wait = true
}

func (exec *executor) Message(msg string) {
	exec.message = msg
}

func (exec *executor) Reason(reason string) {
	exec.reason = reason
}

func (exec *executor) Handle(ctx wfContext.Context, provider string, do string, v *value.Value) error {
	h, exist := exec.handlers.GetHandler(provider, do)
	if !exist {
		return errors.Errorf("handler(provider=%s,do=%s) not found", provider, do)
	}
	return h(ctx, v, exec)
}

func (exec *executor) doSteps(ctx wfContext.Context, v *value.Value) error {
	return v.StepFields(func(in *value.Value) (bool, error) {
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
				return false, err
			}
		}

		if exec.suspend || exec.terminated || exec.wait {
			return true, nil
		}
		return false, nil
	})
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

func NewTaskLoader(lt LoadTaskTemplate, pkgDiscover *packages.PackageDiscover, handlers providers.Providers) *taskLoader {
	return &taskLoader{
		loadTemplate: lt,
		pd:           pkgDiscover,
		handlers:     handlers,
	}
}
