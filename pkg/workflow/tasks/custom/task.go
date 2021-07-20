package custom

import (
	"context"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
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
)

// LoadTaskTemplate gets the workflowStep definition from cluster and resolve it.
type LoadTaskTemplate func(ctx context.Context, name string) (string, error)

type taskLoader struct {
	loadTemplate func(ctx context.Context, name string) (string, error)
	pd           *packages.PackageDiscover
	handlers     providers.Providers
}

// GetTaskGenerator get TaskGenerator by name.
func (t *taskLoader) GetTaskGenerator(name string) (wfTypes.TaskGenerator, error) {
	templ, err := t.loadTemplate(context.Background(), name)
	if err != nil {
		return nil, err
	}
	return t.makeTaskGenerator(templ)
}

func (t *taskLoader) makeTaskGenerator(templ string) (wfTypes.TaskGenerator, error) {
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

			if outputs != nil {
				for _, output := range outputs {
					params[output.ExportKey] = output.Name
				}
			}

			paramsValue, err := ctx.MakeParameter(params)
			if err != nil {
				return common.WorkflowStepStatus{}, nil, errors.WithMessage(err, "make parameter")
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
				paramFile = fmt.Sprintf(velacue.ParameterTag+": {%s}\n", ps)
			}

			taskv, err := value.NewValue(templ+"\n"+paramFile, t.pd)
			if err != nil {
				exec.err(err, StatusReasonRendering)
				return exec.status(), exec.operation(), nil
			}

			if err := exec.doSteps(ctx, taskv); err != nil {
				exec.err(err, StatusReasonExecute)
				return exec.status(), exec.operation(), nil
			}

			return exec.status(), exec.operation(), nil
		}, nil

	}, nil
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
		return errors.Errorf("handler(provider=%s,do=%s) not found", provider, do)
	}
	return h(ctx, v, exec)
}

func (exec *executor) doSteps(ctx wfContext.Context, v *value.Value) error {
	return v.StepByFields(func(in *value.Value) (bool, error) {
		if in.CueValue().Kind() == cue.BottomKind {
			return true, errors.New("value is _|_")
		}
		if in.CueValue().Err() != nil {
			return true, in.CueValue().Err()
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
				return false, errors.WithMessagef(err, "handle (provider=%s,do=%s)", provider, do)
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

// NewTaskLoader create a tasks loader.
func NewTaskLoader(lt LoadTaskTemplate, pkgDiscover *packages.PackageDiscover, handlers providers.Providers) *taskLoader {
	return &taskLoader{
		loadTemplate: lt,
		pd:           pkgDiscover,
		handlers:     handlers,
	}
}
