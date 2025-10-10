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
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
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
	// EmptyTraitType indicates a resource is not a trait (used for filtering outputs)
	EmptyTraitType = ""
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
	run         func(ctx context.Context, c *appfile.Component, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string) (bool, error)
	healthCheck func(ctx context.Context, c *appfile.Component, appRev *v1beta1.ApplicationRevision) (bool, error)
	stage       StageType
}

func (h *AppHandler) generateDispatcher(appRev *v1beta1.ApplicationRevision, readyWorkload *unstructured.Unstructured, readyTraits []*unstructured.Unstructured, manifest *types.ComponentManifest, overrideNamespace string, annotations map[string]string) ([]*manifestDispatcher, error) {
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
		dispatcher.stage = options.Stage
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
		dispatcher.run = func(ctx context.Context, comp *appfile.Component, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string) (bool, error) {
			if err := h.handlePostDispatchStage(ctx, comp, manifest, appRev, clusterName, &options); err != nil {
				return false, err
			}
			skipWorkload, dispatchManifests := assembleManifestFn(comp.SkipApplyWorkload)
			return h.handleDispatchAndHealthCollection(ctx, comp, manifest, options, readyTraits, dispatchManifests, skipWorkload, dispatcher, annotations, clusterName, appRev)
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

	if manifest != nil && len(manifest.DeferredTraits) > 0 {
		if _, ok := traitStageMap[PostDispatch]; !ok {
			traitStageMap[PostDispatch] = []*unstructured.Unstructured{}
		}
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

// fetchComponentOutputsStatus fetches status for all component outputs
func (h *AppHandler) fetchComponentOutputsStatus(ctx context.Context, outputs []*unstructured.Unstructured, clusterName string, overrideNamespace string) (map[string]interface{}, error) {
	outputsStatus := make(map[string]interface{})

	if clusterName != "" && clusterName != pkgmulticluster.Local {
		ctx = pkgmulticluster.WithCluster(ctx, clusterName)
	}

	for _, output := range outputs {
		if output == nil {
			continue
		}

		namespace := oamutil.ResolveNamespace(output.GetNamespace(), overrideNamespace)
		outputName := output.GetLabels()[oam.TraitResource]
		if outputName == "" {
			outputName = strings.ToLower(output.GetKind())
		}

		currentOutput, err := oamutil.GetObjectGivenGVKAndName(ctx, h.Client, output.GroupVersionKind(), namespace, output.GetName())
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				klog.Warningf("Failed to get output %s/%s: %v", namespace, output.GetName(), err)
			}
			continue
		}

		outputsStatus[outputName] = currentOutput.Object
	}

	return outputsStatus, nil
}

// fetchComponentStatus gets component status from cluster
func (h *AppHandler) fetchComponentStatus(ctx context.Context, workload *unstructured.Unstructured, clusterName string, overrideNamespace string) (map[string]interface{}, error) {
	if workload == nil {
		return nil, errors.New("component workload is nil")
	}

	namespace := oamutil.ResolveNamespace(workload.GetNamespace(), overrideNamespace)

	if clusterName != "" && clusterName != pkgmulticluster.Local {
		ctx = pkgmulticluster.WithCluster(ctx, clusterName)
	}

	currentWorkload, err := oamutil.GetObjectGivenGVKAndName(ctx, h.Client, workload.GroupVersionKind(), namespace, workload.GetName())
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrapf(err, "failed to get workload %s/%s", namespace, workload.GetName())
		}
		return map[string]interface{}{}, nil
	}

	return extractStatusFromObject(currentWorkload.Object)
}

func extractStatusFromObject(obj map[string]interface{}) (map[string]interface{}, error) {
	status, found, err := unstructured.NestedFieldNoCopy(obj, "status")
	if err != nil {
		return map[string]interface{}{}, err
	}
	if !found {
		return map[string]interface{}{}, nil
	}

	statusMap, ok := status.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}, nil
	}

	return statusMap, nil
}

// createPendingTraitStatus returns a pending trait status
func createPendingTraitStatus(traitName string) common.ApplicationTraitStatus {
	return common.ApplicationTraitStatus{
		Type:    traitName,
		Healthy: false,
		Pending: true,
		Stage:   "PostDispatch",
		Message: "Waiting for component to be healthy",
	}
}

// createTraitStatus returns a trait status with stage information
func createTraitStatus(traitName string, healthy bool, pending bool, stage string, message string) common.ApplicationTraitStatus {
	return common.ApplicationTraitStatus{
		Type:    traitName,
		Healthy: healthy,
		Pending: pending,
		Stage:   stage,
		Message: message,
	}
}

// determineWorkloadForStatus selects the appropriate workload for status fetching
func (h *AppHandler) determineWorkloadForStatus(options *DispatchOptions, manifest *types.ComponentManifest) *unstructured.Unstructured {
	if options.Workload != nil {
		return options.Workload
	}
	if manifest != nil && manifest.ComponentOutput != nil {
		return manifest.ComponentOutput
	}
	return nil
}

// filterOutputsFromManifest extracts outputs (non-traits) from component manifest resources
func filterOutputsFromManifest(resources []*unstructured.Unstructured) []*unstructured.Unstructured {
	var outputs []*unstructured.Unstructured
	for _, res := range resources {
		if res != nil && res.GetLabels()[oam.TraitTypeLabel] == EmptyTraitType {
			// This is an output, not a trait
			outputs = append(outputs, res)
		}
	}
	return outputs
}

// fetchOutputsForPostDispatch extracts and fetches status for component outputs
func (h *AppHandler) fetchOutputsForPostDispatch(ctx context.Context, manifest *types.ComponentManifest, clusterName string, overrideNamespace string) (map[string]interface{}, error) {
	if manifest == nil || len(manifest.ComponentOutputsAndTraits) == 0 {
		return nil, nil
	}

	outputs := filterOutputsFromManifest(manifest.ComponentOutputsAndTraits)
	if len(outputs) == 0 {
		return nil, nil
	}

	return h.fetchComponentOutputsStatus(ctx, outputs, clusterName, overrideNamespace)
}

// processRenderedTraits adds rendered traits to the dispatch options and manifest
func (h *AppHandler) processRenderedTraits(renderedTraits []*unstructured.Unstructured, options *DispatchOptions, manifest *types.ComponentManifest) {
	options.Traits = renderedTraits

	if manifest != nil {
		manifest.ComponentOutputsAndTraits = append(manifest.ComponentOutputsAndTraits, renderedTraits...)
		manifest.DeferredTraits = nil // cleared after processing
	}
}

// handlePostDispatchStage renders deferred traits with component status
func (h *AppHandler) handlePostDispatchStage(ctx context.Context, comp *appfile.Component, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string, options *DispatchOptions) error {
	if options.Stage != PostDispatch || manifest == nil || len(manifest.DeferredTraits) == 0 {
		return nil
	}

	workloadForStatus := h.determineWorkloadForStatus(options, manifest)

	componentStatus, err := h.fetchComponentStatus(ctx, workloadForStatus, clusterName, options.OverrideNamespace)
	if err != nil {
		klog.Errorf("Failed to fetch component status for PostDispatch traits: %v", err)
		return errors.Wrap(err, "failed to fetch component status for PostDispatch traits")
	}

	outputsStatus, err := h.fetchOutputsForPostDispatch(ctx, manifest, clusterName, options.OverrideNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to fetch outputs status for PostDispatch traits")
	}

	renderedTraits, err := h.renderPostDispatchTraits(ctx, comp, componentStatus, outputsStatus, manifest.DeferredTraits, appRev, options.OverrideNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to render PostDispatch traits")
	}

	h.processRenderedTraits(renderedTraits, options, manifest)

	return nil
}

// extractHealthFromStatus checks health indicators in status
func extractHealthFromStatus(statusMap map[string]interface{}) (healthy bool, message string) {
	healthy = true
	message = ""

	if ready, found, _ := unstructured.NestedBool(statusMap, "ready"); found && !ready {
		return false, "Resource is not ready"
	}

	if phase, found, _ := unstructured.NestedString(statusMap, "phase"); found {
		if phase == "Failed" || phase == "Error" {
			return false, fmt.Sprintf("Resource phase is %s", phase)
		}
	}

	if msg, found, _ := unstructured.NestedString(statusMap, "message"); found && msg != "" {
		message = msg
	}

	return healthy, message
}

// evaluateTraitHealth checks if a trait is healthy
func (h *AppHandler) evaluateTraitHealth(ctx context.Context, trait *unstructured.Unstructured) (bool, string) {
	currentTrait, err := oamutil.GetObjectGivenGVKAndName(ctx, h.Client, trait.GroupVersionKind(), trait.GetNamespace(), trait.GetName())
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return false, fmt.Sprintf("Failed to get trait: %v", err)
		}
		return false, "Trait not found"
	}

	if statusObj, found, _ := unstructured.NestedFieldNoCopy(currentTrait.Object, "status"); found {
		if statusMap, ok := statusObj.(map[string]interface{}); ok {
			healthy, message := extractHealthFromStatus(statusMap)
			return healthy, message
		}
	}

	// For PostDispatch traits, if there's no status yet, they should not be considered healthy
	// They need proper health policy evaluation which happens in the next reconciliation
	return false, "Trait health pending evaluation"
}

// addPendingTraitStatuses adds pending trait status entries
func (h *AppHandler) addPendingTraitStatuses(manifest *types.ComponentManifest, status *common.ApplicationComponentStatus, stage StageType) {
	if stage == PostDispatch || manifest == nil || len(manifest.DeferredTraits) == 0 {
		return
	}

	for _, deferredTrait := range manifest.DeferredTraits {
		trait, ok := deferredTrait.(*appfile.Trait)
		if !ok {
			klog.Warningf("Expected *appfile.Trait but got %T", deferredTrait)
			continue
		}
		traitStatus := createPendingTraitStatus(trait.Name)
		status.Traits = append(status.Traits, traitStatus)
	}
}

// handlePostDispatchTraitStatuses updates trait statuses for PostDispatch stage
func (h *AppHandler) handlePostDispatchTraitStatuses(ctx context.Context, options DispatchOptions, status *common.ApplicationComponentStatus) bool {
	if options.Stage != PostDispatch || len(options.Traits) == 0 {
		return true
	}

	deployedTraitNames := make(map[string]bool)
	for _, trait := range options.Traits {
		if traitName := trait.GetLabels()[oam.TraitTypeLabel]; traitName != "" {
			deployedTraitNames[traitName] = true
		}
	}

	var filteredTraits []common.ApplicationTraitStatus
	for _, existingTrait := range status.Traits {
		if deployedTraitNames[existingTrait.Type] && existingTrait.Pending {
			continue
		}
		filteredTraits = append(filteredTraits, existingTrait)
	}
	status.Traits = filteredTraits

	isHealth := true
	for _, trait := range options.Traits {
		traitName := trait.GetLabels()[oam.TraitTypeLabel]
		if traitName != "" {
			traitHealthy, traitMessage := h.evaluateTraitHealth(ctx, trait)

			// PostDispatch traits should be marked as pending until health evaluation completes
			// They need proper health policy evaluation which happens in subsequent reconciliations
			isPending := true
			if traitHealthy {
				// Only mark as not pending if the trait is actually healthy
				isPending = false
			}
			traitStatus := createTraitStatus(traitName, traitHealthy, isPending, stages[options.Stage], traitMessage)
			status.Traits = append(status.Traits, traitStatus)

			if !traitHealthy {
				isHealth = false
				if status.Message == "" {
					status.Message = traitMessage
				}
			}
		}
	}

	return isHealth
}

// handleDispatchAndHealthCollection dispatches resources and collects health
func (h *AppHandler) handleDispatchAndHealthCollection(ctx context.Context, comp *appfile.Component, manifest *types.ComponentManifest, options DispatchOptions, readyTraits []*unstructured.Unstructured, dispatchManifests []*unstructured.Unstructured, skipWorkload bool, dispatcher *manifestDispatcher, annotations map[string]string, clusterName string, appRev *v1beta1.ApplicationRevision) (bool, error) {
	var isAutoUpdateEnabled bool
	if annotations[oam.AnnotationAutoUpdate] == "true" {
		isAutoUpdateEnabled = true
	}

	if isHealth, err := dispatcher.healthCheck(ctx, comp, appRev); !isHealth || err != nil || (!comp.SkipApplyWorkload && isAutoUpdateEnabled) {
		if err := h.Dispatch(ctx, h.Client, clusterName, common.WorkflowResourceCreator, dispatchManifests...); err != nil {
			return false, errors.Wrap(err, "failed to dispatch manifests")
		}

		status, _, _, isHealth, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
			ByTraitType(readyTraits, options.Traits))
		if err != nil {
			return false, errors.Wrap(err, "failed to collect health status")
		}

		if options.Stage < DefaultDispatch {
			status.Healthy = false
			if status.Message == "" {
				status.Message = "waiting for previous stage healthy"
			}
		}

		h.addPendingTraitStatuses(manifest, status, options.Stage)

		traitHealthy := h.handlePostDispatchTraitStatuses(ctx, options, status)
		if !traitHealthy {
			isHealth = false
		}

		h.addServiceStatus(true, *status)
		if !isHealth {
			return false, nil
		}
	}

	return true, nil
}
