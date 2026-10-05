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
	"math/rand"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/slices"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var (
	// MarkWithProbability optimize ResourceTracker gc for legacy resource by reducing the frequency of outdated rt check
	MarkWithProbability = 0.1
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
	disableLegacyGC            bool
	disableRevisionGC          bool

	order v1alpha1.GarbageCollectOrder

	revisionLimit int
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
//
//	a. rootRT and currentRT will be marked as deleted only when application is marked as deleted (DeleteTimestamp is
//	   not nil).
//	b. historyRTs will be marked as deleted if at least one of the below conditions met
//	   i.  GarbageCollectionMode is not set to `passive`
//	   ii. All managed resources are RECYCLED. (RECYCLED means resource does not exist or managed by latest
//	       resourcetrackers)
//
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
//
//	For one single application, the deletion will follow Mark -> Finalize -> Sweep
func (h *resourceKeeper) GarbageCollect(ctx context.Context, options ...GCOption) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	return h.garbageCollect(ctx, h.buildGCConfig(ctx, options...))
}

func (h *resourceKeeper) buildGCConfig(ctx context.Context, options ...GCOption) *gcConfig {
	if h.policies.GarbageCollect != nil {
		if h.policies.GarbageCollect.KeepLegacyResource {
			options = append(options, PassiveGCOption{})
		}
		switch h.policies.GarbageCollect.Order {
		case v1alpha1.OrderDependency:
			options = append(options, DependencyGCOption{})
		default:
		}
		if h.policies.GarbageCollect.ContinueOnFailure && failedRun(ctx) {
			options = slices.Filter(options, func(opt GCOption) bool {
				_, ok := opt.(DisableMarkStageGCOption)
				return !ok
			})
		}
	}
	return newGCConfig(options...)
}

func (h *resourceKeeper) garbageCollect(ctx context.Context, cfg *gcConfig) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	gc := gcHandler{
		resourceKeeper: h,
		cfg:            cfg,
	}
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
	// Garbage Collect Legacy ResourceTrackers
	// Legacy (pre-v1.2) trackers are an Application concern; the creator supplies the step,
	// and it runs once the owner is on new trackers, or while the owner is being deleted.
	if !cfg.disableLegacyGC && h.opts.LegacyGarbageCollect != nil && (h.owner.Deleting() || h._currentRT != nil) {
		if err = h.opts.LegacyGarbageCollect(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to garbage collect legacy resource trackers")
		}
	}

	if h.opts.Collect != nil {
		if err = h.opts.Collect(ctx, gc.collectState()); err != nil {
			return false, waiting, err
		}
	}

	return finished, waiting, nil
}

// collectState is what this pass leaves for Options.Collect.
func (h *gcHandler) collectState() CollectState {
	inUse := func() map[string]bool {
		components := map[string]bool{}
		for _, entry := range h.cache.m.Data() {
			for _, rt := range entry.usedBy {
				if rt.GetDeletionTimestamp() == nil || len(rt.GetFinalizers()) != 0 {
					components[entry.mr.ComponentKey()] = true
				}
			}
		}
		return components
	}
	return CollectState{
		Trackers:                   resourcetracker.Trackers{Root: h._rootRT, Current: h._currentRT, History: h._historyRTs, ComponentRevision: h._crRT},
		InUseComponents:            inUse,
		RevisionLimit:              h.cfg.revisionLimit,
		DisableRevisionGC:          h.cfg.disableRevisionGC,
		DisableComponentRevisionGC: h.cfg.disableComponentRevisionGC,
	}
}

// gcHandler gc detail implementations
type gcHandler struct {
	*resourceKeeper
	cfg *gcConfig
}

func (h *gcHandler) monitor(stage string) func() {
	begin := time.Now()
	return func() {
		h.observeStage("gc-rt."+stage, begin)
	}
}

func (h *gcHandler) regularizeResourceTracker(rts ...*v1beta1.ResourceTracker) {
	for _, rt := range rts {
		if rt == nil {
			continue
		}
		for i, mr := range rt.Spec.ManagedResources {
			if ok, err := kubeutil.IsClusterScope(mr.GroupVersionKind(), h.Client.RESTMapper()); err == nil && ok {
				rt.Spec.ManagedResources[i].Namespace = ""
			}
		}
	}
}

func (h *gcHandler) Init() {
	cb := h.monitor("init")
	defer cb()
	rts := append(h._historyRTs, h._currentRT, h._rootRT) // nolint
	h.regularizeResourceTracker(rts...)
	h.cache.registerResourceTrackers(rts...)
}

func (h *gcHandler) scan(ctx context.Context) (inactiveRTs []*v1beta1.ResourceTracker) {
	if h.owner.Deleting() {
		inactiveRTs = append(inactiveRTs, h._historyRTs...)
		inactiveRTs = append(inactiveRTs, h._currentRT, h._rootRT, h._crRT)
	} else {
		if h.cfg.passive {
			inactiveRTs = []*v1beta1.ResourceTracker{}
			if rand.Float64() > MarkWithProbability { //nolint
				return inactiveRTs
			}
			for _, rt := range h._historyRTs {
				if rt != nil {
					inactive := true
					for _, mr := range rt.Spec.ManagedResources {
						entry := h.cache.get(h.asRequester(ctx), mr)
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
	cb := h.monitor("mark")
	defer cb()
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
		entry := h.cache.get(h.asRequester(ctx), mr)
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
	cb := h.monitor("sweep")
	defer cb()
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
	ctx = h.asRequester(ctx)
	switch h.cfg.order {
	case v1alpha1.OrderDependency:
		// Without dependency information, dependency order is plain order.
		if h.opts.Dependents == nil {
			break
		}
		for _, mr := range rt.Spec.ManagedResources {
			if err := h.deleteIndependentComponent(ctx, mr, rt); err != nil {
				return err
			}
		}
		return nil
	default:
	}
	for _, mr := range rt.Spec.ManagedResources {
		if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
			return err
		}
	}
	return nil
}

func (h *gcHandler) deleteIndependentComponent(ctx context.Context, mr v1beta1.ManagedResource, rt *v1beta1.ResourceTracker) error {
	dependent := h.opts.Dependents(mr.Component)
	if len(dependent) == 0 {
		if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
			return err
		}
	} else {
		dependentClear := true
		for _, mr := range rt.Spec.ManagedResources {
			if slices.Contains(dependent, mr.Component) {
				entry := h.cache.get(ctx, mr)
				if entry.gcExecutorRT != rt {
					continue
				}
				if entry.err != nil {
					continue
				}
				if entry.exists {
					dependentClear = false
					break
				}
			}
		}
		if dependentClear {
			if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateSharedManagedResourceOwner hands a shared resource to the first remaining sharer:
// it records the sharer list and labels the resource as that sharer's (see
// resourcetracker.LabelsForKey), dropping any owner.oam.dev/* labels of the previous owner.
func UpdateSharedManagedResourceOwner(ctx context.Context, cli client.Client, manifest *unstructured.Unstructured, sharedBy string) error {
	labels := resourcetracker.LabelsForKey(apply.FirstSharer(sharedBy))
	if len(labels) == 0 {
		// Nobody to hand it to, which a malformed sharer list can cause. Leaving the marks as
		// they are keeps the resource owned by someone; stripping them would leave it owned by
		// nobody, and the next sharer's ownership check would then refuse to touch it.
		return errors.Errorf("cannot hand over resource %s/%s: no owner in %q", manifest.GetNamespace(), manifest.GetName(), sharedBy)
	}
	kubeutil.AddAnnotations(manifest, map[string]string{oam.AnnotationAppSharedBy: sharedBy})
	kubeutil.RemoveLabels(manifest, resourcetracker.OwnerLabelKeys())
	kubeutil.AddLabels(manifest, labels)
	return cli.Update(ctx, manifest)
}

func (h *gcHandler) deleteManagedResource(ctx context.Context, mr v1beta1.ManagedResource, rt *v1beta1.ResourceTracker) error {
	entry := h.cache.get(ctx, mr)
	if entry.gcExecutorRT != rt {
		return nil
	}
	if entry.err != nil {
		return entry.err
	}
	if !entry.exists {
		return nil
	}
	return DeleteManagedResource(ctx, h.Client, mr, entry.obj, h.owner, h.policies.GarbageCollect)
}

// DeleteManagedResource lets owner go of a resource it manages: if others share it, owner
// leaves the sharer list and control passes to the next sharer; if it is to be kept (skip-GC,
// an orphaning owner, or the garbage-collect policy), owner's marks are removed; otherwise it
// is deleted. garbageCollectPolicy may be nil.
func DeleteManagedResource(ctx context.Context, cli client.Client, mr v1beta1.ManagedResource, obj *unstructured.Unstructured, owner resourcetracker.Tracked, garbageCollectPolicy *v1alpha1.GarbageCollectPolicySpec) error {
	_ctx := pkgmulticluster.WithCluster(ctx, mr.Cluster)
	if annotations := obj.GetAnnotations(); annotations != nil && annotations[oam.AnnotationAppSharedBy] != "" {
		sharedBy := apply.RemoveSharer(annotations[oam.AnnotationAppSharedBy], owner.Key())
		if sharedBy != "" {
			if err := UpdateSharedManagedResourceOwner(_ctx, cli, obj, sharedBy); err != nil {
				return errors.Wrapf(err, "failed to remove sharer from resource %s", mr.ResourceKey())
			}
			return nil
		}
		kubeutil.RemoveAnnotations(obj, []string{oam.AnnotationAppSharedBy})
	}

	var opts []client.DeleteOption
	var isOrphan bool
	if garbageCollectPolicy != nil {
		isOrphan, opts = garbageCollectPolicy.FindDeleteOption(obj)
	}

	if mr.SkipGC || owner.Orphaning() || isOrphan {
		owner.Release(obj)
		return errors.Wrapf(cli.Update(_ctx, obj), "skipping deletion for resource")
	}

	if err := cli.Delete(_ctx, obj, opts...); err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete resource %s", mr.ResourceKey())
	}
	return nil
}

func (h *gcHandler) Finalize(ctx context.Context) error {
	cb := h.monitor("finalize")
	defer cb()
	for _, rt := range append(h._historyRTs, h._currentRT, h._rootRT) {
		if rt != nil && rt.GetDeletionTimestamp() != nil && meta.FinalizerExists(rt, resourcetracker.Finalizer) {
			if err := h.recycleResourceTracker(ctx, rt); err != nil {
				return err
			}
		}
	}
	return nil
}
