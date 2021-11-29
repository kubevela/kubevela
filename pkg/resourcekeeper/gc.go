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

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// GCOption option for gc
type GCOption interface {
	ApplyToGCConfig(*gcConfig)
}

type gcConfig struct {
	passive bool

	disableMark                bool
	disableSweep               bool
	disableFinalize            bool
	disableComponentRevisionGC bool
}

func newGCConfig(options ...GCOption) *gcConfig {
	cfg := &gcConfig{}
	for _, option := range options {
		option.ApplyToGCConfig(cfg)
	}
	return cfg
}

// GarbageCollect recycle resources and handle finalizers for resourcetracker
// Application Resource Garbage Collection follows three stages
//
// 1. Mark Stage
// Controller will find all resourcetrackers for the target application and decide which resourcetrackers should be
// deleted. Decision rules including:
//    a. rootRT and currentRT will be marked as deleted only when application is marked as deleted (DeleteTimestamp is
//       not nil).
//    b. historyRTs will be marked as deleted if at least one of the below conditions met
//       i.  GarbageCollectionMode is not set to `passive`
//       ii. All managed resources are RECYCLED. (RECYCLED means resource does not exist or managed by latest
//           resourcetrackers)
// NOTE: Mark Stage will always work for each application reconcile, not matter whether workflow is ended
//
// 2. Sweep Stage
// Controller will check all resourcetrackers marked to be deleted. If all managed resources are recycled, finalizer in
// resourcetracker will be removed.
//
// 3. Finalize Stage
// Controller will finalize all resourcetrackers marked to be deleted. All managed resources are recycled.
//
// NOTE: Mark Stage will only work when Workflow succeeds. Check/Finalize Stage will always work.
//       For one single application, the deletion will follow Mark -> Finalize -> Sweep
func (h *resourceKeeper) GarbageCollect(ctx context.Context, options ...GCOption) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	if h.garbageCollectPolicy != nil && h.garbageCollectPolicy.KeepLegacyResource {
		options = append(options, PassiveGCOption{})
	}
	cfg := newGCConfig(options...)
	return h.garbageCollect(ctx, cfg)
}

func (h *resourceKeeper) garbageCollect(ctx context.Context, cfg *gcConfig) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	gc := gcHandler{resourceKeeper: h, cfg: cfg}
	gc.Init()
	// Mark Stage
	if !cfg.disableMark {
		if err = gc.Mark(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to mark inactive resourcetrackers")
		}
	}
	// Sweep Stage
	if !cfg.disableSweep {
		if finished, waiting, err = gc.Sweep(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to sweep resourcetrackers to be deleted")
		}
	}
	// Finalize Stage
	if !cfg.disableFinalize && !finished {
		if err = gc.Finalize(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to finalize resourcetrackers to be deleted")
		}
	}
	// Garbage Collect Component Revision in unused components
	if !cfg.disableComponentRevisionGC {
		if err = gc.GarbageCollectComponentRevisionResourceTracker(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to garbage collect component revisions in unused components")
		}
	}
	return finished, waiting, nil
}

// gcHandler gc detail implementations
type gcHandler struct {
	*resourceKeeper
	cfg *gcConfig
}

func (h *gcHandler) Init() {
	h.cache.registerResourceTrackers(append(h._historyRTs, h._currentRT, h._rootRT)...)
}

func (h *gcHandler) scan(ctx context.Context) (inactiveRTs []*v1beta1.ResourceTracker) {
	if h.app.GetDeletionTimestamp() != nil {
		inactiveRTs = append(inactiveRTs, h._historyRTs...)
		inactiveRTs = append(inactiveRTs, h._currentRT, h._rootRT, h._crRT)
	} else {
		if h.cfg.passive {
			inactiveRTs = []*v1beta1.ResourceTracker{}
			for _, rt := range h._historyRTs {
				if rt != nil {
					inactive := true
					for _, mr := range rt.Spec.ManagedResources {
						entry := h.cache.get(ctx, mr)
						if entry.err == nil && (entry.gcExecutorRT != rt || !entry.exists) {
							continue
						}
						inactive = false
					}
					if inactive {
						inactiveRTs = append(inactiveRTs, rt)
					}
				}
			}
		} else {
			inactiveRTs = h._historyRTs
		}
	}
	return inactiveRTs
}

func (h *gcHandler) Mark(ctx context.Context) error {
	inactiveRTs := h.scan(ctx)
	for _, rt := range inactiveRTs {
		if rt != nil && rt.GetDeletionTimestamp() == nil {
			if err := h.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				return err
			}
			_rt := &v1beta1.ResourceTracker{}
			if err := h.Client.Get(ctx, client.ObjectKeyFromObject(rt), _rt); err != nil {
				if !kerrors.IsNotFound(err) {
					return err
				}
			} else {
				_rt.DeepCopyInto(rt)
			}
		}
	}
	return nil
}

// checkAndRemoveResourceTrackerFinalizer return (all resource recycled, error)
func (h *gcHandler) checkAndRemoveResourceTrackerFinalizer(ctx context.Context, rt *v1beta1.ResourceTracker) (bool, v1beta1.ManagedResource, error) {
	for _, mr := range rt.Spec.ManagedResources {
		entry := h.cache.get(ctx, mr)
		if entry.err != nil {
			return false, entry.mr, entry.err
		}
		if entry.exists && entry.gcExecutorRT == rt {
			return false, entry.mr, nil
		}
	}
	meta.RemoveFinalizer(rt, resourcetracker.Finalizer)
	return true, v1beta1.ManagedResource{}, h.Client.Update(ctx, rt)
}

func (h *gcHandler) Sweep(ctx context.Context) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	finished = true
	for _, rt := range append(h._historyRTs, h._currentRT, h._rootRT) {
		if rt != nil && rt.GetDeletionTimestamp() != nil {
			_finished, mr, err := h.checkAndRemoveResourceTrackerFinalizer(ctx, rt)
			if err != nil {
				return false, waiting, err
			}
			if !_finished {
				finished = false
				waiting = append(waiting, mr)
			}
		}
	}
	return finished, waiting, nil
}

func (h *gcHandler) recycleResourceTracker(ctx context.Context, rt *v1beta1.ResourceTracker) error {
	for _, mr := range rt.Spec.ManagedResources {
		entry := h.cache.get(ctx, mr)
		if entry.gcExecutorRT != rt {
			continue
		}
		if entry.err != nil {
			return entry.err
		}
		if entry.exists {
			if err := h.Client.Delete(multicluster.ContextWithClusterName(ctx, mr.Cluster), entry.obj); err != nil && !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete resource %s", mr.ResourceKey())
			}
		}
	}
	return nil
}

func (h *gcHandler) Finalize(ctx context.Context) error {
	for _, rt := range append(h._historyRTs, h._currentRT, h._rootRT) {
		if rt != nil && rt.GetDeletionTimestamp() != nil && meta.FinalizerExists(rt, resourcetracker.Finalizer) {
			if err := h.recycleResourceTracker(ctx, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *gcHandler) GarbageCollectComponentRevisionResourceTracker(ctx context.Context) error {
	if h._crRT == nil {
		return nil
	}
	inUseComponents := map[string]bool{}
	for _, entry := range h.cache.m {
		for _, rt := range entry.usedBy {
			if rt.GetDeletionTimestamp() == nil || len(rt.GetFinalizers()) != 0 {
				inUseComponents[entry.mr.ComponentKey()] = true
			}
		}
	}
	var managedResources []v1beta1.ManagedResource
	for _, cr := range h._crRT.Spec.ManagedResources {
		skipGC := h.app.GetDeletionTimestamp() == nil && (len(h.app.GetAnnotations()[oam.AnnotationAppRollout]) != 0 || h.app.Spec.RolloutPlan != nil) // legacy code for rollout-plan
		if _, exists := inUseComponents[cr.ComponentKey()]; !exists && !skipGC {
			_cr := &v1.ControllerRevision{}
			err := h.Client.Get(multicluster.ContextWithClusterName(ctx, cr.Cluster), cr.NamespacedName(), _cr)
			if err != nil && !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get component revision %s", cr.ResourceKey())
			}
			if err == nil {
				if err = h.Client.Delete(multicluster.ContextWithClusterName(ctx, cr.Cluster), _cr); err != nil && !kerrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete component revision %s", cr.ResourceKey())
				}
			}
		} else {
			managedResources = append(managedResources, cr)
		}
	}
	h._crRT.Spec.ManagedResources = managedResources
	if len(managedResources) == 0 && h._crRT.GetDeletionTimestamp() != nil {
		meta.RemoveFinalizer(h._crRT, resourcetracker.Finalizer)
	}
	if err := h.Client.Update(ctx, h._crRT); err != nil {
		return errors.Wrapf(err, "failed to update controllerrevision RT %s", h._crRT.Name)
	}
	return nil
}
