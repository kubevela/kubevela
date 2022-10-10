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
	"k8s.io/utils/strings/slices"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// ClearNamespaceForClusterScopedResources clear namespace for cluster scoped resources
func (h *resourceKeeper) ClearNamespaceForClusterScopedResources(manifests []*unstructured.Unstructured) {
	for _, manifest := range manifests {
		if ok, err := utils.IsClusterScope(manifest.GroupVersionKind(), h.Client.RESTMapper()); err == nil && ok {
			manifest.SetNamespace("")
		}
	}
}

func (h *resourceKeeper) isShared(manifest *unstructured.Unstructured) bool {
	if h.sharedResourcePolicy == nil {
		return false
	}
	return h.sharedResourcePolicy.FindStrategy(manifest)
}

func (h *resourceKeeper) canTakeOver(manifest *unstructured.Unstructured) bool {
	if h.takeOverPolicy == nil {
		return false
	}
	return h.takeOverPolicy.FindStrategy(manifest)
}

func (h *resourceKeeper) isReadOnly(manifest *unstructured.Unstructured) bool {
	if h.readOnlyPolicy == nil {
		return false
	}
	return h.readOnlyPolicy.FindStrategy(manifest)
}

// hasOrphanFinalizer checks if the target application should orphan child resources
func hasOrphanFinalizer(app *v1beta1.Application) bool {
	return slices.Contains(app.GetFinalizers(), oam.FinalizerOrphanResource)
}
