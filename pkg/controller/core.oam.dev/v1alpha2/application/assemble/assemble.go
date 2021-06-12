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

package assemble

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	ctrlutil "github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// NewAppManifests create a AppManifests
func NewAppManifests(appRevision *v1beta1.ApplicationRevision) *AppManifests {
	return &AppManifests{AppRevision: appRevision}
}

// AppManifests contains configuration to assemble resources recorded in the ApplicationRevision.
// 'Assemble' means expand Application(Component and Trait) into K8s resource and get them ready to go, to be emitted
// into K8s
type AppManifests struct {
	AppRevision     *v1beta1.ApplicationRevision
	WorkloadOptions []WorkloadOption

	appComponents []applicationComponent
	components    []*v1alpha2.Component

	appName        string
	appNamespace   string
	appLabels      map[string]string
	appAnnotations map[string]string
	appOwnerRef    *metav1.OwnerReference

	assembledWorkloads map[string]*unstructured.Unstructured
	assembledTraits    map[string][]*unstructured.Unstructured
	// key is workload reference, values are the references of scopes the workload belongs to
	referencedScopes map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference

	finalized bool
	err       error
}

// this is a helper struct to replace v1alpha2.ApplicationConfiguration for this moment
type applicationComponent struct {
	revisionName string
	traits       []v1alpha2.ComponentTrait
	scopes       []runtimev1alpha1.TypedReference
}

// WorkloadOption will be applied to each workloads AFTER it has been assembled by generic rules shown below:
// 1) use component name as workload name
// 2) use application namespace as workload namespace if unspecified
// 3) set application as workload's owner
// 4) pass all application's labels and annotations to workload's
// Component and ComponentDefinition are enough for caller to manipulate workloads.
// Caller can use below labels of workload to get more information:
// - oam.LabelAppName
// - oam.LabelAppRevision
// - oam.LabelAppRevisionHash
// - oam.LabelAppComponent
// - oam.LabelAppComponentRevision
type WorkloadOption interface {
	ApplyToWorkload(workload *unstructured.Unstructured, comp *v1alpha2.Component, compDefinition *v1beta1.ComponentDefinition) error
}

// WithWorkloadOption add a WorkloadOption to plug in custom logic applied to each workload
func (am *AppManifests) WithWorkloadOption(wo WorkloadOption) *AppManifests {
	if am.WorkloadOptions == nil {
		am.WorkloadOptions = make([]WorkloadOption, 0)
	}
	am.WorkloadOptions = append(am.WorkloadOptions, wo)
	return am
}

// AssembledManifests do assemble and merge all assembled resources(except referenced scopes) into one array
// The result guarantee the order of resources as defined in application originally.
// If it contains more than one component, the resources are well-orderred and also grouped.
// For example, if app = comp1 (wl1 + trait1 + trait2) + comp2 (wl2 + trait3 +trait4),
// the result is [wl1, trait1, trait2, wl2, trait3, trait4]
func (am *AppManifests) AssembledManifests() ([]*unstructured.Unstructured, error) {
	if !am.finalized {
		am.assemble()
	}
	if am.err != nil {
		return nil, am.err
	}
	r := make([]*unstructured.Unstructured, 0)
	for compName, wl := range am.assembledWorkloads {
		r = append(r, wl.DeepCopy())
		ts := am.assembledTraits[compName]
		for _, t := range ts {
			r = append(r, t.DeepCopy())
		}
	}
	return r, nil
}

// ReferencedScopes do assemble and return workload reference and referenced scopes
func (am *AppManifests) ReferencedScopes() (map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference, error) {
	if !am.finalized {
		am.assemble()
	}
	if am.err != nil {
		return nil, am.err
	}
	r := make(map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference)
	for k, refs := range am.referencedScopes {
		r[k] = make([]runtimev1alpha1.TypedReference, len(refs))
		copy(r[k], refs)
	}
	return r, nil
}

// GroupAssembledManifests do assemble and return all resources grouped by components
func (am *AppManifests) GroupAssembledManifests() (
	map[string]*unstructured.Unstructured,
	map[string][]*unstructured.Unstructured,
	map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference, error) {
	if !am.finalized {
		am.assemble()
	}
	if am.err != nil {
		return nil, nil, nil, am.err
	}
	workloads := make(map[string]*unstructured.Unstructured)
	for k, wl := range am.assembledWorkloads {
		workloads[k] = wl.DeepCopy()
	}
	traits := make(map[string][]*unstructured.Unstructured)
	for k, ts := range am.assembledTraits {
		traits[k] = make([]*unstructured.Unstructured, len(ts))
		for i, t := range ts {
			traits[k][i] = t.DeepCopy()
		}
	}
	scopes := make(map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference)
	for k, v := range am.referencedScopes {
		scopes[k] = make([]runtimev1alpha1.TypedReference, len(v))
		copy(scopes[k], v)
	}
	return workloads, traits, scopes, nil
}

func (am *AppManifests) assemble() {
	am.complete()
	klog.InfoS("Assemble manifests for application", "name", am.appName, "revision", am.AppRevision.GetName())
	if err := am.validate(); err != nil {
		am.finalizeAssemble(err)
		return
	}
	for _, ac := range am.appComponents {
		compRevisionName := ac.revisionName
		compName := ctrlutil.ExtractComponentName(compRevisionName)
		commonLabels := am.generateCommonLabels(compName, compRevisionName)
		var workloadRef runtimev1alpha1.TypedReference
		klog.InfoS("Assemble manifests for component", "name", compName)
		for _, comp := range am.components {
			if comp.Name == compName {
				wl, err := am.assembleWorkload(comp, commonLabels)
				if err != nil {
					am.finalizeAssemble(err)
					return
				}
				am.assembledWorkloads[compName] = wl
				workloadRef = runtimev1alpha1.TypedReference{
					APIVersion: wl.GetAPIVersion(),
					Kind:       wl.GetKind(),
					Name:       wl.GetName(),
				}
				break
			}
		}

		am.assembledTraits[compName] = make([]*unstructured.Unstructured, len(ac.traits))
		for i, compTrait := range ac.traits {
			trait, err := am.assembleTrait(compTrait, compName, commonLabels)
			if err != nil {
				am.finalizeAssemble(err)
				return
			}
			if err := am.setWorkloadRefToTrait(workloadRef, trait); err != nil {
				am.finalizeAssemble(errors.WithMessagef(err, "cannot set workload reference to trait %q", trait.GetName()))
				return
			}
			am.assembledTraits[compName][i] = trait
		}

		am.referencedScopes[workloadRef] = make([]runtimev1alpha1.TypedReference, len(ac.scopes))
		for i, scope := range ac.scopes {
			am.referencedScopes[workloadRef][i] = scope
		}
	}
	am.finalizeAssemble(nil)
}

func (am *AppManifests) complete() {
	// safe to skip error-check
	appConfig, _ := convertRawExtention2AppConfig(am.AppRevision.Spec.ApplicationConfiguration)
	// convert v1alpha2.ApplicationConfiguration to a helper struct
	am.appComponents = make([]applicationComponent, len(appConfig.Spec.Components))
	for i, acc := range appConfig.Spec.Components {
		am.appComponents[i] = applicationComponent{
			revisionName: acc.RevisionName,
		}
		am.appComponents[i].traits = make([]v1alpha2.ComponentTrait, len(acc.Traits))
		copy(am.appComponents[i].traits, acc.Traits)
		am.appComponents[i].scopes = make([]runtimev1alpha1.TypedReference, len(acc.Scopes))
		for j, s := range acc.Scopes {
			am.appComponents[i].scopes[j] = s.ScopeReference
		}
	}
	// Application entity in the ApplicationRevision has no metadata,
	// so we have to get below information from AppConfig.
	// Up-stream process must set these to AppConfig.
	am.appName = appConfig.GetName()
	am.appNamespace = appConfig.GetNamespace()
	am.appLabels = appConfig.GetLabels()
	am.appAnnotations = appConfig.GetAnnotations()
	am.appOwnerRef = metav1.GetControllerOf(appConfig)

	am.components = make([]*v1alpha2.Component, len(am.AppRevision.Spec.Components))
	for i, rawComp := range am.AppRevision.Spec.Components {
		// safe to skip error-check
		comp, _ := convertRawExtention2Component(rawComp.Raw)
		am.components[i] = comp
	}

	am.assembledWorkloads = make(map[string]*unstructured.Unstructured)
	am.assembledTraits = make(map[string][]*unstructured.Unstructured)
	am.referencedScopes = make(map[runtimev1alpha1.TypedReference][]runtimev1alpha1.TypedReference)
}

func (am *AppManifests) finalizeAssemble(err error) {
	am.finalized = true
	if err == nil {
		klog.InfoS("Successfully assemble manifests for application", "name", am.appName, "revision", am.AppRevision.GetName(), "namespace", am.appNamespace)
		return
	}
	klog.ErrorS(err, "Failed assembling manifests for application", "name", am.appName, "revision", am.AppRevision.GetName())
	am.err = errors.WithMessagef(err, "cannot assemble resources' manifests for application %q", am.appName)
}

// AssembleOptions is highly coulped with AppRevision, should check the AppRevision provides all info
// required by AssembleOptions
func (am *AppManifests) validate() error {
	if am.appOwnerRef == nil {
		return errors.New("AppRevision must have an Application as owner")
	}
	if len(am.AppRevision.Labels[oam.LabelAppRevisionHash]) == 0 {
		return errors.New("AppRevision must have revision hash recorded in the label")
	}
	return nil
}

// workload and trait in the same component both have these labels
func (am *AppManifests) generateCommonLabels(compName, compRevisionName string) map[string]string {
	Labels := map[string]string{
		oam.LabelAppName:              am.appName,
		oam.LabelAppRevision:          am.AppRevision.Name,
		oam.LabelAppRevisionHash:      am.AppRevision.Labels[oam.LabelAppRevisionHash],
		oam.LabelAppComponent:         compName,
		oam.LabelAppComponentRevision: compRevisionName,
	}
	// pass application's all labels to workload/trait
	return util.MergeMapOverrideWithDst(Labels, am.appLabels)
}

// workload and trait both have these annotations
func (am *AppManifests) setAnnotations(obj *unstructured.Unstructured) {
	// pass application's all annotations
	util.AddAnnotations(obj, am.appAnnotations)
	// remove useless annotations for workload/trait
	util.RemoveAnnotations(obj, []string{
		oam.AnnotationAppRollout,
		oam.AnnotationRollingComponent,
		oam.AnnotationInplaceUpgrade,
	})
}

func (am *AppManifests) setNamespace(obj *unstructured.Unstructured) {
	// only set app's namespace when namespace is unspecified
	// it's by design to set arbitrary namespace in render phase
	if len(obj.GetNamespace()) == 0 {
		obj.SetNamespace(am.appNamespace)
	}
}

func (am *AppManifests) assembleWorkload(comp *v1alpha2.Component, labels map[string]string) (*unstructured.Unstructured, error) {
	compName := comp.Name
	wl, err := util.RawExtension2Unstructured(&comp.Spec.Workload)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot convert raw workload in component %q", compName)
	}
	// use component name as workload name
	// override the name set in render phase if exist
	wl.SetName(compName)
	am.setWorkloadLabels(wl, labels)
	am.setAnnotations(wl)
	am.setNamespace(wl)

	workloadType := wl.GetLabels()[oam.WorkloadTypeLabel]
	compDefinition := am.AppRevision.Spec.ComponentDefinitions[workloadType]
	for _, wo := range am.WorkloadOptions {
		if err := wo.ApplyToWorkload(wl, comp.DeepCopy(), compDefinition.DeepCopy()); err != nil {
			klog.ErrorS(err, "Failed applying a workload option", "workload", klog.KObj(wl), "name", wl.GetName())
			return nil, errors.Wrapf(err, "cannot apply workload option for component %q", compName)
		}
		klog.InfoS("Successfully apply a workload option", "workload", klog.KObj(wl), "name", wl.GetName())
	}
	klog.InfoS("Successfully assemble a workload", "workload", klog.KObj(wl), "APIVersion", wl.GetAPIVersion(), "Kind", wl.GetKind())
	return wl, nil
}

func (am *AppManifests) setWorkloadLabels(wl *unstructured.Unstructured, commonLabels map[string]string) {
	// add more workload-specific labels here
	util.AddLabels(wl, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeWorkload})
	util.AddLabels(wl, commonLabels)

	/* NOTE a workload has these possible labels
	   app.oam.dev/app-revision-hash: ce053923e2fb403f
	   app.oam.dev/appRevision: myapp-v2
	   app.oam.dev/component: mycomp
	   app.oam.dev/name: myapp
	   app.oam.dev/resourceType: WORKLOAD
	   app.oam.dev/revision: mycomp-v2
	   workload.oam.dev/type: kube-worker
	*/
}

func (am *AppManifests) assembleTrait(compTrait v1alpha2.ComponentTrait, compName string, labels map[string]string) (*unstructured.Unstructured, error) {
	trait, err := util.RawExtension2Unstructured(&compTrait.Trait)
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot convert raw trait in component")
	}
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	// only set generated name when name is unspecified
	// it's by design to set arbitrary name in render phase
	if len(trait.GetName()) == 0 {
		traitName := util.GenTraitName(compName, &compTrait, traitType)
		trait.SetName(traitName)
	}
	am.setTraitLabels(trait, labels)
	am.setAnnotations(trait)
	am.setNamespace(trait)
	klog.InfoS("Successfully assemble a trait", "trait", klog.KObj(trait), "APIVersion", trait.GetAPIVersion(), "Kind", trait.GetKind())
	return trait, nil
}

func (am *AppManifests) setTraitLabels(trait *unstructured.Unstructured, commonLabels map[string]string) {
	// add more trait-specific labels here
	util.AddLabels(trait, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeTrait})
	util.AddLabels(trait, commonLabels)

	/* NOTE a trait has these possible labels
	   app.oam.dev/app-revision-hash: ce053923e2fb403f
	   app.oam.dev/appRevision: myapp-v2
	   app.oam.dev/component: mycomp
	   app.oam.dev/name: myapp
	   app.oam.dev/resourceType: TRAIT
	   app.oam.dev/revision: mycomp-v2
	   trait.oam.dev/resource: service
	   trait.oam.dev/type: ingress // already added in render phase
	*/
}

func (am *AppManifests) setWorkloadRefToTrait(wlRef runtimev1alpha1.TypedReference, trait *unstructured.Unstructured) error {
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	traitDef := am.AppRevision.Spec.TraitDefinitions[traitType]
	workloadRefPath := traitDef.Spec.WorkloadRefPath
	// only add workload reference to the trait if it asks for it
	if len(workloadRefPath) != 0 {
		if err := fieldpath.Pave(trait.UnstructuredContent()).SetValue(workloadRefPath, wlRef); err != nil {
			return err
		}
	}
	return nil
}
