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
	"fmt"

	velaslices "github.com/kubevela/pkg/util/slices"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
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
	creator  string
}

func newDispatchConfig(options ...DispatchOption) *dispatchConfig {
	cfg := &dispatchConfig{}
	for _, option := range options {
		option.ApplyToDispatchConfig(cfg)
	}
	return cfg
}

// Dispatch dispatch resources
func (h *resourceKeeper) Dispatch(ctx context.Context, manifests []*unstructured.Unstructured, applyOpts []apply.ApplyOption, options ...DispatchOption) (err error) {
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ApplyOnce) ||
		(h.applyOncePolicy != nil && h.applyOncePolicy.Enable && h.applyOncePolicy.Rules == nil) {
		options = append(options, MetaOnlyOption{})
	}
	h.ClearNamespaceForClusterScopedResources(manifests)
	// 0. check admission
	if err = h.AdmissionCheck(ctx, manifests); err != nil {
		return err
	}
	// 1. pre-dispatch check
	opts := []apply.ApplyOption{apply.MustBeControlledByApp(h.app), apply.NotUpdateRenderHashEqual()}
	if len(applyOpts) > 0 {
		opts = append(opts, applyOpts...)
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.PreDispatchDryRun) {
		// Create a tracker for namespaces created during dry-run
		tracker := &apply.DryRunNamespaceTracker{}
		dryRunCtx := apply.WithDryRunNamespaceTracker(ctx, tracker)
		
		if err = h.dispatch(dryRunCtx,
			velaslices.Map(manifests, func(manifest *unstructured.Unstructured) *unstructured.Unstructured { return manifest.DeepCopy() }),
			append([]apply.ApplyOption{apply.DryRunAll()}, opts...)); err != nil {
			// Only clean up namespaces if dry-run FAILS
			// This prevents resources from being created if validation fails
			if cleanupErr := tracker.CleanupNamespaces(ctx, h.Client); cleanupErr != nil {
				klog.Warningf("Failed to cleanup dry-run namespaces after failure: %v", cleanupErr)
			}
			return fmt.Errorf("pre-dispatch dryrun failed: %w", err)
		}
		
		// DO NOT clean up namespaces on successful dry-run
		// They will be used by the actual deployment immediately after
		// The namespace will be managed normally by Kubernetes/user after this point
		klog.V(4).Infof("Dry-run succeeded, keeping %d namespace(s) for actual deployment", len(tracker.GetNamespaces()))
	}
	// 2. record manifests in resourcetracker
	if err = h.record(ctx, manifests, options...); err != nil {
		return err
	}
	// 3. apply manifests
	return h.dispatch(ctx, manifests, opts)
}

func (h *resourceKeeper) record(ctx context.Context, manifests []*unstructured.Unstructured, options ...DispatchOption) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var skipGCManifests []*unstructured.Unstructured
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
			switch {
			case cfg.skipGC:
				skipGCManifests = append(skipGCManifests, manifest)
			case cfg.useRoot:
				rootManifests = append(rootManifests, manifest)
			default:
				versionManifests = append(versionManifests, manifest)
			}
		}
	}

	cfg := newDispatchConfig(options...)
	ctx = auth.ContextClearUserInfo(ctx)
	if len(rootManifests)+len(skipGCManifests) != 0 {
		rt, err := h.getRootRT(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker")
		}
		if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, rootManifests, cfg.metaOnly, false, cfg.creator); err != nil {
			return errors.Wrapf(err, "failed to record resources in resourcetracker %s", rt.Name)
		}
		if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, skipGCManifests, cfg.metaOnly, true, cfg.creator); err != nil {
			return errors.Wrapf(err, "failed to record resources (skip-gc) in resourcetracker %s", rt.Name)
		}
	}

	rt, err := h.getCurrentRT(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, versionManifests, cfg.metaOnly, false, cfg.creator); err != nil {
		return errors.Wrapf(err, "failed to record resources in resourcetracker %s", rt.Name)
	}
	return nil
}

// applyManifest applies a single manifest with all necessary options and context setup
func (h *resourceKeeper) applyManifest(ctx context.Context, manifest *unstructured.Unstructured, baseOpts []apply.ApplyOption) error {
	// Setup context with cluster and auth info
	applyCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(manifest))
	applyCtx = auth.ContextWithUserInfo(applyCtx, h.app)
	
	// Preserve namespace tracker if present
	if tracker := apply.GetDryRunNamespaceTracker(ctx); tracker != nil {
		applyCtx = apply.WithDryRunNamespaceTracker(applyCtx, tracker)
	}
	
	// Build apply options based on manifest properties
	ao := baseOpts
	if h.isShared(manifest) {
		ao = append([]apply.ApplyOption{apply.SharedByApp(h.app)}, ao...)
	}
	if h.isReadOnly(manifest) {
		ao = append([]apply.ApplyOption{apply.ReadOnly()}, ao...)
	}
	if h.canTakeOver(manifest) {
		ao = append([]apply.ApplyOption{apply.TakeOver()}, ao...)
	}
	if strategy := h.getUpdateStrategy(manifest); strategy != nil {
		ao = append([]apply.ApplyOption{apply.WithUpdateStrategy(*strategy)}, ao...)
	}
	
	// Apply strategies
	manifest, err := ApplyStrategies(applyCtx, h, manifest, v1alpha1.ApplyOnceStrategyOnAppUpdate)
	if err != nil {
		return errors.Wrapf(err, "failed to apply once policy for application %s,%s", h.app.Name, err.Error())
	}
	
	// Apply the manifest
	return h.applicator.Apply(applyCtx, manifest, ao...)
}

func (h *resourceKeeper) dispatch(ctx context.Context, manifests []*unstructured.Unstructured, applyOpts []apply.ApplyOption) error {
	// Separate resources by dependency priority for staged dispatch
	var crds, namespaces, others []*unstructured.Unstructured
	for _, manifest := range manifests {
		switch manifest.GetKind() {
		case "CustomResourceDefinition":
			crds = append(crds, manifest)
		case "Namespace":
			namespaces = append(namespaces, manifest)
		default:
			others = append(others, manifest)
		}
	}
	
	klog.V(2).Infof("Staged dispatch: %d CRDs, %d namespaces, %d others", len(crds), len(namespaces), len(others))
	
	// Stage 0: Apply CRDs sequentially first (highest priority dependencies)
	for _, manifest := range crds {
		if err := h.applyManifest(ctx, manifest, applyOpts); err != nil {
			return err
		}
	}
	
	// Stage 1: Apply namespaces sequentially second
	for _, manifest := range namespaces {
		if err := h.applyManifest(ctx, manifest, applyOpts); err != nil {
			return err
		}
	}
	
	// Stage 2: Apply other resources in parallel
	if len(others) > 0 {
		errs := velaslices.ParMap(others, func(manifest *unstructured.Unstructured) error {
			return h.applyManifest(ctx, manifest, applyOpts)
		}, velaslices.Parallelism(MaxDispatchConcurrent))
		return velaerrors.AggregateErrors(errs)
	}
	
	return nil
}
