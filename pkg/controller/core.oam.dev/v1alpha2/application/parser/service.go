package parser

import (
	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/template"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

const AppfileBuiltinConfig = "config"

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
	scopes   []Scope
}

// GetUserConfigName get user config from AppFile, it will contain config file in it.
func (wl *Workload) GetUserConfigName() string {
	if wl.params == nil {
		return ""
	}
	t, ok := wl.params[AppfileBuiltinConfig]
	if !ok {
		return ""
	}
	ts, ok := t.(string)
	if !ok {
		return ""
	}
	return ts
}

// Type export workload's type
func (wl *Workload) Type() string {
	return wl.typ
}

// Name export workload's name
func (wl *Workload) Name() string {
	return wl.name
}

// Traits export workload's traits
func (wl *Workload) Traits() []*Trait {
	return wl.traits
}

// Scopes export workload's traits
func (wl *Workload) Scopes() []Scope {
	return wl.scopes
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return definition.NewWDTemplater(wl.name, wl.template).Params(wl.params).Complete(ctx)
}

type Scope struct {
	Name string
	GVK  schema.GroupVersionKind
}

// Trait is ComponentTrait
type Trait struct {
	name     string
	params   map[string]interface{}
	template string
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return definition.NewTDTemplater(trait.name, trait.template).Params(trait.params).Complete(ctx)
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
	cli   defclient.DefinitionClient
}

// NewParser create appfile parser
func NewParser(cli defclient.DefinitionClient) *Parser {
	return &Parser{templ: template.GetHandler(cli)}
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
	templ, err := pser.templ(workload.typ, types.TypeWorkload)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	workload.template = templ
	settings, err := DecodeJSONMarshaler(comp.Settings)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", comp.Name)
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
	scopeSettings, err := DecodeJSONMarshaler(comp.Scopes)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse scopes for %s", comp.Name)
	}
	for healthType, instanceName := range scopeSettings {
		name, ok := instanceName.(string)
		if !ok {
			return nil, errors.Errorf("name of scope %s for %s must be string", healthType, comp.Name)
		}
		gvk, err := pser.cli.GetScopeGVK(healthType)
		if err != nil {
			return nil, err
		}
		workload.scopes = append(workload.scopes, Scope{
			Name: name,
			GVK:  gvk,
		})
	}
	return workload, nil
}

func (pser *Parser) parseTrait(name string, properties map[string]interface{}) (*Trait, error) {
	templ, err := pser.templ(name, types.TypeTrait)
	if kerrors.IsNotFound(err) {
		return nil, errors.Errorf("trait definition of %s not found", name)
	}
	if err != nil {
		return nil, err
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
