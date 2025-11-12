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
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
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
	DeferredTraits    []*appfile.Trait
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

	var deferredTraitDefs []*appfile.Trait
	if manifest != nil && len(manifest.DeferredTraits) > 0 {
		if _, ok := traitStageMap[PostDispatch]; !ok {
			traitStageMap[PostDispatch] = []*unstructured.Unstructured{}
		}
		deferredTraitDefs = convertDeferredTraits(manifest.DeferredTraits)
	} else if manifest != nil && len(manifest.ProcessedDeferredTraits) > 0 {
		if _, ok := traitStageMap[PostDispatch]; !ok {
			traitStageMap[PostDispatch] = []*unstructured.Unstructured{}
		}
		deferredTraitDefs = convertDeferredTraits(manifest.ProcessedDeferredTraits)
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
		if stage == PostDispatch && len(deferredTraitDefs) > 0 {
			option.DeferredTraits = deferredTraitDefs
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
		outputName := outputKeyForResource(output)

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
		return map[string]interface{}{}, nil
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
		Message: "Waiting for component to be healthy",
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

// convertDeferredTraits safely converts the manifest deferred traits to appfile traits
func convertDeferredTraits(items []interface{}) []*appfile.Trait {
	var traits []*appfile.Trait
	for _, item := range items {
		if item == nil {
			continue
		}
		if trait, ok := item.(*appfile.Trait); ok {
			traits = append(traits, trait)
		}
	}
	return traits
}

// buildPostDispatchTraitDefinitionMap collects trait definitions from dispatch options or manifest caches.
func buildPostDispatchTraitDefinitionMap(options DispatchOptions, manifest *types.ComponentManifest) map[string]*appfile.Trait {
	traitDefinitionMap := make(map[string]*appfile.Trait)
	for _, tr := range options.DeferredTraits {
		if tr == nil {
			continue
		}
		traitDefinitionMap[tr.Name] = tr
	}
	if len(traitDefinitionMap) == 0 && manifest != nil {
		for _, tr := range convertDeferredTraits(manifest.DeferredTraits) {
			if tr == nil {
				continue
			}
			traitDefinitionMap[tr.Name] = tr
		}
		for _, tr := range convertDeferredTraits(manifest.ProcessedDeferredTraits) {
			if tr == nil {
				continue
			}
			traitDefinitionMap[tr.Name] = tr
		}
	}
	return traitDefinitionMap
}

func extractHealthFromStatus(statusMap map[string]interface{}) (healthy bool, message string) {
	healthy = true
	message = ""

	setUnhealthy := func(msg string) {
		if healthy {
			healthy = false
		}
		if msg != "" && message == "" {
			message = msg
		}
	}

	if ready, found, _ := unstructured.NestedBool(statusMap, "ready"); found && !ready {
		return false, "Resource is not ready"
	}

	if phase, found, _ := unstructured.NestedString(statusMap, "phase"); found {
		if phase == "Failed" || phase == "Error" {
			return false, fmt.Sprintf("Resource phase is %s", phase)
		}
	}

	if desired, foundDesired, _ := unstructured.NestedInt64(statusMap, "replicas"); foundDesired {
		if readyReplicas, foundReady, _ := unstructured.NestedInt64(statusMap, "readyReplicas"); foundReady && readyReplicas < desired {
			setUnhealthy(fmt.Sprintf("%d/%d replicas are ready", readyReplicas, desired))
		}
		if availableReplicas, foundAvailable, _ := unstructured.NestedInt64(statusMap, "availableReplicas"); foundAvailable && availableReplicas < desired {
			setUnhealthy(fmt.Sprintf("%d/%d replicas are available", availableReplicas, desired))
		}
	}

	if conds, found, _ := unstructured.NestedSlice(statusMap, "conditions"); found {
		for _, cond := range conds {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			statusVal, _ := condMap["status"].(string)
			if strings.EqualFold(statusVal, "True") {
				continue
			}

			condMsg := ""
			if msgVal, ok := condMap["message"].(string); ok && msgVal != "" {
				condMsg = msgVal
			} else if reasonVal, ok := condMap["reason"].(string); ok && reasonVal != "" {
				condMsg = reasonVal
			} else if typeVal, ok := condMap["type"].(string); ok && typeVal != "" {
				condMsg = fmt.Sprintf("%s condition status is %s", typeVal, statusVal)
			}

			setUnhealthy(condMsg)
		}
	}

	if msg, found, _ := unstructured.NestedString(statusMap, "message"); found && msg != "" {
		if message == "" {
			message = msg
		}
	}

	if !healthy && message == "" {
		message = "Resource is not healthy"
	}

	return healthy, message
}

// filterOutputsFromManifest extracts resources produced by component outputs and trait assists.
func filterOutputsFromManifest(resources []*unstructured.Unstructured) []*unstructured.Unstructured {
	var outputs []*unstructured.Unstructured
	for _, res := range resources {
		if res == nil {
			continue
		}

		labels := res.GetLabels()
		if labels[oam.TraitTypeLabel] == EmptyTraitType || labels[oam.TraitResource] != "" {
			outputs = append(outputs, res)
		}
	}
	return outputs
}

func outputKeyForResource(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}

	if name := obj.GetLabels()[oam.TraitResource]; name != "" {
		return name
	}

	return strings.ToLower(obj.GetKind())
}

func mapKeys(m map[string]interface{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
		if len(options.DeferredTraits) > 0 {
			var retained []interface{}
			for _, tr := range options.DeferredTraits {
				retained = append(retained, tr)
			}
			manifest.ProcessedDeferredTraits = retained
		}
		manifest.DeferredTraits = nil // cleared after processing to avoid re-rendering
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

	return true, ""
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
func (h *AppHandler) handlePostDispatchTraitStatuses(ctx context.Context, comp *appfile.Component, manifest *types.ComponentManifest, options DispatchOptions, status *common.ApplicationComponentStatus, clusterName string, appRev *v1beta1.ApplicationRevision) bool {
	if options.Stage != PostDispatch {
		return true
	}

	traitDefs := buildPostDispatchTraitDefinitionMap(options, manifest)
	tracked := collectTrackedPostDispatchTraits(options.Traits, traitDefs)
	if len(tracked) == 0 {
		return true
	}
	pruneExistingTraitStatuses(status, tracked)

	var componentStatus, outputsStatus map[string]interface{}
	if len(traitDefs) > 0 {
		componentStatus, outputsStatus = h.fetchPostDispatchBaseStatuses(ctx, manifest, options, clusterName)
	}

	traitOutputs := groupTraitOutputsForPostDispatch(manifest, traitDefs)

	isHealth := true
	processed := make(map[string]bool)

	appendStatus := func(name string, ts common.ApplicationTraitStatus) {
		ts.Type = name
		status.Traits = append(status.Traits, ts)
		if !ts.Healthy {
			isHealth = false
			if status.Message == "" {
				status.Message = ts.Message
			}
		}
	}

	// Evaluate traits present in options
	for _, trObj := range options.Traits {
		if trObj == nil {
			continue
		}
		label := trObj.GetLabels()[oam.TraitTypeLabel]
		traitName, traitDef := resolvePostDispatchTraitName(label, traitDefs)
		if traitName == "" {
			continue
		}
		ts := h.evaluateSinglePostDispatchTrait(ctx, comp, trObj, traitName, traitDef, appRev, options, componentStatus, outputsStatus)
		overridePostDispatchTraitStatusWithOutputs(ctx, h, traitOutputs[traitName], &ts, options.OverrideNamespace, clusterName)
		klog.Infof("PostDispatch trait %s evaluated status healthy=%v message=%q", traitName, ts.Healthy, ts.Message)
		appendStatus(traitName, ts)
		processed[traitName] = true
	}

	// Remaining definitions not instantiated yet
	for name, def := range traitDefs {
		if processed[name] {
			continue
		}
		ts := h.evaluateSinglePostDispatchTrait(ctx, comp, nil, name, def, appRev, options, componentStatus, outputsStatus)
		overridePostDispatchTraitStatusWithOutputs(ctx, h, traitOutputs[name], &ts, options.OverrideNamespace, clusterName)
		klog.Infof("PostDispatch trait %s evaluated status healthy=%v message=%q", name, ts.Healthy, ts.Message)
		appendStatus(name, ts)
	}

	return isHealth
}

// resolvePostDispatchTraitName resolves the base trait name from label and definitions.
func resolvePostDispatchTraitName(label string, defs map[string]*appfile.Trait) (string, *appfile.Trait) {
	if label == "" {
		return "", nil
	}
	if tr, ok := defs[label]; ok {
		return label, tr
	}
	name := label
	for strings.Contains(name, "-") {
		name = name[:strings.LastIndex(name, "-")]
		if tr, ok := defs[name]; ok {
			return name, tr
		}
	}
	return label, nil
}

func collectTrackedPostDispatchTraits(traits []*unstructured.Unstructured, defs map[string]*appfile.Trait) map[string]bool {
	tracked := make(map[string]bool)
	for _, t := range traits {
		if t == nil {
			continue
		}
		name, _ := resolvePostDispatchTraitName(t.GetLabels()[oam.TraitTypeLabel], defs)
		if name != "" {
			tracked[name] = true
		}
	}
	for def := range defs {
		tracked[def] = true
	}
	return tracked
}

func pruneExistingTraitStatuses(status *common.ApplicationComponentStatus, tracked map[string]bool) {
	if status == nil || len(status.Traits) == 0 {
		return
	}
	var kept []common.ApplicationTraitStatus
	for _, ts := range status.Traits {
		if !tracked[ts.Type] {
			kept = append(kept, ts)
		}
	}
	status.Traits = kept
}

func (h *AppHandler) fetchPostDispatchBaseStatuses(ctx context.Context, manifest *types.ComponentManifest, options DispatchOptions, clusterName string) (componentStatus map[string]interface{}, outputsStatus map[string]interface{}) {
	workloadForStatus := h.determineWorkloadForStatus(&options, manifest)
	if workloadForStatus != nil {
		if s, err := h.fetchComponentStatus(ctx, workloadForStatus, clusterName, options.OverrideNamespace); err == nil {
			componentStatus = s
		} else {
			klog.Warningf("Failed to fetch component status for PostDispatch trait health: %v", err)
		}
	}
	if out, err := h.fetchOutputsForPostDispatch(ctx, manifest, clusterName, options.OverrideNamespace); err == nil {
		outputsStatus = out
	} else {
		klog.Warningf("Failed to fetch outputs status for PostDispatch trait health: %v", err)
	}
	return
}

func groupTraitOutputsForPostDispatch(manifest *types.ComponentManifest, defs map[string]*appfile.Trait) map[string][]*unstructured.Unstructured {
	out := map[string][]*unstructured.Unstructured{}
	if manifest == nil {
		return out
	}
	for _, res := range manifest.ComponentOutputsAndTraits {
		if res == nil {
			continue
		}
		base, _ := resolvePostDispatchTraitName(res.GetLabels()[oam.TraitTypeLabel], defs)
		if base == "" {
			continue
		}
		out[base] = append(out[base], res)
	}
	return out
}

func (h *AppHandler) evaluateSinglePostDispatchTrait(ctx context.Context, comp *appfile.Component, traitObj *unstructured.Unstructured, traitName string, traitDef *appfile.Trait, appRev *v1beta1.ApplicationRevision, options DispatchOptions, componentStatus, outputsStatus map[string]interface{}) common.ApplicationTraitStatus {
	var ts common.ApplicationTraitStatus
	if traitDef != nil {
		ts = h.evaluatePostDispatchTraitWithPolicy(comp, traitName, traitDef, appRev, options, componentStatus, outputsStatus)
		// fallback check real object if policy says healthy
		if ts.Healthy && traitObj != nil {
			fh, fm := h.evaluateTraitHealth(ctx, traitObj)
			if !fh {
				ts.Healthy = false
				if ts.Message == "" {
					if fm != "" {
						ts.Message = fm
					} else {
						ts.Message = "trait resource is not healthy"
					}
				}
			}
		}
	} else {
		// no definition
		fh, fm := h.evaluateTraitHealth(ctx, traitObj)
		ts = common.ApplicationTraitStatus{Healthy: fh, Message: fm}
	}
	return ts
}

func overridePostDispatchTraitStatusWithOutputs(ctx context.Context, h *AppHandler, outputs []*unstructured.Unstructured, statusEntry *common.ApplicationTraitStatus, overrideNS, clusterName string) {
	if statusEntry == nil || len(outputs) == 0 {
		return
	}
	for _, res := range outputs {
		if res == nil {
			continue
		}
		ns := oamutil.ResolveNamespace(res.GetNamespace(), overrideNS)
		fetchCtx := ctx
		if clusterName != "" && clusterName != pkgmulticluster.Local {
			fetchCtx = pkgmulticluster.WithCluster(fetchCtx, clusterName)
		}
		currentObj, err := oamutil.GetObjectGivenGVKAndName(fetchCtx, h.Client, res.GroupVersionKind(), ns, res.GetName())
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				statusEntry.Healthy = false
				if statusEntry.Message == "" {
					statusEntry.Message = fmt.Sprintf("failed to get trait resource %s/%s: %v", ns, res.GetName(), err)
				}
				return
			}
			statusEntry.Healthy = false
			if statusEntry.Message == "" {
				statusEntry.Message = fmt.Sprintf("trait resource %s/%s not found", ns, res.GetName())
			}
			return
		}
		statusMap, found, err := unstructured.NestedMap(currentObj.Object, "status")
		if err != nil || !found {
			continue
		}
		resHealthy, resMsg := extractHealthFromStatus(statusMap)
		if !resHealthy {
			statusEntry.Healthy = false
			if statusEntry.Message == "" {
				statusEntry.Message = resMsg
			}
			return
		}
	}
}

// evaluatePostDispatchTraitWithPolicy evaluates PostDispatch trait health using the trait definition health policy
func (h *AppHandler) evaluatePostDispatchTraitWithPolicy(comp *appfile.Component, traitName string, trait *appfile.Trait, appRev *v1beta1.ApplicationRevision, options DispatchOptions, componentStatus map[string]interface{}, outputsStatus map[string]interface{}) common.ApplicationTraitStatus {
	traitStatus := common.ApplicationTraitStatus{
		Type:    traitName,
		Healthy: true,
	}

	if trait == nil {
		traitStatus.Healthy = false
		traitStatus.Message = "trait definition not found for PostDispatch health evaluation"
		return traitStatus
	}

	namespace := h.app.Namespace
	if options.OverrideNamespace != "" {
		namespace = options.OverrideNamespace
	}

	appRevName := ""
	if appRev != nil {
		appRevName = appRev.Name
	}

	af := &appfile.Appfile{
		Name:            h.app.Name,
		Namespace:       namespace,
		AppRevisionName: appRevName,
	}

	ctxData := appfile.GenerateContextDataFromAppFile(af, comp.Name)
	if options.OverrideNamespace != "" {
		ctxData.Namespace = options.OverrideNamespace
	}

	pCtx := appfile.NewBasicContext(ctxData, comp.Params)
	pCtx.SetCtx(comp.Ctx.GetCtx())

	if componentStatus != nil {
		pCtx.PushData("output", map[string]interface{}{"status": componentStatus})
	}
	if len(outputsStatus) > 0 {
		pCtx.PushData("outputs", outputsStatus)
	}
	if klog.V(4).Enabled() {
		keys := make([]string, 0, len(outputsStatus))
		for k := range outputsStatus {
			keys = append(keys, k)
		}
		klog.Infof("PostDispatch trait %s evaluating with componentStatusKeys=%v outputs keys=%v", traitName, mapKeys(componentStatus), keys)
	}
	pCtx.PushData(velaprocess.ContextComponentType, comp.Type)
	// keep legacy context.type for templates that still reference it
	pCtx.PushData("type", comp.Type)

	if err := trait.EvalContext(pCtx); err != nil {
		traitStatus.Healthy = false
		traitStatus.Message = fmt.Sprintf("failed to evaluate PostDispatch trait context: %v", err)
		return traitStatus
	}

	traitOverrideNamespace := options.OverrideNamespace
	if trait.FullTemplate != nil && trait.FullTemplate.TraitDefinition.Spec.ControlPlaneOnly {
		traitOverrideNamespace = h.app.Namespace
		if appRev != nil {
			traitOverrideNamespace = appRev.GetNamespace()
		}
		pCtx.SetCtx(pkgmulticluster.WithCluster(pCtx.GetCtx(), pkgmulticluster.Local))
	}

	accessor := oamutil.NewApplicationResourceNamespaceAccessor(h.app.Namespace, traitOverrideNamespace)
	templateContext, err := trait.GetTemplateContext(pCtx, h.Client, accessor)
	if err != nil {
		traitStatus.Healthy = false
		traitStatus.Message = fmt.Sprintf("failed to prepare health context: %v", err)
		return traitStatus
	}

	statusResult, err := trait.EvalStatus(templateContext)
	if err != nil {
		traitStatus.Healthy = false
		traitStatus.Message = fmt.Sprintf("failed to evaluate health policy: %v", err)
		return traitStatus
	}

	if statusResult != nil {
		traitStatus.Healthy = statusResult.Healthy
		traitStatus.Message = statusResult.Message
		traitStatus.Details = statusResult.Details
	}

	return traitStatus
}

// handleDispatchAndHealthCollection dispatches resources and collects health
func (h *AppHandler) handleDispatchAndHealthCollection(ctx context.Context, comp *appfile.Component, manifest *types.ComponentManifest, options DispatchOptions, readyTraits []*unstructured.Unstructured, dispatchManifests []*unstructured.Unstructured, skipWorkload bool, dispatcher *manifestDispatcher, annotations map[string]string, clusterName string, appRev *v1beta1.ApplicationRevision) (bool, error) {
	var isAutoUpdateEnabled bool
	if annotations[oam.AnnotationAutoUpdate] == "true" {
		isAutoUpdateEnabled = true
	}

	isHealth, err := dispatcher.healthCheck(ctx, comp, appRev)
	if err != nil {
		return false, err
	}

	needDispatch := !isHealth || (!comp.SkipApplyWorkload && isAutoUpdateEnabled)
	if needDispatch {
		if err := h.Dispatch(ctx, h.Client, clusterName, common.WorkflowResourceCreator, dispatchManifests...); err != nil {
			return false, errors.Wrap(err, "failed to dispatch manifests")
		}
	}

	status, _, _, isHealthAfterCollect, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
		ByTraitType(readyTraits, options.Traits))
	if err != nil {
		return false, errors.Wrap(err, "failed to collect health status")
	}
	isHealth = isHealthAfterCollect

	if options.Stage < DefaultDispatch {
		status.Healthy = false
		if status.Message == "" {
			status.Message = "waiting for previous stage healthy"
		}
	}

	h.addPendingTraitStatuses(manifest, status, options.Stage)

	traitHealthy := h.handlePostDispatchTraitStatuses(ctx, comp, manifest, options, status, clusterName, appRev)
	if !traitHealthy {
		isHealth = false
	}
	status.Healthy = status.Healthy && traitHealthy

	h.addServiceStatus(true, *status)
	if !isHealth {
		return false, nil
	}

	return true, nil
}
