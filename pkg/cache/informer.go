/*
Copyright 2023 The KubeVela Authors.

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

package cache

import (
	"context"
	"sync"
	"time"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	ctrlutils "github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/features"
)

var (
	// ApplicationRevisionDefinitionCachePruneDuration the prune duration for application revision definition cache
	ApplicationRevisionDefinitionCachePruneDuration = time.Hour
)

// ObjectCacheEntry entry for object cache
type ObjectCacheEntry[T any] struct {
	ptr          *T
	refs         sets.Set[string]
	lastAccessed time.Time
}

// ObjectCache cache for objects
type ObjectCache[T any] struct {
	mu      sync.RWMutex
	objects map[string]*ObjectCacheEntry[T]
}

// NewObjectCache create an object cache
func NewObjectCache[T any]() *ObjectCache[T] {
	return &ObjectCache[T]{
		objects: map[string]*ObjectCacheEntry[T]{},
	}
}

// Get retrieve the cache entry
func (in *ObjectCache[T]) Get(hash string) *T {
	in.mu.RLock()
	defer in.mu.RUnlock()
	if entry, found := in.objects[hash]; found {
		return entry.ptr
	}
	return nil
}

// Add insert cache entry with ref, return the ptr of the entry
func (in *ObjectCache[T]) Add(hash string, obj *T, ref string) *T {
	in.mu.Lock()
	defer in.mu.Unlock()
	if entry, found := in.objects[hash]; found {
		entry.refs.Insert(ref)
		entry.lastAccessed = time.Now()
		return entry.ptr
	}
	in.objects[hash] = &ObjectCacheEntry[T]{
		ptr:          obj,
		refs:         sets.New[string](ref),
		lastAccessed: time.Now(),
	}
	return obj
}

// DeleteRef delete ref for an obj
func (in *ObjectCache[T]) DeleteRef(hash string, ref string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if entry, found := in.objects[hash]; found {
		entry.refs.Delete(ref)
		if entry.refs.Len() == 0 {
			delete(in.objects, hash)
		}
	}
}

// Remap relocate the object ptr with given ref
func (in *ObjectCache[T]) Remap(m map[string]*T, ref string) {
	for key, o := range m {
		if hash, err := ctrlutils.ComputeSpecHash(o); err == nil {
			m[key] = in.Add(hash, o, ref)
		}
	}
}

// Unmap drop all the hash object from the map
func (in *ObjectCache[T]) Unmap(m map[string]*T, ref string) {
	for _, o := range m {
		if hash, err := ctrlutils.ComputeSpecHash(o); err == nil {
			in.DeleteRef(hash, ref)
		}
	}
}

// Size get the size of the cache
func (in *ObjectCache[T]) Size() int {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return len(in.objects)
}

// Prune remove outdated cache, return the pruned count
func (in *ObjectCache[T]) Prune(outdated time.Duration) int {
	in.mu.Lock()
	defer in.mu.Unlock()
	cnt := 0
	for key, entry := range in.objects {
		if time.Now().After(entry.lastAccessed.Add(outdated)) {
			delete(in.objects, key)
			cnt++
		}
	}
	return cnt
}

// DefinitionCache cache for definitions
type DefinitionCache struct {
	ComponentDefinitionCache    *ObjectCache[v1beta1.ComponentDefinition]
	TraitDefinitionCache        *ObjectCache[v1beta1.TraitDefinition]
	WorkflowStepDefinitionCache *ObjectCache[v1beta1.WorkflowStepDefinition]
}

// NewDefinitionCache create DefinitionCache
func NewDefinitionCache() *DefinitionCache {
	return &DefinitionCache{
		ComponentDefinitionCache:    NewObjectCache[v1beta1.ComponentDefinition](),
		TraitDefinitionCache:        NewObjectCache[v1beta1.TraitDefinition](),
		WorkflowStepDefinitionCache: NewObjectCache[v1beta1.WorkflowStepDefinition](),
	}
}

// RemapRevision remap all definitions in the given revision
func (in *DefinitionCache) RemapRevision(rev *v1beta1.ApplicationRevision) {
	ref := client.ObjectKeyFromObject(rev).String()
	in.ComponentDefinitionCache.Remap(rev.Spec.ComponentDefinitions, ref)
	in.TraitDefinitionCache.Remap(rev.Spec.TraitDefinitions, ref)
	in.WorkflowStepDefinitionCache.Remap(rev.Spec.WorkflowStepDefinitions, ref)
}

// UnmapRevision unmap definitions from the provided revision by the given ref
func (in *DefinitionCache) UnmapRevision(rev *v1beta1.ApplicationRevision) {
	ref := client.ObjectKeyFromObject(rev).String()
	in.ComponentDefinitionCache.Unmap(rev.Spec.ComponentDefinitions, ref)
	in.TraitDefinitionCache.Unmap(rev.Spec.TraitDefinitions, ref)
	in.WorkflowStepDefinitionCache.Unmap(rev.Spec.WorkflowStepDefinitions, ref)
}

// Start clear cache every duration
func (in *DefinitionCache) Start(ctx context.Context, store cache.Cache, duration time.Duration) {
	informer := runtime.Must(store.GetInformer(ctx, &v1beta1.ApplicationRevision{}))
	_, err := informer.AddEventHandler(kcache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if rev, ok := obj.(*v1beta1.ApplicationRevision); ok {
				in.RemapRevision(rev)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if rev, ok := oldObj.(*v1beta1.ApplicationRevision); ok {
				in.UnmapRevision(rev)
			}
			if rev, ok := newObj.(*v1beta1.ApplicationRevision); ok {
				in.RemapRevision(rev)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if rev, ok := obj.(*v1beta1.ApplicationRevision); ok {
				in.UnmapRevision(rev)
			}
		},
	})
	if err != nil {
		klog.ErrorS(err, "failed to add event handler for definition cache")
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			t0 := time.Now()
			compDefPruned := in.ComponentDefinitionCache.Prune(duration)
			traitDefPruned := in.TraitDefinitionCache.Prune(duration)
			wsDefPruned := in.WorkflowStepDefinitionCache.Prune(duration)
			klog.Infof("DefinitionCache prune finished. ComponentDefinition: %d(-%d), TraitDefinition: %d(-%d), WorkflowStepDefinition: %d(-%d). Time cost: %d ms.",
				in.ComponentDefinitionCache.Size(), compDefPruned,
				in.TraitDefinitionCache.Size(), traitDefPruned,
				in.WorkflowStepDefinitionCache.Size(), wsDefPruned,
				time.Since(t0).Microseconds())
			time.Sleep(duration)
		}
	}
}

// DefaultDefinitionCache the default definition cache
var DefaultDefinitionCache = singleton.NewSingleton(NewDefinitionCache)

func filterUnnecessaryField(o client.Object) {
	_ = k8s.DeleteAnnotation(o, "kubectl.kubernetes.io/last-applied-configuration")
	o.SetManagedFields(nil)
}

func wrapTransformFunc[T client.Object](fn func(T)) kcache.TransformFunc {
	return func(i interface{}) (interface{}, error) {
		if o, ok := i.(T); ok {
			filterUnnecessaryField(o)
			fn(o)
			return o, nil
		}
		return i, nil
	}
}

// AddInformerTransformFuncToCacheOption add informer transform func to cache option
// This will filter out the unnecessary fields for cached objects and use definition cache
// to reduce the duplicated storage of same definitions
func AddInformerTransformFuncToCacheOption(opts *cache.Options) {
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.InformerCacheFilterUnnecessaryFields) {
		if opts.TransformByObject == nil {
			opts.TransformByObject = map[client.Object]kcache.TransformFunc{}
		}
		opts.TransformByObject[&v1beta1.ApplicationRevision{}] = wrapTransformFunc(func(rev *v1beta1.ApplicationRevision) {
			if utilfeature.DefaultMutableFeatureGate.Enabled(features.SharedDefinitionStorageForApplicationRevision) {
				DefaultDefinitionCache.Get().RemapRevision(rev)
			}
		})
		opts.TransformByObject[&v1beta1.Application{}] = wrapTransformFunc(func(app *v1beta1.Application) {})
		opts.TransformByObject[&v1beta1.ResourceTracker{}] = wrapTransformFunc(func(rt *v1beta1.ResourceTracker) {})
	}
}

// NewResourcesToDisableCache get ClientDisableCacheFor for building controller
func NewResourcesToDisableCache() []client.Object {
	var objs []client.Object
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.DisableWorkflowContextConfigMapCache) {
		objs = append(objs, &corev1.ConfigMap{})
	}
	return objs
}
