package remote

import (
	"context"
	"cuelang.org/go/cue/build"
	"fmt"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/workflow"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (params workflow.Value, td tasks.TaskDiscovery, pds workflow.Providers) (workflow.TaskRunner, error)

type taskLoader struct {
	dm  discoverymapper.DiscoveryMapper
	cli client.Reader
}

func (t *taskLoader) GetTaskGenerator(name string) (tasks.TaskGenerator, error) {
	templ, err := t.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	return func(params workflow.Value, td tasks.TaskDiscovery, pds workflow.Providers) (workflow.TaskRunner, error) {

	}, nil
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
	return func(params *model.Value, td tasks.TaskDiscovery, pds workflow.Providers) (workflow.TaskRunner, error) {
		bi := build.NewContext().NewInstance("", nil)
		var paramFile = velacue.ParameterTag + ": {}"
		if params != nil {
			ps, err := params.String()
			if err != nil {
				return nil, errors.WithMessage(err, "params encode")
			}
			paramFile = fmt.Sprintf("%s: %s", velacue.ParameterTag, ps)
		}
		if err := bi.AddFile("parameter", paramFile); err != nil {
			return nil, errors.WithMessage(err, "invalid parameter")
		}
		if err := bi.AddFile("-", templ); err != nil {
			return nil, errors.WithMessage(err, "invalid schematic template")
		}

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
	td  tasks.TaskDiscovery
	pds workflow.Providers
}

func (exec *executor) apply(v *model.Value) error {

	v.LookupDef("#provider")
}

func (exec *executor) doSteps(v *model.Value) error {
	fields, err := v.ObjectFileds()
	if err != nil {
		return err
	}
	for _, field := range fields {
		if do:=opTpy(field.Value);do!=""{
			provider:=opProvider(field.Value)
			if err:=exec.pds.Handle(provider,do,field.Value);err!=nil{
				return err
			}
			v.FillObject(field.Value,field.Name)
		}
	}
}




func opTpy(v *model.Value) string {
	return getLabel(v,"#do")
}

func opProvider(v *model.Value) string {
	provider:=getLabel(v,"#provider")
	if provider==""{
		provider="_builtin_"
	}
	return provider
}

func getLabel(v *model.Value,label string)string{
	do, err := v.Filed(label)
	if err == nil && do.Exists() {
		if str, err := do.String(); err == nil {
			return str
		}
	}
	return ""
}

func (exec *executor) doStep(v *model.Value) error {
	do, err := v.LookupDef("#do").String()
	if err != nil {
		return err
	}
	providerName, err := v.LookupDef("#provider").String()
	if err != nil {
		return err
	}
	provider, ok := exec.pds[providerName]
	if !ok {
		return errors.Errorf("provider %s not supported", providerName)
	}
	handle, ok := provider[do]
	if !ok {
		return errors.Errorf("handle %s not supported in porvider %s", do, providerName)
	}
	result, err := handle(v)
	if !ok {
		return errors.WithMessagef(err, "handle %s (porvider %s)", do, providerName)
	}
	v.FillObject(result)
}

func isTaskStep(v *model.Value) bool {
	v.Filed()
	return v.LookupDef("#step_type").Exists()
}

func NewTaskLoader(dm discoverymapper.DiscoveryMapper, cli client.Reader) (*taskLoader, error) {

}
