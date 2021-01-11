package parser

import (
	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/template"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

// AppfileBuiltinConfig defines the built-in config variable
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
	Name     string
	Type     string
	Params   map[string]interface{}
	Template string
	Traits   []*Trait
	Scopes   []Scope
}

// GetUserConfigName get user config from AppFile, it will contain config file in it.
func (wl *Workload) GetUserConfigName() string {
	if wl.Params == nil {
		return ""
	}
	t, ok := wl.Params[AppfileBuiltinConfig]
	if !ok {
		return ""
	}
	ts, ok := t.(string)
	if !ok {
		return ""
	}
	return ts
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return definition.NewWDTemplater(wl.Name, wl.Template).Params(wl.Params).Complete(ctx)
}

// Scope defines the scope of workload
type Scope struct {
	Name string
	GVK  schema.GroupVersionKind
}

// Trait is ComponentTrait
type Trait struct {
	Name     string
	Params   map[string]interface{}
	Template string
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return definition.NewTDTemplater(trait.Name, trait.Template).Params(trait.Params).Complete(ctx)
}

// Appfile describes application
type Appfile struct {
	Name     string
	Services []*Workload
}

// TemplateValidate validate Template format
func (af *Appfile) TemplateValidate() error {
	return nil
}

// Parser is appfile parser
type Parser struct {
	templ template.Handler
	cli   defclient.DefinitionClient
}

// NewParser create appfile parser
func NewParser(cli defclient.DefinitionClient) *Parser {
	return &Parser{templ: template.GetHandler(cli), cli: cli}
}

// Parse convert map to Appfile
func (pser *Parser) Parse(name string, app *v1alpha2.Application) (*Appfile, error) {

	appfile := new(Appfile)
	appfile.Name = name
	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := pser.parseWorkload(comp)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.Services = wds

	return appfile, nil
}

func (pser *Parser) parseWorkload(comp v1alpha2.ApplicationComponent) (*Workload, error) {
	workload := new(Workload)
	workload.Traits = []*Trait{}
	workload.Name = comp.Name
	workload.Type = comp.WorkloadType
	templ, err := pser.templ(workload.Type, types.TypeWorkload)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	workload.Template = templ
	settings, err := util.RawExtension2Map(&comp.Settings)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", comp.Name)
	}
	workload.Params = settings
	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(&traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Name, comp.Name)
		}
		trait, err := pser.parseTrait(traitValue.Name, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Name)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		gvk, err := pser.cli.GetScopeGVK(scopeType)
		if err != nil {
			return nil, err
		}
		workload.Scopes = append(workload.Scopes, Scope{
			Name: instanceName,
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

	return &Trait{
		Name:     name,
		Params:   properties,
		Template: templ,
	}, nil
}

// TestExceptApp is test data
var TestExceptApp = &Appfile{
	Name: "test",
	Services: []*Workload{
		{
			Name: "myweb",
			Type: "worker",
			Params: map[string]interface{}{
				"image": "busybox",
				"cmd":   []interface{}{"sleep", "1000"},
			},
			Template: `
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
			Traits: []*Trait{
				{
					Name: "scaler",
					Params: map[string]interface{}{
						"replicas": float64(10),
					},
					Template: `
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
