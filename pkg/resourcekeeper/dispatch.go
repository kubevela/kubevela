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

	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	"github.com/oam-dev/kubevela/pkg/utils/parallel"
)

// MaxDispatchConcurrent is the max dispatch concurrent number
var MaxDispatchConcurrent = 10

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
	// 0. check admission
	if err = h.AdmissionCheck(ctx, manifests); err != nil {
		return err
	}
	// 1. record manifests in resourcetracker
	if err = h.record(ctx, manifests, options...); err != nil {
		return err
	}
	// 2. apply manifests
	if err = h.dispatch(ctx, manifests); err != nil {
		return err
	}
	return nil
}

func (h *resourceKeeper) record(ctx context.Context, manifests []*unstructured.Unstructured, options ...DispatchOption) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var rootManifests []*unstructured.Unstructured
	var versionManifests []*unstructured.Unstructured

	for _, manifest := range manifests {
		if manifest != nil {
			_options := options
			if h.garbageCollectPolicy != nil {
				if strategy := h.garbageCollectPolicy.FindStrategy(manifest); strategy != nil {
					_options = append(_options, GarbageCollectStrategyOption(*strategy))
				}
			}
			cfg := newDispatchConfig(_options...)
			if !cfg.skipRT {
				if cfg.useRoot {
					rootManifests = append(rootManifests, manifest)
				} else {
					versionManifests = append(versionManifests, manifest)
				}
			}
		}
	}

	cfg := newDispatchConfig(options...)
	if len(rootManifests) != 0 {
		rt, err := h.getRootRT(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker")
		}
		if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, rootManifests, cfg.metaOnly); err != nil {
			return errors.Wrapf(err, "failed to record resources in resourcetracker %s", rt.Name)
		}
	}

	rt, err := h.getCurrentRT(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, versionManifests, cfg.metaOnly); err != nil {
		return errors.Wrapf(err, "failed to record resources in resourcetracker %s", rt.Name)
	}
	return nil
}

func (h *resourceKeeper) dispatch(ctx context.Context, manifests []*unstructured.Unstructured) error {
	applyOpts := []apply.ApplyOption{apply.MustBeControlledByApp(h.app), apply.NotUpdateRenderHashEqual()}
	errs := parallel.Run(func(manifest *unstructured.Unstructured) error {
		applyCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(manifest))
		applyCtx = oamutil.SetServiceAccountInContext(applyCtx, h.app.Namespace, h.app.Spec.ServiceAccountName)
		return h.applicator.Apply(applyCtx, manifest, applyOpts...)
	}, manifests, MaxDispatchConcurrent)
	return velaerrors.AggregateErrors(errs.([]error))
}
