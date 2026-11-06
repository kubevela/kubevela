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

package resourcekeeper

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ContainsResources check if resources all exist
func (h *resourceKeeper) ContainsResources(resources []*unstructured.Unstructured) bool {
	h.ClearNamespaceForClusterScopedResources(resources)
	for _, rsc := range resources {
		if rsc == nil {
			continue
		}
		if (h._currentRT != nil && h._currentRT.ContainsManagedResource(rsc)) ||
			(h._rootRT != nil && h._rootRT.ContainsManagedResource(rsc)) {
			continue
		}
		return false
	}
	return true
}

// MetadataRemoval names labels and annotations to take off resources: a key and the value it
// must still have, so a key someone has changed since is left alone.
type MetadataRemoval struct {
	Labels      map[string]string
	Annotations map[string]string
}

func (r MetadataRemoval) empty() bool { return len(r.Labels) == 0 && len(r.Annotations) == 0 }

// removeCarried deletes from metadata the keys to remove that still hold their value, and reports
// whether any was deleted.
func removeCarried(remove, metadata map[string]string) bool {
	changed := false
	for k, v := range remove {
		if got, ok := metadata[k]; ok && got == v {
			delete(metadata, k)
			changed = true
		}
	}
	return changed
}

// ManifestMetadataChanged reports whether any resource renders a label or annotation that its
// recorded manifest lacks or holds with another value, including metadata a component inherits
// from its Application (such as labels set by a global policy), which no property change
// reports. Only rendered keys are compared: dispatch stamps more (owner labels, the render hash)
// onto what it records. Resources recorded without their manifest (meta only) or not recorded at
// all report no change.
func (h *resourceKeeper) ManifestMetadataChanged(resources []*unstructured.Unstructured) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ClearNamespaceForClusterScopedResources(resources)
	for _, rsc := range resources {
		if rsc == nil {
			continue
		}
		recorded := h.recordedManifest(rsc)
		if recorded == nil {
			continue
		}
		if !subsetOf(rsc.GetLabels(), recorded.GetLabels()) || !subsetOf(rsc.GetAnnotations(), recorded.GetAnnotations()) {
			return true
		}
	}
	return false
}

// subsetOf reports whether every key in rendered is in recorded with the same value.
func subsetOf(rendered, recorded map[string]string) bool {
	for k, v := range rendered {
		if got, ok := recorded[k]; !ok || got != v {
			return false
		}
	}
	return true
}

// recordedManifest returns the manifest the trackers hold for rsc, or nil when none is stored.
func (h *resourceKeeper) recordedManifest(rsc *unstructured.Unstructured) *unstructured.Unstructured {
	for _, rt := range []*v1beta1.ResourceTracker{h._currentRT, h._rootRT} {
		if rt == nil {
			continue
		}
		for i := range rt.Spec.ManagedResources {
			mr := &rt.Spec.ManagedResources[i]
			if mr.Deleted || mr.Data == nil || !sameResource(mr, rsc) {
				continue
			}
			// A manifest recorded in this reconcile is still the object; one read back from the
			// API is raw JSON.
			if obj, ok := mr.Data.Object.(*unstructured.Unstructured); ok {
				return obj
			}
			if obj, err := mr.ToUnstructuredWithData(); err == nil {
				return obj
			}
			return nil
		}
	}
	return nil
}

func sameResource(mr *v1beta1.ManagedResource, rsc *unstructured.Unstructured) bool {
	gvk := rsc.GroupVersionKind()
	return mr.Name == rsc.GetName() && mr.Namespace == rsc.GetNamespace() &&
		mr.GroupVersionKind().Group == gvk.Group && mr.Kind == gvk.Kind && mr.Cluster == oam.GetCluster(rsc)
}

// RefreshRecordedMetadata sets labels and annotations on the manifests recorded for the
// application's component resources, for keys those manifests already carry, removes the keys in
// removed that still hold their value, and saves the trackers it changed. StateKeep then applies
// them. It is how a changed value of metadata the application passes down (such as a label from a
// policy reading cluster data) reaches resources without a new revision. Keys a manifest does not
// carry are left out: inheritance may have filtered them, and they arrive at the next revision.
// Resources recorded without their manifest, or not rendered from a component, are left alone.
func (h *resourceKeeper) RefreshRecordedMetadata(ctx context.Context, labels, annotations map[string]string, removed MetadataRemoval) (bool, error) {
	if len(labels) == 0 && len(annotations) == 0 && removed.empty() {
		return false, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	for _, rt := range []*v1beta1.ResourceTracker{h._currentRT, h._rootRT} {
		if rt == nil || rt.GetDeletionTimestamp() != nil {
			continue
		}
		updated := false
		for i := range rt.Spec.ManagedResources {
			mr := &rt.Spec.ManagedResources[i]
			if mr.Deleted || mr.Component == "" || mr.Data == nil {
				continue
			}
			obj, ok := mr.Data.Object.(*unstructured.Unstructured)
			if !ok {
				var err error
				if obj, err = mr.ToUnstructuredWithData(); err != nil {
					continue
				}
			}
			newLabels, labelsChanged := overwriteKeys(obj.GetLabels(), labels)
			newAnnotations, annotationsChanged := overwriteKeys(obj.GetAnnotations(), annotations)
			if newLabels != nil && removeCarried(removed.Labels, newLabels) {
				labelsChanged = true
			}
			if newAnnotations != nil && removeCarried(removed.Annotations, newAnnotations) {
				annotationsChanged = true
			}
			if !labelsChanged && !annotationsChanged {
				continue
			}
			obj.SetLabels(newLabels)
			obj.SetAnnotations(newAnnotations)
			raw, err := obj.MarshalJSON()
			if err != nil {
				return changed, err
			}
			mr.Data = &runtime.RawExtension{Raw: raw}
			updated = true
		}
		if updated {
			if err := h.Client.Update(multicluster.ContextInLocalCluster(ctx), rt); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	return changed, nil
}

// overwriteKeys returns current with the values of the keys it already has replaced from values,
// and whether any value changed.
func overwriteKeys(current, values map[string]string) (map[string]string, bool) {
	changed := false
	for k, v := range values {
		if got, ok := current[k]; ok && got != v {
			current[k] = v
			changed = true
		}
	}
	return current, changed
}
