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
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
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
	dispatcherGenerator := func(options DispatchOptions) *manifestDispatcher {
		assembleManifestFn := func(skipApplyWorkload bool) (bool, []*unstructured.Unstructured) {
			manifests := options.Traits
			skipWorkload := skipApplyWorkload || options.Workload == nil
			if !skipWorkload {
				manifests = append([]*unstructured.Unstructured{options.Workload}, options.Traits...)
			}
			return skipWorkload, manifests
		}

		dispatcher := new(manifestDispatcher)
		dispatcher.healthCheck = func(ctx context.Context, comp *appfile.Component, appRev *v1beta1.ApplicationRevision) (bool, error) {
			skipWorkload, manifests := assembleManifestFn(comp.SkipApplyWorkload)
			if !h.resourceKeeper.ContainsResources(manifests) {
				return false, nil
			}
			_, _, _, isHealth, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
				ByTraitType(readyTraits, options.Traits))
			if err != nil {
				return false, err
			}
			return isHealth, nil
		}
		dispatcher.run = func(ctx context.Context, comp *appfile.Component, appRev *v1beta1.ApplicationRevision, clusterName string) (bool, error) {
			skipWorkload, dispatchManifests := assembleManifestFn(comp.SkipApplyWorkload)

			var isAutoUpdateEnabled bool
			if annotations[oam.AnnotationAutoUpdate] == "true" {
				isAutoUpdateEnabled = true
			}

			// For PostDispatch stage, check if the component workload is healthy first
			if options.Stage == PostDispatch {
				// Check workload health status (without PostDispatch traits)
				workloadStatus, _, _, workloadHealthy, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
					func(trait appfile.Trait) bool {
						// Filter out PostDispatch traits when checking workload health
						traitStage, err := getTraitDispatchStage(h.Client, trait.Name, appRev, annotations)
						return err == nil && traitStage == PostDispatch
					})
				if err != nil {
					return false, errors.WithMessage(err, "CollectHealthStatus for workload")
				}
				
				// If workload is not healthy, don't dispatch PostDispatch traits yet
				// Add pending status for PostDispatch traits
				if !workloadHealthy {
					// Add pending status for PostDispatch traits that are in this dispatcher's options
					for _, traitManifest := range options.Traits {
						traitType := traitManifest.GetLabels()[oam.TraitTypeLabel]
						if traitType != "" {
							pendingStatus := createPendingTraitStatus(traitType)
							workloadStatus.Traits = append(workloadStatus.Traits, pendingStatus)
						}
					}
					h.addServiceStatus(true, *workloadStatus)
					return false, nil
				}
			}

			if isHealth, err := dispatcher.healthCheck(ctx, comp, appRev); !isHealth || err != nil || (!comp.SkipApplyWorkload && isAutoUpdateEnabled) {
				if err := h.Dispatch(ctx, h.Client, clusterName, common.WorkflowResourceCreator, dispatchManifests...); err != nil {
					return false, errors.WithMessage(err, "Dispatch")
				}
				status, _, _, isHealth, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
					ByTraitType(readyTraits, options.Traits))
				if err != nil {
					return false, errors.WithMessage(err, "CollectHealthStatus")
				}
				if options.Stage < DefaultDispatch {
					status.Healthy = false
					if status.Message == "" {
						status.Message = "waiting for previous stage healthy"
					}
					h.addServiceStatus(true, *status)
				}
				if !isHealth {
					return false, nil
				}
			}
			return true, nil
		}
		return dispatcher
	}

	traitStageMap := make(map[StageType][]*unstructured.Unstructured)
	for _, readyTrait := range readyTraits {
		var (
			traitType = readyTrait.GetLabels()[oam.TraitTypeLabel]
			stageType = DefaultDispatch
			err       error
		)
		switch {
		case traitType == definition.AuxiliaryWorkload:
		case traitType != "":
			if strings.Contains(traitType, "-") {
				splitName := traitType[0:strings.LastIndex(traitType, "-")]
				if _, ok := appRev.Spec.TraitDefinitions[splitName]; ok {
					traitType = splitName
				}
			}
			stageType, err = getTraitDispatchStage(h.Client, traitType, appRev, annotations)
			if err != nil {
				return nil, err
			}
		}
		traitStageMap[stageType] = append(traitStageMap[stageType], readyTrait)
	}
	var optionList SortDispatchOptions
	if _, ok := traitStageMap[DefaultDispatch]; !ok {
		traitStageMap[DefaultDispatch] = []*unstructured.Unstructured{}
	}
	for stage, traits := range traitStageMap {
		option := DispatchOptions{
			Stage:             stage,
			Traits:            traits,
			OverrideNamespace: overrideNamespace,
		}
		if stage == DefaultDispatch {
			option.Workload = readyWorkload
		}
		optionList = append(optionList, option)
	}
	sort.Sort(optionList)

	var manifestDispatchers []*manifestDispatcher
	for _, option := range optionList {
		manifestDispatchers = append(manifestDispatchers, dispatcherGenerator(option))
	}
	return manifestDispatchers, nil
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
		Message: "⏳ Waiting for component to be healthy",
	}
}

// addPendingPostDispatchTraits adds pending trait status entries for PostDispatch traits that haven't been collected yet
func (h *AppHandler) addPendingPostDispatchTraits(comp *appfile.Component, status *common.ApplicationComponentStatus) {
	// Build a map of already reported traits
	reportedTraits := make(map[string]bool)
	for _, ts := range status.Traits {
		reportedTraits[ts.Type] = true
	}

	// Check all traits in the component
	for _, trait := range comp.Traits {
		// Skip if already reported
		if reportedTraits[trait.Name] {
			continue
		}
		
		// Check if this trait is PostDispatch
		traitStage, err := getTraitDispatchStage(h.Client, trait.Name, h.currentAppRev, h.app.Annotations)
		if err == nil && traitStage == PostDispatch {
			// Add pending status for this trait
			klog.V(4).Infof("Adding pending status for PostDispatch trait %s", trait.Name)
			traitStatus := createPendingTraitStatus(trait.Name)
			status.Traits = append(status.Traits, traitStatus)
		}
	}
}
