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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// NewAppManifests create a AppManifests
func NewAppManifests(appRevision *v1beta1.ApplicationRevision, parser *appfile.Parser) *AppManifests {
	return &AppManifests{AppRevision: appRevision, parser: parser}
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
	referencedScopes      map[corev1.ObjectReference][]corev1.ObjectReference
	skipWorkloadApplyComp map[string]bool

	finalized bool
	err       error

	parser *appfile.Parser
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

// WithComponentManifests set component manifests with the given one
func (am *AppManifests) WithComponentManifests(componentManifests []*types.ComponentManifest) *AppManifests {
	am.componentManifests = componentManifests
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
		skipApplyWorkload := false
		ts := am.assembledTraits[compName]
		for _, t := range ts {
			r = append(r, t.DeepCopy())
			if v := t.GetLabels()[oam.LabelManageWorkloadTrait]; v == "true" {
				skipApplyWorkload = true
			}
		}
		if !skipApplyWorkload {
			r = append(r, wl.DeepCopy())
		} else {
			klog.InfoS("assemble meet a managedByTrait workload, so skip apply it",
				"namespace", am.AppRevision.Namespace, "appRev", am.AppRevision.Name)
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
	if err := am.complete(); err != nil {
		am.finalizeAssemble(err)
		return
	}

	klog.InfoS("Assemble manifests for application", "name", am.appName, "revision", am.AppRevision.GetName())
	if err := am.validate(); err != nil {
		am.finalizeAssemble(err)
		return
	}
	for _, comp := range am.componentManifests {
		klog.InfoS("Assemble manifests for component", "name", comp.Name)
		wl, traits, err := PrepareBeforeApply(comp, am.AppRevision, am.WorkloadOptions)
		if err != nil {
			am.finalizeAssemble(err)
			return
		}
		if wl == nil {
			klog.Warningf("component without specify workloadDef can not attach traits currently")
			continue
		}

		am.assembledWorkloads[comp.Name] = wl
		workloadRef := corev1.ObjectReference{
			APIVersion: wl.GetAPIVersion(),
			Kind:       wl.GetKind(),
			Name:       wl.GetName(),
		}
		am.assembledTraits[comp.Name] = traits
		am.referencedScopes[workloadRef] = make([]corev1.ObjectReference, len(comp.Scopes))
		for i, scope := range comp.Scopes {
			am.referencedScopes[workloadRef][i] = *scope
		}
	}
	am.finalizeAssemble(nil)
}

// PrepareBeforeApply will prepare for some necessary info before apply
func PrepareBeforeApply(comp *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, workloadOpt []WorkloadOption) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	if checkAutoDetectComponent(comp.StandardWorkload) {
		return nil, nil, nil
	}
	compRevisionName := comp.RevisionName
	compName := comp.Name
	additionalLabel := map[string]string{
		oam.LabelAppComponentRevision: compRevisionName,
		oam.LabelAppRevisionHash:      appRev.Labels[oam.LabelAppRevisionHash],
	}
	wl, err := assembleWorkload(compName, comp.StandardWorkload, additionalLabel, comp.PackagedWorkloadResources, appRev, workloadOpt)
	if err != nil {
		return nil, nil, err
	}

	assembledTraits := make([]*unstructured.Unstructured, len(comp.Traits))

	HandleCheckManageWorkloadTrait(*appRev, []*types.ComponentManifest{comp})

	for i, trait := range comp.Traits {
		setTraitLabels(trait, additionalLabel)
		assembledTraits[i] = trait
	}

	return wl, assembledTraits, nil
}

func (am *AppManifests) complete() error {
	if len(am.componentManifests) == 0 {
		var err error
		af, err := am.parser.GenerateAppFileFromRevision(am.AppRevision)
		if err != nil {
			return errors.WithMessage(err, "fail to generate appfile from revision for app manifests complete")
		}
		am.componentManifests, err = af.GenerateComponentManifests()
		if err != nil {
			return errors.WithMessage(err, "fail to complete manifests as generate from app revision failed")
		}
	}
	am.appNamespace = am.AppRevision.GetNamespace()
	am.appLabels = am.AppRevision.GetLabels()
	am.appName = am.AppRevision.GetLabels()[oam.LabelAppName]
	am.appAnnotations = am.AppRevision.GetAnnotations()
	am.appOwnerRef = metav1.GetControllerOf(am.AppRevision)

	am.assembledWorkloads = make(map[string]*unstructured.Unstructured)
	am.assembledTraits = make(map[string][]*unstructured.Unstructured)
	am.referencedScopes = make(map[corev1.ObjectReference][]corev1.ObjectReference)
	am.skipWorkloadApplyComp = make(map[string]bool)
	return nil
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

func assembleWorkload(compName string, wl *unstructured.Unstructured,
	labels map[string]string, resources []*unstructured.Unstructured, appRev *v1beta1.ApplicationRevision, wop []WorkloadOption) (*unstructured.Unstructured, error) {
	// use component name as workload name if workload name is not specified
	// don't override the name set in render phase if exist
	if len(wl.GetName()) == 0 {
		wl.SetName(compName)
	}
	setWorkloadLabels(wl, labels)

	workloadType := wl.GetLabels()[oam.WorkloadTypeLabel]
	compDefinition := appRev.Spec.ComponentDefinitions[workloadType]
	copyPackagedResources := make([]*unstructured.Unstructured, len(resources))
	for i, v := range resources {
		copyPackagedResources[i] = v.DeepCopy()
	}
	for _, wo := range wop {
		if err := wo.ApplyToWorkload(wl, compDefinition.DeepCopy(), copyPackagedResources); err != nil {
			klog.ErrorS(err, "Failed applying a workload option", "workload", klog.KObj(wl), "name", wl.GetName())
			return nil, errors.Wrapf(err, "cannot apply workload option for component %q", compName)
		}
		klog.InfoS("Successfully apply a workload option", "workload", klog.KObj(wl), "name", wl.GetName())
	}
	klog.InfoS("Successfully assemble a workload", "workload", klog.KObj(wl), "APIVersion", wl.GetAPIVersion(), "Kind", wl.GetKind())
	return wl, nil
}

// component revision label added here
// label key: app.oam.dev/revision
func setWorkloadLabels(wl *unstructured.Unstructured, additionalLabels map[string]string) {
	// add more workload-specific labels here
	util.AddLabels(wl, additionalLabels)
}

// component revision label added here
// label key: app.oam.dev/revision
func setTraitLabels(trait *unstructured.Unstructured, additionalLabels map[string]string) {
	// add more trait-specific labels here
	util.AddLabels(trait, additionalLabels)
}

// HandleCheckManageWorkloadTrait will checkout every trait whether a manage-workload trait, if yes set label and annotation in trait
func HandleCheckManageWorkloadTrait(appRev v1beta1.ApplicationRevision, comps []*types.ComponentManifest) {
	traitDefs := appRev.Spec.TraitDefinitions
	manageWorkloadTrait := map[string]bool{}
	for traitName, definition := range traitDefs {
		if definition.Spec.ManageWorkload {
			manageWorkloadTrait[traitName] = true
		}
	}
	if len(manageWorkloadTrait) == 0 {
		return
	}
	for _, comp := range comps {
		for _, trait := range comp.Traits {
			traitType := trait.GetLabels()[oam.TraitTypeLabel]
			if manageWorkloadTrait[traitType] {
				trait.SetLabels(util.MergeMapOverrideWithDst(trait.GetLabels(), map[string]string{oam.LabelManageWorkloadTrait: "true"}))
			}
		}
	}
}
