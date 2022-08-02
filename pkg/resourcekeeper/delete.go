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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// DeleteOption option for delete
type DeleteOption interface {
	ApplyToDeleteConfig(*deleteConfig)
}

type deleteConfig struct {
	rtConfig
}

func newDeleteConfig(options ...DeleteOption) *deleteConfig {
	cfg := &deleteConfig{}
	for _, option := range options {
		option.ApplyToDeleteConfig(cfg)
	}
	return cfg
}

// Delete delete resources
func (h *resourceKeeper) Delete(ctx context.Context, manifests []*unstructured.Unstructured, options ...DeleteOption) (err error) {
	h.ClearNamespaceForClusterScopedResources(manifests)
	if err = h.AdmissionCheck(ctx, manifests); err != nil {
		return err
	}
	for _, manifest := range manifests {
		if manifest != nil {
			_options := options
			if h.garbageCollectPolicy != nil {
				if strategy := h.garbageCollectPolicy.FindStrategy(manifest); strategy != nil {
					_options = append(_options, GarbageCollectStrategyOption(*strategy))
				}
			}
			cfg := newDeleteConfig(_options...)
			if err = h.delete(ctx, manifest, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *resourceKeeper) delete(ctx context.Context, manifest *unstructured.Unstructured, cfg *deleteConfig) (err error) {
	// 1. mark manifests as deleted in resourcetracker
	var rt *v1beta1.ResourceTracker
	if cfg.useRoot || cfg.skipGC {
		rt, err = h.getRootRT(ctx)
	} else {
		rt, err = h.getCurrentRT(ctx)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	if err = resourcetracker.DeletedManifestInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, manifest, false); err != nil {
		return errors.Wrapf(err, "failed to delete resources in resourcetracker")
	}
	// 2. delete manifests
	deleteCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(manifest))
	deleteCtx = auth.ContextWithUserInfo(deleteCtx, h.app)
	if err = h.Client.Delete(deleteCtx, manifest); err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "cannot delete manifest, name: %s apiVersion: %s kind: %s", manifest.GetName(), manifest.GetAPIVersion(), manifest.GetKind())
	}
	return nil
}
