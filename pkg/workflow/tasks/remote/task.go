package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/workflow"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type taskLoader struct {
	dm       discoverymapper.DiscoveryMapper
	cli      client.Reader
	pd       *packages.PackageDiscover
	handlers providers.Providers
}

func (t *taskLoader) GetTaskGenerator(name string) (tasks.TaskGenerator, error) {
	templ, err := t.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	return t.makeTaskGenerator(templ)
}

func (t *taskLoader) loadTemplate(name string) (string, error) {
	templ, err := appfile.LoadTemplate(context.Background(), t.dm, t.cli, name, types.TypeWorkflowStep)
	if err != nil {
		return "", err
	}
	schematic := templ.WorkflowStepDefinition.Spec.Schematic
	if schematic != nil && schematic.CUE != nil {
		return schematic.CUE.Template, nil
	}
	return "", errors.New("custom workflowStep only support cue")
}

func (t *taskLoader) makeTaskGenerator(templ string) (tasks.TaskGenerator, error) {
	return func(wfStep v1beta1.WorkflowStep) (workflow.TaskRunner, error) {

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
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *workflow.Operation, error) {

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
			taskv, err := model.NewValue(paramFile+templ, t.pd)
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

			operation := &workflow.Operation{
				Terminated: exec.terminated,
				Suspend:    exec.suspend,
			}

			return status, operation, nil
		}, nil

		return nil, nil

	}, nil
}

/*

#do: "steps"

load: op.#Load & {
   #do: "load"
   #provider: "_builtin_"
   #component: "xxx"
}

app: op.#Read & {
    #do: "read"
    #provider: "kube"
}

step: op.#Apply & {
   #do: "apply"
   #provider: "kube"
   #up: {
      hook: op.#Apply & {

      }
   }
}

*/

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

func (exec *executor) Handle(ctx wfContext.Context, provider string, do string, v *model.Value) error {
	h, exist := exec.handlers.GetHandler(provider, do)
	if !exist {
		return errors.Errorf("handler(provider=%s,do=%s) not found", provider, do)
	}
	return h(ctx, v, exec)
}

func (exec *executor) doSteps(ctx wfContext.Context, v *model.Value) error {
	return v.StepFields(func(in *model.Value) (bool, error) {
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

func opTpy(v *model.Value) string {
	return getLabel(v, "#do")
}

func opProvider(v *model.Value) string {
	provider := getLabel(v, "#provider")
	if provider == "" {
		provider = "builtin"
	}
	return provider
}

func getLabel(v *model.Value, label string) string {
	do, err := v.Field(label)
	if err == nil && do.Exists() {
		if str, err := do.String(); err == nil {
			return str
		}
	}
	return ""
}

func NewTaskLoader(dm discoverymapper.DiscoveryMapper, cli client.Reader, pkgDiscover *packages.PackageDiscover, handlers providers.Providers) *taskLoader {
	return &taskLoader{
		dm:       dm,
		cli:      cli,
		pd:       pkgDiscover,
		handlers: handlers,
	}
}
