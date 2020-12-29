package parser

import (
	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/template"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

// Render is cue render
type Render interface {
	// WithContext(ctx interface{}) Render
	WithParams(params interface{}) Render
	WithTemplate(raw string) Render
	Complete() (*cue.Instance, error)
}

// Workload is component
type Workload struct {
	name     string
	typ      string
	params   map[string]interface{}
	template string
	traits   []*Trait
}

// Name export workload's name
func (wl *Workload) Name() string {
	return wl.name
}

// Traits export workload's traits
func (wl *Workload) Traits() []*Trait {
	return wl.traits
}

// Eval convert template to Component
func (wl *Workload) Eval(render Render) (*v1alpha2.Component, error) {
	inst, err := render.WithParams(wl.params).WithTemplate(wl.template).Complete()
	if err != nil {
		return nil, errors.WithMessagef(err, "component %s eval", wl.name)
	}
	output := inst.Lookup("output")
	if !output.Exists() {
		return nil, errors.Errorf("output not found in component %s ", wl.name)
	}

	data, err := cueJson.Marshal(output)
	if err != nil {
		return nil, errors.WithMessagef(err, "component %s marshal", wl.name)
	}

	componentRef := &unstructured.Unstructured{}
	if err := componentRef.UnmarshalJSON([]byte(data)); err != nil {
		return nil, errors.WithMessagef(err, "component %s UnmarshalJSON to unstructured", wl.name)
	}

	component := new(v1alpha2.Component)

	component.Spec.Workload.Object = componentRef
	return component, nil
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return definition.NewWDTemplater("-", wl.template).Params(wl.params).Complete(ctx)
}

// Trait is ComponentTrait
type Trait struct {
	name     string
	params   map[string]interface{}
	template string
}

// Eval convert template to ComponentTrait
func (trait *Trait) Eval(render Render) ([]v1alpha2.ComponentTrait, error) {
	inst, err := render.WithParams(trait.params).WithTemplate(trait.template).Complete()
	if err != nil {
		return nil, errors.WithMessagef(err, "traitDef %s eval", trait.name)
	}
	cueValues := []cue.Value{}
	output := inst.Lookup("output")
	if !output.Exists() {

		outputs := inst.Lookup("outputs")
		iter, err := outputs.List()
		if err != nil {
			return nil, errors.Errorf("'output' or 'outputs' not found in trait definition %s ", trait.name)
		}
		for iter.Next() {
			cueValues = append(cueValues, iter.Value())
		}

	} else {
		cueValues = append(cueValues, output)
	}

	if len(cueValues) == 0 {
		return nil, errors.Errorf("output|outputs not found in traitDef %s ", trait.name)
	}

	compTraits := []v1alpha2.ComponentTrait{}
	for _, cv := range cueValues {
		data, err := cueJson.Marshal(cv)
		if err != nil {
			return nil, errors.WithMessagef(err, "traitDef %s marshal", trait.name)
		}
		traitRef := &unstructured.Unstructured{}
		if err := traitRef.UnmarshalJSON([]byte(data)); err != nil {
			return nil, errors.WithMessagef(err, "traitDef %s UnmarshalJSON to unstructured", trait.name)
		}
		compTrait := v1alpha2.ComponentTrait{}
		compTrait.Trait.Object = traitRef
		compTraits = append(compTraits, compTrait)
	}

	return compTraits, nil
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return definition.NewTDTemplater("-", trait.template).Params(trait.params).Complete(ctx)
}

// Appfile describle application
type Appfile struct {
	name     string
	services []*Workload
}

// TemplateValidate validate Template format
func (af *Appfile) TemplateValidate() error {
	return nil
}

// Services export Services
func (af *Appfile) Services() []*Workload {
	return af.services
}

// Name export appfile name
func (af *Appfile) Name() string {
	return af.name
}

// Parser is appfile parser
type Parser struct {
	templ template.Handler
}

// NewParser create appfile parser
func NewParser(handler template.Handler) *Parser {
	return &Parser{templ: handler}
}

// Parse convert map to Appfile
func (pser *Parser) Parse(name string, app *v1alpha2.Application) (*Appfile, error) {

	appfile := new(Appfile)
	appfile.name = name
	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := pser.parseWorkload(comp)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.services = wds

	return appfile, nil
}

func (pser *Parser) parseWorkload(comp v1alpha2.ApplicationComponent) (*Workload, error) {
	workload := new(Workload)
	workload.traits = []*Trait{}
	workload.name = comp.Name
	workload.typ = comp.WorkloadType
	templ, kind, err := pser.templ(workload.typ)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	if kind != template.WorkloadKind {
		return nil, errors.Errorf("%s type (%s) invalid", comp.Name, workload.typ)
	}
	workload.template = templ
	settings, err := DecodeJSONMarshaler(comp.Settings)
	if err != nil {
		return nil, errors.Errorf("fail to parse settings for %s", comp.Name)
	}
	workload.params = settings
	for _, traitValue := range comp.Traits {
		properties, err := DecodeJSONMarshaler(traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Name, comp.Name)
		}
		trait, err := pser.parseTrait(traitValue.Name, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Name)
		}

		workload.traits = append(workload.traits, trait)

	}

	return workload, nil
}

func (pser *Parser) parseTrait(name string, properties map[string]interface{}) (*Trait, error) {

	templ, kind, err := pser.templ(name)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, err
	}
	if kind != template.TraitKind {
		return nil, errors.Errorf("kind of %s is not trait", name)
	}
	trait := new(Trait)
	trait.template = templ
	trait.name = name
	trait.params = properties
	return trait, nil
}

// TestExceptApp is test data
var TestExceptApp = &Appfile{
	name: "test",
	services: []*Workload{
		{
			name: "myweb",
			typ:  "worker",
			params: map[string]interface{}{
				"image": "busybox",
				"cmd":   []interface{}{"sleep", "1000"},
			},
			template: `
      output: {
        apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}
      
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}
      
      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image
      
      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}
      
      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }
      
      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	cmd?: [...string]
      }`,
			traits: []*Trait{
				{
					name: "scaler",
					params: map[string]interface{}{
						"replicas": float64(10),
					},
					template: `
      output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }
`,
				},
			},
		},
	},
}
