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
	"reflect"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationrollout"
)

type rolloutHandler struct {
	*Reconciler
	appRollout *v1beta1.AppRollout
	parser     *appfile.Parser

	// source/targetRevName represent this round reconcile using source and target revision
	// in most cases they are equal to appRollout.spec.target/sourceRevName but if roll forward or revert in middle of rollout
	// source/targetRevName are equal to previous rollout
	sourceRevName string
	targetRevName string

	sourceAppRevision *v1beta1.ApplicationRevision
	targetAppRevision *v1beta1.ApplicationRevision

	// sourceWorkloads is assembled by appRevision in assemble phase
	// please be aware that they are not real status in k8s, they are just generate from appRevision include GVK+namespace+name
	sourceWorkloads map[string]*unstructured.Unstructured
	targetWorkloads map[string]*unstructured.Unstructured

	// targetManifests used by dispatch(template targetRevision) and handleSucceed(GC) phase
	targetManifests []*unstructured.Unstructured

	// needRollComponent is find common component between source and target revision
	needRollComponent string
}

// prepareWorkloads call assemble func to prepare workload of every component
// please be aware that this func isn't generated final resources emitted to k8s.
// it just help for the phase determiningCommonComponent
func (h *rolloutHandler) prepareWorkloads(ctx context.Context) error {
	var err error
	h.targetAppRevision = new(v1beta1.ApplicationRevision)
	if err := h.Get(ctx, types.NamespacedName{Namespace: h.appRollout.Namespace, Name: h.targetRevName}, h.targetAppRevision); err != nil {
		return err
	}

	// construct a assemble manifest for targetAppRevision
	targetAssemble := assemble.NewAppManifests(h.targetAppRevision, h.parser).
		WithWorkloadOption(RolloutWorkloadName(h.needRollComponent)).
		WithWorkloadOption(assemble.PrepareWorkloadForRollout(h.needRollComponent))

	h.targetWorkloads, _, _, err = targetAssemble.GroupAssembledManifests()
	if err != nil {
		klog.Error("appRollout targetAppRevision failed to assemble target workload", "appRollout", klog.KRef(h.appRollout.Namespace, h.appRollout.Name))
		return err
	}

	if len(h.sourceRevName) != 0 {
		h.sourceAppRevision = new(v1beta1.ApplicationRevision)
		if err := h.Get(ctx, types.NamespacedName{Namespace: h.appRollout.Namespace, Name: h.sourceRevName}, h.sourceAppRevision); err != nil {
			return err
		}
		// construct a assemble manifest for sourceAppRevision
		sourceAssemble := assemble.NewAppManifests(h.sourceAppRevision, h.parser).
			WithWorkloadOption(assemble.PrepareWorkloadForRollout(h.needRollComponent)).
			WithWorkloadOption(RolloutWorkloadName(h.needRollComponent))
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
	if h.needRollComponent != "" {
		return nil
	}

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
		klog.Errorf("specified sourceRevName but cannot fetch source workload %s: %v",
			h.appRollout.Spec.SourceAppRevisionName, err)
		return nil, nil, err
	}
	if targetWorkload, err = h.extractWorkload(ctx, *h.targetWorkloads[h.needRollComponent]); err != nil {
		klog.Errorf("cannot fetch target workload %s: %v", h.appRollout.Spec.TargetAppRevisionName, err)
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

func getAppFromAppRev(appRev *v1beta1.ApplicationRevision) *v1beta1.Application {
	app := appRev.Spec.Application.DeepCopy()
	owner := metav1.GetControllerOf(appRev)
	if owner != nil {
		app.SetUID(owner.UID)
	}
	return app
}

// templateTargetManifest call dispatch to template target app revision's manifests to k8s
func (h *rolloutHandler) templateTargetManifest(ctx context.Context) error {
	var rt *v1beta1.ResourceTracker
	// if sourceAppRevision is not nil, we should upgrade existing resources which are also needed by target app
	// revision
	if h.sourceAppRevision != nil {
		rt = new(v1beta1.ResourceTracker)
		err := h.Get(ctx, types.NamespacedName{Name: ConstructResourceTrackerName(h.appRollout.Spec.SourceAppRevisionName, h.appRollout.Namespace)}, rt)
		if err != nil {
			klog.Errorf("specified sourceAppRevisionName %s but cannot fetch the sourceResourceTracker %v",
				h.appRollout.Spec.SourceAppRevisionName, err)
			return err
		}
	}

	// use source resourceTracker to handle same resource owner transfer
	handler, err := resourcekeeper.NewResourceKeeper(ctx, h.Client, getAppFromAppRev(h.targetAppRevision))
	if err != nil {
		klog.Errorf("failed to create resource keeper")
		return err
	}
	if err := handler.Dispatch(ctx, h.targetManifests); err != nil {
		klog.Errorf("dispatch targetRevision error %s:%v", h.appRollout.Spec.TargetAppRevisionName, err)
		return err
	}

	// The workload not in target workloads can be not ready for insertSecret case
	targetWL := h.targetWorkloads[h.needRollComponent]
	if targetWL == nil {
		return errors.Errorf("target workload for component %s for app %s is not ready", h.needRollComponent, h.targetAppRevision.Spec.Application.Name)
	}

	workload, err := h.extractWorkload(ctx, *targetWL)
	if err != nil {
		return err
	}
	ref := metav1.GetControllerOfNoCopy(workload)
	if ref != nil && ref.Kind == v1beta1.ResourceTrackerKind {
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
			Name: ConstructResourceTrackerName(h.sourceAppRevision.Name, h.sourceAppRevision.Namespace)}, oldRT)
		if err != nil && apierrors.IsNotFound(err) {
			// end finalizing if source revision's tracker is already gone
			// this guarantees finalizeRollingSucceeded will only GC once
			return nil
		}
		if err != nil {
			return err
		}
		handler, err := resourcekeeper.NewResourceKeeper(ctx, h.Client, getAppFromAppRev(h.targetAppRevision))
		if err != nil {
			klog.Errorf("failed to create resource keeper")
			return err
		}
		// no need to dispatch manifest again, just do GC
		if err = handler.Dispatch(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}

// this func handle two case
// 1. handle 1.0.x lagacy workload, their owner is appcontext so let resourceTracker take over it
// 2. disable resourceTracker controller owner
func (h *rolloutHandler) handleSourceWorkload(ctx context.Context) error {
	workload, err := h.extractWorkload(ctx, *h.sourceWorkloads[h.needRollComponent])
	if err != nil {
		return err
	}
	owners := workload.GetOwnerReferences()
	var wantOwner []metav1.OwnerReference
	wlPatch := client.MergeFrom(workload.DeepCopy())
	for _, owner := range owners {
		if owner.Kind == v1beta1.ResourceTrackerKind {
			wantOwner = append(wantOwner, owner)
		}
	}
	// logic here is  compatibility code for 1.0.X lagacy workload, their ownerReference is appcontext, so remove it
	if len(wantOwner) == 0 {
		klog.InfoS("meet a lagacy workload, should let resourceTracker take over it")
		rtName := ConstructResourceTrackerName(h.sourceRevName, h.appRollout.Namespace)
		rt := v1beta1.ResourceTracker{}
		if err := h.Get(ctx, types.NamespacedName{Name: rtName}, &rt); err != nil {
			if apierrors.IsNotFound(err) {
				if err = h.Create(ctx, &rt); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		ownerRef := metav1.OwnerReference{
			APIVersion:         v1beta1.SchemeGroupVersion.String(),
			Kind:               reflect.TypeOf(v1beta1.ResourceTracker{}).Name(),
			Name:               rt.Name,
			UID:                rt.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		}
		workload.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	}
	// after last succeed rollout finish, workload's ownerReference controller have been set true
	// so we should disable the controller owner firstly
	disableControllerOwner(workload)
	if err = h.Client.Patch(ctx, workload, wlPatch, client.FieldOwner(h.appRollout.UID)); err != nil {
		return err
	}
	return nil
}

// assembleManifest generate target manifest(workloads/traits)
// please notice that we shouldn't modify the replicas of workload if the workload already exist in k8s.
// so we use HandleReplicas as assemble option
// And this phase must call after determine component phase.
func (h *rolloutHandler) assembleManifest(ctx context.Context) error {
	if h.appRollout.Status.RollingState != v1alpha1.LocatingTargetAppState {
		return nil
	}
	var err error
	// construct a assemble manifest for targetAppRevision
	targetAssemble := assemble.NewAppManifests(h.targetAppRevision, h.parser).
		WithWorkloadOption(RolloutWorkloadName(h.needRollComponent)).
		WithWorkloadOption(assemble.PrepareWorkloadForRollout(h.needRollComponent)).WithWorkloadOption(HandleReplicas(ctx, h.needRollComponent, h.Client))

	// in template phase, we should use targetManifests including target workloads/traits to
	h.targetManifests, err = targetAssemble.AssembledManifests()
	if err != nil {
		klog.Error("appRollout targetAppRevision failed to assemble manifest", "appRollout", klog.KRef(h.appRollout.Namespace, h.appRollout.Name))
		return err
	}
	// we never need generate source manifest, cause we needn't template source ever.
	return nil
}

// ConstructResourceTrackerName to be deprecated
func ConstructResourceTrackerName(appRevName, ns string) string {
	return fmt.Sprintf("%s-%s", appRevName, ns)
}
