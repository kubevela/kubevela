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

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
)

// ClearNamespaceForClusterScopedResources clear namespace for cluster scoped resources
func (h *resourceKeeper) ClearNamespaceForClusterScopedResources(manifests []*unstructured.Unstructured) {
	for _, manifest := range manifests {
		if ok, err := kubeutil.IsClusterScope(manifest.GroupVersionKind(), h.Client.RESTMapper()); err == nil && ok {
			manifest.SetNamespace("")
		}
	}
}

func (h *resourceKeeper) isShared(manifest *unstructured.Unstructured) bool {
	if h.policies.SharedResource == nil {
		return false
	}
	return h.policies.SharedResource.FindStrategy(manifest)
}

func (h *resourceKeeper) canTakeOver(manifest *unstructured.Unstructured) bool {
	if h.policies.TakeOver == nil {
		return false
	}
	return h.policies.TakeOver.FindStrategy(manifest)
}

func (h *resourceKeeper) isReadOnly(manifest *unstructured.Unstructured) bool {
	if h.policies.ReadOnly == nil {
		return false
	}
	return h.policies.ReadOnly.FindStrategy(manifest)
}

func (h *resourceKeeper) getUpdateStrategy(manifest *unstructured.Unstructured) *v1alpha1.ResourceUpdateStrategy {
	if h.policies.ResourceUpdate == nil {
		return nil
	}
	return h.policies.ResourceUpdate.FindStrategy(manifest)
}

// localCluster addresses the cluster the keeper runs in (where ResourceTrackers live).
func localCluster(ctx context.Context) context.Context {
	return pkgmulticluster.WithCluster(ctx, pkgmulticluster.Local)
}
