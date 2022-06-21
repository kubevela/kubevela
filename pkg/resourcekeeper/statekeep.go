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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// StateKeep run this function to keep resources up-to-date
func (h *resourceKeeper) StateKeep(ctx context.Context) error {
	if h.applyOncePolicy != nil && h.applyOncePolicy.Enable && h.applyOncePolicy.Rules == nil {
		return nil
	}
	ctx = auth.ContextWithUserInfo(ctx, h.app)
	for _, rt := range []*v1beta1.ResourceTracker{h._currentRT, h._rootRT} {
		if rt != nil && rt.GetDeletionTimestamp() == nil {
			for _, mr := range rt.Spec.ManagedResources {
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
						continue
					}
					manifest, err := mr.ToUnstructuredWithData()
					if err != nil {
						return errors.Wrapf(err, "failed to decode resource %s from resourcetracker", mr.ResourceKey())
					}
					applyCtx := multicluster.ContextWithClusterName(ctx, mr.Cluster)
					manifest, err = ApplyStrategies(applyCtx, h, manifest)
					if err != nil {
						return errors.Wrapf(err, "failed to apply once resource %s from resourcetracker %s", mr.ResourceKey(), rt.Name)
					}
					ao := []apply.ApplyOption{apply.MustBeControlledByApp(h.app)}
					if h.isShared(manifest) {
						ao = append([]apply.ApplyOption{apply.SharedByApp(h.app)}, ao...)
					}
					if err = h.applicator.Apply(applyCtx, manifest, ao...); err != nil {
						return errors.Wrapf(err, "failed to re-apply resource %s from resourcetracker %s", mr.ResourceKey(), rt.Name)
					}
				}
			}
		}
	}
	return nil
}

// ApplyStrategies will generate manifest with applyOnceStrategy
func ApplyStrategies(ctx context.Context, h *resourceKeeper, manifest *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if h.applyOncePolicy == nil {
		return manifest, nil
	}
	applyOncePath := h.applyOncePolicy.FindStrategy(manifest)
	if applyOncePath != nil {
		un := new(unstructured.Unstructured)
		un.SetAPIVersion(manifest.GetAPIVersion())
		un.SetKind(manifest.GetKind())
		err := h.Get(ctx, types.NamespacedName{Name: manifest.GetName(), Namespace: manifest.GetNamespace()}, un)
		if err != nil {
			return nil, err
		}
		for _, path := range applyOncePath.Path {
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
	return manifest, nil
}
