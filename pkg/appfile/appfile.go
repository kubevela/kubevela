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
	"reflect"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	json2cue "cuelang.org/go/encoding/json"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	velaclient "github.com/kubevela/pkg/controller/client"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utilscommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

// constant error information
const (
	errInvalidValueType                          = "require %q type parameter value"
	errTerraformConfigurationIsNotSet            = "terraform configuration is not set"
	errTerraformComponentDefinition              = "terraform component definition is not valid"
	errFailToConvertTerraformComponentProperties = "failed to convert Terraform component properties"
)

const (
	// WriteConnectionSecretToRefKey is used to create a secret for cloud resource connection
	WriteConnectionSecretToRefKey = "writeConnectionSecretToRef"
	// RegionKey is the region of a Cloud Provider
	// It's used to override the region of a Cloud Provider
	// Refer to https://github.com/oam-dev/terraform-controller/blob/master/api/v1beta2/configuration_types.go#L66 for details
	RegionKey = "customRegion"
	// ProviderRefKey is the reference of a Provider
	ProviderRefKey = "providerRef"
	// ForceDeleteKey is used to force delete Configuration
	ForceDeleteKey = "forceDelete"
	// GitCredentialsSecretReferenceKey is the reference to a secret with git ssh private key & known hosts
	GitCredentialsSecretReferenceKey = "gitCredentialsSecretReference"
)

// Workload is component
type Workload struct {
	Name               string
	Type               string
	ExternalRevision   string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}
	Traits             []*Trait
	Scopes             []Scope
	ScopeDefinition    []*v1beta1.ScopeDefinition
	FullTemplate       *Template
	Ctx                process.Context
	Patch              *value.Value
	engine             definition.AbstractEngine
	SkipApplyWorkload  bool
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return wl.engine.Complete(ctx, wl.FullTemplate.TemplateStr, wl.Params)
}

// GetTemplateContext get workload template context, it will be used to eval status and health
func (wl *Workload) GetTemplateContext(ctx process.Context, client client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	// if the standard workload is managed by trait, just return empty context
	if wl.SkipApplyWorkload {
		return nil, nil
	}
	templateContext, err := wl.engine.GetTemplateContext(ctx, client, accessor)
	if templateContext != nil {
		templateContext[velaprocess.ParameterFieldName] = wl.Params
	}
	return templateContext, err
}

// EvalStatus eval workload status
func (wl *Workload) EvalStatus(templateContext map[string]interface{}) (string, error) {
	// if the standard workload is managed by trait always return empty message
	if wl.SkipApplyWorkload {
		return "", nil
	}
	return wl.engine.Status(templateContext, wl.FullTemplate.CustomStatus, wl.Params)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(templateContext map[string]interface{}) (bool, error) {
	// if health of template is not set or standard workload is managed by trait always return true
	if wl.SkipApplyWorkload {
		return true, nil
	}
	return wl.engine.HealthCheck(templateContext, wl.FullTemplate.Health, wl.Params)
}

// Scope defines the scope of workload
type Scope struct {
	Name            string
	GVK             metav1.GroupVersionKind
	ResourceVersion string
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

	// RequiredSecrets stores secret names which the trait needs from cloud resource component and its context
	RequiredSecrets []process.RequiredSecrets

	FullTemplate *Template
	engine       definition.AbstractEngine
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return trait.engine.Complete(ctx, trait.Template, trait.Params)
}

// GetTemplateContext get trait template context, it will be used to eval status and health
func (trait *Trait) GetTemplateContext(ctx process.Context, client client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	templateContext, err := trait.engine.GetTemplateContext(ctx, client, accessor)
	if templateContext != nil {
		templateContext[velaprocess.ParameterFieldName] = trait.Params
	}
	return templateContext, err
}

// EvalStatus eval trait status
func (trait *Trait) EvalStatus(templateContext map[string]interface{}) (string, error) {
	return trait.engine.Status(templateContext, trait.CustomStatusFormat, trait.Params)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(templateContext map[string]interface{}) (bool, error) {
	return trait.engine.HealthCheck(templateContext, trait.HealthCheckPolicy, trait.Params)
}

// Appfile describes application
type Appfile struct {
	Name      string
	Namespace string
	Workloads []*Workload

	AppRevision     *v1beta1.ApplicationRevision
	AppRevisionName string
	AppRevisionHash string

	AppLabels      map[string]string
	AppAnnotations map[string]string

	RelatedTraitDefinitions        map[string]*v1beta1.TraitDefinition
	RelatedComponentDefinitions    map[string]*v1beta1.ComponentDefinition
	RelatedWorkflowStepDefinitions map[string]*v1beta1.WorkflowStepDefinition
	RelatedScopeDefinitions        map[string]*v1beta1.ScopeDefinition

	Policies        []v1beta1.AppPolicy
	PolicyWorkloads []*Workload
	Components      []common.ApplicationComponent
	Artifacts       []*types.ComponentManifest
	WorkflowSteps   []workflowv1alpha1.WorkflowStep
	WorkflowMode    *workflowv1alpha1.WorkflowExecuteMode

	ExternalPolicies map[string]*v1alpha1.Policy
	ExternalWorkflow *workflowv1alpha1.Workflow
	ReferredObjects  []*unstructured.Unstructured

	parser *Parser
	app    *v1beta1.Application

	Debug bool
}

// GeneratePolicyManifests generates policy manifests from an appFile
// internal policies like apply-once, topology, will not render manifests
func (af *Appfile) GeneratePolicyManifests(ctx context.Context) ([]*unstructured.Unstructured, error) {
	var manifests []*unstructured.Unstructured
	for _, policy := range af.PolicyWorkloads {
		un, err := af.generatePolicyUnstructured(policy)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, un...)
	}
	return manifests, nil
}

func (af *Appfile) generatePolicyUnstructured(workload *Workload) ([]*unstructured.Unstructured, error) {
	ctxData := GenerateContextDataFromAppFile(af, workload.Name)
	uns, err := generatePolicyUnstructuredFromCUEModule(workload, af.Artifacts, ctxData)
	if err != nil {
		return nil, err
	}
	for _, un := range uns {
		if len(un.GetName()) == 0 {
			un.SetName(workload.Name)
		}
		if len(un.GetNamespace()) == 0 {
			un.SetNamespace(af.Namespace)
		}
	}
	return uns, nil
}

func generatePolicyUnstructuredFromCUEModule(wl *Workload, artifacts []*types.ComponentManifest, ctxData velaprocess.ContextData) ([]*unstructured.Unstructured, error) {
	pCtx := velaprocess.NewContext(ctxData)
	pCtx.PushData(velaprocess.ContextDataArtifacts, prepareArtifactsData(artifacts))
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	base, auxs := pCtx.Output()
	workload, err := base.Unstructured()
	if err != nil {
		return nil, errors.Wrapf(err, "evaluate base template policy=%s app=%s", wl.Name, ctxData.AppName)
	}
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	util.AddLabels(workload, commonLabels)

	var res = []*unstructured.Unstructured{workload}
	for _, assist := range auxs {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate auxiliary=%s template for policy=%s app=%s", assist.Name, wl.Name, ctxData.AppName)
		}
		util.AddLabels(tr, commonLabels)
		res = append(res, tr)
	}
	return res, nil
}

// artifacts contains resources in unstructured shape of all components
// it allows to access values of workloads and traits in CUE template, i.g.,
// `if context.artifacts.<compName>.ready` to determine whether it's ready to access
// `context.artifacts.<compName>.workload` to access a workload
// `context.artifacts.<compName>.traits.<traitType>.<traitResource>` to access a trait
func prepareArtifactsData(comps []*types.ComponentManifest) map[string]interface{} {
	artifacts := unstructured.Unstructured{Object: make(map[string]interface{})}
	for _, pComp := range comps {
		if pComp.StandardWorkload != nil {
			_ = unstructured.SetNestedField(artifacts.Object, pComp.StandardWorkload.Object, pComp.Name, "workload")
		}
		for _, t := range pComp.Traits {
			if t == nil {
				continue
			}
			_ = unstructured.SetNestedField(artifacts.Object, t.Object, pComp.Name,
				"traits",
				t.GetLabels()[oam.TraitTypeLabel],
				t.GetLabels()[oam.TraitResource])
		}
	}
	return artifacts.Object
}

// GenerateComponentManifests converts an appFile to a slice of ComponentManifest
func (af *Appfile) GenerateComponentManifests() ([]*types.ComponentManifest, error) {
	compManifests := make([]*types.ComponentManifest, len(af.Workloads))
	af.Artifacts = make([]*types.ComponentManifest, len(af.Workloads))
	for i, wl := range af.Workloads {
		cm, err := af.GenerateComponentManifest(wl, nil)
		if err != nil {
			return nil, err
		}
		err = af.SetOAMContract(cm)
		if err != nil {
			return nil, err
		}
		compManifests[i] = cm
		af.Artifacts[i] = cm
	}
	return compManifests, nil
}

// GenerateComponentManifest generate only one ComponentManifest
func (af *Appfile) GenerateComponentManifest(wl *Workload, mutate func(*velaprocess.ContextData)) (*types.ComponentManifest, error) {
	if af.Namespace == "" {
		af.Namespace = corev1.NamespaceDefault
	}
	ctxData := GenerateContextDataFromAppFile(af, wl.Name)
	if mutate != nil {
		mutate(&ctxData)
	}
	// generate context here to avoid nil pointer panic
	wl.Ctx = NewBasicContext(ctxData, wl.Params)
	switch wl.CapabilityCategory {
	case types.HelmCategory:
		return generateComponentFromHelmModule(wl, ctxData)
	case types.KubeCategory:
		return generateComponentFromKubeModule(wl, ctxData)
	case types.TerraformCategory:
		return generateComponentFromTerraformModule(wl, af.Name, af.Namespace)
	default:
		return generateComponentFromCUEModule(wl, ctxData)
	}
}

// SetOAMContract will set OAM labels and annotations for resources as contract
func (af *Appfile) SetOAMContract(comp *types.ComponentManifest) error {

	compName := comp.Name
	commonLabels := af.generateAndFilterCommonLabels(compName)
	af.assembleWorkload(comp.StandardWorkload, compName, commonLabels)

	workloadRef := corev1.ObjectReference{
		APIVersion: comp.StandardWorkload.GetAPIVersion(),
		Kind:       comp.StandardWorkload.GetKind(),
		Name:       comp.StandardWorkload.GetName(),
	}
	for _, trait := range comp.Traits {
		af.assembleTrait(trait, compName, commonLabels)
		if err := af.setWorkloadRefToTrait(workloadRef, trait); err != nil && !IsNotFoundInAppFile(err) {
			return errors.WithMessagef(err, "cannot set workload reference to trait %q", trait.GetName())
		}
	}
	return nil
}

// workload and trait in the same component both have these labels, except componentRevision which should be evaluated with input/output
func (af *Appfile) generateAndFilterCommonLabels(compName string) map[string]string {
	filter := func(labels map[string]string, notAllowedKey []string) {
		for _, l := range notAllowedKey {
			delete(labels, strings.TrimSpace(l))
		}
	}
	Labels := map[string]string{
		oam.LabelAppName:      af.Name,
		oam.LabelAppNamespace: af.Namespace,
		oam.LabelAppRevision:  af.AppRevisionName,
		oam.LabelAppComponent: compName,
	}
	// merge application's all labels
	finalLabels := util.MergeMapOverrideWithDst(af.AppLabels, Labels)
	filterLabels, ok := af.AppAnnotations[oam.AnnotationFilterLabelKeys]
	if ok {
		filter(finalLabels, strings.Split(filterLabels, ","))
	}
	return finalLabels
}

// workload and trait both have these annotations
func (af *Appfile) filterAndSetAnnotations(obj *unstructured.Unstructured) {
	var allFilterAnnotation []string
	allFilterAnnotation = append(allFilterAnnotation, types.DefaultFilterAnnots...)

	passedFilterAnnotation, ok := af.AppAnnotations[oam.AnnotationFilterAnnotationKeys]
	if ok {
		allFilterAnnotation = append(allFilterAnnotation, strings.Split(passedFilterAnnotation, ",")...)
	}

	// pass application's all annotations
	util.AddAnnotations(obj, af.AppAnnotations)
	// remove useless annotations for workload/trait
	util.RemoveAnnotations(obj, allFilterAnnotation)
}

func (af *Appfile) setNamespace(obj *unstructured.Unstructured) {

	// we should not set namespace for namespace resources
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk == corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Namespace{}).Name()) {
		return
	}

	// only set app's namespace when namespace is unspecified
	// it's by design to set arbitrary namespace in render phase
	if len(obj.GetNamespace()) == 0 {
		obj.SetNamespace(af.Namespace)
	}
}

func (af *Appfile) assembleWorkload(wl *unstructured.Unstructured, compName string, labels map[string]string) {
	// use component name as workload name if workload name is not specified
	// don't override the name set in render phase if exist
	if len(wl.GetName()) == 0 {
		wl.SetName(compName)
	}
	af.setNamespace(wl)
	af.setWorkloadLabels(wl, labels)
	af.filterAndSetAnnotations(wl)
}

/*
	NOTE a workload has these possible labels
	  app.oam.dev/app-revision-hash: ce053923e2fb403f
	  app.oam.dev/appRevision: myapp-v2
	  app.oam.dev/component: mycomp
	  app.oam.dev/name: myapp
	  app.oam.dev/resourceType: WORKLOAD
	  app.oam.dev/revision: mycomp-v2
	  workload.oam.dev/type: kube-worker

// Component Revision name was not added here (app.oam.dev/revision: mycomp-v2)
*/
func (af *Appfile) setWorkloadLabels(wl *unstructured.Unstructured, commonLabels map[string]string) {
	// add more workload-specific labels here
	util.AddLabels(wl, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeWorkload})
	util.AddLabels(wl, commonLabels)
}

func (af *Appfile) assembleTrait(trait *unstructured.Unstructured, compName string, labels map[string]string) {
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	// only set generated name when name is unspecified
	// it's by design to set arbitrary name in render phase
	if len(trait.GetName()) == 0 {
		cpTrait := trait.DeepCopy()
		// remove labels that should not be calculated into hash
		util.RemoveLabels(cpTrait, []string{oam.LabelAppRevision})
		traitName := util.GenTraitNameCompatible(compName, cpTrait, traitType)
		trait.SetName(traitName)
	}
	af.setTraitLabels(trait, labels)
	af.filterAndSetAnnotations(trait)
	af.setNamespace(trait)
}

/*
	NOTE a trait has these possible labels
	  app.oam.dev/app-revision-hash: ce053923e2fb403f
	  app.oam.dev/appRevision: myapp-v2
	  app.oam.dev/component: mycomp
	  app.oam.dev/name: myapp
	  app.oam.dev/resourceType: TRAIT
	  trait.oam.dev/resource: service
	  trait.oam.dev/type: ingress // already added in render phase

// Component Revision name was not added here (app.oam.dev/revision: mycomp-v2)
*/
func (af *Appfile) setTraitLabels(trait *unstructured.Unstructured, commonLabels map[string]string) {
	// add more trait-specific labels here
	util.AddLabels(trait, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeTrait})
	util.AddLabels(trait, commonLabels)
}

func (af *Appfile) setWorkloadRefToTrait(wlRef corev1.ObjectReference, trait *unstructured.Unstructured) error {
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	if traitType == definition.AuxiliaryWorkload {
		return nil
	}
	if strings.Contains(traitType, "-") {
		splitName := traitType[0:strings.LastIndex(traitType, "-")]
		_, ok := af.RelatedTraitDefinitions[splitName]
		if ok {
			traitType = splitName
		}
	}
	traitDef, ok := af.RelatedTraitDefinitions[traitType]
	if !ok {
		return errors.Errorf("TraitDefinition %s not found in appfile", traitType)
	}
	workloadRefPath := traitDef.Spec.WorkloadRefPath
	// only add workload reference to the trait if it asks for it
	if len(workloadRefPath) != 0 {
		tmpWLRef := corev1.ObjectReference{
			APIVersion: wlRef.APIVersion,
			Kind:       wlRef.Kind,
			Name:       wlRef.Name,
		}
		if err := fieldpath.Pave(trait.UnstructuredContent()).SetValue(workloadRefPath, tmpWLRef); err != nil {
			return err
		}
	}
	return nil
}

// IsNotFoundInAppFile check if the target error is `not found in appfile`
func IsNotFoundInAppFile(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found in appfile")
}

// PrepareProcessContext prepares a DSL process Context
func PrepareProcessContext(wl *Workload, ctxData velaprocess.ContextData) (process.Context, error) {
	if wl.Ctx == nil {
		wl.Ctx = NewBasicContext(ctxData, wl.Params)
	}
	if err := wl.EvalContext(wl.Ctx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	return wl.Ctx, nil
}

// NewBasicContext prepares a basic DSL process Context
func NewBasicContext(contextData velaprocess.ContextData, params map[string]interface{}) process.Context {
	pCtx := velaprocess.NewContext(contextData)
	if params != nil {
		pCtx.SetParameters(params)
	}
	return pCtx
}

func generateComponentFromCUEModule(wl *Workload, ctxData velaprocess.ContextData) (*types.ComponentManifest, error) {
	pCtx, err := PrepareProcessContext(wl, ctxData)
	if err != nil {
		return nil, err
	}
	return baseGenerateComponent(pCtx, wl, ctxData.AppName, ctxData.Namespace)
}

func generateComponentFromTerraformModule(wl *Workload, appName, ns string) (*types.ComponentManifest, error) {
	return baseGenerateComponent(wl.Ctx, wl, appName, ns)
}

func baseGenerateComponent(pCtx process.Context, wl *Workload, appName, ns string) (*types.ComponentManifest, error) {
	var err error
	pCtx.PushData(velaprocess.ContextComponentType, wl.Type)
	for _, tr := range wl.Traits {
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, wl.Name)
		}
	}
	if patcher := wl.Patch; patcher != nil {
		workload, auxiliaries := pCtx.Output()
		if p, err := patcher.LookupValue("workload"); err == nil {
			if err := workload.Unify(p.CueValue()); err != nil {
				return nil, errors.WithMessage(err, "patch workload")
			}
		}
		for _, aux := range auxiliaries {
			if p, err := patcher.LookupByScript(fmt.Sprintf("traits[\"%s\"]", aux.Name)); err == nil && p.CueValue().Err() == nil {
				if err := aux.Ins.Unify(p.CueValue()); err != nil {
					return nil, errors.WithMessagef(err, "patch outputs.%s", aux.Name)
				}
			}
		}
	}
	compManifest, err := evalWorkloadWithContext(pCtx, wl, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.Name = wl.Name
	compManifest.Namespace = ns
	// we record the external revision name in ExternalRevision field
	compManifest.ExternalRevision = wl.ExternalRevision

	compManifest.Scopes = make([]*corev1.ObjectReference, len(wl.Scopes))
	for i, s := range wl.Scopes {
		compManifest.Scopes[i] = &corev1.ObjectReference{
			APIVersion: metav1.GroupVersion{
				Group:   s.GVK.Group,
				Version: s.GVK.Version,
			}.String(),
			Kind: s.GVK.Kind,
			Name: s.Name,
		}
	}
	return compManifest, nil
}

// makeWorkloadWithContext evaluate the workload's template to unstructured resource.
func makeWorkloadWithContext(pCtx process.Context, wl *Workload, ns, appName string) (*unstructured.Unstructured, error) {
	var (
		workload *unstructured.Unstructured
		err      error
	)
	base, _ := pCtx.Output()
	switch wl.CapabilityCategory {
	case types.TerraformCategory:
		workload, err = generateTerraformConfigurationWorkload(wl, ns)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate Terraform Configuration workload for workload %s", wl.Name)
		}
	default:
		workload, err = base.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate base template component=%s app=%s", wl.Name, appName)
		}
	}
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	util.AddLabels(workload, util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.WorkloadTypeLabel: wl.Type}))
	return workload, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component manifest
func evalWorkloadWithContext(pCtx process.Context, wl *Workload, ns, appName string) (*types.ComponentManifest, error) {
	compManifest := &types.ComponentManifest{}
	workload, err := makeWorkloadWithContext(pCtx, wl, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.StandardWorkload = workload

	_, assists := pCtx.Output()
	compManifest.Traits = make([]*unstructured.Unstructured, len(assists))
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	for i, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, wl.Name, appName)
		}
		labels := util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.TraitTypeLabel: assist.Type})
		if assist.Name != "" {
			labels[oam.TraitResource] = assist.Name
		}
		util.AddLabels(tr, labels)
		compManifest.Traits[i] = tr
	}
	return compManifest, nil
}

// GenerateCUETemplate generate CUE Template from Kube module and Helm module
func GenerateCUETemplate(wl *Workload) (string, error) {
	var templateStr string
	switch wl.CapabilityCategory {
	case types.KubeCategory:
		kubeObj := &unstructured.Unstructured{}

		err := json.Unmarshal(wl.FullTemplate.Kube.Template.Raw, kubeObj)
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot decode Kube template into K8s object")
		}

		paramValues, err := resolveKubeParameters(wl.FullTemplate.Kube.Parameters, wl.Params)
		if err != nil {
			return templateStr, errors.WithMessage(err, "cannot resolve parameter settings")
		}
		if err := setParameterValuesToKubeObj(kubeObj, paramValues); err != nil {
			return templateStr, errors.WithMessage(err, "cannot set parameters value")
		}

		// convert structured kube obj into CUE (go ==marshal==> json ==decoder==> cue)
		objRaw, err := kubeObj.MarshalJSON()
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot marshal kube object")
		}
		cuectx := cuecontext.New()
		expr, err := json2cue.Extract("", objRaw)
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot extract object into CUE")
		}
		v := cuectx.BuildExpr(expr)
		cueRaw, err := format.Node(v.Syntax())
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot format CUE")
		}

		// NOTE a hack way to enable using CUE capabilities on KUBE schematic workload
		templateStr = fmt.Sprintf(`
output: %s`, string(cueRaw))
	case types.HelmCategory:
		gv, err := schema.ParseGroupVersion(wl.FullTemplate.Reference.Definition.APIVersion)
		if err != nil {
			return templateStr, err
		}
		targetWorkloadGVK := gv.WithKind(wl.FullTemplate.Reference.Definition.Kind)
		// NOTE this is a hack way to enable using CUE module capabilities on Helm module workload
		// construct an empty base workload according to its GVK
		templateStr = fmt.Sprintf(`
output: {
	apiVersion: "%s"
	kind: "%s"
}`, targetWorkloadGVK.GroupVersion().String(), targetWorkloadGVK.Kind)
	default:
	}
	return templateStr, nil
}

func generateComponentFromKubeModule(wl *Workload, ctxData velaprocess.ContextData) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest, err := generateComponentFromCUEModule(wl, ctxData)
	if err != nil {
		return nil, err
	}
	return compManifest, nil
}

func generateTerraformConfigurationWorkload(wl *Workload, ns string) (*unstructured.Unstructured, error) {
	if wl.FullTemplate == nil || wl.FullTemplate.Terraform == nil || wl.FullTemplate.Terraform.Configuration == "" {
		return nil, errors.New(errTerraformConfigurationIsNotSet)
	}
	params, err := json.Marshal(wl.Params)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	if wl.FullTemplate.ComponentDefinition == nil || wl.FullTemplate.ComponentDefinition.Spec.Schematic == nil ||
		wl.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform == nil {
		return nil, errors.New(errTerraformComponentDefinition)
	}

	configuration := terraformapi.Configuration{
		TypeMeta: metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta2", Kind: "Configuration"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        wl.Name,
			Namespace:   ns,
			Annotations: wl.FullTemplate.ComponentDefinition.Annotations,
		},
	}
	// 1. parse the spec of configuration
	var spec terraformapi.ConfigurationSpec
	if err := json.Unmarshal(params, &spec); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	configuration.Spec = spec

	if configuration.Spec.WriteConnectionSecretToReference == nil {
		configuration.Spec.WriteConnectionSecretToReference = wl.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.WriteConnectionSecretToReference
	}
	if configuration.Spec.WriteConnectionSecretToReference != nil && configuration.Spec.WriteConnectionSecretToReference.Namespace == "" {
		configuration.Spec.WriteConnectionSecretToReference.Namespace = ns
	}

	if configuration.Spec.ProviderReference == nil {
		configuration.Spec.ProviderReference = wl.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.ProviderReference
	}

	if configuration.Spec.GitCredentialsSecretReference == nil {
		configuration.Spec.GitCredentialsSecretReference = wl.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.GitCredentialsSecretReference
	}

	switch wl.FullTemplate.Terraform.Type {
	case "hcl":
		configuration.Spec.HCL = wl.FullTemplate.Terraform.Configuration
	case "remote":
		configuration.Spec.Remote = wl.FullTemplate.Terraform.Configuration
		configuration.Spec.Path = wl.FullTemplate.Terraform.Path
	}

	// 2. parse variable
	variableRaw := &runtime.RawExtension{}
	if err := json.Unmarshal(params, &variableRaw); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	variableMap, err := util.RawExtension2Map(variableRaw)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	delete(variableMap, WriteConnectionSecretToRefKey)
	delete(variableMap, RegionKey)
	delete(variableMap, ProviderRefKey)
	delete(variableMap, ForceDeleteKey)
	delete(variableMap, GitCredentialsSecretReferenceKey)

	data, err := json.Marshal(variableMap)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	configuration.Spec.Variable = &runtime.RawExtension{Raw: data}
	raw := util.Object2RawExtension(&configuration)
	return util.RawExtension2Unstructured(raw)
}

// a helper map whose key is parameter name
type paramValueSettings map[string]paramValueSetting
type paramValueSetting struct {
	Value      interface{}
	ValueType  common.ParameterValueType
	FieldPaths []string
}

func resolveKubeParameters(params []common.KubeParameter, settings map[string]interface{}) (paramValueSettings, error) {
	supported := map[string]*common.KubeParameter{}
	for _, p := range params {
		supported[p.Name] = p.DeepCopy()
	}

	values := make(paramValueSettings)
	for name, v := range settings {
		// check unsupported parameter setting
		if supported[name] == nil {
			return nil, errors.Errorf("unsupported parameter %q", name)
		}
		// construct helper map
		values[name] = paramValueSetting{
			Value:      v,
			ValueType:  supported[name].ValueType,
			FieldPaths: supported[name].FieldPaths,
		}
	}

	// check required parameter
	for _, p := range params {
		if p.Required != nil && *p.Required {
			if _, ok := values[p.Name]; !ok {
				return nil, errors.Errorf("require parameter %q", p.Name)
			}
		}
	}
	return values, nil
}

func setParameterValuesToKubeObj(obj *unstructured.Unstructured, values paramValueSettings) error {
	paved := fieldpath.Pave(obj.Object)
	for paramName, v := range values {
		for _, f := range v.FieldPaths {
			switch v.ValueType {
			case common.StringType:
				vString, ok := v.Value.(string)
				if !ok {
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
				if err := paved.SetString(f, vString); err != nil {
					return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
				}
			case common.NumberType:
				switch v.Value.(type) {
				case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
					if err := paved.SetValue(f, v.Value); err != nil {
						return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
					}
				default:
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
			case common.BooleanType:
				vBoolean, ok := v.Value.(bool)
				if !ok {
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
				if err := paved.SetValue(f, vBoolean); err != nil {
					return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
				}
			}
		}
	}
	return nil
}

func generateComponentFromHelmModule(wl *Workload, ctxData velaprocess.ContextData) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest := &types.ComponentManifest{
		Name:             wl.Name,
		Namespace:        ctxData.Namespace,
		ExternalRevision: wl.ExternalRevision,
		StandardWorkload: &unstructured.Unstructured{},
	}

	if wl.FullTemplate.Reference.Type != types.AutoDetectWorkloadDefinition {
		compManifest, err = generateComponentFromCUEModule(wl, ctxData)
		if err != nil {
			return nil, err
		}
	}

	rls, repo, err := helm.RenderHelmReleaseAndHelmRepo(wl.FullTemplate.Helm, wl.Name, ctxData.AppName, ctxData.Namespace, wl.Params)
	if err != nil {
		return nil, err
	}
	compManifest.PackagedWorkloadResources = []*unstructured.Unstructured{rls, repo}
	return compManifest, nil
}

// GenerateContextDataFromAppFile generates process context data from app file
func GenerateContextDataFromAppFile(appfile *Appfile, wlName string) velaprocess.ContextData {
	data := velaprocess.ContextData{
		Namespace:       appfile.Namespace,
		AppName:         appfile.Name,
		CompName:        wlName,
		AppRevisionName: appfile.AppRevisionName,
		Components:      appfile.Components,
	}
	if appfile.AppAnnotations != nil {
		data.WorkflowName = appfile.AppAnnotations[oam.AnnotationWorkflowName]
		data.PublishVersion = appfile.AppAnnotations[oam.AnnotationPublishVersion]
		data.AppAnnotations = appfile.AppAnnotations
	}
	if appfile.AppLabels != nil {
		data.AppLabels = appfile.AppLabels
	}
	return data
}

// WorkflowClient cache retrieved workflow if ApplicationRevision not exists in appfile
// else use the workflow in ApplicationRevision
func (af *Appfile) WorkflowClient(cli client.Client) client.Client {
	return velaclient.DelegatingHandlerClient{
		Client: cli,
		Getter: func(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			if wf, ok := obj.(*workflowv1alpha1.Workflow); ok {
				if af.AppRevision != nil {
					if af.ExternalWorkflow != nil && af.ExternalWorkflow.Name == key.Name && af.ExternalWorkflow.Namespace == key.Namespace {
						af.ExternalWorkflow.DeepCopyInto(wf)
						return nil
					}
					return kerrors.NewNotFound(v1alpha1.SchemeGroupVersion.WithResource("workflow").GroupResource(), key.Name)
				}
				if err := cli.Get(ctx, key, obj); err != nil {
					return err
				}
				af.ExternalWorkflow = obj.(*workflowv1alpha1.Workflow)
				return nil
			}
			return cli.Get(ctx, key, obj)
		},
	}
}

// PolicyClient cache retrieved policy if ApplicationRevision not exists in appfile
// else use the policy in ApplicationRevision
func (af *Appfile) PolicyClient(cli client.Client) client.Client {
	return velaclient.DelegatingHandlerClient{
		Client: cli,
		Getter: func(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			if po, ok := obj.(*v1alpha1.Policy); ok {
				if af.AppRevision != nil {
					if p, found := af.ExternalPolicies[key.String()]; found {
						p.DeepCopyInto(po)
						return nil
					}
					return kerrors.NewNotFound(v1alpha1.SchemeGroupVersion.WithResource("policy").GroupResource(), key.Name)
				}
				if err := cli.Get(ctx, key, obj); err != nil {
					return err
				}
				af.ExternalPolicies[key.String()] = obj.(*v1alpha1.Policy)
				return nil
			}
			return cli.Get(ctx, key, obj)
		},
	}
}

// LoadDynamicComponent for ref-objects typed components, this function will load referred objects from stored revisions
func (af *Appfile) LoadDynamicComponent(ctx context.Context, cli client.Client, comp *common.ApplicationComponent) (*common.ApplicationComponent, error) {
	if comp.Type != v1alpha1.RefObjectsComponentType {
		return comp, nil
	}
	_comp := comp.DeepCopy()
	spec := &v1alpha1.RefObjectsComponentSpec{}
	if err := json.Unmarshal(comp.Properties.Raw, spec); err != nil {
		return nil, errors.Wrapf(err, "invalid ref-objects component properties")
	}
	var uns []*unstructured.Unstructured
	ctx = auth.ContextWithUserInfo(ctx, af.app)
	for _, selector := range spec.Objects {
		objs, err := component.SelectRefObjectsForDispatch(ctx, component.ReferredObjectsDelegatingClient(cli, af.ReferredObjects), af.Namespace, comp.Name, selector)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to select objects from referred objects in revision storage")
		}
		uns = component.AppendUnstructuredObjects(uns, objs...)
	}
	// nolint
	for _, url := range spec.URLs {
		objs := utilscommon.FilterObjectsByCondition(af.ReferredObjects, func(obj *unstructured.Unstructured) bool {
			return obj.GetAnnotations() != nil && obj.GetAnnotations()[oam.AnnotationResourceURL] == url
		})
		uns = component.AppendUnstructuredObjects(uns, objs...)
	}
	refObjs, err := component.ConvertUnstructuredsToReferredObjects(uns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal referred object")
	}
	bs, err := json.Marshal(&common.ReferredObjectList{Objects: refObjs})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal loaded ref-objects")
	}
	_comp.Properties = &runtime.RawExtension{Raw: bs}
	return _comp, nil
}
