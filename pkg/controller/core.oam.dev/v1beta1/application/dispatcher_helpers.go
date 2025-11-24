package application

import (
	"context"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// buildTraitStageMap groups traits by dispatch stage
func (h *AppHandler) buildTraitStageMap(appRev *v1beta1.ApplicationRevision, readyTraits []*unstructured.Unstructured, annotations map[string]string) (map[StageType][]*unstructured.Unstructured, error) {
	traitStageMap := make(map[StageType][]*unstructured.Unstructured)
	for _, readyTrait := range readyTraits {
		traitType := readyTrait.GetLabels()[oam.TraitTypeLabel]
		stageType := DefaultDispatch
		if traitType == definition.AuxiliaryWorkload { // keep default stage
			traitStageMap[stageType] = append(traitStageMap[stageType], readyTrait)
			continue
		}
		if traitType != "" {
			if strings.Contains(traitType, "-") { // trim suffix if base definition exists
				if idx := strings.LastIndex(traitType, "-"); idx > 0 {
					base := traitType[:idx]
					if _, ok := appRev.Spec.TraitDefinitions[base]; ok {
						traitType = base
					}
				}
			}
			st, err := getTraitDispatchStage(h.Client, traitType, appRev, annotations)
			if err != nil {
				return nil, err
			}
			stageType = st
		}
		traitStageMap[stageType] = append(traitStageMap[stageType], readyTrait)
	}
	if _, ok := traitStageMap[DefaultDispatch]; !ok { // ensure default key exists
		traitStageMap[DefaultDispatch] = []*unstructured.Unstructured{}
	}
	return traitStageMap, nil
}

// newManifestDispatcher constructs a dispatcher for given dispatch options
func (h *AppHandler) newManifestDispatcher(options DispatchOptions, readyTraits []*unstructured.Unstructured, annotations map[string]string) *manifestDispatcher {
	assembleManifestFn := func(skipApplyWorkload bool) (bool, []*unstructured.Unstructured) {
		manifests := options.Traits
		skipWorkload := skipApplyWorkload || options.Workload == nil
		if !skipWorkload {
			manifests = append([]*unstructured.Unstructured{options.Workload}, options.Traits...)
		}
		return skipWorkload, manifests
	}
	md := &manifestDispatcher{}
	md.healthCheck = func(ctx context.Context, comp *appfile.Component, appRev *v1beta1.ApplicationRevision) (bool, error) {
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
	md.run = func(ctx context.Context, comp *appfile.Component, appRev *v1beta1.ApplicationRevision, clusterName string) (bool, error) {
		skipWorkload, dispatchManifests := assembleManifestFn(comp.SkipApplyWorkload)
		isAutoUpdate := annotations[oam.AnnotationAutoUpdate] == "true"
		if options.Stage == PostDispatch { // workload must be healthy before post traits
			workloadStatus, _, _, workloadHealthy, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
				func(trait appfile.Trait) bool {
					stage, err := getTraitDispatchStage(h.Client, trait.Name, appRev, annotations)
					return err == nil && stage == PostDispatch
				})
			if err != nil {
				return false, errors.WithMessage(err, "CollectHealthStatus for workload")
			}
			if !workloadHealthy {
				for _, tm := range options.Traits {
					if tt := tm.GetLabels()[oam.TraitTypeLabel]; tt != "" {
						workloadStatus.Traits = append(workloadStatus.Traits, createPendingTraitStatus(tt))
					}
				}
				h.addServiceStatus(true, *workloadStatus)
				return false, nil
			}
		}
		if isHealth, err := md.healthCheck(ctx, comp, appRev); !isHealth || err != nil || (!comp.SkipApplyWorkload && isAutoUpdate) {
			if err := h.Dispatch(ctx, h.Client, clusterName, common.WorkflowResourceCreator, dispatchManifests...); err != nil {
				return false, errors.WithMessage(err, "Dispatch")
			}
			status, _, _, isHealth, err := h.collectHealthStatus(ctx, comp, options.OverrideNamespace, skipWorkload,
				ByTraitType(readyTraits, options.Traits))
			if err != nil {
				return false, errors.WithMessage(err, "CollectHealthStatus")
			}
			if options.Stage < DefaultDispatch { // mark earlier stage as waiting
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
	return md
}

// generateDispatcherImpl orchestrates dispatcher generation (low complexity)
func (h *AppHandler) generateDispatcherImpl(appRev *v1beta1.ApplicationRevision, readyWorkload *unstructured.Unstructured, readyTraits []*unstructured.Unstructured, overrideNamespace string, annotations map[string]string) ([]*manifestDispatcher, error) {
	traitStageMap, err := h.buildTraitStageMap(appRev, readyTraits, annotations)
	if err != nil {
		return nil, err
	}
	var optionList SortDispatchOptions
	for stage, traits := range traitStageMap {
		opt := DispatchOptions{Stage: stage, Traits: traits, OverrideNamespace: overrideNamespace}
		if stage == DefaultDispatch {
			opt.Workload = readyWorkload
		}
		optionList = append(optionList, opt)
	}
	sort.Sort(optionList)
	var dispatchers []*manifestDispatcher
	for _, opt := range optionList {
		dispatchers = append(dispatchers, h.newManifestDispatcher(opt, readyTraits, annotations))
	}
	return dispatchers, nil
}
