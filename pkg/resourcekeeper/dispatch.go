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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// DispatchOption option for dispatch
type DispatchOption interface {
	ApplyToDispatchConfig(*dispatchConfig)
}

type dispatchConfig struct {
	rtConfig
	metaOnly bool
}

func newDispatchConfig(options ...DispatchOption) *dispatchConfig {
	cfg := &dispatchConfig{}
	for _, option := range options {
		option.ApplyToDispatchConfig(cfg)
	}
	return cfg
}

// Dispatch dispatch resources
func (h *resourceKeeper) Dispatch(ctx context.Context, manifests []*unstructured.Unstructured, options ...DispatchOption) (err error) {
	if h.applyOncePolicy != nil && h.applyOncePolicy.Enable {
		options = append(options, MetaOnlyOption{})
	}
	for _, manifest := range manifests {
		if manifest != nil {
			_options := options
			if h.garbageCollectPolicy != nil {
				if strategy := h.garbageCollectPolicy.FindStrategy(manifest); strategy != nil {
					_options = append(_options, GarbageCollectStrategyOption(*strategy))
				}
			}
			cfg := newDispatchConfig(_options...)
			if err = h.dispatch(ctx, manifest, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *resourceKeeper) dispatch(ctx context.Context, manifest *unstructured.Unstructured, cfg *dispatchConfig) (err error) {
	// 1. record manifests in resourcetracker
	if !cfg.skipRT {
		var rt *v1beta1.ResourceTracker
		if cfg.useRoot {
			rt, err = h.getRootRT(ctx)
		} else {
			rt, err = h.getCurrentRT(ctx)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker")
		}
		if err = resourcetracker.RecordManifestInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, manifest, cfg.metaOnly); err != nil {
			return errors.Wrapf(err, "failed to record resources in resourcetracker %s", rt.Name)
		}
	}
	// 2. apply manifests
	applyOpts := []apply.ApplyOption{apply.MustBeControlledByApp(h.app), apply.NotUpdateRenderHashEqual()}
	if err := h.applicator.Apply(multicluster.ContextWithClusterName(ctx, oam.GetCluster(manifest)), manifest, applyOpts...); err != nil {
		return errors.Wrapf(err, "cannot apply manifest, name: %s apiVersion: %s kind: %s", manifest.GetName(), manifest.GetAPIVersion(), manifest.GetKind())
	}
	return nil
}
