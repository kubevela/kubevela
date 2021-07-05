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
	"strings"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DefaultFilterAnnots are annotations won't pass to workload or trait
var DefaultFilterAnnots = []string{
	oam.AnnotationAppRollout,
	oam.AnnotationRollingComponent,
	oam.AnnotationInplaceUpgrade,
	oam.AnnotationFilterLabelKeys,
	oam.AnnotationFilterAnnotationKeys,
}

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

	componentManifests []*types.ComponentManifest
	appName            string
	appNamespace       string
	appLabels          map[string]string
	appAnnotations     map[string]string
	appOwnerRef        *metav1.OwnerReference

	assembledWorkloads map[string]*unstructured.Unstructured
	assembledTraits    map[string][]*unstructured.Unstructured
	// key is workload reference, values are the references of scopes the workload belongs to
	referencedScopes map[corev1.ObjectReference][]corev1.ObjectReference

	finalized bool
	err       error
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
	ApplyToWorkload(*unstructured.Unstructured, *v1beta1.ComponentDefinition, []*unstructured.Unstructured) error
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
func (am *AppManifests) ReferencedScopes() (map[corev1.ObjectReference][]corev1.ObjectReference, error) {
	if !am.finalized {
		am.assemble()
	}
	if am.err != nil {
		return nil, am.err
	}
	r := make(map[corev1.ObjectReference][]corev1.ObjectReference)
	for k, refs := range am.referencedScopes {
		r[k] = make([]corev1.ObjectReference, len(refs))
		copy(r[k], refs)
	}
	return r, nil
}

// GroupAssembledManifests do assemble and return all resources grouped by components
func (am *AppManifests) GroupAssembledManifests() (
	map[string]*unstructured.Unstructured,
	map[string][]*unstructured.Unstructured,
	map[corev1.ObjectReference][]corev1.ObjectReference, error) {
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
	scopes := make(map[corev1.ObjectReference][]corev1.ObjectReference)
	for k, v := range am.referencedScopes {
		scopes[k] = make([]corev1.ObjectReference, len(v))
		copy(scopes[k], v)
	}
	return workloads, traits, scopes, nil
}

// checkAutoDetectComponent will check if the standardWorkload is empty,
// currently only Helm-based component is possible to be auto-detected
// TODO implement auto-detect mechanism
func checkAutoDetectComponent(wl *unstructured.Unstructured) bool {
	return wl == nil || (len(wl.GetAPIVersion()) == 0 && len(wl.GetKind()) == 0)
}

func (am *AppManifests) assemble() {
	am.complete()
	klog.InfoS("Assemble manifests for application", "name", am.appName, "revision", am.AppRevision.GetName())
	if err := am.validate(); err != nil {
		am.finalizeAssemble(err)
		return
	}
	for _, comp := range am.componentManifests {
		if comp.InsertConfigNotReady {
			continue
		}
		if checkAutoDetectComponent(comp.StandardWorkload) {
			klog.Warningf("component without specify workloadDef can not attach traits currently")
			continue
		}
		compRevisionName := comp.RevisionName
		compName := comp.Name
		commonLabels := am.generateAndFilterCommonLabels(compName, compRevisionName)
		klog.InfoS("Assemble manifests for component", "name", compName)
		wl, err := am.assembleWorkload(compName, comp.StandardWorkload, commonLabels, comp.PackagedWorkloadResources)
		if err != nil {
			am.finalizeAssemble(err)
			return
		}
		am.assembledWorkloads[compName] = wl
		workloadRef := corev1.ObjectReference{
			APIVersion: wl.GetAPIVersion(),
			Kind:       wl.GetKind(),
			Name:       wl.GetName(),
		}
		am.assembledTraits[compName] = make([]*unstructured.Unstructured, len(comp.Traits))
		for i, trait := range comp.Traits {
			trait := am.assembleTrait(trait, compName, commonLabels)
			if err := am.setWorkloadRefToTrait(workloadRef, trait); err != nil {
				am.finalizeAssemble(errors.WithMessagef(err, "cannot set workload reference to trait %q", trait.GetName()))
				return
			}
			am.assembledTraits[compName][i] = trait
		}

		am.referencedScopes[workloadRef] = make([]corev1.ObjectReference, len(comp.Scopes))
		for i, scope := range comp.Scopes {
			am.referencedScopes[workloadRef][i] = *scope
		}
	}
	am.finalizeAssemble(nil)
}

func (am *AppManifests) complete() {
	if len(am.componentManifests) == 0 {
		am.componentManifests, _ = util.AppConfig2ComponentManifests(am.AppRevision.Spec.ApplicationConfiguration,
			am.AppRevision.Spec.Components)
	}
	am.appNamespace = am.AppRevision.GetNamespace()
	am.appLabels = am.AppRevision.GetLabels()
	am.appName = am.AppRevision.GetLabels()[oam.LabelAppName]
	am.appAnnotations = am.AppRevision.GetAnnotations()
	am.appOwnerRef = metav1.GetControllerOf(am.AppRevision)

	am.assembledWorkloads = make(map[string]*unstructured.Unstructured)
	am.assembledTraits = make(map[string][]*unstructured.Unstructured)
	am.referencedScopes = make(map[corev1.ObjectReference][]corev1.ObjectReference)
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
	if len(am.AppRevision.Labels[oam.LabelAppName]) == 0 {
		return errors.New("AppRevision must have app name in the label")
	}
	if len(am.AppRevision.Labels[oam.LabelAppRevisionHash]) == 0 {
		return errors.New("AppRevision must have revision hash in the label")
	}
	return nil
}

// workload and trait in the same component both have these labels
func (am *AppManifests) generateAndFilterCommonLabels(compName, compRevisionName string) map[string]string {
	filter := func(labels map[string]string, notAllowedKey []string) {
		for _, l := range notAllowedKey {
			delete(labels, strings.TrimSpace(l))
		}
	}
	Labels := map[string]string{
		oam.LabelAppName:              am.appName,
		oam.LabelAppRevision:          am.AppRevision.Name,
		oam.LabelAppRevisionHash:      am.AppRevision.Labels[oam.LabelAppRevisionHash],
		oam.LabelAppComponent:         compName,
		oam.LabelAppComponentRevision: compRevisionName,
	}
	// merge application's all labels
	finalLabels := util.MergeMapOverrideWithDst(Labels, am.appLabels)
	filterLabels, ok := am.appAnnotations[oam.AnnotationFilterLabelKeys]
	if ok {
		filter(finalLabels, strings.Split(filterLabels, ","))
	}
	return finalLabels
}

// workload and trait both have these annotations
func (am *AppManifests) filterAndSetAnnotations(obj *unstructured.Unstructured) {
	var allFilterAnnotation []string
	allFilterAnnotation = append(allFilterAnnotation, DefaultFilterAnnots...)

	passedFilterAnnotation, ok := am.appAnnotations[oam.AnnotationFilterAnnotationKeys]
	if ok {
		allFilterAnnotation = append(allFilterAnnotation, strings.Split(passedFilterAnnotation, ",")...)
	}

	// pass application's all annotations
	util.AddAnnotations(obj, am.appAnnotations)
	// remove useless annotations for workload/trait
	util.RemoveAnnotations(obj, allFilterAnnotation)
}

func (am *AppManifests) setNamespace(obj *unstructured.Unstructured) {
	// only set app's namespace when namespace is unspecified
	// it's by design to set arbitrary namespace in render phase
	if len(obj.GetNamespace()) == 0 {
		obj.SetNamespace(am.appNamespace)
	}
}

func (am *AppManifests) assembleWorkload(compName string, wl *unstructured.Unstructured,
	labels map[string]string, resources []*unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// use component name as workload name
	// override the name set in render phase if exist
	wl.SetName(compName)
	am.setWorkloadLabels(wl, labels)
	am.filterAndSetAnnotations(wl)
	am.setNamespace(wl)

	workloadType := wl.GetLabels()[oam.WorkloadTypeLabel]
	compDefinition := am.AppRevision.Spec.ComponentDefinitions[workloadType]
	copyPackagedResources := make([]*unstructured.Unstructured, len(resources))
	for i, v := range resources {
		copyPackagedResources[i] = v.DeepCopy()
	}
	for _, wo := range am.WorkloadOptions {
		if err := wo.ApplyToWorkload(wl, compDefinition.DeepCopy(), copyPackagedResources); err != nil {
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

func (am *AppManifests) assembleTrait(trait *unstructured.Unstructured, compName string, labels map[string]string) *unstructured.Unstructured {
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	// only set generated name when name is unspecified
	// it's by design to set arbitrary name in render phase
	if len(trait.GetName()) == 0 {
		traitName := util.GenTraitNameCompatible(compName, trait, traitType)
		trait.SetName(traitName)
	}
	am.setTraitLabels(trait, labels)
	am.filterAndSetAnnotations(trait)
	am.setNamespace(trait)
	klog.InfoS("Successfully assemble a trait", "trait", klog.KObj(trait), "APIVersion", trait.GetAPIVersion(), "Kind", trait.GetKind())
	return trait
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

func (am *AppManifests) setWorkloadRefToTrait(wlRef corev1.ObjectReference, trait *unstructured.Unstructured) error {
	traitType := trait.GetLabels()[oam.TraitTypeLabel]
	traitDef := am.AppRevision.Spec.TraitDefinitions[traitType]
	workloadRefPath := traitDef.Spec.WorkloadRefPath
	// only add workload reference to the trait if it asks for it
	if len(workloadRefPath) != 0 {
		// TODO(roywang) this is for backward compatibility, remove crossplane/runtime/v1alpha1 in the future
		tmpWLRef := runtimev1alpha1.TypedReference{
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
