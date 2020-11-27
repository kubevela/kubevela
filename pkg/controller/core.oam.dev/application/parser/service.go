package parser

import (
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/template"
)

// Render is cue render
type Render interface {
	//WithContext(ctx interface{}) Render
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
			return nil, errors.Errorf("output|outputs not found in traitDef %s ", trait.name)
			//return nil,errors.WithMessagef(err,"traitDef %s outputs must be list",trait.name)
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
func (pser *Parser) Parse(name string, expr map[string]interface{}) (*Appfile, error) {
	var svcs interface{}
	for _, name := range []string{"Service", "Services", "service", "services"} {
		if v, ok := expr[name]; ok {
			svcs = v
		}
	}
	if svcs == nil {
		return nil, errors.Errorf("services require")
	}
	appfile := new(Appfile)
	appfile.name = name
	switch v := svcs.(type) {
	case map[string]interface{}:
		wds := []*Workload{}
		for name, wi := range v {
			wd, err := pser.parseWorkload(name, wi)
			if err != nil {
				return nil, err
			}
			wds = append(wds, wd)
		}
		appfile.services = wds
	default:
		return nil, errors.Errorf("services format invalid(must be map)")
	}
	return appfile, nil
}

func (pser *Parser) parseWorkload(name string, expr interface{}) (*Workload, error) {
	workload := new(Workload)
	workload.traits = []*Trait{}
	workload.name = name
	switch v := expr.(type) {
	case map[string]interface{}:
		_type, ok := v["type"]
		if !ok {
			return nil, errors.Errorf("type not specify")
		}
		workload.typ = fmt.Sprint(_type)
		templ, kind, err := pser.templ(workload.typ)
		if err != nil && !kerrors.IsNotFound(err) {
			return nil, errors.WithMessagef(err, "fetch %s' type", name)
		}
		if kind == template.Unkownkind || kind == template.TraitKind {
			return nil, errors.Errorf("%s type (%s) invalid", name, workload.typ)
		}
		workload.template = templ
		params := map[string]interface{}{}
		for lable, value := range v {
			if lable == "type" {
				continue
			}
			trait, err := pser.parseTrait(lable, value)
			if err != nil {
				return nil, errors.WithMessagef(err, "service(%s) parse trait(%s)", name, lable)
			}
			if trait == nil {
				params[lable] = value
			} else {
				workload.traits = append(workload.traits, trait)
			}
		}
		workload.params = params
	default:
		return nil, errors.Errorf("service(%s) format invalid", name)
	}

	return workload, nil
}

func (pser *Parser) parseTrait(label string, expr interface{}) (*Trait, error) {

	templ, kind, err := pser.templ(label)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, err
	}
	if kind != template.TraitKind {
		return nil, nil
	}
	trait := new(Trait)
	trait.template = templ
	trait.name = label
	switch v := expr.(type) {
	case map[string]interface{}:
		trait.params = v
	default:
		return nil, errors.Errorf("trait %s params must be map", label)
	}
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
