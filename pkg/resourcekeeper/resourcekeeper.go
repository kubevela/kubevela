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
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// ResourceKeeper handler for dispatching and deleting resources
type ResourceKeeper interface {
	Dispatch(context.Context, []*unstructured.Unstructured, []apply.ApplyOption, ...DispatchOption) error
	Delete(context.Context, []*unstructured.Unstructured, ...DeleteOption) error
	GarbageCollect(context.Context, ...GCOption) (bool, []v1beta1.ManagedResource, error)
	StateKeep(context.Context) error
	ContainsResources([]*unstructured.Unstructured) bool
	// PruneComponentResources deletes what a component no longer renders. See
	// the implementation for why this is not part of Dispatch.
	PruneComponentResources(context.Context, string, []*unstructured.Unstructured) ([]v1beta1.ManagedResource, error)

	// GetAppliedResources returns the current applied resources from the ResourceTracker.
	GetAppliedResources() []common.ClusterObjectReference
}

type resourceKeeper struct {
	client.Client
	// owner is who the keeper acts for.
	owner    resourcetracker.Tracked
	policies Policies
	opts     Options
	mu       sync.Mutex

	applicator  apply.Applicator
	_rootRT     *v1beta1.ResourceTracker
	_currentRT  *v1beta1.ResourceTracker
	_historyRTs []*v1beta1.ResourceTracker
	_crRT       *v1beta1.ResourceTracker

	cache *resourceCache
}

func (h *resourceKeeper) getRootRT(ctx context.Context) (rootRT *v1beta1.ResourceTracker, err error) {
	if h._rootRT == nil {
		if h._rootRT, err = resourcetracker.CreateTracker(localCluster(ctx), h.Client, h.owner, v1beta1.ResourceTrackerTypeRoot); err != nil {
			return nil, err
		}
	}
	return h._rootRT, nil
}

func (h *resourceKeeper) getCurrentRT(ctx context.Context) (currentRT *v1beta1.ResourceTracker, err error) {
	if h._currentRT == nil {
		if h._currentRT, err = resourcetracker.CreateTracker(localCluster(ctx), h.Client, h.owner, v1beta1.ResourceTrackerTypeVersioned); err != nil {
			return nil, err
		}
	}
	return h._currentRT, nil
}

func (h *resourceKeeper) loadResourceTrackers(ctx context.Context) error {
	ctx = localCluster(ctx) // trackers live on the hub, whichever cluster the caller is working in
	trackers, err := h.owner.LoadTrackers(ctx, h.Client)
	if err != nil {
		return err
	}
	h._rootRT, h._currentRT, h._historyRTs, h._crRT = trackers.Root, trackers.Current, trackers.History, trackers.ComponentRevision
	h.labelTrackers(ctx, append([]*v1beta1.ResourceTracker{h._rootRT, h._currentRT, h._crRT}, h._historyRTs...)...)
	return nil
}

// Migration: owner labels. Adds any of the owner's tracker labels a live tracker lacks.
// Trackers are rewritten only when what they record changes, so an idle owner's would never
// gain them. Best-effort: a failed write is logged and retried on the next load. The tracker
// is updated in place, so later writes build on the new version.
func (h *resourceKeeper) labelTrackers(ctx context.Context, rts ...*v1beta1.ResourceTracker) {
	want := h.owner.TrackerLabels()
	for _, rt := range rts {
		if rt == nil || rt.GetDeletionTimestamp() != nil {
			continue
		}
		// No resource version means this did not come from the API server but from a
		// projection, as vela-prism serves to callers without tracker permission. Its name is
		// the real tracker's, and a write carrying no resource version has no precondition,
		// so it would overwrite that tracker with the projection.
		if rt.GetResourceVersion() == "" {
			continue
		}
		missing := false
		for k, v := range want {
			if rt.GetLabels()[k] != v {
				missing = true
				break
			}
		}
		if !missing {
			continue
		}
		// Patch, not update: a tracker's records can be large, and this writes labels only.
		patch := client.MergeFrom(rt.DeepCopy())
		kubeutil.AddLabels(rt, want)
		if err := h.Client.Patch(ctx, rt, patch); err != nil {
			klog.InfoS("could not add owner labels to resource tracker; will retry", "resourcetracker", rt.Name, "err", err)
		}
	}
}

// GetAppliedResources returns all resources from the current ResourceTracker as ClusterObjectReferences.
// Resources pending deletion (Deleted=true) are included as they still exist in the cluster.
// Returns an empty slice if no current ResourceTracker is loaded.
func (h *resourceKeeper) GetAppliedResources() []common.ClusterObjectReference {
	if h._currentRT == nil {
		return []common.ClusterObjectReference{}
	}
	refs := make([]common.ClusterObjectReference, 0, len(h._currentRT.Spec.ManagedResources))
	for _, mr := range h._currentRT.Spec.ManagedResources {
		refs = append(refs, mr.ClusterObjectReference)
	}
	return refs
}
