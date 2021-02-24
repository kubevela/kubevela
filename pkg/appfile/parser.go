package appfile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/config"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// AppfileBuiltinConfig defines the built-in config variable
	AppfileBuiltinConfig = "config"
)

// Workload is component
type Workload struct {
	Name               string
	Type               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}
	Traits             []*Trait
	Scopes             []Scope

	Template           string
	HealthCheckPolicy  string
	CustomStatusFormat string

	Helm                *v1alpha2.Helm
	DefinitionReference v1alpha2.DefinitionReference
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
	return definition.NewWorkloadAbstractEngine(wl.Name).Params(wl.Params).Complete(ctx, wl.Template)
}

// EvalStatus eval workload status
func (wl *Workload) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return definition.NewWorkloadAbstractEngine(wl.Name).Status(ctx, cli, ns, wl.CustomStatusFormat)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return definition.NewWorkloadAbstractEngine(wl.Name).HealthCheck(ctx, client, namespace, wl.HealthCheckPolicy)
}

// Scope defines the scope of workload
type Scope struct {
	Name string
	GVK  schema.GroupVersionKind
}

// Trait is ComponentTrait
type Trait struct {
	// The Name is name of TraitDefinition, actually it's a type of the trait instance
	Name               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}

	Template           string
	HealthCheckPolicy  string
	CustomStatusFormat string
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return definition.NewTraitAbstractEngine(trait.Name).Params(trait.Params).Complete(ctx, trait.Template)
}

// EvalStatus eval trait status
func (trait *Trait) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return definition.NewTraitAbstractEngine(trait.Name).Status(ctx, cli, ns, trait.CustomStatusFormat)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return definition.NewTraitAbstractEngine(trait.Name).HealthCheck(ctx, client, namespace, trait.HealthCheckPolicy)
}

// Appfile describes application
type Appfile struct {
	Name         string
	RevisionName string
	Workloads    []*Workload
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
func (p *Parser) GenerateAppFile(ctx context.Context, name string, app *v1alpha2.Application) (*Appfile, error) {
	appfile := new(Appfile)
	appfile.Name = name
	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := p.parseWorkload(ctx, comp)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.Workloads = wds
	appfile.RevisionName, _ = utils.GetAppNextRevision(app)
	return appfile, nil
}

func (p *Parser) parseWorkload(ctx context.Context, comp v1alpha2.ApplicationComponent) (*Workload, error) {
	workload := new(Workload)
	workload.Traits = []*Trait{}
	workload.Name = comp.Name
	workload.Type = comp.WorkloadType
	templ, err := util.LoadTemplate(ctx, p.client, workload.Type, types.TypeWorkload)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	workload.CapabilityCategory = templ.CapabilityCategory
	workload.Template = templ.TemplateStr
	workload.HealthCheckPolicy = templ.Health
	workload.CustomStatusFormat = templ.CustomStatus
	workload.DefinitionReference = templ.Reference
	workload.Helm = templ.Helm
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
		trait, err := p.parseTrait(ctx, traitValue.Name, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Name)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		gvk, err := util.GetScopeGVK(ctx, p.client, p.dm, scopeType)
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

func (p *Parser) parseTrait(ctx context.Context, name string, properties map[string]interface{}) (*Trait, error) {
	templ, err := util.LoadTemplate(ctx, p.client, name, types.TypeTrait)
	if kerrors.IsNotFound(err) {
		return nil, errors.Errorf("trait definition of %s not found", name)
	}
	if err != nil {
		return nil, err
	}

	return &Trait{
		Name:               name,
		CapabilityCategory: templ.CapabilityCategory,
		Params:             properties,
		Template:           templ.TemplateStr,
		HealthCheckPolicy:  templ.Health,
		CustomStatusFormat: templ.CustomStatus,
	}, nil
}

// GenerateApplicationConfiguration converts an appFile to applicationConfig & Components
func (p *Parser) GenerateApplicationConfiguration(app *Appfile, ns string) (*v1alpha2.ApplicationConfiguration,
	[]*v1alpha2.Component, error) {
	appconfig := &v1alpha2.ApplicationConfiguration{}
	appconfig.SetGroupVersionKind(v1alpha2.ApplicationConfigurationGroupVersionKind)
	appconfig.Name = app.Name
	appconfig.Namespace = ns

	if appconfig.Labels == nil {
		appconfig.Labels = map[string]string{}
	}
	appconfig.Labels[oam.LabelAppName] = app.Name

	var components []*v1alpha2.Component
	for _, wl := range app.Workloads {
		var comp *v1alpha2.Component
		var acComp *v1alpha2.ApplicationConfigurationComponent
		var err error

		switch wl.CapabilityCategory {
		case types.HelmCategory:
			comp, acComp, err = generateComponentFromHelmModule(p.client, p.dm, wl, app.Name, app.RevisionName, ns)
			if err != nil {
				return nil, nil, err
			}
		default:
			comp, acComp, err = generateComponentFromCUEModule(p.client, wl, app.Name, app.RevisionName, ns)
			if err != nil {
				return nil, nil, err
			}
		}
		components = append(components, comp)
		appconfig.Spec.Components = append(appconfig.Spec.Components, *acComp)
	}
	return appconfig, components, nil
}

func generateComponentFromCUEModule(c client.Client, wl *Workload, appName, revision, ns string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	pCtx, err := PrepareProcessContext(c, wl, appName, revision, ns)
	if err != nil {
		return nil, nil, err
	}
	for _, tr := range wl.Traits {
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, wl.Name)
		}
	}
	var comp *v1alpha2.Component
	var acComp *v1alpha2.ApplicationConfigurationComponent
	comp, acComp, err = evalWorkloadWithContext(pCtx, wl, appName, wl.Name)
	if err != nil {
		return nil, nil, err
	}
	comp.Name = wl.Name
	acComp.ComponentName = comp.Name

	for _, sc := range wl.Scopes {
		acComp.Scopes = append(acComp.Scopes, v1alpha2.ComponentScope{ScopeReference: v1alpha1.TypedReference{
			APIVersion: sc.GVK.GroupVersion().String(),
			Kind:       sc.GVK.Kind,
			Name:       sc.Name,
		}})
	}

	comp.Namespace = ns
	if comp.Labels == nil {
		comp.Labels = map[string]string{}
	}
	comp.Labels[oam.LabelAppName] = appName
	comp.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)

	return comp, acComp, nil
}

func generateComponentFromHelmModule(c client.Client, dm discoverymapper.DiscoveryMapper, wl *Workload, appName, revision, ns string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	targetWokrloadGVK, err := util.GetGVKFromDefinition(dm, wl.DefinitionReference)
	if err != nil {
		return nil, nil, err
	}

	// NOTE this is a hack way to enable using CUE module capabilities on Helm module workload
	// construct an empty base workload according to its GVK
	wl.Template = fmt.Sprintf(`
output: {
	apiVersion: "%s"
	kind: "%s"
}`, targetWokrloadGVK.GroupVersion().String(), targetWokrloadGVK.Kind)

	// re-use the way CUE module generates comp & acComp
	comp, acComp, err := generateComponentFromCUEModule(c, wl, appName, revision, ns)
	if err != nil {
		return nil, nil, err
	}

	release, repo, err := helm.RenderHelmReleaseAndHelmRepo(wl.Helm, wl.Name, appName, ns, wl.Params)
	if err != nil {
		return nil, nil, err
	}
	rlsBytes, err := json.Marshal(release.Object)
	if err != nil {
		return nil, nil, err
	}
	repoBytes, err := json.Marshal(repo.Object)
	if err != nil {
		return nil, nil, err
	}
	comp.Spec.Helm = &v1alpha2.Helm{
		Release:    runtime.RawExtension{Raw: rlsBytes},
		Repository: runtime.RawExtension{Raw: repoBytes},
	}
	return comp, acComp, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component and ACComponent
func evalWorkloadWithContext(pCtx process.Context, wl *Workload, appName, compName string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	base, assists := pCtx.Output()
	componentWorkload, err := base.Unstructured()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "evaluate base template component=%s app=%s", compName, appName)
	}

	labels := map[string]string{
		oam.WorkloadTypeLabel: wl.Type,
		oam.LabelAppName:      appName,
		oam.LabelAppComponent: compName,
	}
	util.AddLabels(componentWorkload, labels)

	component := &v1alpha2.Component{}
	component.Spec.Workload = util.Object2RawExtension(componentWorkload)

	acComponent := &v1alpha2.ApplicationConfigurationComponent{}
	for _, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, compName, appName)
		}
		labels := map[string]string{
			oam.TraitTypeLabel:    assist.Type,
			oam.LabelAppName:      appName,
			oam.LabelAppComponent: compName,
		}
		if assist.Name != "" {
			labels[oam.TraitResource] = assist.Name
		}
		util.AddLabels(tr, labels)
		acComponent.Traits = append(acComponent.Traits, v1alpha2.ComponentTrait{
			// we need to marshal the trait to byte array before sending them to the k8s
			Trait: util.Object2RawExtension(tr),
		})
	}
	return component, acComponent, nil
}

// PrepareProcessContext prepares a DSL process Context
func PrepareProcessContext(k8sClient client.Client, wl *Workload, applicationName, revision string, namespace string) (process.Context, error) {
	pCtx := process.NewContext(wl.Name, applicationName, revision)
	userConfig := wl.GetUserConfigName()
	if userConfig != "" {
		cg := config.Configmap{Client: k8sClient}
		// TODO(wonderflow): envName should not be namespace when we have serverside env
		var envName = namespace
		data, err := cg.GetConfigData(config.GenConfigMapName(applicationName, wl.Name, userConfig), envName)
		if err != nil {
			return nil, errors.Wrapf(err, "get config=%s for app=%s in namespace=%s", userConfig, applicationName, namespace)
		}
		pCtx.SetConfigs(data)
	}
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", applicationName, namespace)
	}
	return pCtx, nil
}
