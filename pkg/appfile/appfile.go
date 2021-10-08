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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	json2cue "cuelang.org/go/encoding/json"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// constant error information
const (
	errInvalidValueType                                = "require %q type parameter value"
	errTerraformConfigurationIsNotSet                  = "terraform configuration is not set"
	errFailToConvertTerraformComponentProperties       = "failed to convert Terraform component properties"
	errTerraformNameOfWriteConnectionSecretToRefNotSet = "the name of writeConnectionSecretToRef of terraform component is not set"
)

// WriteConnectionSecretToRefKey is used to create a secret for cloud resource connection
const WriteConnectionSecretToRefKey = "writeConnectionSecretToRef"

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
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return wl.engine.Complete(ctx, wl.FullTemplate.TemplateStr, wl.Params)
}

// EvalStatus eval workload status
func (wl *Workload) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return wl.engine.Status(ctx, cli, ns, wl.FullTemplate.CustomStatus, wl.Params)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	if wl.FullTemplate.Health == "" {
		return true, nil
	}
	return wl.engine.HealthCheck(ctx, client, namespace, wl.FullTemplate.Health)
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

// EvalStatus eval trait status
func (trait *Trait) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return trait.engine.Status(ctx, cli, ns, trait.CustomStatusFormat, trait.Params)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	if trait.FullTemplate.Health == "" {
		return true, nil
	}
	return trait.engine.HealthCheck(ctx, client, namespace, trait.HealthCheckPolicy)
}

// Appfile describes application
type Appfile struct {
	Name            string
	Namespace       string
	AppRevisionName string
	Workloads       []*Workload

	AppRevisionHash string
	AppLabels       map[string]string
	AppAnnotations  map[string]string

	RelatedTraitDefinitions     map[string]*v1beta1.TraitDefinition
	RelatedComponentDefinitions map[string]*v1beta1.ComponentDefinition
	RelatedScopeDefinitions     map[string]*v1beta1.ScopeDefinition

	Policies      []*Workload
	WorkflowSteps []v1beta1.WorkflowStep
	Components    []common.ApplicationComponent
	Artifacts     []*types.ComponentManifest
	WorkflowMode  common.WorkflowMode

	parser *Parser
}

// Handler handles reconcile
type Handler interface {
	HandleComponentsRevision(ctx context.Context, compManifests []*types.ComponentManifest) error
	Dispatch(ctx context.Context, manifests ...*unstructured.Unstructured) error
}

// PrepareWorkflowAndPolicy generates workflow steps and policies from an appFile
func (af *Appfile) PrepareWorkflowAndPolicy() (policies []*unstructured.Unstructured, err error) {
	policies, err = af.generateUnstructureds(af.Policies)
	if err != nil {
		return
	}

	af.generateSteps()
	return
}

func (af *Appfile) generateUnstructureds(workloads []*Workload) ([]*unstructured.Unstructured, error) {
	var uns []*unstructured.Unstructured
	for _, wl := range workloads {
		un, err := generateUnstructuredFromCUEModule(wl, af.Name, af.AppRevisionName, af.Namespace, af.Components, af.Artifacts)
		if err != nil {
			return nil, err
		}
		un.SetName(wl.Name)
		if len(un.GetNamespace()) == 0 {
			un.SetNamespace(af.Namespace)
		}
		uns = append(uns, un)
	}
	return uns, nil
}

func (af *Appfile) generateSteps() {
	if len(af.WorkflowSteps) == 0 {
		for _, comp := range af.Components {
			af.WorkflowSteps = append(af.WorkflowSteps, v1beta1.WorkflowStep{
				Name: comp.Name,
				Type: "apply-component",
				Properties: util.Object2RawExtension(map[string]string{
					"component": comp.Name,
				}),
			})
		}
	}
}

func generateUnstructuredFromCUEModule(wl *Workload, appName, revision, ns string, components []common.ApplicationComponent, artifacts []*types.ComponentManifest) (*unstructured.Unstructured, error) {
	pCtx := process.NewPolicyContext(ns, wl.Name, appName, revision, components)
	pCtx.PushData(model.ContextDataArtifacts, prepareArtifactsData(artifacts))
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", appName, ns)
	}
	return makeWorkloadWithContext(pCtx, wl, ns, appName)
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
		cm, err := af.GenerateComponentManifest(wl)
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
func (af *Appfile) GenerateComponentManifest(wl *Workload) (*types.ComponentManifest, error) {
	if af.Namespace == "" {
		af.Namespace = corev1.NamespaceDefault
	}
	switch wl.CapabilityCategory {
	case types.HelmCategory:
		return generateComponentFromHelmModule(wl, af.Name, af.AppRevisionName, af.Namespace)
	case types.KubeCategory:
		return generateComponentFromKubeModule(wl, af.Name, af.AppRevisionName, af.Namespace)
	case types.TerraformCategory:
		return generateComponentFromTerraformModule(wl, af.Name, af.AppRevisionName, af.Namespace)
	default:
		return generateComponentFromCUEModule(wl, af.Name, af.AppRevisionName, af.Namespace)
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
		if err := af.setWorkloadRefToTrait(workloadRef, trait); err != nil {
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
		oam.LabelAppRevision:  af.AppRevisionName,
		oam.LabelAppComponent: compName,
	}
	// merge application's all labels
	finalLabels := util.MergeMapOverrideWithDst(Labels, af.AppLabels)
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

/* NOTE a workload has these possible labels
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
		// remove labels that should not be calculate into hash
		util.RemoveLabels(cpTrait, []string{oam.LabelAppRevision})
		traitName := util.GenTraitNameCompatible(compName, cpTrait, traitType)
		trait.SetName(traitName)
	}
	af.setTraitLabels(trait, labels)
	af.filterAndSetAnnotations(trait)
	af.setNamespace(trait)
}

/* NOTE a trait has these possible labels
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

// PrepareProcessContext prepares a DSL process Context
func PrepareProcessContext(wl *Workload, applicationName, revision, namespace string) (process.Context, error) {
	pCtx := NewBasicContext(wl, applicationName, revision, namespace)
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", applicationName, namespace)
	}
	return pCtx, nil
}

// NewBasicContext prepares a basic DSL process Context
func NewBasicContext(wl *Workload, applicationName, revision, namespace string) process.Context {
	pCtx := process.NewContext(namespace, wl.Name, applicationName, revision)
	if wl.Params != nil {
		pCtx.SetParameters(wl.Params)
	}
	return pCtx
}

func generateComponentFromCUEModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	pCtx, err := PrepareProcessContext(wl, appName, revision, ns)
	if err != nil {
		return nil, err
	}
	wl.Ctx = pCtx
	return baseGenerateComponent(pCtx, wl, appName, ns)
}

func generateComponentFromTerraformModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	pCtx := NewBasicContext(wl, appName, revision, ns)
	return baseGenerateComponent(pCtx, wl, appName, ns)
}

func baseGenerateComponent(pCtx process.Context, wl *Workload, appName, ns string) (*types.ComponentManifest, error) {
	var err error

	for _, tr := range wl.Traits {
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, wl.Name)
		}
	}
	if patcher := wl.Patch; patcher != nil {
		workload, auxiliaries := pCtx.Output()
		if p, err := patcher.LookupValue("workload"); err == nil {
			pi, err := model.NewOther(p.CueValue())
			if err != nil {
				return nil, errors.WithMessage(err, "patch workload")
			}
			if err := workload.Unify(pi); err != nil {
				return nil, errors.WithMessage(err, "patch workload")
			}
		}
		for _, aux := range auxiliaries {
			if p, err := patcher.LookupByScript(fmt.Sprintf("traits[\"%s\"]", aux.Name)); err == nil && p.CueValue().Err() == nil {
				pi, err := model.NewOther(p.CueValue())
				if err != nil {
					return nil, errors.WithMessagef(err, "patch outputs.%s", aux.Name)
				}
				if err := aux.Ins.Unify(pi); err != nil {
					return nil, errors.WithMessagef(err, "patch outputs.%s", aux.Name)
				}
			}
		}
	}
	compManifest, err := evalWorkloadWithContext(pCtx, wl, ns, appName, wl.Name)
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
	commonLabels := definition.GetCommonLabels(pCtx.BaseContextLabels())
	util.AddLabels(workload, util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.WorkloadTypeLabel: wl.Type}))
	return workload, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component manifest
func evalWorkloadWithContext(pCtx process.Context, wl *Workload, ns, appName, compName string) (*types.ComponentManifest, error) {
	compManifest := &types.ComponentManifest{}
	workload, err := makeWorkloadWithContext(pCtx, wl, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.StandardWorkload = workload

	_, assists := pCtx.Output()
	compManifest.Traits = make([]*unstructured.Unstructured, len(assists))
	commonLabels := definition.GetCommonLabels(pCtx.BaseContextLabels())
	for i, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, compName, appName)
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
		ins, err := json2cue.Decode(&cue.Runtime{}, "", objRaw)
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot decode object into CUE")
		}
		cueRaw, err := format.Node(ins.Value().Syntax())
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot format CUE")
		}

		// NOTE a hack way to enable using CUE capabilities on KUBE schematic workload
		templateStr = fmt.Sprintf(`
output: { 
%s 
}`, string(cueRaw))
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

func generateComponentFromKubeModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest, err := generateComponentFromCUEModule(wl, appName, revision, ns)
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

	configuration := terraformapi.Configuration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta1", Kind: "Configuration"},
		ObjectMeta: metav1.ObjectMeta{Name: wl.Name, Namespace: ns},
	}

	switch wl.FullTemplate.Terraform.Type {
	case "hcl":
		configuration.Spec.HCL = wl.FullTemplate.Terraform.Configuration
	case "json":
		configuration.Spec.JSON = wl.FullTemplate.Terraform.Configuration
	case "remote":
		configuration.Spec.Remote = wl.FullTemplate.Terraform.Configuration
	}

	if wl.FullTemplate.Terraform.ProviderReference != nil {
		configuration.Spec.ProviderReference = wl.FullTemplate.Terraform.ProviderReference
	}

	// 1. parse writeConnectionSecretToRef
	if err := json.Unmarshal(params, &configuration.Spec); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	if configuration.Spec.WriteConnectionSecretToReference != nil {
		if configuration.Spec.WriteConnectionSecretToReference.Name == "" {
			return nil, errors.New(errTerraformNameOfWriteConnectionSecretToRefNotSet)
		}
		// set namespace for writeConnectionSecretToRef, developer needn't manually set it
		if configuration.Spec.WriteConnectionSecretToReference.Namespace == "" {
			configuration.Spec.WriteConnectionSecretToReference.Namespace = ns
		}
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

	data, err := json.Marshal(variableMap)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	configuration.Spec.Variable = &runtime.RawExtension{Raw: data}
	raw := util.Object2RawExtension(&configuration)
	return util.RawExtension2Unstructured(&raw)
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

func generateComponentFromHelmModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest := &types.ComponentManifest{
		Name:             wl.Name,
		Namespace:        ns,
		ExternalRevision: wl.ExternalRevision,
		StandardWorkload: &unstructured.Unstructured{},
	}

	if wl.FullTemplate.Reference.Type != types.AutoDetectWorkloadDefinition {
		compManifest, err = generateComponentFromCUEModule(wl, appName, revision, ns)
		if err != nil {
			return nil, err
		}
	}

	rls, repo, err := helm.RenderHelmReleaseAndHelmRepo(wl.FullTemplate.Helm, wl.Name, appName, ns, wl.Params)
	if err != nil {
		return nil, err
	}
	compManifest.PackagedWorkloadResources = []*unstructured.Unstructured{rls, repo}
	return compManifest, nil
}
