/*
Copyright 2026 The KubeVela Authors.

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
	"time"

	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// Options holds the behaviour a keeper's creator decides. Switches are functions,
// evaluated on each use, so a creator can back them with feature gates that change at
// runtime. A nil field takes the default documented on it, which matches KubeVela's
// default. pkg/appkeeper fills these from KubeVela's gates, auth and metrics.
type Options struct {
	// ApplyOnce applies every resource once and never updates it. Default false
	// (KubeVela: ApplyOnce).
	ApplyOnce func() bool
	// PreDispatchDryRun dry-runs a dispatch before recording and applying it. Default
	// true (KubeVela: PreDispatchDryRun).
	PreDispatchDryRun func() bool
	// Requester returns ctx acting as whoever the owner acts for, so applies and deletes
	// run with their permissions. Default: ctx unchanged, the keeper's own identity
	// (KubeVela: the user recorded on the Application).
	Requester func(context.Context) context.Context
	// ObserveStage records how long a garbage-collection stage took. Default: nothing
	// (KubeVela: the reconcile stage duration histogram).
	ObserveStage func(stage string, took time.Duration)
	// LegacyGarbageCollect removes trackers from an older tracker scheme. The keeper runs it
	// during garbage collection once the owner has a current tracker, or while the owner is
	// being deleted. Default: nothing (KubeVela: pre-v1.2 Application trackers).
	LegacyGarbageCollect func(context.Context) error
	// Collect is garbage collection the owner's kind needs beyond the keeper's own, run at
	// the end of each pass. Default: nothing (KubeVela: Application and component revisions).
	Collect func(context.Context, CollectState) error
	// Dependents returns the components that depend on a component, so dependency-ordered
	// garbage collection deletes dependents first. Default: none, so dependency order is
	// plain order (KubeVela: from the Application's dependsOn, inputs and outputs).
	Dependents func(component string) []string
}

// CollectState is what the keeper knows at the end of a garbage-collection pass, for
// Options.Collect.
type CollectState struct {
	// Trackers are the owner's trackers as loaded, after this pass marked inactive ones.
	Trackers resourcetracker.Trackers
	// InUseComponents returns the components with resources in live or unfinished trackers.
	// It walks the whole resource cache, so it is computed only if the hook asks for it.
	InUseComponents func() map[string]bool
	// RevisionLimit is the revision limit set by RevisionLimitGCOption.
	RevisionLimit int
	// DisableRevisionGC and DisableComponentRevisionGC reflect DisableRevisionGCOption
	// and DisableGCComponentRevisionOption.
	DisableRevisionGC, DisableComponentRevisionGC bool
}

func (h *resourceKeeper) applyOnceSwitch() bool {
	return h.opts.ApplyOnce != nil && h.opts.ApplyOnce()
}

func (h *resourceKeeper) preDispatchDryRun() bool {
	return h.opts.PreDispatchDryRun == nil || h.opts.PreDispatchDryRun()
}

// asRequester acts for whoever the owner acts for.
func (h *resourceKeeper) asRequester(ctx context.Context) context.Context {
	if h.opts.Requester == nil {
		return asSelf(ctx) // no requester: act as the keeper, never as whoever called it
	}
	return h.opts.Requester(ctx)
}

// asSelf drops any requester identity from ctx, so the keeper writes its own
// ResourceTrackers with its own permissions.
func asSelf(ctx context.Context) context.Context {
	return request.WithUser(ctx, nil)
}

func (h *resourceKeeper) observeStage(stage string, since time.Time) {
	if h.opts.ObserveStage != nil {
		h.opts.ObserveStage(stage, time.Since(since))
	}
}
