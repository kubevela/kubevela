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

package resourcekeeper

import (
	"context"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	pkgmaps "github.com/kubevela/pkg/util/maps"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

type resourceCacheEntry struct {
	exists         bool
	err            error
	obj            *unstructured.Unstructured
	mr             v1beta1.ManagedResource
	loaded         bool
	usedBy         []*v1beta1.ResourceTracker
	latestActiveRT *v1beta1.ResourceTracker
	gcExecutorRT   *v1beta1.ResourceTracker
}

type resourceCache struct {
	// tracked decides whether a live object is still the owner's; nil treats every object as its own.
	tracked resourcetracker.Tracked
	cli     client.Client
	m       *pkgmaps.SyncMap[string, *resourceCacheEntry]
}

func newResourceCache(cli client.Client, tracked resourcetracker.Tracked) *resourceCache {
	return &resourceCache{
		tracked: tracked,
		cli:     cli,
		m:       pkgmaps.NewSyncMap[string, *resourceCacheEntry](),
	}
}

func (cache *resourceCache) registerResourceTrackers(rts ...*v1beta1.ResourceTracker) {
	for _, rt := range rts {
		if rt == nil {
			continue
		}
		for _, mr := range rt.Spec.ManagedResources {
			key := mr.ResourceKey()
			entry, cached := cache.m.Get(key)
			if !cached {
				entry = &resourceCacheEntry{obj: mr.ToUnstructured(), mr: mr}
				cache.m.Set(key, entry)
			}
			entry.usedBy = append(entry.usedBy, rt)
		}
	}
	for _, entry := range cache.m.Data() {
		for i := len(entry.usedBy) - 1; i >= 0; i-- {
			if entry.usedBy[i].GetDeletionTimestamp() == nil {
				entry.latestActiveRT = entry.usedBy[i]
				break
			}
		}
		if entry.latestActiveRT != nil {
			entry.gcExecutorRT = entry.latestActiveRT
		} else if len(entry.usedBy) > 0 {
			entry.gcExecutorRT = entry.usedBy[len(entry.usedBy)-1]
		}
	}
}

func (cache *resourceCache) get(ctx context.Context, mr v1beta1.ManagedResource) *resourceCacheEntry {
	key := mr.ResourceKey()
	entry, cached := cache.m.Get(key)
	if !cached {
		entry = &resourceCacheEntry{obj: mr.ToUnstructured(), mr: mr}
		cache.m.Set(key, entry)
	}
	if !entry.loaded {
		if err := cache.cli.Get(pkgmulticluster.WithCluster(ctx, mr.Cluster), mr.NamespacedName(), entry.obj); err != nil {
			if kubeutil.IsNotFoundOrClusterNotExists(err) || meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
				entry.exists = false
			} else {
				entry.err = errors.Wrapf(err, "failed to get resource %s", key)
			}
		} else {
			entry.exists = cache.exists(entry.obj)
		}
		entry.loaded = true
	}
	return entry
}

func (cache *resourceCache) exists(manifest *unstructured.Unstructured) bool {
	if cache.tracked == nil {
		return true
	}
	return IsResourceManaged(manifest, cache.tracked)
}

// IsResourceManaged reports whether a live resource is still the owner's: it controls it, it
// is a sharer of it, or the object has no resource version (an API response rather than a
// stored object, e.g. from vela-prism) and the owner is not orphaning its resources.
func IsResourceManaged(manifest *unstructured.Unstructured, owner resourcetracker.Tracked) bool {
	if owner.ControlledBy(manifest) == owner.Key() || (manifest.GetResourceVersion() == "" && !owner.Orphaning()) {
		return true
	}
	annotations := manifest.GetAnnotations()
	if annotations == nil || annotations[oam.AnnotationAppSharedBy] == "" {
		return false
	}
	return apply.ContainsSharer(annotations[oam.AnnotationAppSharedBy], owner.Key())
}
