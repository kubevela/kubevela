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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

// ComponentResources is every resource the tracked Application has applied for a
// component and not since deleted. Only trackers already loaded are read: a
// tracker that does not exist yet holds nothing applied.
//
// Held under the keeper's lock, since a deploy step records into the trackers
// from parallel tasks; the entries are copied out, without the stored manifest.
func (h *resourceKeeper) ComponentResources(component string) []v1beta1.ManagedResource {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []v1beta1.ManagedResource
	for _, rt := range []*v1beta1.ResourceTracker{h._currentRT, h._rootRT} {
		if rt == nil {
			continue
		}
		for _, mr := range rt.Spec.ManagedResources {
			if !mr.Deleted && mr.Component == component {
				// The stored manifest is shared with the tracker, and not needed to
				// find the resource.
				mr.Data = nil
				out = append(out, mr)
			}
		}
	}
	return out
}
