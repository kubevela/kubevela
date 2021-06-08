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

package applicationrollout

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/event"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationrollout"
)

type rolloutHandler struct {
	*Reconciler
	appRollout *v1beta1.AppRollout

	// source/targetRevName represent this round reconcile using source and target revision
	// in most cases they are equal to appRollout.spec.target/sourceRevName but if roll forward or revert in middle of rollout
	// source/targetRevName are equal to previous rollout
	sourceRevName string
	targetRevName string

	sourceAppRevision *v1beta1.ApplicationRevision
	targetAppRevision *v1beta1.ApplicationRevision

	// sourceWorkloads is assembled by appRevision in assemble phase
	// please be aware that they are real status in k8s, they are just generate from appRevision include GVK+namespace+name
	sourceWorkloads map[string]*unstructured.Unstructured
	targetWorkloads map[string]*unstructured.Unstructured

	// targetManifests used by dispatch(template targetRevision) and handleSucceed(GC) phase
	targetManifests []*unstructured.Unstructured

	// needRollComponent is find common component between source and target revision
	needRollComponent string
}

// prepareRollout  call assemble func to prepare info needed in whole reconcile loop
func (h *rolloutHandler) prepareRollout(ctx context.Context) error {
	var err error
	h.targetAppRevision = new(v1beta1.ApplicationRevision)
	if err := h.Get(ctx, types.NamespacedName{Namespace: h.appRollout.Namespace, Name: h.targetRevName}, h.targetAppRevision); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// construct a assemble manifest for targetAppRevision
	targetAssemble := assemble.NewAppManifests(h.targetAppRevision).
		WithWorkloadOption(rolloutWorkloadName()).
		WithWorkloadOption(assemble.PrepareWorkloadForRollout())

	// in template phase, we should use targetManifests including target workloads/traits to
	h.targetManifests, err = targetAssemble.AssembledManifests()
	if err != nil {
		klog.Error("appRollout targetAppRevision failed to assemble manifest", "appRollout", klog.KRef(h.appRollout.Namespace, h.appRollout.Name))
		return err
	}

	// we only use workloads group by component name to find out witch same workload in source and target worklaod
	h.targetWorkloads, _, _, err = targetAssemble.GroupAssembledManifests()
	if err != nil {
		klog.Error("appRollout targetAppRevision failed to assemble target workload", "appRollout", klog.KRef(h.appRollout.Namespace, h.appRollout.Name))
		return err
	}

	if len(h.sourceRevName) != 0 {
		h.sourceAppRevision = new(v1beta1.ApplicationRevision)
		if err := h.Get(ctx, types.NamespacedName{Namespace: h.appRollout.Namespace, Name: h.sourceRevName}, h.sourceAppRevision); err != nil && apierrors.IsNotFound(err) {
			return err
		}
		// construct a assemble manifest for sourceAppRevision
		sourceAssemble := assemble.NewAppManifests(h.sourceAppRevision).
			WithWorkloadOption(assemble.PrepareWorkloadForRollout()).
			WithWorkloadOption(rolloutWorkloadName())
		h.sourceWorkloads, _, _, err = sourceAssemble.GroupAssembledManifests()
		if err != nil {
			klog.Error("appRollout sourceAppRevision failed to assemble workloads", "appRollout", klog.KRef(h.appRollout.Namespace, h.appRollout.Name))
			return err
		}
	}
	return nil
}

// we only support one workload now, so this func is to determine witch component is need to rollout
func (h *rolloutHandler) determineRolloutComponent() error {
	componentList := h.appRollout.Spec.ComponentList
	// if user not set ComponentList in AppRollout we also find a common component between source and target
	if len(componentList) == 0 {
		// we need to find a default component
		commons := appUtil.FindCommonComponentWithManifest(h.targetWorkloads, h.sourceWorkloads)
		if len(commons) != 1 {
			return fmt.Errorf("cannot find a default component, too many common components: %+v", commons)
		}
		h.needRollComponent = commons[0]
	} else {
		// assume that the validator webhook has already guaranteed that there is no more than one component for now
		// and the component exists in both the target and source app
		h.needRollComponent = componentList[0]
	}
	return nil
}

// fetch source and target workload
func (h *rolloutHandler) fetchSourceAndTargetWorkload(ctx context.Context) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	var sourceWorkload, targetWorkload *unstructured.Unstructured
	var err error
	if len(h.sourceRevName) == 0 {
		klog.Info("source app fields not filled, this is a scale operation")
	} else if sourceWorkload, err = h.extractWorkload(ctx, *h.sourceWorkloads[h.needRollComponent]); err != nil {
		klog.Error("specified sourceRevName bug cannot fetch source workload")
		return nil, nil, err
	}
	if targetWorkload, err = h.extractWorkload(ctx, *h.targetWorkloads[h.needRollComponent]); err != nil {
		return nil, nil, err
	}
	return sourceWorkload, targetWorkload, nil
}

// extractWorkload use GVK and name of workload(assembled result) to fetch real workload in cluster
func (h *rolloutHandler) extractWorkload(ctx context.Context, workload unstructured.Unstructured) (*unstructured.Unstructured, error) {
	wl, err := oamutil.GetObjectGivenGVKAndName(ctx, h, workload.GroupVersionKind(), workload.GetNamespace(), workload.GetName())
	if err != nil {
		return nil, err
	}
	return wl, nil
}

// if in middle of previous rollout, continue use previous source and target appRevision as this round rollout
func (h *rolloutHandler) handleRolloutModified() {
	klog.InfoS("rollout target changed, restart the rollout", "new source", h.appRollout.Spec.SourceAppRevisionName,
		"new target", h.appRollout.Spec.TargetAppRevisionName)
	h.record.Event(h.appRollout, event.Normal("Rollout Restarted",
		"rollout target changed, restart the rollout", "new source", h.appRollout.Spec.SourceAppRevisionName,
		"new target", h.appRollout.Spec.TargetAppRevisionName))
	// we are okay to move directly to restart the rollout since we are at the terminal state
	// however, we need to make sure we properly finalizing the existing rollout before restart if it's
	// still in the middle of rolling out
	if h.appRollout.Status.RollingState != v1alpha1.RolloutSucceedState &&
		h.appRollout.Status.RollingState != v1alpha1.RolloutFailedState {
		// happen when roll forward or revert in middle of rollout, previous rollout haven't finished
		// continue to handle the previous resources until we are okay to move forward
		h.targetRevName = h.appRollout.Status.LastUpgradedTargetAppRevision
		h.sourceRevName = h.appRollout.Status.LastSourceAppRevision
	} else {
		// previous rollout have finished, go ahead using new source/target revision
		h.targetRevName = h.appRollout.Spec.TargetAppRevisionName
		h.sourceRevName = h.appRollout.Spec.SourceAppRevisionName
		// mark so that we don't think we are modified again
		h.appRollout.Status.LastUpgradedTargetAppRevision = h.appRollout.Spec.TargetAppRevisionName
		h.appRollout.Status.LastSourceAppRevision = h.appRollout.Spec.SourceAppRevisionName
	}
	h.appRollout.Status.StateTransition(v1alpha1.RollingModifiedEvent)
}

// templateTargetManifest call dispatch to template target manifest to cluster
func (h *rolloutHandler) templateTargetManifest(ctx context.Context) error {
	var rt *v1beta1.ResourceTracker
	// only when sourceAppRevision is not nil, we need gc old revision resources
	if h.sourceAppRevision != nil {
		rt = new(v1beta1.ResourceTracker)
		err := h.Get(ctx, types.NamespacedName{Name: dispatch.ConstructResourceTrackerName(h.appRollout.Spec.SourceAppRevisionName, h.appRollout.Namespace)}, rt)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	// use source resourceTracker to handle same resource owner transfer
	dispatcher := dispatch.NewAppManifestsDispatcher(h, h.targetAppRevision).EnableUpgradeAndSkipGC(rt)
	_, err := dispatcher.Dispatch(ctx, h.targetManifests)
	if err != nil {
		return err
	}
	workload, err := h.extractWorkload(ctx, *h.targetWorkloads[h.needRollComponent])
	if err != nil {
		return err
	}
	ref := metav1.GetControllerOfNoCopy(workload)
	if ref.Kind == v1beta1.ResourceTrackerKind {
		wlPatch := client.MergeFrom(workload.DeepCopy())
		// guarantee resourceTracker isn't controller owner of workload
		disableControllerOwner(workload)
		if err = h.Client.Patch(ctx, workload, wlPatch, client.FieldOwner(h.appRollout.UID)); err != nil {
			return err
		}
	}
	return nil
}

// handle rollout succeed work left
func (h *rolloutHandler) finalizeRollingSucceeded(ctx context.Context) error {
	// yield controller owner back to resourceTracker
	workload, err := h.extractWorkload(ctx, *h.targetWorkloads[h.needRollComponent])
	if err != nil {
		return err
	}
	wlPatch := client.MergeFrom(workload.DeepCopy())
	enableControllerOwner(workload)
	if err = h.Client.Patch(ctx, workload, wlPatch, client.FieldOwner(h.appRollout.UID)); err != nil {
		return err
	}

	// only when sourceAppRevision is not nil, we need gc old revision resources
	if h.sourceAppRevision != nil {
		oldRT := &v1beta1.ResourceTracker{}
		err := h.Client.Get(ctx, client.ObjectKey{
			Name: dispatch.ConstructResourceTrackerName(h.sourceAppRevision.Name, h.sourceAppRevision.Namespace)}, oldRT)
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		d := dispatch.NewAppManifestsDispatcher(h.Client, h.targetAppRevision).
			EnableUpgradeAndGC(oldRT)
		// no need to dispatch manifest again, just do GC
		if _, err := d.Dispatch(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}
