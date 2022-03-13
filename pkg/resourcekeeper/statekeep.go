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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// StateKeep run this function to keep resources up-to-date
func (h *resourceKeeper) StateKeep(ctx context.Context) error {
	if h.applyOncePolicy != nil && h.applyOncePolicy.Enable {
		return nil
	}
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
						deleteCtx = oamutil.SetServiceAccountInContext(deleteCtx, h.app.Namespace, h.app.Spec.ServiceAccountName)
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
					applyCtx = oamutil.SetServiceAccountInContext(applyCtx, h.app.Namespace, h.app.Spec.ServiceAccountName)
					if err = h.applicator.Apply(applyCtx, manifest, apply.MustBeControlledByApp(h.app)); err != nil {
						return errors.Wrapf(err, "failed to re-apply resource %s from resourcetracker %s", mr.ResourceKey(), rt.Name)
					}
				}
			}
		}
	}
	return nil
}
