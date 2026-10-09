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

// Package appkeeper builds a ResourceKeeper for an Application: it is the Application's
// implementation of the kind-agnostic library in pkg/resourcekeeper. It reads the resource
// policies from the Application's spec with pkg/policy, which merges several policies of one
// type, and passes them to the keeper as data.
package appkeeper

import (
	"context"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// New creates a ResourceKeeper for app, with the resource policies in its spec.
func New(ctx context.Context, cli client.Client, app *v1beta1.Application) (resourcekeeper.ResourceKeeper, error) {
	policies, err := Policies(app)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse resource policy")
	}
	return resourcekeeper.New(ctx, cli, NewAppResourceTracker(app), policies, KeeperOptions(cli, app))
}

// KeeperOptions backs the keeper's switches with KubeVela's feature gates (read on each
// use), acts as the user recorded on app, records GC stage timings in the reconcile stage
// histogram, and supplies the pre-v1.2 tracker clean-up.
func KeeperOptions(cli client.Client, app *v1beta1.Application) resourcekeeper.Options {
	opts := resourcekeeper.Options{
		ApplyOnce:         gate(features.ApplyOnce),
		PreDispatchDryRun: gate(features.PreDispatchDryRun),
		Requester: func(ctx context.Context) context.Context {
			return auth.ContextWithUserInfo(ctx, app)
		},
		ObserveStage: func(stage string, took time.Duration) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues(stage).Observe(took.Seconds())
		},
		LegacyGarbageCollect: func(ctx context.Context) error {
			return garbageCollectLegacyResourceTrackers(ctx, cli, app)
		},
		Dependents: applicationDependents(app),
	}
	opts.Collect = (&appCollector{cli: cli, app: app, tracked: NewAppResourceTracker(app), requester: opts.Requester, observe: opts.ObserveStage}).collect
	return opts
}

// IsResourceManagedByApplication is resourcekeeper.IsResourceManaged for an Application.
func IsResourceManagedByApplication(manifest *unstructured.Unstructured, app *v1beta1.Application) bool {
	return resourcekeeper.IsResourceManaged(manifest, NewAppResourceTracker(app))
}

// DeleteManagedResourceInApplication is resourcekeeper.DeleteManagedResource for an Application.
func DeleteManagedResourceInApplication(ctx context.Context, cli client.Client, mr v1beta1.ManagedResource, obj *unstructured.Unstructured, app *v1beta1.Application, garbageCollectPolicy *v1alpha1.GarbageCollectPolicySpec) error {
	return resourcekeeper.DeleteManagedResource(ctx, cli, mr, obj, NewAppResourceTracker(app), garbageCollectPolicy)
}

func gate(f featuregate.Feature) func() bool {
	return func() bool { return utilfeature.DefaultMutableFeatureGate.Enabled(f) }
}

// Policies parses the resource policies (apply-once, garbage-collect, shared-resource,
// take-over, read-only, resource-update) from app's spec. An absent policy is nil.
func Policies(app *v1beta1.Application) (p resourcekeeper.Policies, err error) {
	if p.ApplyOnce, err = policy.ParsePolicy[v1alpha1.ApplyOncePolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse apply-once policy")
	}
	if p.GarbageCollect, err = policy.ParsePolicy[v1alpha1.GarbageCollectPolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse garbage-collect policy")
	}
	if p.SharedResource, err = policy.ParsePolicy[v1alpha1.SharedResourcePolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse shared-resource policy")
	}
	if p.TakeOver, err = policy.ParsePolicy[v1alpha1.TakeOverPolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse take-over policy")
	}
	if p.ReadOnly, err = policy.ParsePolicy[v1alpha1.ReadOnlyPolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse read-only policy")
	}
	if p.ResourceUpdate, err = policy.ParsePolicy[v1alpha1.ResourceUpdatePolicySpec](app); err != nil {
		return p, errors.Wrapf(err, "failed to parse resource-update policy")
	}
	// Addons are applied once unless their Application says otherwise.
	if p.ApplyOnce == nil && metav1.HasLabel(app.ObjectMeta, oam.LabelAddonName) {
		p.ApplyOnce = &v1alpha1.ApplyOncePolicySpec{Enable: true}
	}
	return p, nil
}
