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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// GCOptions contains options for gc
type GCOptions struct {
	KeepLegacyResource bool `json:"keepLegacyResource,omitempty"`
}

// GarbageCollector do GC according two resource trackers
type GarbageCollector interface {
	GarbageCollect(ctx context.Context, oldRT, newRT *v1beta1.ResourceTracker, legacyRTs []*v1beta1.ResourceTracker) error
	SetGCOptions(gcOptions GCOptions)
}

// NewGCHandler create a GCHandler
func NewGCHandler(c client.Client, ns string, appRev v1beta1.ApplicationRevision) *GCHandler {
	return &GCHandler{
		c:         c,
		namespace: ns,
		appRev:    appRev,
	}
}

// GCHandler implement GarbageCollector interface
type GCHandler struct {
	c         client.Client
	namespace string
	gcOptions GCOptions

	oldRT *v1beta1.ResourceTracker
	newRT *v1beta1.ResourceTracker

	appRev v1beta1.ApplicationRevision
}

// GarbageCollect delete the old resources that are no longer in the new resource tracker
func (h *GCHandler) GarbageCollect(ctx context.Context, oldRT, newRT *v1beta1.ResourceTracker, legacyRTs []*v1beta1.ResourceTracker) error {
	if oldRT != nil && oldRT != newRT {
		h.oldRT = oldRT
		h.newRT = newRT
		if err := h.validate(); err != nil {
			return err
		}
		klog.InfoS("Garbage collect for application", "old", h.oldRT.Name, "new", h.newRT.Name)

		// if enabled KeepLegacyResource gc options, GCHandler will:
		// 1. keep legacy resources created by old application
		// 2. keep legacy resourceTrackers, if legacy resourceTracker not track any resources, delete it.
		if h.gcOptions.KeepLegacyResource {
			// if previous resourceTracker not track any resources, delete it
			if !isTrackedResources(oldRT, newRT) {
				if err := h.c.Delete(ctx, oldRT); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete resource tracker", "name", h.oldRT.Name)
					return errors.Wrapf(err, "cannot delete resource tracker %q", oldRT.Name)
				}
				klog.InfoS("Successfully GC a resource tracker which not track any resources", "name", oldRT.Name)
			}
			// if legacy resourceTracker not track any resources, delete it.
			return h.cleanUpResourceTracker(ctx, legacyRTs)
		}
		for _, oldRsc := range h.oldRT.Status.TrackedResources {
			reused := false
			for _, newRsc := range h.newRT.Status.TrackedResources {
				if oldRsc.APIVersion == newRsc.APIVersion && oldRsc.Kind == newRsc.Kind &&
					oldRsc.Namespace == newRsc.Namespace && oldRsc.Name == newRsc.Name {
					reused = true
					break
				}
			}
			if !reused {
				toBeDeleted := &unstructured.Unstructured{}
				toBeDeleted.SetAPIVersion(oldRsc.APIVersion)
				toBeDeleted.SetKind(oldRsc.Kind)
				toBeDeleted.SetNamespace(oldRsc.Namespace)
				toBeDeleted.SetName(oldRsc.Name)

				isSkip := false
				var err error
				if isSkip, err = h.handleResourceSkipGC(ctx, toBeDeleted, oldRT); err != nil {
					return errors.Wrap(err, "cannot handle resource skipResourceGC")
				}
				if isSkip {
					// the resource have skipGC annotation, will not delete the resource
					continue
				}

				if err := h.c.Delete(ctx, toBeDeleted); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete a resource", "name", oldRsc.Name, "apiVersion", oldRsc.APIVersion, "kind", oldRsc.Kind)
					return errors.Wrapf(err, "cannot delete resource %q", oldRsc)
				}
				klog.InfoS("Successfully GC a resource", "name", oldRsc.Name, "apiVersion", oldRsc.APIVersion, "kind", oldRsc.Kind)
			}
		}
		// delete the old resource tracker
		if err := h.c.Delete(ctx, h.oldRT); err != nil && !kerrors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete resource tracker", "name", h.oldRT.Name)
			return errors.Wrapf(err, "cannot delete resource tracker %q", h.oldRT.Name)
		}
		klog.InfoS("Successfully GC a resource tracker and its resources", "name", h.oldRT.Name)
	}
	for _, rt := range legacyRTs {
		if err := h.c.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete a legacy resource tracker", "legacy", rt.Name)
			return errors.Wrap(err, "cannot delete legacy resource tracker")
		}
		klog.InfoS("Successfully delete a legacy resource tracker", "legacy", rt.Name, "latest", h.newRT.Name)
	}
	return nil
}

// validate two resource trackers come from the same application
func (h *GCHandler) validate() error {
	oldRTName := h.oldRT.Name
	newRTName := h.newRT.Name
	if strings.HasSuffix(oldRTName, h.namespace) && strings.HasSuffix(newRTName, h.namespace) {
		if ExtractAppName(oldRTName, h.namespace) ==
			ExtractAppName(newRTName, h.namespace) {
			return nil
		}
	}
	return errors.Errorf("two resource trackers must come from the same application")
}

// handleResourceSkipGC will check resource have skipGC annotation,if yes patch the resource to orphan the resource and return true
func (h *GCHandler) handleResourceSkipGC(ctx context.Context, u *unstructured.Unstructured, oldRt *v1beta1.ResourceTracker) (bool, error) {
	// deepCopy avoid modify origin resource
	res := u.DeepCopy()
	if err := h.c.Get(ctx, types.NamespacedName{Namespace: res.GetNamespace(), Name: res.GetName()}, res); err != nil {
		if !kerrors.IsNotFound(err) {
			klog.ErrorS(err, "handleResourceSkipGC failed cannot get res kind ", res.GetKind(), "namespace", res.GetNamespace(), "name", res.GetName())
			return false, err
		}
		// resource have gone, skip delete it
		return true, nil
	}
	if _, exist := res.GetAnnotations()[oam.AnnotationSkipGC]; !exist {
		return false, nil
	}
	// if the component have been deleted don't skipGC
	if checkResourceRelatedCompDeleted(*res, h.appRev.Spec.Application.Spec.Components) {
		return false, nil
	}
	var owners []metav1.OwnerReference
	for _, ownerReference := range res.GetOwnerReferences() {
		if ownerReference.UID == oldRt.GetUID() {
			continue
		}
		owners = append(owners, ownerReference)
	}
	res.SetOwnerReferences(owners)
	if err := h.c.Update(ctx, res); err != nil {
		klog.ErrorS(err, "handleResourceSkipGC failed cannot orphan a res kind ", res.GetKind(), "namespace", res.GetNamespace(), "name", res.GetName())
		return false, err
	}
	klog.InfoS("succeed to handle a skipGC res kind ", res.GetKind(), "namespace", res.GetNamespace(), "name", res.GetName())
	return true, nil
}

// SetGCOptions set gc options for GCHandler
func (h *GCHandler) SetGCOptions(gcOptions GCOptions) {
	h.gcOptions = gcOptions
}

func isTrackedResources(oldRT *v1beta1.ResourceTracker, newRT *v1beta1.ResourceTracker) bool {
	if len(oldRT.Status.TrackedResources) == 0 {
		return false
	}
	if len(newRT.Status.TrackedResources) == 0 {
		return true
	}
	type TrackedResourcesKey struct {
		schema.GroupVersionKind
		types.NamespacedName
	}
	resourceRecord := make(map[TrackedResourcesKey]bool)
	for _, obj := range newRT.Status.TrackedResources {
		objKey := TrackedResourcesKey{
			obj.GroupVersionKind(),
			types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name},
		}
		resourceRecord[objKey] = true
	}
	for _, obj := range oldRT.Status.TrackedResources {
		objKey := TrackedResourcesKey{
			obj.GroupVersionKind(),
			types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name},
		}
		if !resourceRecord[objKey] {
			return true
		}
	}
	return false
}

func checkResourceRelatedCompDeleted(res unstructured.Unstructured, comps []common.ApplicationComponent) bool {
	compName := res.GetLabels()[oam.LabelAppComponent]
	deleted := true
	for _, comp := range comps {
		if compName == comp.Name {
			deleted = false
		}
	}
	return deleted
}

func (h *GCHandler) cleanUpResourceTracker(ctx context.Context, legacyRTs []*v1beta1.ResourceTracker) error {
	for _, rt := range legacyRTs {
		needDeleted := true
		for _, resource := range rt.Status.TrackedResources {
			rsc := new(unstructured.Unstructured)
			rsc.SetGroupVersionKind(resource.GroupVersionKind())
			objKey := client.ObjectKey{Name: resource.Name, Namespace: resource.Namespace}
			if err := h.c.Get(ctx, objKey, rsc); err != nil {
				if kerrors.IsNotFound(err) {
					continue
				}
				return err
			}
			if IsOwningObject(rsc, rt) {
				needDeleted = false
				break
			}
		}
		if needDeleted {
			if err := h.c.Delete(ctx, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsOwningObject check if owner owning the object
func IsOwningObject(obj metav1.Object, owner metav1.Object) bool {
	ownerReferences := obj.GetOwnerReferences()
	for _, ownerRef := range ownerReferences {
		if ownerRef.UID == owner.GetUID() {
			return true
		}
	}
	return false
}
