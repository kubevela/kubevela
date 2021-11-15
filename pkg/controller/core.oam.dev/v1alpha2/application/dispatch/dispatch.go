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

package dispatch

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// NewAppManifestsDispatcher creates an AppManifestsDispatcher.
func NewAppManifestsDispatcher(c client.Client, appRev *v1beta1.ApplicationRevision) *AppManifestsDispatcher {
	return &AppManifestsDispatcher{
		c:          c,
		applicator: apply.NewAPIApplicator(c),
		appRev:     appRev,
		gcHandler:  NewGCHandler(c, appRev.Namespace, *appRev),
	}
}

// AppManifestsDispatcher dispatch application manifests into K8s and record the dispatched manifests' references in a
// resource tracker which is named by a particular rule: name = appRevision's Name + appRevision's namespace.
// A bundle of manifests to be dispatched MUST come from the given application revision.
type AppManifestsDispatcher struct {
	c          client.Client
	applicator apply.Applicator
	gcHandler  GarbageCollector

	appRev     *v1beta1.ApplicationRevision
	previousRT *v1beta1.ResourceTracker
	skipGC     bool

	appRevName    string
	namespace     string
	currentRTName string
	currentRT     *v1beta1.ResourceTracker
	legacyRTs     []*v1beta1.ResourceTracker
}

// EndAndGC return an AppManifestsDispatcher that do GC after dispatching resources.
// For resources exists in two revision, dispatcher will update their owner to the new resource tracker.
// GC will calculate diff between the dispatched resources and ones recorded in the given resource tracker.
func (a *AppManifestsDispatcher) EndAndGC(rt *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if rt != nil {
		a.previousRT = rt.DeepCopy()
		a.skipGC = false
	}
	return a
}

// StartAndSkipGC return an AppManifestsDispatcher that skips GC after dispatching resources.
// For resources exists in two revision, dispatcher will update their owner to the new resource tracker.
// It's helpful in a rollout scenario where new revision is going to create a new workload while the old one should not
// be deleted before rollout is terminated.
func (a *AppManifestsDispatcher) StartAndSkipGC(previousRT *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if previousRT != nil {
		a.previousRT = previousRT.DeepCopy()
		a.skipGC = true
	}
	return a
}

// WithGCOptions set gcOptions for AppManifestsDispatcher
func (a *AppManifestsDispatcher) WithGCOptions(gcOptions GCOptions) *AppManifestsDispatcher {
	a.gcHandler.SetGCOptions(gcOptions)
	return a
}

// Dispatch apply manifests into k8s and return a resource tracker recording applied manifests' references.
// If GC is enabled, it will do GC after applying.
// If 'UpgradeAndSkipGC' is enabled, it will:
// - create new resources if not exist before
// - update unchanged resources' owner from the previous resource tracker to the new one
// - skip deleting(GC) any resources
func (a *AppManifestsDispatcher) Dispatch(ctx monitorContext.Context, manifests []*unstructured.Unstructured) (*v1beta1.ResourceTracker, error) {
	if err := a.validateAndComplete(ctx); err != nil {
		return nil, err
	}
	if err := a.createOrGetResourceTracker(ctx); err != nil {
		return nil, err
	}
	if err := a.retrieveLegacyResourceTrackers(ctx); err != nil {
		return nil, err
	}
	if err := a.applyAndRecordManifests(ctx, manifests); err != nil {
		return nil, err
	}
	if !a.skipGC && ((a.previousRT != nil && a.previousRT.Name != a.currentRTName) || len(a.legacyRTs) > 0) {
		if err := a.gcHandler.GarbageCollect(ctx, a.previousRT, a.currentRT, a.legacyRTs); err != nil {
			return nil, errors.WithMessagef(err, "cannot do GC based on resource trackers %q and %q", a.previousRT.Name, a.currentRTName)
		}
	}
	return a.currentRT.DeepCopy(), nil
}

// ReferenceScopes add workload reference to scopes' workloadRefPath
func (a *AppManifestsDispatcher) ReferenceScopes(ctx context.Context, wlRef *v1.ObjectReference, scopes []*v1.ObjectReference) error {
	// TODO handle scopes
	return nil
}

// DereferenceScopes remove workload reference from scopes' workloadRefPath
func (a *AppManifestsDispatcher) DereferenceScopes(ctx context.Context, wlRef *v1.ObjectReference, scopes []*v1.ObjectReference) error {
	// TODO handle scopes
	return nil
}

func (a *AppManifestsDispatcher) validateAndComplete(ctx monitorContext.Context) error {
	if a.appRev == nil {
		return errors.New("given application revision is nil")
	}
	if a.appRev.Name == "" || a.appRev.Namespace == "" {
		return errors.New("given application revision has no name or namespace")
	}
	a.appRevName = a.appRev.Name
	a.namespace = a.appRev.Namespace
	a.currentRTName = ConstructResourceTrackerName(a.appRevName, a.namespace)

	// if upgrade is enabled (no matter GC or skip GC), it requires a valid existing resource tracker
	if a.previousRT != nil && a.previousRT.Name != a.currentRTName {
		ctx.Info("Validate previous resource tracker exists", "previous", klog.KObj(a.previousRT))
		gotPreviousRT := &v1beta1.ResourceTracker{}
		if err := a.c.Get(ctx, client.ObjectKey{Name: a.previousRT.Name}, gotPreviousRT); err != nil {
			if !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get previous resource tracker")
			}
			a.previousRT = nil // if resourcetracker has been removed, ignore it
		} else {
			a.previousRT = gotPreviousRT
		}
	}
	ctx.Info("Given previous resource tracker is nil or same as current one, so skip GC", "appRevision", klog.KObj(a.appRev))
	return nil
}

func (a *AppManifestsDispatcher) createOrGetResourceTracker(ctx monitorContext.Context) error {
	rt := &v1beta1.ResourceTracker{}
	err := a.c.Get(ctx, client.ObjectKey{Name: a.currentRTName}, rt)
	if err == nil {
		ctx.Info("Found a resource tracker matching current app revision", "resourceTracker", rt.Name)
		// already exists, no need to update
		// because we assume the manifests' references from a specific application revision never change
		a.currentRT = rt
		return nil
	}
	if !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "cannot get resource tracker")
	}
	ctx.Info("Going to create a resource tracker", "resourceTracker", a.currentRTName)
	rt.SetName(a.currentRTName)
	// these labels can help to list resource trackers of a specific application
	rt.SetLabels(map[string]string{
		oam.LabelAppName:      ExtractAppName(a.currentRTName, a.namespace),
		oam.LabelAppNamespace: a.namespace,
	})
	if err := a.c.Create(ctx, rt); err != nil {
		ctx.Error(err, "Failed to create a resource tracker", "resourceTracker", a.currentRTName)
		return errors.Wrap(err, "cannot create resource tracker")
	}
	a.currentRT = rt
	return nil
}

// Besides current and previous resource trackers, other resource trackers are regarded as legacy ones.
// Legacy resource trackers come from unsuccessful dispatch, for example, error occrus in the middle of applying
// resources. They may cause resources leak or race.
// GarbageCollector should delete legacy resource trackers after dispatcher applies manifests successfully.
func (a *AppManifestsDispatcher) retrieveLegacyResourceTrackers(ctx context.Context) error {
	a.legacyRTs = []*v1beta1.ResourceTracker{}
	rtList := &v1beta1.ResourceTrackerList{}
	if err := a.c.List(ctx, rtList, client.MatchingLabels{
		oam.LabelAppName:      ExtractAppName(a.currentRTName, a.namespace),
		oam.LabelAppNamespace: a.namespace,
	}); err != nil {
		return errors.Wrap(err, "cannot retrieve legacy resource trackers")
	}
	for _, rt := range rtList.Items {
		if rt.Name != a.currentRTName &&
			(a.previousRT != nil && rt.Name != a.previousRT.Name) && !rt.IsLifeLong() {
			a.legacyRTs = append(a.legacyRTs, rt.DeepCopy())
		}
	}

	// compatibility code for label typo. more info: https://github.com/oam-dev/kubevela/issues/2464
	// TODO(wangyikewxgm)  delete after appRollout deprecated.
	oldRtList := &v1beta1.ResourceTrackerList{}
	if err := a.c.List(ctx, oldRtList, client.MatchingLabels{
		oam.LabelAppName:        ExtractAppName(a.currentRTName, a.namespace),
		"app.oam.dev/namesapce": a.namespace,
	}); err != nil {
		return errors.Wrap(err, "cannot retrieve legacy resource trackers with miss-spell label")
	}
	if len(oldRtList.Items) != 0 {
		for _, rt := range oldRtList.Items {
			if rt.Name != a.currentRTName &&
				(a.previousRT != nil && rt.Name != a.previousRT.Name) && !rt.IsLifeLong() {
				a.legacyRTs = append(a.legacyRTs, rt.DeepCopy())
			}
		}
	}

	return nil
}

func (a *AppManifestsDispatcher) applyAndRecordManifests(ctx monitorContext.Context, manifests []*unstructured.Unstructured) error {
	applyOpts := []apply.ApplyOption{apply.MustBeControlledByApp(&a.appRev.Spec.Application), apply.NotUpdateRenderHashEqual()}
	for _, rsc := range manifests {
		if rsc == nil {
			continue
		}

		immutable, err := a.ImmutableResourcesUpdate(ctx, rsc, a.currentRT, applyOpts)
		if immutable {
			if err != nil {
				ctx.Error(err, "Failed to apply immutable resource with new ownerReference", "object",
					klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
				return errors.Wrapf(err, "cannot apply immutable resource with new ownerReference, name: %q apiVersion: %q kind: %q",
					rsc.GetName(), rsc.GetAPIVersion(), rsc.GetKind())
			}
			continue
		}

		// each resource applied by dispatcher MUST be controlled by resource tracker
		a.currentRT.AddOwnerReferenceToTrackerResource(rsc)
		if err := a.applicator.Apply(ctx, rsc, applyOpts...); err != nil {
			ctx.Error(err, "Failed to apply a resource", "object",
				klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
			return errors.Wrapf(err, "cannot apply manifest, name: %q apiVersion: %q kind: %q",
				rsc.GetName(), rsc.GetAPIVersion(), rsc.GetKind())
		}
		ctx.Info("Successfully apply a resource", "object",
			klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
	}
	return a.updateResourceTrackerStatus(ctx, manifests)
}

// ImmutableResourcesUpdate only updates the ownerReference
// TODO(wonderflow): we should allow special fields to be updated. e.g. the resources.requests for bound claims for PV should be able to update
func (a *AppManifestsDispatcher) ImmutableResourcesUpdate(ctx context.Context, res *unstructured.Unstructured, rt *v1beta1.ResourceTracker, applyOpts []apply.ApplyOption) (bool, error) {
	if res == nil {
		return false, nil
	}
	switch res.GroupVersionKind() {
	case v1.SchemeGroupVersion.WithKind(reflect.TypeOf(v1.PersistentVolume{}).Name()):
		pv := new(v1.PersistentVolume)
		err := a.c.Get(ctx, client.ObjectKey{Name: res.GetName(), Namespace: res.GetNamespace()}, pv)
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return true, err
		}
		rt.AddOwnerReferenceToTrackerResource(pv)
		pv.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind(reflect.TypeOf(v1.PersistentVolume{}).Name()))
		return true, a.applicator.Apply(ctx, pv, applyOpts...)
	default:
	}
	return false, nil
}

func (a *AppManifestsDispatcher) updateResourceTrackerStatus(ctx monitorContext.Context, appliedManifests []*unstructured.Unstructured) error {
	// merge applied resources and already tracked ones
	if a.currentRT.Status.TrackedResources == nil {
		a.currentRT.Status.TrackedResources = make([]common.ClusterObjectReference, 0)
	}
	for _, rsc := range appliedManifests {
		if rsc == nil {
			continue
		}
		a.currentRT.AddTrackedResource(rsc)
	}

	// TODO move TrackedResources from status to spec
	// update status with retry
	copyRT := a.currentRT.DeepCopy()
	sts := copyRT.Status
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = a.c.Get(ctx, client.ObjectKey{Name: a.currentRTName}, copyRT); err != nil {
			return
		}
		copyRT.Status = sts
		return a.c.Status().Update(ctx, copyRT)
	}); err != nil {
		ctx.Error(err, "Failed to update resource tracker status", "resourceTracker", a.currentRTName)
		return errors.Wrap(err, "cannot update resource tracker status")
	}
	ctx.Info("Successfully update resource tracker status", "resourceTracker", a.currentRTName)
	return nil
}
