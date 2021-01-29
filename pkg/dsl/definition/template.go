package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/dsl/task"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// OutputFieldName is the name of the struct contains the CR data
	OutputFieldName = "output"
	// OutputsFieldName is the name of the struct contains the map[string]CR data
	OutputsFieldName = "outputs"
	// OutputObjectPath is the path of output object in template
	OutputObjectPath = "path"
	// PatchFieldName is the name of the struct contains the patch of CR data
	PatchFieldName = "patch"
)

const (
	// AuxiliaryWorkload defines the extra workload obj from a workloadDefinition,
	// e.g. a workload composed by deployment and service, the service will be marked as AuxiliaryWorkload
	AuxiliaryWorkload = "AuxiliaryWorkload"
)

var (
	metadataAccessor = meta.NewAccessor()
)

// Template defines Definition's Render interface
type Template interface {
	Params(params interface{}) Template
	Complete(ctx process.Context) error
	Output(ctx process.Context, client client.Client, name string) Template
	HealthCheck() error
	Status(ctx process.Context, cli client.Client, ns string, handleTempl string) (string, error)
}

type def struct {
	name   string
	templ  string
	health string
	params interface{}
	output map[string]interface{}
}

type workloadDef struct {
	def
}

// NewWDTemplater create Workload Definition templater
func NewWDTemplater(name, templ, health string) Template {
	return &workloadDef{
		def: def{
			name:   name,
			templ:  templ,
			health: health,
			params: nil,
			output: nil,
		},
	}
}

// Params set definition's params
func (wd *workloadDef) Params(params interface{}) Template {
	wd.params = params
	return wd
}

// Complete do workload definition's rendering
func (wd *workloadDef) Complete(ctx process.Context) error {
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", wd.templ); err != nil {
		return err
	}
	if wd.params != nil {
		bt, _ := json.Marshal(wd.params)
		if err := bi.AddFile("parameter", fmt.Sprintf("parameter: %s", string(bt))); err != nil {
			return err
		}
	}

	if err := bi.AddFile("-", ctx.Compile("context")); err != nil {
		return err
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {
		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "workloadDef %s eval", wd.name)
		}
		output := inst.Lookup(OutputFieldName)
		base, err := model.NewBase(output)
		if err != nil {
			return errors.WithMessagef(err, "workloadDef %s new base", wd.name)
		}
		ctx.SetBase(base)

		// we will support outputs for workload composition, and it will become trait in AppConfig.
		outputs := inst.Lookup(OutputsFieldName)
		st, err := outputs.Struct()
		if err == nil {
			for i := 0; i < st.Len(); i++ {
				fieldInfo := st.Field(i)
				if fieldInfo.IsDefinition || fieldInfo.IsHidden || fieldInfo.IsOptional {
					continue
				}
				other, err := model.NewOther(fieldInfo.Value)
				if err != nil {
					return errors.WithMessagef(err, "parse WorkloadDefinition %s outputs(%s)", wd.name, fieldInfo.Name)
				}
				ctx.PutAssistants(process.Assistant{Ins: other, Type: AuxiliaryWorkload})
			}
		}
	}
	return nil
}

// Output fetch the workload cr and set result to context
func (wd *workloadDef) Output(ctx process.Context, client client.Client, name string) Template {
	base, _ := ctx.Output()
	componentWorkload, err := base.Unstructured()
	if err != nil {
		return wd
	}
	workloadCr, err := getObj(client, componentWorkload, name)
	if err != nil {
		return wd
	}
	wd.output = workloadCr
	return wd
}

// HealthCheck address health check for workload
func (wd *workloadDef) HealthCheck() error {
	if wd.health == "" {
		return nil
	}
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", wd.health); err != nil {
		return err
	}
	if wd.output != nil {
		bt, _ := json.Marshal(wd.output)
		if err := bi.AddFile(OutputFieldName, fmt.Sprintf("output: %s", string(bt))); err != nil {
			return err
		}
	} else {
		return errors.WithMessagef(errors.New("there is no workload output cr for health check"), "workload %s health check", wd.name)
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {
		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "workload %s check", wd.name)
		}
		isHealthVal := inst.Lookup("isHealth")
		if isHealthVal.Exists() {
			healthRs := isHealthVal.Eval()
			if isHealth, err := healthRs.Bool(); err != nil || !isHealth {
				return errors.WithMessage(err, "the workload is unhealthy")
			}
		}
	}
	return nil
}

// Status get workload status
func (wd *workloadDef) Status(ctx process.Context, cli client.Client, ns string, handleTempl string) (string, error) {
	return "", nil
}

type traitDef struct {
	def
}

// NewTDTemplater create Trait Definition templater
func NewTDTemplater(name, templ, health string) Template {
	return &traitDef{
		def: def{
			name:   name,
			templ:  templ,
			health: health,
		},
	}
}

// Params set definition's params
func (td *traitDef) Params(params interface{}) Template {
	td.params = params
	return td
}

// Complete do trait definition's rendering
func (td *traitDef) Complete(ctx process.Context) error {
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", td.templ); err != nil {
		return err
	}
	if td.params != nil {
		bt, _ := json.Marshal(td.params)
		if err := bi.AddFile("parameter", fmt.Sprintf("parameter: %s", string(bt))); err != nil {
			return err
		}
	}

	if err := bi.AddFile("f", ctx.Compile("context")); err != nil {
		return err
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {

		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "traitDef %s build", td.name)
		}

		processing := inst.Lookup("processing")
		var err error
		if processing.Exists() {
			if inst, err = task.Process(inst); err != nil {
				return errors.WithMessagef(err, "traitDef %s build", td.name)
			}
		}

		output := inst.Lookup(OutputFieldName)
		if output.Exists() {
			other, err := model.NewOther(output)
			if err != nil {
				return errors.WithMessagef(err, "traitDef %s new Assist", td.name)
			}
			other.SetTag(OutputObjectPath, OutputFieldName)
			ctx.PutAssistants(process.Assistant{Ins: other, Type: td.name})
		}

		outputs := inst.Lookup(OutputsFieldName)
		st, err := outputs.Struct()
		if err == nil {
			for i := 0; i < st.Len(); i++ {
				fieldInfo := st.Field(i)
				if fieldInfo.IsDefinition || fieldInfo.IsHidden || fieldInfo.IsOptional {
					continue
				}
				other, err := model.NewOther(fieldInfo.Value)
				if err != nil {
					return errors.WithMessagef(err, "traitDef %s new Assists(%s)", td.name, fieldInfo.Name)
				}
				other.SetTag(OutputObjectPath, strings.Join([]string{OutputsFieldName, fieldInfo.Name}, "."))
				ctx.PutAssistants(process.Assistant{Ins: other, Type: td.name})
			}
		}

		patcher := inst.Lookup(PatchFieldName)
		if patcher.Exists() {
			base, _ := ctx.Output()
			p, err := model.NewOther(patcher)
			if err != nil {
				return errors.WithMessagef(err, "traitDef %s patcher NewOther", td.name)
			}
			if err := base.Unify(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Status get trait status by handleTempl
func (td *traitDef) Status(ctx process.Context, cli client.Client, ns string, handleTempl string) (string, error) {
	_, assists := ctx.Output()
	var root = map[string]interface{}{}
	for _, assist := range assists {
		if assist.Type != td.name {
			continue
		}
		traitRef, err := assist.Ins.Unstructured()
		if err != nil {
			return "", err
		}

		if err := cli.Get(context.Background(), client.ObjectKey{
			Namespace: ns,
			Name:      traitRef.GetName(),
		}, traitRef); err != nil {
			return "", err
		}

		paths := strings.Split(assist.Ins.GetTag(OutputObjectPath), ".")

		x := traitRef.Object
		for i := len(paths) - 1; i >= 0; i-- {
			x = map[string]interface{}{paths[i]: x}
		}
		for k, v := range x {
			root[k] = v
		}
	}

	bt, _ := json.Marshal(root)
	var buff = "context: " + string(bt)

	buff += "\n" + handleTempl
	var r cue.Runtime
	inst, err := r.Compile("-", buff)
	if err != nil {
		return "", err
	}
	return inst.Lookup("output").String()
}

// Output fetch the trait cr and set result to context
func (td *traitDef) Output(ctx process.Context, client client.Client, name string) Template {
	_, assists := ctx.Output()
	for _, assist := range assists {
		if assist.Type != td.name {
			continue
		}
		traitRef, err := assist.Ins.Unstructured()
		if err != nil {
			return td
		}
		traitCr, err := getObj(client, traitRef, name)
		if err != nil {
			return td
		}
		td.output = traitCr
		return td
	}
	return td
}

// HealthCheck address health check for trait
func (td *traitDef) HealthCheck() error {
	if td.health == "" {
		return nil
	}
	bi := build.NewContext().NewInstance("", nil)
	if err := bi.AddFile("-", td.health); err != nil {
		return err
	}
	if td.output != nil {
		bt, _ := json.Marshal(td.output)
		if err := bi.AddFile("output", fmt.Sprintf("output: %s", string(bt))); err != nil {
			return err
		}
	} else {
		return errors.WithMessagef(errors.New("there is no trait output cr for health check"), "trait %s health check", td.name)
	}
	insts := cue.Build([]*build.Instance{bi})
	for _, inst := range insts {
		if err := inst.Value().Err(); err != nil {
			return errors.WithMessagef(err, "trait %s check", td.name)
		}
		isHealthVal := inst.Lookup("isHealth")
		if isHealthVal.Exists() {
			if isHealth, err := isHealthVal.Bool(); err != nil || !isHealth {
				return errors.WithMessage(err, "the trait is unhealthy")
			}
		}
	}
	return nil
}

func getObj(cli client.Client, obj runtime.Object, name string) (map[string]interface{}, error) {
	var kind, apiVersion string
	var err error
	kind, err = metadataAccessor.Kind(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot access object kind")
	}
	apiVersion, err = metadataAccessor.APIVersion(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot access object kind")
	}
	unList := &unstructured.UnstructuredList{}
	unList.SetKind(kind)
	unList.SetAPIVersion(apiVersion)
	if err := cli.List(context.Background(), unList, client.MatchingLabels{oam.LabelAppName: name}); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(unList.Items) == 0 {
		return nil, nil
	}
	return unList.Items[0].Object, nil
}
