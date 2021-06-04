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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// NewAppManifestsDispatcher creates an AppManifestsDispatcher.
func NewAppManifestsDispatcher(c client.Client, appRev *v1beta1.ApplicationRevision) *AppManifestsDispatcher {
	return &AppManifestsDispatcher{
		c:          c,
		applicator: apply.NewAPIApplicator(c),
		appRev:     appRev,
		gcHandler:  NewGCHandler(c, appRev.Namespace),
	}
}

// AppManifestsDispatcher dispatch application manifests into K8s and record the dispatched manifests' references in a
// resource tracker which is named by a particular rule: name = appRevision's Name + appRevision's namespace.
// A bundle of manifests to be dispatched MUST come from the given application revision.
type AppManifestsDispatcher struct {
	c          client.Client
	applicator apply.Applicator
	gcHandler  GarbageCollector

	appRev *v1beta1.ApplicationRevision
	oldRT  *v1beta1.ResourceTracker
	skipGC bool

	appRevName string
	namespace  string
	newRTName  string
	newRT      *v1beta1.ResourceTracker
}

// EnableGC return an AppManifestsDispatcher that always do GC after dispatching resources.
// GC will calculate diff between the dispatched resouces and ones recorded in the given resource tracker.
func (a *AppManifestsDispatcher) EnableGC(rt *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if rt != nil {
		a.oldRT = rt.DeepCopy()
	}
	return a
}

// EnableUpgradeAndSkipGC return an AppManifestsDispatcher that skips GC after dispatching resources.
// For the unchanged resources, dispatcher will update their owner to the newly created resource tracker.
// It's helpful in a rollout scenario where new revision is going to create a new workload while the old one should not
// be deleted before rollout is terminated.
func (a *AppManifestsDispatcher) EnableUpgradeAndSkipGC(rt *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if rt != nil {
		a.oldRT = rt.DeepCopy()
		a.skipGC = true
	}
	return a
}

// Dispatch apply manifests into k8s and return a resource tracker recording applied manifests' references.
// If GC is enabled, it will do GC after applying.
// If 'UpgradeAndSkipGC' is enabled, it will:
// - create new resources if not exist before
// - update unchanged resources' owner from the old resource tracker to the new one
// - skip deleting(GC) any resources
func (a *AppManifestsDispatcher) Dispatch(ctx context.Context, manifests []*unstructured.Unstructured) (*v1beta1.ResourceTracker, error) {
	if err := a.validateAndComplete(ctx); err != nil {
		return nil, err
	}
	if err := a.createOrGetResourceTracker(ctx); err != nil {
		return nil, err
	}
	if err := a.applyAndRecordManifests(ctx, manifests); err != nil {
		return nil, err
	}
	if !a.skipGC && a.oldRT != nil && a.oldRT.Name != a.newRTName {
		if err := a.gcHandler.GarbageCollect(ctx, a.oldRT, a.newRT); err != nil {
			return nil, errors.WithMessagef(err, "cannot do GC based on resource trackers %q and %q", a.oldRT.Name, a.newRTName)
		}
	}
	return a.newRT.DeepCopy(), nil
}

// ReferenceScopes add workload reference to scopes' workloadRefPath
func (a *AppManifestsDispatcher) ReferenceScopes(ctx context.Context, wlRef *v1beta1.TypedReference, scopes []*v1beta1.TypedReference) error {
	// TODO handle scopes
	return nil
}

// DereferenceScopes remove workload reference from scopes' workloadRefPath
func (a *AppManifestsDispatcher) DereferenceScopes(ctx context.Context, wlRef *v1beta1.TypedReference, scopes []*v1beta1.TypedReference) error {
	// TODO handle scopes
	return nil
}

func (a *AppManifestsDispatcher) validateAndComplete(ctx context.Context) error {
	if a.appRev == nil {
		return errors.New("given application revision is nil")
	}
	if a.appRev.Name == "" || a.appRev.Namespace == "" {
		return errors.New("given application revision has no name or namespace")
	}
	a.appRevName = a.appRev.Name
	a.namespace = a.appRev.Namespace
	a.newRTName = ConstructResourceTrackerName(a.appRevName, a.namespace)

	// no matter GC or UpgradeAndSkipGC, it requires a valid and existing resource tracker
	if a.oldRT != nil {
		existingOldRT := &v1beta1.ResourceTracker{}
		if err := a.c.Get(ctx, client.ObjectKey{Name: a.oldRT.Name}, existingOldRT); err != nil {
			return errors.Errorf("given resource tracker %q doesn't exist", a.oldRT.Name)
		}
		a.oldRT = existingOldRT
	}
	klog.InfoS("Given old resource tracker is nil, so skip GC", "appRevision", klog.KObj(a.appRev))
	return nil
}

func (a *AppManifestsDispatcher) createOrGetResourceTracker(ctx context.Context) error {
	rt := &v1beta1.ResourceTracker{}
	err := a.c.Get(ctx, client.ObjectKey{Name: a.newRTName}, rt)
	if err == nil {
		klog.InfoS("Found a resource tracker matching current app revision", "resourceTracker", rt.Name)
		// already exists, no need to update
		// because we assume the manifests' references from a specific application revision never change
		a.newRT = rt
		return nil
	}
	if !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "cannot get resource tracker")
	}
	klog.InfoS("Going to create a resource tracker", "resourceTracker", a.newRTName)
	rt.SetName(a.newRTName)
	if err := a.c.Create(ctx, rt); err != nil {
		klog.ErrorS(err, "Failed to create a resource tracker", "resourceTracker", a.newRTName)
		return errors.Wrap(err, "cannot create resource tracker")
	}
	a.newRT = rt
	return nil
}

func (a *AppManifestsDispatcher) applyAndRecordManifests(ctx context.Context, manifests []*unstructured.Unstructured) error {
	applyOpts := []apply.ApplyOption{}
	if a.oldRT != nil && a.oldRT.Name != a.newRTName {
		klog.InfoS("Going to apply and upgrade resources", "from", a.oldRT.Name, "to", a.newRTName)
		// if two RT's names are different, it means dispatching operation happens in an upgrade or rollout scenario
		// in such two scenarios, for those unchanged manifests, we will
		// - check existing resources are controlled by the old resource tracker
		// - set new resource tracker as their controller owner
		applyOpts = append(applyOpts, apply.MustBeControllableBy(a.oldRT.UID))
	} else {
		applyOpts = append(applyOpts, apply.MustBeControllableBy(a.newRT.UID))
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         v1beta1.SchemeGroupVersion.String(),
		Kind:               reflect.TypeOf(v1beta1.ResourceTracker{}).Name(),
		Name:               a.newRT.Name,
		UID:                a.newRT.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	for _, rsc := range manifests {
		// each resource applied by dispatcher MUST be controlled by resource tracker
		setOrOverrideControllerOwner(rsc, ownerRef)
		if err := a.applicator.Apply(ctx, rsc, applyOpts...); err != nil {
			klog.ErrorS(err, "Failed to apply a resource", "object",
				klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
			return errors.Wrapf(err, "cannot apply manifest, name: %q apiVersion: %q kind: %q",
				rsc.GetName(), rsc.GetAPIVersion(), rsc.GetKind())
		}
		klog.InfoS("Successfully apply a resource", "object",
			klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
	}
	return a.updateResourceTrackerStatus(ctx, manifests)
}

func (a *AppManifestsDispatcher) updateResourceTrackerStatus(ctx context.Context, appliedManifests []*unstructured.Unstructured) error {
	// merge applied resources and already recorded ones
	trackedResources := []v1beta1.TypedReference{}
	for _, rsc := range appliedManifests {
		ref := v1beta1.TypedReference{
			APIVersion: rsc.GetAPIVersion(),
			Kind:       rsc.GetKind(),
			Name:       rsc.GetName(),
			Namespace:  rsc.GetNamespace(),
		}
		alreadyTracked := false
		for _, existing := range a.newRT.Status.TrackedResources {
			if existing.APIVersion == ref.APIVersion && existing.Kind == ref.Kind &&
				existing.Name == ref.Name && existing.Namespace == ref.Namespace {
				alreadyTracked = true
				break
			}
		}
		if alreadyTracked {
			continue
		}
		trackedResources = append(trackedResources, ref)
	}
	a.newRT.Status.TrackedResources = trackedResources

	// TODO move TrackedResources from status to spec
	// update status with retry
	copyRT := a.newRT.DeepCopy()
	sts := copyRT.Status
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = a.c.Get(ctx, client.ObjectKey{Name: a.newRTName}, copyRT); err != nil {
			return
		}
		copyRT.Status = sts
		return a.c.Status().Update(ctx, copyRT)
	}); err != nil {
		klog.ErrorS(err, "Failed to update resource tracker status", "resourceTracker", a.newRTName)
		return errors.Wrap(err, "cannot update resource tracker status")
	}
	klog.InfoS("Successfully update resource tracker status", "resourceTracker", a.newRTName)
	return nil
}

func setOrOverrideControllerOwner(obj *unstructured.Unstructured, controllerOwner metav1.OwnerReference) {
	ownerRefs := []metav1.OwnerReference{controllerOwner}
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller &&
			owner.UID != controllerOwner.UID {
			owner.Controller = pointer.BoolPtr(false)
		}
		ownerRefs = append(ownerRefs, owner)
	}
	obj.SetOwnerReferences(ownerRefs)
}
