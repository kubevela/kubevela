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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

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

// checkAutoDetectComponent will check if the standardWorkload is empty,
// currently only Helm-based component is possible to be auto-detected
// TODO implement auto-detect mechanism
func checkAutoDetectComponent(wl *unstructured.Unstructured) bool {
	return wl == nil || (len(wl.GetAPIVersion()) == 0 && len(wl.GetKind()) == 0)
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
