/*
Copyright 2022 The KubeVela Authors.

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

package application

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

// DispatchOptions is the options for dispatch
type DispatchOptions struct {
	Workload          *unstructured.Unstructured
	Traits            []*unstructured.Unstructured
	OverrideNamespace string
	Stage             StageType
}

// SortDispatchOptions describe the sorting for options
type SortDispatchOptions []DispatchOptions

func (s SortDispatchOptions) Len() int {
	return len(s)
}

func (s SortDispatchOptions) Less(i, j int) bool {
	return s[i].Stage < s[j].Stage
}

func (s SortDispatchOptions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

var _ sort.Interface = new(SortDispatchOptions)

// StageType is a valid value for TraitDefinitionSpec.Stage
type StageType int

const (
	// PreDispatch means that pre dispatch for manifests
	PreDispatch StageType = iota
	// DefaultDispatch means that default dispatch for manifests
	DefaultDispatch
	// PostDispatch means that post dispatch for manifests
	PostDispatch
)

var stages = map[StageType]string{
	PreDispatch:     "PreDispatch",
	DefaultDispatch: "DefaultDispatch",
	PostDispatch:    "PostDispatch",
}

// ParseStageType parse the StageType from a string
func ParseStageType(s string) (StageType, error) {
	for k, v := range stages {
		if v == s {
			return k, nil
		}
	}
	return -1, errors.New("unknown stage type")
}

// TraitFilter is used to filter trait object.
type TraitFilter func(trait appfile.Trait) bool

// ByTraitType returns a filter that does not match the given type and belongs to readyTraits.
func ByTraitType(readyTraits, checkTraits []*unstructured.Unstructured) TraitFilter {
	generateFn := func(manifests []*unstructured.Unstructured) map[string]bool {
		out := map[string]bool{}
		for _, obj := range manifests {
			out[obj.GetLabels()[oam.TraitTypeLabel]] = true
		}
		return out
	}
	readyMap := generateFn(readyTraits)
	checkMap := generateFn(checkTraits)
	return func(trait appfile.Trait) bool {
		return !checkMap[trait.Name] && readyMap[trait.Name]
	}
}

// manifestDispatcher is a manifest dispatcher
type manifestDispatcher struct {
	run         func(ctx context.Context, c *appfile.Component, appRev *v1beta1.ApplicationRevision, clusterName string) (bool, error)
	healthCheck func(ctx context.Context, c *appfile.Component, appRev *v1beta1.ApplicationRevision) (bool, error)
}

func (h *AppHandler) generateDispatcher(appRev *v1beta1.ApplicationRevision, readyWorkload *unstructured.Unstructured, readyTraits []*unstructured.Unstructured, overrideNamespace string, annotations map[string]string) ([]*manifestDispatcher, error) {
	return h.generateDispatcherImpl(appRev, readyWorkload, readyTraits, overrideNamespace, annotations)
}

func getTraitDispatchStage(client client.Client, traitType string, appRev *v1beta1.ApplicationRevision, annotations map[string]string) (StageType, error) {
	trait, ok := appRev.Spec.TraitDefinitions[traitType]
	if !ok {
		trait = &v1beta1.TraitDefinition{}
		err := oamutil.GetCapabilityDefinition(context.Background(), client, trait, traitType, annotations)
		if err != nil {
			return DefaultDispatch, err
		}
	}
	_stageType := trait.Spec.Stage
	if len(_stageType) == 0 {
		_stageType = v1beta1.DefaultDispatch
	}
	stageType, err := ParseStageType(string(_stageType))
	if err != nil {
		return DefaultDispatch, err
	}
	return stageType, nil
}

// createPendingTraitStatus returns a pending trait status
func createPendingTraitStatus(traitName string) common.ApplicationTraitStatus {
	return common.ApplicationTraitStatus{
		Type:    traitName,
		Healthy: false,
		Pending: true,
		Message: "Waiting for component to be healthy",
	}
}

// addPendingPostDispatchTraits adds pending trait status entries for PostDispatch traits that haven't been collected yet
func (h *AppHandler) addPendingPostDispatchTraits(comp *appfile.Component, status *common.ApplicationComponentStatus, processedTraits map[string]bool) {
	// Build a map of already reported traits (all traits in the status list)
	reportedTraits := make(map[string]bool)
	for _, ts := range status.Traits {
		reportedTraits[ts.Type] = true
	}

	// Check all traits in the component
	for _, trait := range comp.Traits {
		// Skip if already reported or already processed (includes pending traits added earlier)
		if reportedTraits[trait.Name] || processedTraits[trait.Name] {
			continue
		}

		// Check if this trait is PostDispatch
		traitStage, err := getTraitDispatchStage(h.Client, trait.Name, h.currentAppRev, h.app.Annotations)
		if err == nil && traitStage == PostDispatch {
			// Add pending status for this trait
			klog.V(4).Infof("Adding pending status for PostDispatch trait %s", trait.Name)
			traitStatus := createPendingTraitStatus(trait.Name)
			status.Traits = append(status.Traits, traitStatus)
			// Mark as processed to prevent future duplicates
			processedTraits[trait.Name] = true
		}
	}
}
