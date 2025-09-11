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

	"github.com/oam-dev/kubevela/pkg/cue/definition/health"

	"cuelang.org/go/cue"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/kubevela/pkg/util/slices"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	velaclient "github.com/kubevela/pkg/controller/client"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
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

// Component is an internal struct for component in application
// User-defined policies are parsed as a Component without any Traits because their purpose is dispatching some resources
// Internal policies are NOT parsed as a Component
type Component struct {
	Name               string
	Type               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}
	Traits             []*Trait
	FullTemplate       *Template
	Ctx                process.Context
	Patch              *cue.Value
	engine             definition.AbstractEngine
	SkipApplyWorkload  bool
}

// EvalContext eval workload template and set the result to context
func (comp *Component) EvalContext(ctx process.Context) error {
	return comp.engine.Complete(ctx, comp.FullTemplate.TemplateStr, comp.Params)
}

// GetTemplateContext get workload template context, it will be used to eval status and health
func (comp *Component) GetTemplateContext(ctx process.Context, client client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	// if the standard workload is managed by trait, just return empty context
	if comp.SkipApplyWorkload {
		return nil, nil
	}
	templateContext, err := comp.engine.GetTemplateContext(ctx, client, accessor)
	if templateContext != nil {
		templateContext[velaprocess.ParameterFieldName] = comp.Params
	}
	return templateContext, err
}

// EvalStatus eval workload status
func (comp *Component) EvalStatus(templateContext map[string]interface{}) (*health.StatusResult, error) {
	// if the standard workload is managed by trait always return empty message
	if comp.SkipApplyWorkload {
		return nil, nil
	}
	return comp.engine.Status(templateContext, comp.FullTemplate.AsStatusRequest(comp.Params))
}

// Trait is ComponentTrait
type Trait struct {
	// The Name is name of TraitDefinition, actually it's a type of the trait instance
	Name               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}

	Template           string
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

// EvalStatus eval trait status (including health)
func (trait *Trait) EvalStatus(templateContext map[string]interface{}) (*health.StatusResult, error) {
	return trait.engine.Status(templateContext, trait.FullTemplate.AsStatusRequest(trait.Params))
}

// Appfile describes application
type Appfile struct {
	Name             string
	Namespace        string
	ParsedComponents []*Component
	ParsedPolicies   []*Component

	AppRevision     *v1beta1.ApplicationRevision
	AppRevisionName string
	AppRevisionHash string

	AppLabels      map[string]string
	AppAnnotations map[string]string

	RelatedTraitDefinitions        map[string]*v1beta1.TraitDefinition
	RelatedComponentDefinitions    map[string]*v1beta1.ComponentDefinition
	RelatedWorkflowStepDefinitions map[string]*v1beta1.WorkflowStepDefinition

	Policies      []v1beta1.AppPolicy
	Components    []common.ApplicationComponent
	Artifacts     []*types.ComponentManifest
	WorkflowSteps []workflowv1alpha1.WorkflowStep
	WorkflowMode  *workflowv1alpha1.WorkflowExecuteMode

	ExternalPolicies map[string]*v1alpha1.Policy
	ExternalWorkflow *workflowv1alpha1.Workflow
	ReferredObjects  []*unstructured.Unstructured

	app *v1beta1.Application

	Debug bool
}

// GeneratePolicyManifests generates policy manifests from an appFile
// internal policies like apply-once, topology, will not render manifests
func (af *Appfile) GeneratePolicyManifests(_ context.Context) ([]*unstructured.Unstructured, error) {
	var manifests []*unstructured.Unstructured
	for _, policy := range af.ParsedPolicies {
		un, err := af.generatePolicyUnstructured(policy)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, un...)
	}
	return manifests, nil
}

func (af *Appfile) generatePolicyUnstructured(workload *Component) ([]*unstructured.Unstructured, error) {
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

func generatePolicyUnstructuredFromCUEModule(comp *Component, artifacts []*types.ComponentManifest, ctxData velaprocess.ContextData) ([]*unstructured.Unstructured, error) {
	pCtx := velaprocess.NewContext(ctxData)
	pCtx.PushData(velaprocess.ContextDataArtifacts, prepareArtifactsData(artifacts))
	if err := comp.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	base, auxs := pCtx.Output()
	workload, err := base.Unstructured()
	if err != nil {
		return nil, errors.Wrapf(err, "evaluate base template policy=%s app=%s", comp.Name, ctxData.AppName)
	}
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	util.AddLabels(workload, commonLabels)

	var res = []*unstructured.Unstructured{workload}
	for _, assist := range auxs {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate auxiliary=%s template for policy=%s app=%s", assist.Name, comp.Name, ctxData.AppName)
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
		if pComp.ComponentOutput != nil {
			_ = unstructured.SetNestedField(artifacts.Object, pComp.ComponentOutput.Object, pComp.Name, "workload")
		}
		for _, t := range pComp.ComponentOutputsAndTraits {
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
	compManifests := make([]*types.ComponentManifest, len(af.ParsedComponents))
	af.Artifacts = make([]*types.ComponentManifest, len(af.ParsedComponents))
	for i, comp := range af.ParsedComponents {
		cm, err := af.GenerateComponentManifest(comp, nil)
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
func (af *Appfile) GenerateComponentManifest(comp *Component, mutate func(*velaprocess.ContextData)) (*types.ComponentManifest, error) {
	if af.Namespace == "" {
		af.Namespace = corev1.NamespaceDefault
	}
	ctxData := GenerateContextDataFromAppFile(af, comp.Name)
	if mutate != nil {
		mutate(&ctxData)
	}
	// generate context here to avoid nil pointer panic
	comp.Ctx = NewBasicContext(ctxData, comp.Params)
	switch comp.CapabilityCategory {
	case types.TerraformCategory:
		return generateComponentFromTerraformModule(comp, af.Name, af.Namespace)
	default:
		return generateComponentFromCUEModule(comp, ctxData)
	}
}

// SetOAMContract will set OAM labels and annotations for resources as contract
func (af *Appfile) SetOAMContract(comp *types.ComponentManifest) error {

	compName := comp.Name
	commonLabels := af.generateAndFilterCommonLabels(compName)
	af.assembleWorkload(comp.ComponentOutput, compName, commonLabels)

	workloadRef := corev1.ObjectReference{
		APIVersion: comp.ComponentOutput.GetAPIVersion(),
		Kind:       comp.ComponentOutput.GetKind(),
		Name:       comp.ComponentOutput.GetName(),
	}
	for _, trait := range comp.ComponentOutputsAndTraits {
		af.assembleTrait(trait, comp.Name, commonLabels)
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

func (af *Appfile) assembleWorkload(comp *unstructured.Unstructured, compName string, labels map[string]string) {
	// use component name as workload name if workload name is not specified
	// don't override the name set in render phase if exist
	if len(comp.GetName()) == 0 {
		comp.SetName(compName)
	}
	af.setNamespace(comp)
	af.setWorkloadLabels(comp, labels)
	af.filterAndSetAnnotations(comp)
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
func (af *Appfile) setWorkloadLabels(comp *unstructured.Unstructured, commonLabels map[string]string) {
	// add more workload-specific labels here
	util.AddLabels(comp, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeWorkload})
	util.AddLabels(comp, commonLabels)
}

func (af *Appfile) assembleTrait(trait *unstructured.Unstructured, compName string, labels map[string]string) {
	if len(trait.GetName()) == 0 {
		traitType := trait.GetLabels()[oam.TraitTypeLabel]
		cpTrait := trait.DeepCopy()
		// remove labels that should not be calculated into hash
		util.RemoveLabels(cpTrait, []string{oam.LabelAppRevision})
		traitName := util.GenTraitName(compName, cpTrait, traitType)
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
func PrepareProcessContext(comp *Component, ctxData velaprocess.ContextData) (process.Context, error) {
	if comp.Ctx == nil {
		comp.Ctx = NewBasicContext(ctxData, comp.Params)
	}
	if err := comp.EvalContext(comp.Ctx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	return comp.Ctx, nil
}

// NewBasicContext prepares a basic DSL process Context
func NewBasicContext(contextData velaprocess.ContextData, params map[string]interface{}) process.Context {
	pCtx := velaprocess.NewContext(contextData)
	if params != nil {
		pCtx.SetParameters(params)
	}
	return pCtx
}

func generateComponentFromCUEModule(comp *Component, ctxData velaprocess.ContextData) (*types.ComponentManifest, error) {
	pCtx, err := PrepareProcessContext(comp, ctxData)
	if err != nil {
		return nil, err
	}
	return baseGenerateComponent(pCtx, comp, ctxData.AppName, ctxData.Namespace)
}

func generateComponentFromTerraformModule(comp *Component, appName, ns string) (*types.ComponentManifest, error) {
	return baseGenerateComponent(comp.Ctx, comp, appName, ns)
}

func baseGenerateComponent(pCtx process.Context, comp *Component, appName, ns string) (*types.ComponentManifest, error) {
	var err error
	pCtx.PushData(velaprocess.ContextComponentType, comp.Type)
	for _, tr := range comp.Traits {
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, comp.Name)
		}
	}
	if patcher := comp.Patch; patcher != nil {
		workload, auxiliaries := pCtx.Output()
		if p := patcher.LookupPath(cue.ParsePath("workload")); p.Exists() {
			if err := workload.Unify(p); err != nil {
				return nil, errors.WithMessage(err, "patch workload")
			}
		}
		for _, aux := range auxiliaries {
			if p, err := value.LookupValueByScript(*patcher, fmt.Sprintf("traits[\"%s\"]", aux.Name)); err == nil && p.Err() == nil {
				if err := aux.Ins.Unify(p); err != nil {
					return nil, errors.WithMessagef(err, "patch outputs.%s", aux.Name)
				}
			}
		}
	}
	compManifest, err := evalWorkloadWithContext(pCtx, comp, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.Name = comp.Name
	compManifest.Namespace = ns
	return compManifest, nil
}

// makeWorkloadWithContext evaluate the workload's template to unstructured resource.
func makeWorkloadWithContext(pCtx process.Context, comp *Component, ns, appName string) (*unstructured.Unstructured, error) {
	var (
		workload *unstructured.Unstructured
		err      error
	)
	base, _ := pCtx.Output()
	switch comp.CapabilityCategory {
	case types.TerraformCategory:
		workload, err = generateTerraformConfigurationWorkload(comp, ns)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate Terraform Configuration workload for workload %s", comp.Name)
		}
	default:
		workload, err = base.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate base template component=%s app=%s", comp.Name, appName)
		}
	}
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	util.AddLabels(workload, util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.WorkloadTypeLabel: comp.Type}))
	return workload, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component manifest
func evalWorkloadWithContext(pCtx process.Context, comp *Component, ns, appName string) (*types.ComponentManifest, error) {
	compManifest := &types.ComponentManifest{}
	workload, err := makeWorkloadWithContext(pCtx, comp, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.ComponentOutput = workload

	_, assists := pCtx.Output()
	compManifest.ComponentOutputsAndTraits = make([]*unstructured.Unstructured, len(assists))
	commonLabels := definition.GetCommonLabels(definition.GetBaseContextLabels(pCtx))
	for i, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, comp.Name, appName)
		}
		labels := util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.TraitTypeLabel: assist.Type})
		if assist.Name != "" {
			labels[oam.TraitResource] = assist.Name
		}
		util.AddLabels(tr, labels)
		compManifest.ComponentOutputsAndTraits[i] = tr
	}
	return compManifest, nil
}

func generateTerraformConfigurationWorkload(comp *Component, ns string) (*unstructured.Unstructured, error) {
	if comp.FullTemplate == nil || comp.FullTemplate.Terraform == nil || comp.FullTemplate.Terraform.Configuration == "" {
		return nil, errors.New(errTerraformConfigurationIsNotSet)
	}
	params, err := json.Marshal(comp.Params)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	if comp.FullTemplate.ComponentDefinition == nil || comp.FullTemplate.ComponentDefinition.Spec.Schematic == nil ||
		comp.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform == nil {
		return nil, errors.New(errTerraformComponentDefinition)
	}

	configuration := terraformapi.Configuration{
		TypeMeta: metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta2", Kind: "Configuration"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        comp.Name,
			Namespace:   ns,
			Annotations: comp.FullTemplate.ComponentDefinition.Annotations,
		},
	}
	// 1. parse the spec of configuration
	var spec terraformapi.ConfigurationSpec
	if err := json.Unmarshal(params, &spec); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	configuration.Spec = spec

	if configuration.Spec.WriteConnectionSecretToReference == nil {
		configuration.Spec.WriteConnectionSecretToReference = comp.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.WriteConnectionSecretToReference
	}
	if configuration.Spec.WriteConnectionSecretToReference != nil && configuration.Spec.WriteConnectionSecretToReference.Namespace == "" {
		configuration.Spec.WriteConnectionSecretToReference.Namespace = ns
	}

	if configuration.Spec.ProviderReference == nil {
		configuration.Spec.ProviderReference = comp.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.ProviderReference
	}

	if configuration.Spec.GitCredentialsSecretReference == nil {
		configuration.Spec.GitCredentialsSecretReference = comp.FullTemplate.ComponentDefinition.Spec.Schematic.Terraform.GitCredentialsSecretReference
	}

	switch comp.FullTemplate.Terraform.Type {
	case "hcl":
		configuration.Spec.HCL = comp.FullTemplate.Terraform.Configuration
	case "remote":
		configuration.Spec.Remote = comp.FullTemplate.Terraform.Configuration
		configuration.Spec.Path = comp.FullTemplate.Terraform.Path
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
		objs := slices.Filter(af.ReferredObjects, func(obj *unstructured.Unstructured) bool {
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
