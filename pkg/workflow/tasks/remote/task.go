package remote

import (
	"context"
	"fmt"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
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
	dm  discoverymapper.DiscoveryMapper
	cli client.Reader
	pd  *packages.PackageDiscover
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
	return func(params *model.Value, td tasks.TaskDiscover, pds providers.Providers) (workflow.TaskRunner, error) {

		var paramFile = velacue.ParameterTag + ": {}"
		if params != nil {
			ps, err := params.String()
			if err != nil {
				return nil, errors.WithMessage(err, "params encode")
			}
			paramFile = fmt.Sprintf("%s: %s", velacue.ParameterTag, ps)
		}

		taskv, err := model.NewValue(paramFile+"\n"+templ, t.pd)
		if err != nil {
			return nil, errors.WithMessagef(err, "convert cue template to task value")
		}
		exec := &executor{
			td:  td,
			pds: pds,
		}
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *workflow.Operation, error) {
			if err := exec.doSteps(ctx, taskv); err != nil {
				return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed, Message: exec.message, Reason: exec.reason}, nil, nil
			}
			status := common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseSucceeded, Message: exec.message, Reason: exec.reason}
			if exec.wait {
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
	td  tasks.TaskDiscover
	pds providers.Providers

	wfStatus common.WorkflowStepStatus
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
	h, exist := exec.pds.GetHandler(provider, do)
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

func NewTaskLoader(dm discoverymapper.DiscoveryMapper, cli client.Reader, pd *packages.PackageDiscover) *taskLoader {
	return &taskLoader{
		dm:  dm,
		cli: cli,
		pd:  pd,
	}
}
