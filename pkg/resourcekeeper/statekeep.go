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

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/kubevela/pkg/util/maps"
	"github.com/kubevela/pkg/util/slices"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// StateKeep run this function to keep resources up-to-date
func (h *resourceKeeper) StateKeep(ctx context.Context) error {
	if h.applyOncePolicy != nil && h.applyOncePolicy.Enable && h.applyOncePolicy.Rules == nil {
		return nil
	}
	ctx = auth.ContextWithUserInfo(ctx, h.app)
	mrs := make(map[string]v1beta1.ManagedResource)
	belongs := make(map[string]*v1beta1.ResourceTracker)
	for _, rt := range []*v1beta1.ResourceTracker{h._currentRT, h._rootRT} {
		if rt != nil && rt.GetDeletionTimestamp() == nil {
			for _, mr := range rt.Spec.ManagedResources {
				key := mr.ResourceKey()
				mrs[key] = mr
				belongs[key] = rt
			}
		}
	}
	errs := slices.ParMap(maps.Values(mrs), func(mr v1beta1.ManagedResource) error {
		rt := belongs[mr.ResourceKey()]
		entry := h.cache.get(ctx, mr)
		if entry.err != nil {
			return entry.err
		}
		if mr.Deleted {
			if entry.exists && entry.obj != nil && entry.obj.GetDeletionTimestamp() == nil {
				deleteCtx := multicluster.ContextWithClusterName(ctx, mr.Cluster)
				if err := h.Client.Delete(deleteCtx, entry.obj); err != nil {
					return errors.Wrapf(err, "failed to delete outdated resource %s in resourcetracker %s", mr.ResourceKey(), rt.Name)
				}
			}
		} else {
			if mr.Data == nil || mr.Data.Raw == nil {
				// no state-keep
				return nil
			}
			manifest, err := mr.ToUnstructuredWithData()
			if err != nil {
				return errors.Wrapf(err, "failed to decode resource %s from resourcetracker", mr.ResourceKey())
			}
			applyCtx := multicluster.ContextWithClusterName(ctx, mr.Cluster)
			manifest, err = ApplyStrategies(applyCtx, h, manifest, v1alpha1.ApplyOnceStrategyOnAppStateKeep)
			if err != nil {
				return errors.Wrapf(err, "failed to apply once resource %s from resourcetracker %s", mr.ResourceKey(), rt.Name)
			}
			ao := []apply.ApplyOption{apply.MustBeControlledByApp(h.app)}
			if h.isShared(manifest) {
				ao = append([]apply.ApplyOption{apply.SharedByApp(h.app)}, ao...)
			}
			if h.isReadOnly(manifest) {
				ao = append([]apply.ApplyOption{apply.ReadOnly()}, ao...)
			}
			if h.canTakeOver(manifest) {
				ao = append([]apply.ApplyOption{apply.TakeOver()}, ao...)
			}
			if err = h.applicator.Apply(applyCtx, manifest, ao...); err != nil {
				return errors.Wrapf(err, "failed to re-apply resource %s from resourcetracker %s", mr.ResourceKey(), rt.Name)
			}
		}
		return nil
	}, slices.Parallelism(MaxDispatchConcurrent))
	return velaerrors.AggregateErrors(errs)
}

// ApplyStrategies will generate manifest with applyOnceStrategy
func ApplyStrategies(ctx context.Context, h *resourceKeeper, manifest *unstructured.Unstructured, matchedAffectStage v1alpha1.ApplyOnceAffectStrategy) (*unstructured.Unstructured, error) {
	if h.applyOncePolicy == nil {
		return manifest, nil
	}
	strategy := h.applyOncePolicy.FindStrategy(manifest)
	if strategy != nil {
		affectStage := strategy.ApplyOnceAffectStrategy
		if shouldMerge(affectStage, matchedAffectStage) {
			un := new(unstructured.Unstructured)
			un.SetAPIVersion(manifest.GetAPIVersion())
			un.SetKind(manifest.GetKind())
			err := h.Get(ctx, types.NamespacedName{Name: manifest.GetName(), Namespace: manifest.GetNamespace()}, un)
			if err != nil {
				if kerrors.IsNotFound(err) {
					return manifest, nil
				}
				return nil, err
			}
			return mergeValue(strategy.Path, manifest, un)
		}

	}
	return manifest, nil
}

func shouldMerge(affectStage v1alpha1.ApplyOnceAffectStrategy, matchedAffectType v1alpha1.ApplyOnceAffectStrategy) bool {
	return affectStage == "" || affectStage == v1alpha1.ApplyOnceStrategyAlways || affectStage == matchedAffectType
}

func mergeValue(paths []string, manifest *unstructured.Unstructured, un *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	for _, path := range paths {
		if path == "*" {
			manifest = un.DeepCopy()
			break
		}
		value, err := fieldpath.Pave(un.UnstructuredContent()).GetValue(path)
		if err != nil {
			return nil, err
		}
		err = fieldpath.Pave(manifest.UnstructuredContent()).SetValue(path, value)
		if err != nil {
			return nil, err
		}
	}
	return manifest, nil
}
