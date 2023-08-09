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
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	policypkg "github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/utils"
	utilscommon "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// TemplateLoaderFn load template of a capability definition
type TemplateLoaderFn func(context.Context, client.Client, string, types.CapType) (*Template, error)

// LoadTemplate load template of a capability definition
func (fn TemplateLoaderFn) LoadTemplate(ctx context.Context, c client.Client, capName string, capType types.CapType) (*Template, error) {
	return fn(ctx, c, capName, capType)
}

// Parser is an application parser
type Parser struct {
	client     client.Client
	pd         *packages.PackageDiscover
	tmplLoader TemplateLoaderFn
}

// NewApplicationParser create appfile parser
func NewApplicationParser(cli client.Client, pd *packages.PackageDiscover) *Parser {
	return &Parser{
		client:     cli,
		pd:         pd,
		tmplLoader: LoadTemplate,
	}
}

// NewDryRunApplicationParser create an appfile parser for DryRun
func NewDryRunApplicationParser(cli client.Client, pd *packages.PackageDiscover, defs []*unstructured.Unstructured) *Parser {
	return &Parser{
		client:     cli,
		pd:         pd,
		tmplLoader: DryRunTemplateLoader(defs),
	}
}

// GenerateAppFile generate appfile for the application to run, if the application is controlled by PublishVersion,
// the application revision will be used to create the appfile
func (p *Parser) GenerateAppFile(ctx context.Context, app *v1beta1.Application) (*Appfile, error) {
	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("generate-app-file", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("generate-appfile").Observe(v)
		}))
		defer subCtx.Commit("finish generate appFile")
	}
	if isLatest, appRev, err := p.isLatestPublishVersion(ctx, app); err != nil {
		return nil, err
	} else if isLatest {
		app.Spec = appRev.Spec.Application.Spec
		return p.GenerateAppFileFromRevision(appRev)
	}
	return p.GenerateAppFileFromApp(ctx, app)
}

// GenerateAppFileFromApp converts an application to an Appfile
func (p *Parser) GenerateAppFileFromApp(ctx context.Context, app *v1beta1.Application) (*Appfile, error) {

	for idx := range app.Spec.Policies {
		if app.Spec.Policies[idx].Name == "" {
			app.Spec.Policies[idx].Name = fmt.Sprintf("%s:auto-gen:%d", app.Spec.Policies[idx].Type, idx)
		}
	}

	appFile := newAppFile(app)
	if app.Status.LatestRevision != nil {
		appFile.AppRevisionName = app.Status.LatestRevision.Name
	}

	var err error
	if err = p.parseComponents(ctx, appFile); err != nil {
		return nil, errors.Wrap(err, "failed to parseComponents")
	}
	if err = p.parseWorkflowSteps(ctx, appFile); err != nil {
		return nil, errors.Wrap(err, "failed to parseWorkflowSteps")
	}
	if err = p.parsePolicies(ctx, appFile); err != nil {
		return nil, errors.Wrap(err, "failed to parsePolicies")
	}
	if err = p.parseReferredObjects(ctx, appFile); err != nil {
		return nil, errors.Wrap(err, "failed to parseReferredObjects")
	}

	return appFile, nil
}

func newAppFile(app *v1beta1.Application) *Appfile {
	file := &Appfile{
		Name:      app.Name,
		Namespace: app.Namespace,

		AppLabels:                      make(map[string]string),
		AppAnnotations:                 make(map[string]string),
		RelatedTraitDefinitions:        make(map[string]*v1beta1.TraitDefinition),
		RelatedComponentDefinitions:    make(map[string]*v1beta1.ComponentDefinition),
		RelatedWorkflowStepDefinitions: make(map[string]*v1beta1.WorkflowStepDefinition),

		ExternalPolicies: make(map[string]*v1alpha1.Policy),

		app: app,
	}
	for k, v := range app.Annotations {
		file.AppAnnotations[k] = v
	}
	for k, v := range app.Labels {
		file.AppLabels[k] = v
	}
	return file
}

// isLatestPublishVersion checks if the latest application revision has the same publishVersion with the application,
// return true and the latest ApplicationRevision if they share the same publishVersion
func (p *Parser) isLatestPublishVersion(ctx context.Context, app *v1beta1.Application) (bool, *v1beta1.ApplicationRevision, error) {
	if !metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationPublishVersion) {
		return false, nil, nil
	}
	if app.Status.LatestRevision == nil {
		return false, nil, nil
	}
	appRev := &v1beta1.ApplicationRevision{}
	if err := p.client.Get(ctx, ktypes.NamespacedName{Name: app.Status.LatestRevision.Name, Namespace: app.GetNamespace()}, appRev); err != nil {
		if kerrors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, errors.Wrapf(err, "failed to load latest application revision")
	}
	if !metav1.HasAnnotation(appRev.ObjectMeta, oam.AnnotationPublishVersion) {
		return false, nil, nil
	}
	if app.GetAnnotations()[oam.AnnotationPublishVersion] != appRev.GetAnnotations()[oam.AnnotationPublishVersion] {
		return false, nil, nil
	}
	return true, appRev, nil
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

	ctx := context.Background()
	appfile := newAppFile(appRev.Spec.Application.DeepCopy())
	appfile.AppRevision = appRev
	appfile.AppRevisionName = appRev.Name
	appfile.AppRevisionHash = appRev.Labels[oam.LabelAppRevisionHash]
	appfile.ExternalPolicies = make(map[string]*v1alpha1.Policy)
	for key, po := range appRev.Spec.Policies {
		appfile.ExternalPolicies[key] = po.DeepCopy()
	}
	appfile.ExternalWorkflow = appRev.Spec.Workflow

	if err := p.parseComponentsFromRevision(appfile); err != nil {
		return nil, errors.Wrap(err, "failed to parseComponentsFromRevision")
	}
	if err := p.parseWorkflowStepsFromRevision(ctx, appfile); err != nil {
		return nil, errors.Wrap(err, "failed to parseWorkflowStepsFromRevision")
	}
	if err := p.parsePoliciesFromRevision(ctx, appfile); err != nil {
		return nil, errors.Wrap(err, "failed to parsePolicies")
	}
	if err := p.parseReferredObjectsFromRevision(appfile); err != nil {
		return nil, errors.Wrap(err, "failed to parseReferredObjects")
	}

	// add compatible code for upgrading to v1.3 as the workflow steps were not recorded before v1.2
	if len(appfile.RelatedWorkflowStepDefinitions) == 0 && len(appfile.WorkflowSteps) > 0 {
		if err := p.parseWorkflowStepsForLegacyRevision(ctx, appfile); err != nil {
			return nil, errors.Wrap(err, "failed to parseWorkflowStepsForLegacyRevision")
		}
	}

	return appfile, nil
}

// parseWorkflowStepsForLegacyRevision compatible for upgrading to v1.3 as the workflow steps were not recorded before v1.2
func (p *Parser) parseWorkflowStepsForLegacyRevision(ctx context.Context, af *Appfile) error {
	for _, workflowStep := range af.WorkflowSteps {
		if step.IsBuiltinWorkflowStepType(workflowStep.Type) {
			continue
		}
		if _, found := af.RelatedWorkflowStepDefinitions[workflowStep.Type]; found {
			continue
		}
		def := &v1beta1.WorkflowStepDefinition{}
		if err := util.GetCapabilityDefinition(ctx, p.client, def, workflowStep.Type); err != nil {
			return errors.Wrapf(err, "failed to get workflow step definition %s", workflowStep.Type)
		}
		af.RelatedWorkflowStepDefinitions[workflowStep.Type] = def
	}

	af.AppRevision.Spec.WorkflowStepDefinitions = make(map[string]*v1beta1.WorkflowStepDefinition)
	for name, def := range af.RelatedWorkflowStepDefinitions {
		af.AppRevision.Spec.WorkflowStepDefinitions[name] = def
	}
	return nil
}

func (p *Parser) parseReferredObjectsFromRevision(af *Appfile) error {
	af.ReferredObjects = []*unstructured.Unstructured{}
	for _, obj := range af.AppRevision.Spec.ReferredObjects {
		un := &unstructured.Unstructured{}
		if err := json.Unmarshal(obj.Raw, un); err != nil {
			return errors.Errorf("failed to unmarshal referred objects %s", obj.Raw)
		}
		af.ReferredObjects = append(af.ReferredObjects, un)
	}
	return nil
}

func (p *Parser) parseReferredObjects(ctx context.Context, af *Appfile) error {
	ctx = auth.ContextWithUserInfo(ctx, af.app)
	for _, comp := range af.Components {
		if comp.Type != v1alpha1.RefObjectsComponentType {
			continue
		}
		spec := &v1alpha1.RefObjectsComponentSpec{}
		if err := utils.StrictUnmarshal(comp.Properties.Raw, spec); err != nil {
			return errors.Wrapf(err, "invalid properties for ref-objects in component %s", comp.Name)
		}
		for _, selector := range spec.Objects {
			objs, err := component.SelectRefObjectsForDispatch(ctx, p.client, af.app.GetNamespace(), comp.Name, selector)
			if err != nil {
				return err
			}
			af.ReferredObjects = component.AppendUnstructuredObjects(af.ReferredObjects, objs...)
		}
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.DisableReferObjectsFromURL) && len(spec.URLs) > 0 {
			return fmt.Errorf("referring objects from url is disabled")
		}
		for _, url := range spec.URLs {
			objs, err := utilscommon.HTTPGetKubernetesObjects(ctx, url)
			if err != nil {
				return fmt.Errorf("failed to load Kubernetes objects from url %s: %w", url, err)
			}
			for _, obj := range objs {
				util.AddAnnotations(obj, map[string]string{oam.AnnotationResourceURL: url})
			}
			af.ReferredObjects = component.AppendUnstructuredObjects(af.ReferredObjects, objs...)
		}
	}
	sort.Slice(af.ReferredObjects, func(i, j int) bool {
		a, b := af.ReferredObjects[i], af.ReferredObjects[j]
		keyA := a.GroupVersionKind().String() + "|" + client.ObjectKeyFromObject(a).String()
		keyB := b.GroupVersionKind().String() + "|" + client.ObjectKeyFromObject(b).String()
		return keyA < keyB
	})
	return nil
}

func (p *Parser) parsePoliciesFromRevision(ctx context.Context, af *Appfile) (err error) {
	af.Policies, err = step.LoadExternalPoliciesForWorkflow(ctx, af.PolicyClient(p.client), af.app.GetNamespace(), af.WorkflowSteps, af.app.Spec.Policies)
	if err != nil {
		return err
	}
	for _, policy := range af.Policies {
		if policy.Properties == nil && policy.Type != v1alpha1.DebugPolicyType {
			return fmt.Errorf("policy %s named %s must not have empty properties", policy.Type, policy.Name)
		}
		switch policy.Type {
		case v1alpha1.GarbageCollectPolicyType:
		case v1alpha1.ApplyOncePolicyType:
		case v1alpha1.SharedResourcePolicyType:
		case v1alpha1.TakeOverPolicyType:
		case v1alpha1.ReadOnlyPolicyType:
		case v1alpha1.ResourceUpdatePolicyType:
		case v1alpha1.EnvBindingPolicyType:
		case v1alpha1.TopologyPolicyType:
		case v1alpha1.OverridePolicyType:
		case v1alpha1.DebugPolicyType:
			af.Debug = true
		default:
			w, err := p.makeComponentFromRevision(policy.Name, policy.Type, types.TypePolicy, policy.Properties, af.AppRevision)
			if err != nil {
				return err
			}
			af.ParsedPolicies = append(af.ParsedPolicies, w)
		}
	}
	return nil
}

func (p *Parser) parsePolicies(ctx context.Context, af *Appfile) (err error) {
	af.Policies, err = step.LoadExternalPoliciesForWorkflow(ctx, af.PolicyClient(p.client), af.app.GetNamespace(), af.WorkflowSteps, af.app.Spec.Policies)
	if err != nil {
		return err
	}
	for _, policy := range af.Policies {
		if policy.Properties == nil && policy.Type != v1alpha1.DebugPolicyType {
			return fmt.Errorf("policy %s named %s must not have empty properties", policy.Type, policy.Name)
		}
		switch policy.Type {
		case v1alpha1.GarbageCollectPolicyType:
		case v1alpha1.ApplyOncePolicyType:
		case v1alpha1.SharedResourcePolicyType:
		case v1alpha1.TakeOverPolicyType:
		case v1alpha1.ReadOnlyPolicyType:
		case v1alpha1.ResourceUpdatePolicyType:
		case v1alpha1.EnvBindingPolicyType:
		case v1alpha1.TopologyPolicyType:
		case v1alpha1.ReplicationPolicyType:
		case v1alpha1.DebugPolicyType:
			af.Debug = true
		case v1alpha1.OverridePolicyType:
			compDefs, traitDefs, err := policypkg.ParseOverridePolicyRelatedDefinitions(ctx, p.client, af.app, policy)
			if err != nil {
				return err
			}
			for _, def := range compDefs {
				af.RelatedComponentDefinitions[def.Name] = def
			}
			for _, def := range traitDefs {
				af.RelatedTraitDefinitions[def.Name] = def
			}
		default:
			w, err := p.makeComponent(ctx, policy.Name, policy.Type, types.TypePolicy, policy.Properties)
			if err != nil {
				return err
			}
			af.ParsedPolicies = append(af.ParsedPolicies, w)
		}
	}
	return nil
}

func (p *Parser) loadWorkflowToAppfile(ctx context.Context, af *Appfile) error {
	var err error
	// parse workflow steps
	af.WorkflowMode = &workflowv1alpha1.WorkflowExecuteMode{
		Steps:    workflowv1alpha1.WorkflowModeDAG,
		SubSteps: workflowv1alpha1.WorkflowModeDAG,
	}
	if wfSpec := af.app.Spec.Workflow; wfSpec != nil {
		app := af.app
		mode := wfSpec.Mode
		if wfSpec.Ref != "" && mode == nil {
			wf := &workflowv1alpha1.Workflow{}
			if err := af.WorkflowClient(p.client).Get(ctx, ktypes.NamespacedName{Namespace: af.app.Namespace, Name: app.Spec.Workflow.Ref}, wf); err != nil {
				return err
			}
			mode = wf.Mode
		}
		af.WorkflowSteps = wfSpec.Steps
		af.WorkflowMode.Steps = workflowv1alpha1.WorkflowModeStep
		if mode != nil {
			if mode.Steps != "" {
				af.WorkflowMode.Steps = mode.Steps
			}
			if mode.SubSteps != "" {
				af.WorkflowMode.SubSteps = mode.SubSteps
			}
		}
	}
	af.WorkflowSteps, err = step.NewChainWorkflowStepGenerator(
		&step.RefWorkflowStepGenerator{Client: af.WorkflowClient(p.client), Context: ctx},
		&step.DeployWorkflowStepGenerator{},
		&step.Deploy2EnvWorkflowStepGenerator{},
		&step.ApplyComponentWorkflowStepGenerator{},
	).Generate(af.app, af.WorkflowSteps)
	return err
}

func (p *Parser) parseWorkflowStepsFromRevision(ctx context.Context, af *Appfile) error {
	if err := p.loadWorkflowToAppfile(ctx, af); err != nil {
		return err
	}
	// Definitions are already in AppRevision
	for k, v := range af.AppRevision.Spec.WorkflowStepDefinitions {
		af.RelatedWorkflowStepDefinitions[k] = v.DeepCopy()
	}
	return nil
}

func (p *Parser) parseWorkflowSteps(ctx context.Context, af *Appfile) error {
	if err := p.loadWorkflowToAppfile(ctx, af); err != nil {
		return err
	}
	for _, workflowStep := range af.WorkflowSteps {
		err := p.fetchAndSetWorkflowStepDefinition(ctx, af, workflowStep.Type)
		if err != nil {
			return err
		}

		if workflowStep.SubSteps != nil {
			for _, workflowSubStep := range workflowStep.SubSteps {
				err := p.fetchAndSetWorkflowStepDefinition(ctx, af, workflowSubStep.Type)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p *Parser) fetchAndSetWorkflowStepDefinition(ctx context.Context, af *Appfile, workflowStepType string) error {
	if step.IsBuiltinWorkflowStepType(workflowStepType) {
		return nil
	}
	if _, found := af.RelatedWorkflowStepDefinitions[workflowStepType]; found {
		return nil
	}
	def := &v1beta1.WorkflowStepDefinition{}
	if err := util.GetCapabilityDefinition(ctx, p.client, def, workflowStepType); err != nil {
		return errors.Wrapf(err, "failed to get workflow step definition %s", workflowStepType)
	}
	af.RelatedWorkflowStepDefinitions[workflowStepType] = def
	return nil
}

func (p *Parser) makeComponent(ctx context.Context, name, typ string, capType types.CapType, props *runtime.RawExtension) (*Component, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.client, typ, capType)
	if err != nil {
		return nil, errors.WithMessagef(err, "fetch component/policy type of %s", name)
	}
	return p.convertTemplate2Component(name, typ, props, templ)
}

func (p *Parser) makeComponentFromRevision(name, typ string, capType types.CapType, props *runtime.RawExtension, appRev *v1beta1.ApplicationRevision) (*Component, error) {
	templ, err := LoadTemplateFromRevision(typ, capType, appRev, p.client.RESTMapper())
	if err != nil {
		return nil, errors.WithMessagef(err, "fetch component/policy type of %s from revision", name)
	}

	return p.convertTemplate2Component(name, typ, props, templ)
}

func (p *Parser) convertTemplate2Component(name, typ string, props *runtime.RawExtension, templ *Template) (*Component, error) {
	settings, err := util.RawExtension2Map(props)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", name)
	}
	cpType, err := util.ConvertDefinitionRevName(typ)
	if err != nil {
		cpType = typ
	}
	return &Component{
		Traits:             []*Trait{},
		Name:               name,
		Type:               cpType,
		CapabilityCategory: templ.CapabilityCategory,
		FullTemplate:       templ,
		Params:             settings,
		engine:             definition.NewWorkloadAbstractEngine(name, p.pd),
	}, nil
}

// parseComponents resolve an Application Components and Traits to generate Component
func (p *Parser) parseComponents(ctx context.Context, af *Appfile) error {
	var comps []*Component
	for _, c := range af.app.Spec.Components {
		comp, err := p.parseComponent(ctx, c)
		if err != nil {
			return err
		}
		comps = append(comps, comp)
	}

	af.ParsedComponents = comps
	af.Components = af.app.Spec.Components
	setComponentDefinitions(af, comps)

	return nil
}

func setComponentDefinitions(af *Appfile, comps []*Component) {
	for _, comp := range comps {
		if comp == nil {
			continue
		}
		if comp.FullTemplate.ComponentDefinition != nil {
			cd := comp.FullTemplate.ComponentDefinition.DeepCopy()
			cd.Status = v1beta1.ComponentDefinitionStatus{}
			af.RelatedComponentDefinitions[comp.FullTemplate.ComponentDefinition.Name] = cd
		}
		for _, t := range comp.Traits {
			if t == nil {
				continue
			}
			if t.FullTemplate.TraitDefinition != nil {
				td := t.FullTemplate.TraitDefinition.DeepCopy()
				td.Status = v1beta1.TraitDefinitionStatus{}
				af.RelatedTraitDefinitions[t.FullTemplate.TraitDefinition.Name] = td
			}
		}
	}
}

// setComponentDefinitionsFromRevision can set related definitions directly from app revision
func setComponentDefinitionsFromRevision(af *Appfile) {
	for k, v := range af.AppRevision.Spec.ComponentDefinitions {
		af.RelatedComponentDefinitions[k] = v.DeepCopy()
	}
	for k, v := range af.AppRevision.Spec.TraitDefinitions {
		af.RelatedTraitDefinitions[k] = v.DeepCopy()
	}
}

// parseComponent resolve an ApplicationComponent and generate a Component
// containing ALL information required by an Appfile.
func (p *Parser) parseComponent(ctx context.Context, comp common.ApplicationComponent) (*Component, error) {
	workload, err := p.makeComponent(ctx, comp.Name, comp.Type, types.TypeComponentDefinition, comp.Properties)
	if err != nil {
		return nil, err
	}

	if err = p.parseTraits(ctx, workload, comp); err != nil {
		return nil, err
	}
	return workload, nil
}

func (p *Parser) parseTraits(ctx context.Context, workload *Component, comp common.ApplicationComponent) error {
	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(traitValue.Properties)
		if err != nil {
			return errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, comp.Name)
		}
		trait, err := p.parseTrait(ctx, traitValue.Type, properties)
		if err != nil {
			return errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Type)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	return nil
}

func (p *Parser) parseComponentsFromRevision(af *Appfile) error {
	var comps []*Component
	for _, c := range af.app.Spec.Components {
		comp, err := p.ParseComponentFromRevision(c, af.AppRevision)
		if err != nil {
			return err
		}
		comps = append(comps, comp)
	}
	af.ParsedComponents = comps
	af.Components = af.app.Spec.Components
	// Definitions are already in AppRevision
	setComponentDefinitionsFromRevision(af)
	return nil
}

// ParseComponentFromRevision resolve an ApplicationComponent and generate a Component
// containing ALL information required by an Appfile from app revision.
func (p *Parser) ParseComponentFromRevision(comp common.ApplicationComponent, appRev *v1beta1.ApplicationRevision) (*Component, error) {
	workload, err := p.makeComponentFromRevision(comp.Name, comp.Type, types.TypeComponentDefinition, comp.Properties, appRev)
	if err != nil {
		return nil, err
	}

	if err = p.parseTraitsFromRevision(comp, appRev, workload); err != nil {
		return nil, err
	}

	return workload, nil
}

func (p *Parser) parseTraitsFromRevision(comp common.ApplicationComponent, appRev *v1beta1.ApplicationRevision, workload *Component) error {
	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(traitValue.Properties)
		if err != nil {
			return errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, comp.Name)
		}
		trait, err := p.parseTraitFromRevision(traitValue.Type, properties, appRev)
		if err != nil {
			return errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Type)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	return nil
}

// ParseComponentFromRevisionAndClient resolve an ApplicationComponent and generate a Component
// containing ALL information required by an Appfile from app revision, and will fall back to
// load external definitions if not found
func (p *Parser) ParseComponentFromRevisionAndClient(ctx context.Context, c common.ApplicationComponent, appRev *v1beta1.ApplicationRevision) (*Component, error) {
	comp, err := p.makeComponentFromRevision(c.Name, c.Type, types.TypeComponentDefinition, c.Properties, appRev)
	if IsNotFoundInAppRevision(err) {
		comp, err = p.makeComponent(ctx, c.Name, c.Type, types.TypeComponentDefinition, c.Properties)
	}
	if err != nil {
		return nil, err
	}

	for _, traitValue := range c.Traits {
		properties, err := util.RawExtension2Map(traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, c.Name)
		}
		trait, err := p.parseTraitFromRevision(traitValue.Type, properties, appRev)
		if IsNotFoundInAppRevision(err) {
			trait, err = p.parseTrait(ctx, traitValue.Type, properties)
		}
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", c.Name, traitValue.Type)
		}

		comp.Traits = append(comp.Traits, trait)
	}

	return comp, nil
}

func (p *Parser) parseTrait(ctx context.Context, name string, properties map[string]interface{}) (*Trait, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.client, name, types.TypeTrait)
	if kerrors.IsNotFound(err) {
		return nil, errors.Errorf("trait definition of %s not found", name)
	}
	if err != nil {
		return nil, err
	}
	return p.convertTemplate2Trait(name, properties, templ)
}

func (p *Parser) parseTraitFromRevision(name string, properties map[string]interface{}, appRev *v1beta1.ApplicationRevision) (*Trait, error) {
	templ, err := LoadTemplateFromRevision(name, types.TypeTrait, appRev, p.client.RESTMapper())
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

// ValidateComponentNames validate all component names whether repeat in app
func (p *Parser) ValidateComponentNames(app *v1beta1.Application) (int, error) {
	compNames := map[string]struct{}{}
	for idx, comp := range app.Spec.Components {
		if _, found := compNames[comp.Name]; found {
			return idx, fmt.Errorf("duplicated component name %s", comp.Name)
		}
		compNames[comp.Name] = struct{}{}
	}
	return 0, nil
}
