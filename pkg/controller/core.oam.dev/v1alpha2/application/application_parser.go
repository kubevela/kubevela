package application

import (
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// AppfileBuiltinConfig defines the built-in config variable
const AppfileBuiltinConfig = "config"

// Workload is component
type Workload struct {
	Name     string
	Type     string
	Params   map[string]interface{}
	Template string
	Health   string
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
	return definition.NewWDTemplater(wl.Name, wl.Template, "").Params(wl.Params).Complete(ctx)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(ctx process.Context, client client.Client, name string) error {
	return definition.NewWDTemplater(wl.Name, "", wl.Health).Output(ctx, client, name).HealthCheck()
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
	Health   string
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return definition.NewTDTemplater(trait.Name, trait.Template, "").Params(trait.Params).Complete(ctx)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(ctx process.Context, client client.Client, name string) error {
	return definition.NewTDTemplater(trait.Name, "", trait.Health).Output(ctx, client, name).HealthCheck()
}

// Appfile describes application
type Appfile struct {
	Name      string
	Workloads []*Workload
}

// TemplateValidate validate Template format
func (af *Appfile) TemplateValidate() error {
	return nil
}

// Parser is an application parser
type Parser struct {
	client client.Client
	dm     discoverymapper.DiscoveryMapper
}

// NewApplicationParser create appfile parser
func NewApplicationParser(cli client.Client, dm discoverymapper.DiscoveryMapper) *Parser {
	return &Parser{
		client: cli,
		dm:     dm,
	}
}

// GenerateAppFile converts an application to an Appfile
func (p *Parser) GenerateAppFile(name string, app *v1alpha2.Application) (*Appfile, error) {
	appfile := new(Appfile)
	appfile.Name = name
	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := p.parseWorkload(comp)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.Workloads = wds

	return appfile, nil
}

func (p *Parser) parseWorkload(comp v1alpha2.ApplicationComponent) (*Workload, error) {
	workload := new(Workload)
	workload.Traits = []*Trait{}
	workload.Name = comp.Name
	workload.Type = comp.WorkloadType
	templ, health, err := util.LoadTemplate(p.client, workload.Type, types.TypeWorkload)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	workload.Template = templ
	workload.Health = health
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
		trait, err := p.parseTrait(traitValue.Name, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Name)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		gvk, err := util.GetScopeGVK(p.client, p.dm, scopeType)
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

func (p *Parser) parseTrait(name string, properties map[string]interface{}) (*Trait, error) {
	templ, health, err := util.LoadTemplate(p.client, name, types.TypeTrait)
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
		Health:   health,
	}, nil
}
