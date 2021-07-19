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
	"strings"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// GarbageCollector do GC according two resource trackers
type GarbageCollector interface {
	GarbageCollect(ctx context.Context, oldRT, newRT *v1beta1.ResourceTracker, legacyRTs []*v1beta1.ResourceTracker) error
}

// NewGCHandler create a GCHandler
func NewGCHandler(c client.Client, ns string) *GCHandler {
	return &GCHandler{c, ns, nil, nil}
}

// GCHandler implement GarbageCollector interface
type GCHandler struct {
	c         client.Client
	namespace string

	oldRT *v1beta1.ResourceTracker
	newRT *v1beta1.ResourceTracker
}

// GarbageCollect delete the old resources that are no longer in the new resource tracker
func (h *GCHandler) GarbageCollect(ctx context.Context, oldRT, newRT *v1beta1.ResourceTracker, legacyRTs []*v1beta1.ResourceTracker) error {
	h.oldRT = oldRT
	h.newRT = newRT
	if err := h.validate(); err != nil {
		return err
	}
	klog.InfoS("Garbage collect for application", "old", h.oldRT.Name, "new", h.newRT.Name)
	for _, oldRsc := range h.oldRT.Status.TrackedResources {
		isRemoved := true
		for _, newRsc := range h.newRT.Status.TrackedResources {
			if oldRsc.APIVersion == newRsc.APIVersion && oldRsc.Kind == newRsc.Kind &&
				oldRsc.Namespace == newRsc.Namespace && oldRsc.Name == newRsc.Name {
				isRemoved = false
				break
			}
		}
		if isRemoved {
			toBeDeleted := &unstructured.Unstructured{}
			toBeDeleted.SetAPIVersion(oldRsc.APIVersion)
			toBeDeleted.SetKind(oldRsc.Kind)
			toBeDeleted.SetNamespace(oldRsc.Namespace)
			toBeDeleted.SetName(oldRsc.Name)
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
