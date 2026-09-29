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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Policies are the apply-time policies a keeper honours. An Application carries them in
// spec.policies; other owners pass them in directly (Component.spec.apply).
type Policies struct {
	ApplyOnce      *v1alpha1.ApplyOncePolicySpec
	GarbageCollect *v1alpha1.GarbageCollectPolicySpec
	SharedResource *v1alpha1.SharedResourcePolicySpec
	TakeOver       *v1alpha1.TakeOverPolicySpec
	ReadOnly       *v1alpha1.ReadOnlyPolicySpec
	ResourceUpdate *v1alpha1.ResourceUpdatePolicySpec
}

// New creates a keeper for an owner, given its Tracked (see resourcetracker.Tracked). What
// differs by kind comes from the Tracked and from Options (see pkg/appkeeper for Applications).
func New(ctx context.Context, cli client.Client, tracked resourcetracker.Tracked, policies Policies, opts Options) (_ ResourceKeeper, err error) {
	h := &resourceKeeper{
		Client:     cli,
		owner:      tracked,
		policies:   policies,
		opts:       opts,
		applicator: apply.NewAPIApplicator(cli),
		cache:      newResourceCache(cli, tracked),
	}
	if err = h.loadResourceTrackers(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to load resourcetrackers")
	}
	return h, nil
}

// mustBeControlled is the ownership check applied to every dispatched resource.
func (h *resourceKeeper) mustBeControlled() apply.ApplyOption {
	return apply.MustBeControlledBy(h.owner)
}

// policyApplyOptions are the options the owner's policies imply for this manifest, in the
// order they must run: sharing first, because the ownership check reads whether the apply is
// a shared one. Dispatch and state-keep both apply a manifest, so both need them.
func (h *resourceKeeper) policyApplyOptions(manifest *unstructured.Unstructured) []apply.ApplyOption {
	var opts []apply.ApplyOption
	if strategy := h.getUpdateStrategy(manifest); strategy != nil {
		opts = append(opts, apply.WithUpdateStrategy(*strategy))
	}
	if h.canTakeOver(manifest) {
		opts = append(opts, apply.TakeOver())
	}
	if h.isReadOnly(manifest) {
		opts = append(opts, apply.ReadOnly())
	}
	if h.isShared(manifest) {
		opts = append(opts, apply.SharedBy(h.owner))
	}
	return opts
}

// labelWithOwner marks manifests as owned, as the owner's kind does it.
func (h *resourceKeeper) labelWithOwner(manifests []*unstructured.Unstructured) {
	for _, manifest := range manifests {
		if manifest != nil {
			h.owner.Stamp(manifest)
		}
	}
}
