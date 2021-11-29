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

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
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
	cli client.Client
	m   map[string]*resourceCacheEntry
}

func newResourceCache(cli client.Client) *resourceCache {
	return &resourceCache{
		cli: cli,
		m:   map[string]*resourceCacheEntry{},
	}
}

func (cache *resourceCache) registerResourceTrackers(rts ...*v1beta1.ResourceTracker) {
	for _, rt := range rts {
		if rt == nil {
			continue
		}
		for _, mr := range rt.Spec.ManagedResources {
			key := mr.ResourceKey()
			entry, cached := cache.m[key]
			if !cached {
				entry = &resourceCacheEntry{obj: mr.ToUnstructured(), mr: mr}
				cache.m[key] = entry
			}
			entry.usedBy = append(entry.usedBy, rt)
		}
	}
	for _, entry := range cache.m {
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
	entry, cached := cache.m[key]
	if !cached {
		entry = &resourceCacheEntry{obj: mr.ToUnstructured(), mr: mr}
		cache.m[key] = entry
	}
	if !entry.loaded {
		if err := cache.cli.Get(multicluster.ContextWithClusterName(ctx, mr.Cluster), mr.NamespacedName(), entry.obj); err != nil {
			if kerrors.IsNotFound(err) {
				entry.exists = false
			} else {
				entry.err = errors.Wrapf(err, "failed to get resource %s", key)
			}
		} else {
			entry.exists = true
		}
		entry.loaded = true
	}
	return entry
}
