/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package appfile

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// TemplateLoaderFn load template of a capability definition
type TemplateLoaderFn func(context.Context, discoverymapper.DiscoveryMapper, client.Reader, string, types.CapType) (*Template, error)

// LoadTemplate load template of a capability definition
func (fn TemplateLoaderFn) LoadTemplate(ctx context.Context, dm discoverymapper.DiscoveryMapper, c client.Reader, capName string, capType types.CapType) (*Template, error) {
	return fn(ctx, dm, c, capName, capType)
}

// Parser is an application parser
type Parser struct {
	client     client.Client
	dm         discoverymapper.DiscoveryMapper
	pd         *packages.PackageDiscover
	tmplLoader TemplateLoaderFn
}

// NewApplicationParser create appfile parser
func NewApplicationParser(cli client.Client, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover) *Parser {
	return &Parser{
		client:     cli,
		dm:         dm,
		pd:         pd,
		tmplLoader: LoadTemplate,
	}
}

// NewDryRunApplicationParser create an appfile parser for DryRun
func NewDryRunApplicationParser(cli client.Client, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, defs []oam.Object) *Parser {
	return &Parser{
		client:     cli,
		dm:         dm,
		pd:         pd,
		tmplLoader: DryRunTemplateLoader(defs),
	}
}

// GenerateAppFile converts an application to an Appfile
func (p *Parser) GenerateAppFile(ctx context.Context, app *v1beta1.Application) (*Appfile, error) {
	ns := app.Namespace
	appName := app.Name

	appfile := p.newAppfile(appName, ns, app)

	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := p.parseWorkload(ctx, comp)
		if err != nil {
			return nil, err
		}

		wds = append(wds, wd)
	}
	appfile.Workloads = wds
	appfile.Components = app.Spec.Components

	var err error

	appfile.Policies, err = p.parsePolicies(ctx, app.Spec.Policies)
	if err != nil {
		return nil, fmt.Errorf("failed to parsePolicies: %w", err)
	}

	for _, w := range wds {
		if w == nil {
			continue
		}
		if w.FullTemplate.ComponentDefinition != nil {
			cd := w.FullTemplate.ComponentDefinition.DeepCopy()
			cd.Status = v1beta1.ComponentDefinitionStatus{}
			appfile.RelatedComponentDefinitions[w.FullTemplate.ComponentDefinition.Name] = cd
		}
		for _, t := range w.Traits {
			if t == nil {
				continue
			}
			if t.FullTemplate.TraitDefinition != nil {
				td := t.FullTemplate.TraitDefinition.DeepCopy()
				td.Status = v1beta1.TraitDefinitionStatus{}
				appfile.RelatedTraitDefinitions[t.FullTemplate.TraitDefinition.Name] = td
			}
		}
		for _, s := range w.ScopeDefinition {
			if s == nil {
				continue
			}
			appfile.RelatedScopeDefinitions[s.Name] = s.DeepCopy()
		}
	}

	appfile.WorkflowMode = common.WorkflowModeDAG
	if wfSpec := app.Spec.Workflow; wfSpec != nil {
		appfile.WorkflowMode = common.WorkflowModeStep
		appfile.WorkflowSteps = wfSpec.Steps
	}

	return appfile, nil
}

func (p *Parser) newAppfile(appName, ns string, app *v1beta1.Application) *Appfile {
	file := &Appfile{
		Name:      appName,
		Namespace: ns,

		AppLabels:                   make(map[string]string),
		AppAnnotations:              make(map[string]string),
		RelatedTraitDefinitions:     make(map[string]*v1beta1.TraitDefinition),
		RelatedComponentDefinitions: make(map[string]*v1beta1.ComponentDefinition),
		RelatedScopeDefinitions:     make(map[string]*v1beta1.ScopeDefinition),

		parser: p,
		app:    app,
	}
	for k, v := range app.Annotations {
		file.AppAnnotations[k] = v
	}
	for k, v := range app.Labels {
		file.AppLabels[k] = v
	}
	return file
}

// inheritLabelAndAnnotationFromAppRev is a compatible function, that we can't record metadata for application object in AppRev
func inheritLabelAndAnnotationFromAppRev(appRev *v1beta1.ApplicationRevision) {
	if len(appRev.Spec.Application.Annotations) > 0 || len(appRev.Spec.Application.Labels) > 0 {
		return
	}
	appRev.Spec.Application.SetNamespace(appRev.Namespace)
	if appRev.Spec.Application.GetName() == "" {
		appRev.Spec.Application.SetName(appRev.Labels[oam.LabelAppName])
	}
	labels := make(map[string]string)
	for k, v := range appRev.GetLabels() {
		if k == oam.LabelAppRevisionHash || k == oam.LabelAppName {
			continue
		}
		labels[k] = v
	}
	appRev.Spec.Application.SetLabels(labels)

	annotations := make(map[string]string)
	for k, v := range appRev.GetAnnotations() {
		annotations[k] = v
	}
	appRev.Spec.Application.SetAnnotations(annotations)
}

// GenerateAppFileFromRevision converts an application revision to an Appfile
func (p *Parser) GenerateAppFileFromRevision(appRev *v1beta1.ApplicationRevision) (*Appfile, error) {

	inheritLabelAndAnnotationFromAppRev(appRev)

	app := appRev.Spec.Application.DeepCopy()
	ns := app.Namespace
	appName := app.Name
	appfile := p.newAppfile(appName, ns, app)
	appfile.AppRevisionName = appRev.Name
	appfile.AppRevisionHash = appRev.Labels[oam.LabelAppRevisionHash]

	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := p.ParseWorkloadFromRevision(comp, appRev)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.Workloads = wds
	appfile.Components = app.Spec.Components

	var err error
	appfile.Policies, err = p.parsePoliciesFromRevision(app.Spec.Policies, appRev)
	if err != nil {
		return nil, fmt.Errorf("failed to parsePolicies: %w", err)
	}

	for k, v := range appRev.Spec.ComponentDefinitions {
		appfile.RelatedComponentDefinitions[k] = v.DeepCopy()
	}
	for k, v := range appRev.Spec.TraitDefinitions {
		appfile.RelatedTraitDefinitions[k] = v.DeepCopy()
	}
	for k, v := range appRev.Spec.ScopeDefinitions {
		appfile.RelatedScopeDefinitions[k] = v.DeepCopy()
	}

	if wfSpec := app.Spec.Workflow; wfSpec != nil {
		appfile.WorkflowSteps = wfSpec.Steps
	}

	return appfile, nil
}

func (p *Parser) parsePolicies(ctx context.Context, policies []v1beta1.AppPolicy) ([]*Workload, error) {
	ws := []*Workload{}
	for _, policy := range policies {
		var w *Workload
		var err error
		if policy.Type == "garbage-collect" {
			w, err = p.makeBuiltInPolicy(policy.Name, policy.Type, policy.Properties)
		} else {
			w, err = p.makeWorkload(ctx, policy.Name, policy.Type, types.TypePolicy, policy.Properties)
		}
		if err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, nil
}

func (p *Parser) parsePoliciesFromRevision(policies []v1beta1.AppPolicy, appRev *v1beta1.ApplicationRevision) ([]*Workload, error) {
	ws := []*Workload{}
	for _, policy := range policies {
		w, err := p.makeWorkloadFromRevision(policy.Name, policy.Type, types.TypePolicy, policy.Properties, appRev)
		if err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, nil
}

func (p *Parser) makeWorkload(ctx context.Context, name, typ string, capType types.CapType, props *runtime.RawExtension) (*Workload, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.dm, p.client, typ, capType)
	if err != nil {
		return nil, errors.WithMessagef(err, "fetch component/policy type of %s", name)
	}
	return p.convertTemplate2Workload(name, typ, props, templ)
}

func (p *Parser) makeBuiltInPolicy(name, typ string, props *runtime.RawExtension) (*Workload, error) {
	settings, err := util.RawExtension2Map(props)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", name)
	}
	return &Workload{
		Traits:          []*Trait{},
		ScopeDefinition: []*v1beta1.ScopeDefinition{},
		Name:            name,
		Type:            typ,
		Params:          settings,
		engine:          definition.NewWorkloadAbstractEngine(name, p.pd),
	}, nil
}

func (p *Parser) makeWorkloadFromRevision(name, typ string, capType types.CapType, props *runtime.RawExtension, appRev *v1beta1.ApplicationRevision) (*Workload, error) {
	templ, err := LoadTemplateFromRevision(typ, capType, appRev, p.dm)
	if err != nil {
		return nil, errors.WithMessagef(err, "fetch component/policy type of %s from revision", name)
	}

	return p.convertTemplate2Workload(name, typ, props, templ)
}

func (p *Parser) convertTemplate2Workload(name, typ string, props *runtime.RawExtension, templ *Template) (*Workload, error) {
	settings, err := util.RawExtension2Map(props)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", name)
	}
	wlType, err := util.ConvertDefinitionRevName(typ)
	if err != nil {
		wlType = typ
	}
	return &Workload{
		Traits:             []*Trait{},
		ScopeDefinition:    []*v1beta1.ScopeDefinition{},
		Name:               name,
		Type:               wlType,
		CapabilityCategory: templ.CapabilityCategory,
		FullTemplate:       templ,
		Params:             settings,
		engine:             definition.NewWorkloadAbstractEngine(name, p.pd),
	}, nil
}

// parseWorkload resolve an ApplicationComponent and generate a Workload
// containing ALL information required by an Appfile.
func (p *Parser) parseWorkload(ctx context.Context, comp common.ApplicationComponent) (*Workload, error) {
	workload, err := p.makeWorkload(ctx, comp.Name, comp.Type, types.TypeComponentDefinition, comp.Properties)
	if err != nil {
		return nil, err
	}
	workload.ExternalRevision = comp.ExternalRevision

	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, comp.Name)
		}
		trait, err := p.parseTrait(ctx, traitValue.Type, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Type)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		sd, gvk, err := GetScopeDefAndGVK(ctx, p.client, p.dm, scopeType)
		if err != nil {
			return nil, err
		}
		workload.Scopes = append(workload.Scopes, Scope{
			Name:            instanceName,
			GVK:             gvk,
			ResourceVersion: sd.Spec.Reference.Name + "/" + sd.Spec.Reference.Version,
		})
		workload.ScopeDefinition = append(workload.ScopeDefinition, sd)
	}
	return workload, nil
}

// ParseWorkloadFromRevision resolve an ApplicationComponent and generate a Workload
// containing ALL information required by an Appfile from app revision.
func (p *Parser) ParseWorkloadFromRevision(comp common.ApplicationComponent, appRev *v1beta1.ApplicationRevision) (*Workload, error) {
	workload, err := p.makeWorkloadFromRevision(comp.Name, comp.Type, types.TypeComponentDefinition, comp.Properties, appRev)
	if err != nil {
		return nil, err
	}
	workload.ExternalRevision = comp.ExternalRevision

	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, comp.Name)
		}
		trait, err := p.parseTraitFromRevision(traitValue.Type, properties, appRev)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Type)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		sd, gvk, err := GetScopeDefAndGVKFromRevision(scopeType, appRev)
		if err != nil {
			return nil, err
		}
		workload.Scopes = append(workload.Scopes, Scope{
			Name:            instanceName,
			GVK:             gvk,
			ResourceVersion: sd.Spec.Reference.Name + "/" + sd.Spec.Reference.Version,
		})
		workload.ScopeDefinition = append(workload.ScopeDefinition, sd)
	}
	return workload, nil
}

func (p *Parser) parseTrait(ctx context.Context, name string, properties map[string]interface{}) (*Trait, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.dm, p.client, name, types.TypeTrait)
	if kerrors.IsNotFound(err) {
		return nil, errors.Errorf("trait definition of %s not found", name)
	}
	if err != nil {
		return nil, err
	}
	return p.convertTemplate2Trait(name, properties, templ)
}

func (p *Parser) parseTraitFromRevision(name string, properties map[string]interface{}, appRev *v1beta1.ApplicationRevision) (*Trait, error) {
	templ, err := LoadTemplateFromRevision(name, types.TypeTrait, appRev, p.dm)
	if err != nil {
		return nil, err
	}
	return p.convertTemplate2Trait(name, properties, templ)
}

func (p *Parser) convertTemplate2Trait(name string, properties map[string]interface{}, templ *Template) (*Trait, error) {
	traitName, err := util.ConvertDefinitionRevName(name)
	if err != nil {
		traitName = name
	}
	return &Trait{
		Name:               traitName,
		CapabilityCategory: templ.CapabilityCategory,
		Params:             properties,
		Template:           templ.TemplateStr,
		HealthCheckPolicy:  templ.Health,
		CustomStatusFormat: templ.CustomStatus,
		FullTemplate:       templ,
		engine:             definition.NewTraitAbstractEngine(traitName, p.pd),
	}, nil
}

// ValidateComponentNames validate all component name whether repeat in cluster and template
func (p *Parser) ValidateComponentNames(ctx context.Context, af *Appfile) (int, error) {
	existCompNames := make(map[string]string)
	existApps := v1beta1.ApplicationList{}

	listOpts := []client.ListOption{
		client.InNamespace(af.Namespace),
	}
	if err := p.client.List(ctx, &existApps, listOpts...); err != nil {
		return 0, err
	}
	for _, existApp := range existApps.Items {
		ea := existApp.DeepCopy()
		existAf, err := p.GenerateAppFile(ctx, ea)
		if err != nil || existAf.Name == af.Name {
			continue
		}
		for _, existComp := range existAf.Workloads {
			existCompNames[existComp.Name] = existApp.Name
		}
	}

	for i, wl := range af.Workloads {
		if existAfName, ok := existCompNames[wl.Name]; ok {
			return i, fmt.Errorf("component named '%s' is already exist in application '%s'", wl.Name, existAfName)
		}
		for j := i + 1; j < len(af.Workloads); j++ {
			if wl.Name == af.Workloads[j].Name {
				return i, fmt.Errorf("component named '%s' is repeat in this appfile", wl.Name)
			}
		}
	}

	return 0, nil
}

// GetScopeDefAndGVK get grouped API version of the given scope
func GetScopeDefAndGVK(ctx context.Context, cli client.Reader, dm discoverymapper.DiscoveryMapper,
	name string) (*v1beta1.ScopeDefinition, metav1.GroupVersionKind, error) {
	var gvk metav1.GroupVersionKind
	sd := new(v1beta1.ScopeDefinition)
	err := util.GetDefinition(ctx, cli, sd, name)
	if err != nil {
		return nil, gvk, err
	}
	gvk, err = util.GetGVKFromDefinition(dm, sd.Spec.Reference)
	if err != nil {
		return nil, gvk, err
	}
	return sd, gvk, nil
}

// GetScopeDefAndGVKFromRevision get grouped API version of the given scope
func GetScopeDefAndGVKFromRevision(name string, appRev *v1beta1.ApplicationRevision) (*v1beta1.ScopeDefinition, metav1.GroupVersionKind, error) {
	var gvk metav1.GroupVersionKind
	sd, ok := appRev.Spec.ScopeDefinitions[name]
	if !ok {
		return nil, gvk, fmt.Errorf("scope %s not found in application revision", name)
	}
	gvk, ok = appRev.Spec.ScopeGVK[sd.Spec.Reference.Name+"/"+sd.Spec.Reference.Version]
	if !ok {
		return nil, gvk, fmt.Errorf("scope definition found but GVK %s not found in application revision", name)
	}
	return sd.DeepCopy(), gvk, nil
}
