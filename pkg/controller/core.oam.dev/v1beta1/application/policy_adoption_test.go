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

package application

import (
	"context"
	"testing"

	k8scache "k8s.io/apimachinery/pkg/util/cache"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func globalPolicy(name string, priority int32, template string) *v1beta1.PolicyDefinition {
	return &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true, Priority: priority,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}},
	}
}

// liveRollout is an Application pinned to a revision of first and second, where second has since
// changed to live; first is unchanged.
func liveRollout(t *testing.T, appLabels map[string]string, first, pinned, live *v1beta1.PolicyDefinition) (*AppHandler, *v1beta1.Application, *v1beta1.ApplicationRevision) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	setRolloutDefaults(t, false, false, false)
	useTestPolicyScopeIndex(t)
	policyScopeIndex.AddOrUpdate(first)
	policyScopeIndex.AddOrUpdate(live)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", Labels: appLabels}}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: "tenant"}}
	rev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{first.Name: *first, pinned.Name: *pinned}
	h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(first, live).Build(), app: app}
	return h, app, rev
}

func TestLiveUpdateAdoptedAsItRendersInSequence(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)

	t.Run("a live version that reads an earlier policy's output is adopted", func(t *testing.T) {
		live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
output: labels: v: context.appLabels.tier`)
		h, app, rev := liveRollout(t, nil, first, pinned, live)
		results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if !h.policyUpdatesAdopted || len(h.pendingPolicyUpdates) != 0 {
			t.Fatalf("expected the live version adopted, got adopted=%v pending=%v", h.policyUpdatesAdopted, h.pendingPolicyUpdates)
		}
		if got := extractRenderedMetadata(results)["labels"].(map[string]string)["v"]; got != "gold" {
			t.Fatalf("expected the live version's label from the chain, got %q", got)
		}
	})

	t.Run("a live version that fails only in sequence is held", func(t *testing.T) {
		live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
_tier: context.appLabels.tier & "bronze"
output: labels: v: _tier`)
		h, app, rev := liveRollout(t, map[string]string{"tier": "bronze"}, first, pinned, live)
		results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
		if err != nil {
			t.Fatalf("a held update keeps the pinned version rendering, got %v", err)
		}
		if h.policyUpdatesAdopted || len(h.pendingPolicyUpdates) != 1 || h.pendingPolicyUpdates[0] != "second" {
			t.Fatalf("expected the live version held, got adopted=%v pending=%v", h.policyUpdatesAdopted, h.pendingPolicyUpdates)
		}
		if got := extractRenderedMetadata(results)["labels"].(map[string]string)["v"]; got != "pinned" {
			t.Fatalf("expected the pinned version's label, got %q", got)
		}
	})
}

// An adopting render is not cached: the reconcile that adopts can fail before its revision is cut,
// and the next one must adopt again.
func TestAdoptingRenderIsNotCached(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
output: labels: v: "live"`)
	_, app, rev := liveRollout(t, nil, first, pinned, live)
	app.Status.LatestRevision = &common.Revision{Name: rev.Name}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(first, live, rev).Build()
	applicationPolicyCache.InvalidateApplication(app.Namespace, app.Name)
	t.Cleanup(func() { applicationPolicyCache.InvalidateApplication(app.Namespace, app.Name) })

	// Neither reconcile cuts the revision, so both start from the same one.
	for i := 1; i <= 2; i++ {
		h := &AppHandler{Client: cli, app: app}
		if _, err := h.ApplyApplicationScopeTransforms(monitorContext.NewTraceContext(context.Background(), "test"), app.DeepCopy()); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
		if !h.policyUpdatesAdopted || !h.isNewRevision {
			t.Fatalf("reconcile %d: expected the adopted update to cut a revision, got adopted=%v new=%v", i, h.policyUpdatesAdopted, h.isNewRevision)
		}
	}
}

func TestPendingUpdatesFromCachedResults(t *testing.T) {
	results := []RenderedPolicyResult{
		{PolicyType: "held", UpdatePending: true},
		{PolicyType: "current"},
		{PolicyType: "held", UpdatePending: true},
	}
	if got := pendingUpdates(results); len(got) != 1 || got[0] != "held" {
		t.Fatalf("pendingUpdates = %v, want [held]", got)
	}
}

func TestHeldUpdateReportedOnACacheHit(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
output: labels: v: "live"`)
	_, app, rev := liveRollout(t, nil, first, pinned, live)
	app.Status.LatestRevision = &common.Revision{Name: rev.Name}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(first, live, rev).Build()
	applicationPolicyCache.InvalidateApplication(app.Namespace, app.Name)
	t.Cleanup(func() { applicationPolicyCache.InvalidateApplication(app.Namespace, app.Name) })

	for i := 1; i <= 2; i++ {
		h := &AppHandler{Client: cli, app: app}
		if _, err := h.ApplyApplicationScopeTransforms(monitorContext.NewTraceContext(context.Background(), "test"), app.DeepCopy()); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
		if len(h.pendingPolicyUpdates) != 1 || h.pendingPolicyUpdates[0] != "second" {
			t.Fatalf("reconcile %d: expected the held update reported, got %v", i, h.pendingPolicyUpdates)
		}
	}
}

func TestHeldVerdictReused(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
output: labels: v: "live"`)
	_, app, rev := liveRollout(t, nil, first, pinned, live)
	app.UID = "held-verdict"
	// Each policy rendered without pinned version metadata looks its revision up once.
	renders := 0
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(first, live).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1beta1.DefinitionRevisionList); ok {
				renders++
			}
			return c.List(ctx, list, opts...)
		},
	}).Build()

	for i := 1; i <= 2; i++ {
		renders = 0
		h := &AppHandler{Client: cli, app: app}
		if _, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
		if len(h.pendingPolicyUpdates) != 1 {
			t.Fatalf("reconcile %d: expected the update held, got %v", i, h.pendingPolicyUpdates)
		}
		if i == 2 && renders != 2 {
			t.Fatalf("a known held update should render the sequence once (2 policies), rendered %d policies", renders)
		}
	}
}

func TestLiveUpdateThatBreaksALaterPinnedPolicyIsHeld(t *testing.T) {
	// A failing global only stops the render when global policies are mandatory.
	orig := common2.AllowSkipGlobalPolicies
	common2.AllowSkipGlobalPolicies = false
	t.Cleanup(func() { common2.AllowSkipGlobalPolicies = orig })
	later := globalPolicy("later", 2, `parameter: {}
_tier: context.appLabels.tier & "gold"
output: labels: checked: _tier`)
	pinned := globalPolicy("earlier", 1, `parameter: {}
output: labels: tier: "gold"`)
	live := globalPolicy("earlier", 1, `parameter: {}
config: applyPolicyUpdates: true
output: labels: tier: "silver"`)
	h, app, rev := liveRollout(t, nil, later, pinned, live)
	results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
	if err != nil {
		t.Fatalf("an update that breaks a later pinned policy is held, not a failed render: %v", err)
	}
	if h.policyUpdatesAdopted || len(h.pendingPolicyUpdates) != 1 || h.pendingPolicyUpdates[0] != "earlier" {
		t.Fatalf("expected the update held, got adopted=%v pending=%v", h.policyUpdatesAdopted, h.pendingPolicyUpdates)
	}
	if got := extractRenderedMetadata(results)["labels"].(map[string]string)["tier"]; got != "gold" {
		t.Fatalf("expected the pinned sequence's label, got %q", got)
	}
}

func TestHeldVerdictFollowsTheLiveRevision(t *testing.T) {
	cache := heldUpdateCache{entries: k8scache.NewLRUExpireCache(10)}
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: "tenant"}}
	live := globalPolicy("second", 2, `parameter: {}
output: labels: r: "\(context.policyRevision)"`)
	cache.hold(app, rev, "second", live, heldVerdict{})
	if _, known := cache.held(app, rev, "second", live); !known {
		t.Fatal("the held verdict is recorded for the live version")
	}

	live.Status.LatestRevision = &common.Revision{Name: "second-v1", Revision: 1, RevisionHash: "abc"}
	if _, known := cache.held(app, rev, "second", live); known {
		t.Fatal("a live version whose revision the render reads into context is decided again once that revision appears")
	}
}

func TestPinnedFailureHoldsNoCandidate(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: context.appLabels.missing`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
output: labels: v: "live"`)
	orig := common2.AllowSkipGlobalPolicies
	common2.AllowSkipGlobalPolicies = false // with skips allowed, a failing global is skipped, not an error
	t.Cleanup(func() { common2.AllowSkipGlobalPolicies = orig })
	h, app, rev := liveRollout(t, nil, first, pinned, live)
	app.UID = "pinned-failure"
	ctx := monitorContext.NewTraceContext(context.Background(), "test")

	_, _, failing, err := h.renderAdopting(ctx, app, rev, []string{"second"}, map[string]*v1beta1.PolicyDefinition{"second": live})
	if err == nil {
		t.Fatal("the pinned sequence fails whatever the candidates, so its error is returned")
	}
	if len(failing) != 0 {
		t.Fatalf("expected no candidate reported as failing to render, got %v", failing)
	}
	if _, known := heldUpdates.held(app, rev, "second", live); known {
		t.Fatal("a candidate is not held for a failure in a policy it did not change")
	}
}
