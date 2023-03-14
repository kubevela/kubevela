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
	"fmt"
	"sync"
	"time"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	ctrlutils "github.com/oam-dev/kubevela/pkg/controller/utils"
)

var (
	ApplicationRevisionDefinitionStoragePruneDuration = 5 * time.Minute
	OptimizeApplicationRevisionDefinitionStorage      = false
)

func AddFlags(fs *pflag.FlagSet) {
	fs.DurationVarP(&ApplicationRevisionDefinitionStoragePruneDuration, "application-revision-definition-storage-prune-duration", "", ApplicationRevisionDefinitionStoragePruneDuration, "the duration for running application revision storage pruning")
	fs.BoolVarP(&OptimizeApplicationRevisionDefinitionStorage, "optimize-application-revision-definition-storage", "", OptimizeApplicationRevisionDefinitionStorage, "if enabled, application revision definition storage will be optimized by using common cache")
}

type ObjectCache[T any] struct {
	mu      sync.RWMutex
	objects map[string]*T
}

func NewObjectCache[T any]() *ObjectCache[T] {
	return &ObjectCache[T]{
		objects: map[string]*T{},
	}
}

func (in *ObjectCache[T]) GetCacheAddress(obj *T) *T {
	hash, err := ctrlutils.ComputeSpecHash(obj)
	if err != nil {
		return obj
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if _, found := in.objects[hash]; !found {
		in.objects[hash] = obj
	}
	return in.objects[hash]
}

func (in *ObjectCache[T]) Size() int {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return len(in.objects)
}

func (in *ObjectCache[T]) Prune(inuse sets.String) {
	in.mu.Lock()
	defer in.mu.Unlock()
	defs := map[string]*T{}
	for k, v := range in.objects {
		if _, found := inuse[k]; found {
			defs[k] = v
		}
	}
	in.objects = defs
}

type DefinitionCache struct {
	ComponentDefinitionCache    *ObjectCache[v1beta1.ComponentDefinition]
	TraitDefinitionCache        *ObjectCache[v1beta1.TraitDefinition]
	WorkflowStepDefinitionCache *ObjectCache[v1beta1.WorkflowStepDefinition]
}

func NewDefinitionCache() *DefinitionCache {
	return &DefinitionCache{
		ComponentDefinitionCache:    NewObjectCache[v1beta1.ComponentDefinition](),
		TraitDefinitionCache:        NewObjectCache[v1beta1.TraitDefinition](),
		WorkflowStepDefinitionCache: NewObjectCache[v1beta1.WorkflowStepDefinition](),
	}
}

func (in *DefinitionCache) StartPrune(ctx context.Context, store cache.Cache, duration time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			revs := &v1beta1.ApplicationRevisionList{}
			if err := store.List(ctx, revs); err != nil {
				klog.Error("failed to list revisions while cleaning definition cache: %s", err.Error())
				break
			}
			inuseComponentDefinitions := sets.String{}
			inuseTraitDefinitions := sets.String{}
			inuseWorkflowStepDefinitions := sets.String{}
			for _, item := range revs.Items {
				for _, def := range item.Spec.ComponentDefinitions {
					if hash, err := ctrlutils.ComputeSpecHash(def); err == nil {
						inuseComponentDefinitions[hash] = struct{}{}
					}
				}
				for _, def := range item.Spec.TraitDefinitions {
					if hash, err := ctrlutils.ComputeSpecHash(def); err == nil {
						inuseTraitDefinitions[hash] = struct{}{}
					}
				}
				for _, def := range item.Spec.WorkflowStepDefinitions {
					if hash, err := ctrlutils.ComputeSpecHash(def); err == nil {
						inuseWorkflowStepDefinitions[hash] = struct{}{}
					}
				}
			}
			in.ComponentDefinitionCache.Prune(inuseComponentDefinitions)
			in.TraitDefinitionCache.Prune(inuseTraitDefinitions)
			in.WorkflowStepDefinitionCache.Prune(inuseWorkflowStepDefinitions)
			time.Sleep(duration)
		}
	}
}

var DefaultDefinitionCache = singleton.NewSingleton(NewDefinitionCache)

func AddApplicationRevisionTransformFuncToCacheOption(opts *cache.Options) {
	if OptimizeApplicationRevisionDefinitionStorage {
		if opts.TransformByObject == nil {
			opts.TransformByObject = map[client.Object]kcache.TransformFunc{}
		}
		opts.TransformByObject[&v1beta1.ApplicationRevision{}] = func(i interface{}) (interface{}, error) {
			apprev, ok := i.(*v1beta1.ApplicationRevision)
			if !ok {
				return nil, fmt.Errorf("not apprev")
			}
			for key := range apprev.Spec.ComponentDefinitions {
				apprev.Spec.ComponentDefinitions[key] = DefaultDefinitionCache.Get().ComponentDefinitionCache.GetCacheAddress(apprev.Spec.ComponentDefinitions[key])
			}
			for key := range apprev.Spec.TraitDefinitions {
				apprev.Spec.TraitDefinitions[key] = DefaultDefinitionCache.Get().TraitDefinitionCache.GetCacheAddress(apprev.Spec.TraitDefinitions[key])
			}
			for key := range apprev.Spec.WorkflowStepDefinitions {
				apprev.Spec.WorkflowStepDefinitions[key] = DefaultDefinitionCache.Get().WorkflowStepDefinitionCache.GetCacheAddress(apprev.Spec.WorkflowStepDefinitions[key])
			}
			return apprev, nil
		}
	}
}
