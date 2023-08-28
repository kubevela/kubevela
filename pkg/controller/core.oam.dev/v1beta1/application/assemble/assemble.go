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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// checkAutoDetectComponent will check if the standardWorkload is empty,
// currently only Helm-based component is possible to be auto-detected
// TODO implement auto-detect mechanism
func checkAutoDetectComponent(wl *unstructured.Unstructured) bool {
	return wl == nil || (len(wl.GetAPIVersion()) == 0 && len(wl.GetKind()) == 0)
}

// PrepareBeforeApply will prepare for some necessary info before apply
func PrepareBeforeApply(comp *types.ComponentManifest, appRev *v1beta1.ApplicationRevision) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	if checkAutoDetectComponent(comp.ComponentOutput) {
		return nil, nil, nil
	}
	compRevisionName := comp.RevisionName
	compName := comp.Name
	additionalLabel := map[string]string{
		oam.LabelAppComponentRevision: compRevisionName,
		oam.LabelAppRevisionHash:      appRev.Labels[oam.LabelAppRevisionHash],
	}
	wl := assembleWorkload(compName, comp.ComponentOutput, additionalLabel)

	assembledTraits := make([]*unstructured.Unstructured, len(comp.ComponentOutputsAndTraits))

	HandleCheckManageWorkloadTrait(*appRev, []*types.ComponentManifest{comp})

	for i, trait := range comp.ComponentOutputsAndTraits {
		setTraitLabels(trait, additionalLabel)
		assembledTraits[i] = trait
	}

	return wl, assembledTraits, nil
}

func assembleWorkload(compName string, wl *unstructured.Unstructured, labels map[string]string) *unstructured.Unstructured {
	// use component name as workload name if workload name is not specified
	// don't override the name set in render phase if exist
	if len(wl.GetName()) == 0 {
		wl.SetName(compName)
	}
	setWorkloadLabels(wl, labels)

	klog.InfoS("Successfully assemble a workload", "workload", klog.KObj(wl), "APIVersion", wl.GetAPIVersion(), "Kind", wl.GetKind())
	return wl
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
		for _, trait := range comp.ComponentOutputsAndTraits {
			traitType := trait.GetLabels()[oam.TraitTypeLabel]
			if manageWorkloadTrait[traitType] {
				trait.SetLabels(util.MergeMapOverrideWithDst(trait.GetLabels(), map[string]string{oam.LabelManageWorkloadTrait: "true"}))
			}
		}
	}
}
