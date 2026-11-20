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

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestHeldWorkflowRecordsNoPolicyContextHash(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	keeper := &policyHashKeeper{hash: `{"global:p":"h1"}`, recorded: true}
	handler := &AppHandler{resourceKeeper: keeper, heldByGlobalPolicy: &v1beta1.ApplicationRevision{}}
	r := &Reconciler{}
	r.recordPolicyContextHash(monitorContext.NewTraceContext(context.Background(), ""), handler)
	require.Empty(t, keeper.writes, "a held reconcile rendered no policy context, so the applied hash stays")
}

func TestHeldRevisionKeepsTheGenerationItRecorded(t *testing.T) {
	for name, tc := range map[string]struct {
		recorded map[string]string
		want     string
		wantSet  bool
	}{
		"a revision that recorded its generation keeps it":             {recorded: map[string]string{oam.AnnotationAppGeneration: "3"}, want: "3", wantSet: true},
		"a revision cut before generations were recorded records none": {recorded: nil, wantSet: false},
	} {
		t.Run(name, func(t *testing.T) {
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Generation: 5, UID: "uid"}}
			held := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: "default", Annotations: tc.recorded}}
			cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
			h := &AppHandler{Client: cli, app: app, currentAppRev: held, heldByGlobalPolicy: held}
			require.NoError(t, h.FinalizeAndApplyAppRevision(context.Background()))

			stored := &v1beta1.ApplicationRevision{}
			require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "app-v1", Namespace: "default"}, stored))
			got, ok := stored.Annotations[oam.AnnotationAppGeneration]
			require.Equal(t, tc.wantSet, ok, "the held revision never takes the generation of the edit waiting behind it")
			require.Equal(t, tc.want, got)
		})
	}
}

// heldApp is an Application held at revision app-v1, whose tracker recorded the ConfigMap "cm" for
// component "c" at generation 1 and publish version runningPV; the author has since edited it to
// generation 2 with publish version editPV and the given policies.
func heldApp(t *testing.T, runningPV, editPV string, editPolicies []v1beta1.AppPolicy) (*AppHandler, *v1beta1.Application, *v1beta1.ApplicationRevision) {
	t.Helper()
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Generation: 2, UID: "uid"}}
	app.Spec.Components = []common.ApplicationComponent{{Name: "c", Type: "worker"}}
	app.Spec.Policies = editPolicies
	if editPV != "" {
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationPublishVersion, editPV)
	}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: "default"}}
	rev.Spec.Application = *app.DeepCopy()
	rev.Spec.Application.Spec.Policies = nil
	rt := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{Name: "app-v1-default", Labels: map[string]string{
			oam.LabelAppName: "app", oam.LabelAppNamespace: "default", oam.LabelAppUID: "uid"}},
		Spec: v1beta1.ResourceTrackerSpec{Type: v1beta1.ResourceTrackerTypeVersioned, ApplicationGeneration: 1,
			ManagedResources: []v1beta1.ManagedResource{{
				ClusterObjectReference: common.ClusterObjectReference{ObjectReference: corev1.ObjectReference{
					APIVersion: "v1", Kind: "ConfigMap", Namespace: "default", Name: "cm"}},
				OAMObjectReference: common.OAMObjectReference{Component: "c"},
			}}},
	}
	if runningPV != "" {
		metav1.SetMetaDataAnnotation(&rt.ObjectMeta, oam.AnnotationPublishVersion, runningPV)
	}
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(rt).Build()
	return &AppHandler{Client: cli, app: app}, app, rev
}

func TestHeldKeeperFollowsTheRunningTracker(t *testing.T) {
	for name, pv := range map[string][2]string{
		"after an edit":               {"", ""},
		"after a publishVersion bump": {"pv1", "pv2"},
		"after the pin is removed":    {"pv1", ""},
	} {
		t.Run(name, func(t *testing.T) {
			h, app, rev := heldApp(t, pv[0], pv[1], nil)
			require.NoError(t, h.holdResourceKeeper(monitorContext.NewTraceContext(context.Background(), ""), app, rev))
			require.Len(t, h.resourceKeeper.ComponentResources("c"), 1,
				"the held keeper records into the tracker of what is running, so garbage collection keeps it")
		})
	}
}

func TestHeldKeeperAppliesTheHeldResourcePolicies(t *testing.T) {
	readOnly := v1beta1.AppPolicy{Name: "ro", Type: v1alpha1.ReadOnlyPolicyType,
		Properties: &runtime.RawExtension{Raw: []byte(`{"rules":[{"selector":{"resourceTypes":["ConfigMap"]}}]}`)}}
	cm := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "fresh", "namespace": "default"}}}
	}
	ctx := monitorContext.NewTraceContext(context.Background(), "")

	t.Run("the waiting edit's policies do not apply", func(t *testing.T) {
		h, app, rev := heldApp(t, "", "", []v1beta1.AppPolicy{readOnly})
		require.NoError(t, h.holdResourceKeeper(ctx, app, rev))
		require.NoError(t, h.resourceKeeper.Dispatch(ctx, []*unstructured.Unstructured{cm()}, nil),
			"the edit's read-only does not stop the held revision's dispatch")
	})
	t.Run("the held revision's own policies apply", func(t *testing.T) {
		h, app, rev := heldApp(t, "", "", nil)
		rev.Spec.Application.Spec.Policies = []v1beta1.AppPolicy{readOnly}
		require.NoError(t, h.holdResourceKeeper(ctx, app, rev))
		require.ErrorContains(t, h.resourceKeeper.Dispatch(ctx, []*unstructured.Unstructured{cm()}, nil), "read-only",
			"the held revision's read-only still guards its resources")
	})
}

func TestUpdateHeldForARenderFailureSaysSo(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
_tier: context.appLabels.tier & "bronze"
output: labels: v: _tier`)
	_, app, rev := liveRollout(t, map[string]string{"tier": "gold"}, first, pinned, live)
	app.UID = "render-failure-message"
	cli := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(first, live).Build()

	// The second render reuses the held verdict, and must still give the reason.
	for i := 1; i <= 2; i++ {
		h := &AppHandler{Client: cli, app: app}
		results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
		require.NoError(t, err)
		var message string
		for _, result := range results {
			if result.PolicyType == "second" {
				message = pendingUpdateMessage(result)
			}
		}
		require.Contains(t, message, "fails to render", "render %d: a held update says why it is held", i)
		require.NotContains(t, message, oam.AnnotationApplyPolicyUpdates, "render %d", i)
	}
}

func TestUpdateHeldForASpecRewriteSaysSo(t *testing.T) {
	first := globalPolicy("first", 1, `parameter: {}
output: labels: tier: "gold"`)
	pinned := globalPolicy("second", 2, `parameter: {}
output: labels: v: "pinned"`)
	live := globalPolicy("second", 2, `parameter: {}
config: applyPolicyUpdates: true
output: components: [{name: "rewritten", type: "worker"}]`)
	h, app, rev := liveRollout(t, nil, first, pinned, live)
	app.UID = "spec-rewrite-message"
	app.Spec.Components = []common.ApplicationComponent{{Name: "c", Type: "worker"}}
	rev.Spec.Application.Spec = app.Spec

	results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
	require.NoError(t, err)
	var message string
	for _, result := range results {
		if result.PolicyType == "second" {
			message = pendingUpdateMessage(result)
		}
	}
	require.Contains(t, message, "rewrites the application spec", "a held update says why it is held")
	require.NotContains(t, message, oam.AnnotationApplyPolicyUpdates)
}

func TestSpecRewriteHoldsOnlyTheRewritingUpdate(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)
	setRolloutDefaults(t, false, false, false)
	useTestPolicyScopeIndex(t)
	labelsPinned := globalPolicy("labels", 1, `parameter: {}
output: labels: tier: "silver"`)
	labelsLive := globalPolicy("labels", 1, `parameter: {}
config: applyPolicyUpdates: true
output: labels: tier: "gold"`)
	rewritePinned := globalPolicy("rewrite", 2, `parameter: {}
output: labels: v: "pinned"`)
	rewriteLive := globalPolicy("rewrite", 2, `parameter: {}
config: applyPolicyUpdates: true
output: components: [{name: "rewritten", type: "worker"}]`)
	policyScopeIndex.AddOrUpdate(labelsLive)
	policyScopeIndex.AddOrUpdate(rewriteLive)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "mixed-spec-rewrite"}}
	app.Spec.Components = []common.ApplicationComponent{{Name: "c", Type: "worker"}}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Namespace: "tenant"}}
	rev.Spec.Application.Spec = app.Spec
	rev.Spec.PolicyDefinitions = map[string]v1beta1.PolicyDefinition{labelsPinned.Name: *labelsPinned, rewritePinned.Name: *rewritePinned}
	h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(labelsLive, rewriteLive).Build(), app: app}

	results, err := h.renderWithLivePolicies(monitorContext.NewTraceContext(context.Background(), "test"), app, rev)
	require.NoError(t, err)
	require.True(t, h.policyUpdatesAdopted, "the metadata-only update is adopted")
	require.Equal(t, []string{"rewrite"}, h.pendingPolicyUpdates, "only the spec-rewriting update is held")
	require.Equal(t, "gold", extractRenderedMetadata(results)["labels"].(map[string]string)["tier"], "the adopted version's label applies")
	require.False(t, rewritesSpec(extractRenderedSpec(results)), "the held rewrite stays pinned")
	require.Equal(t, "pinned", extractRenderedMetadata(results)["labels"].(map[string]string)["v"], "the held rewrite's pinned output applies")
	for _, result := range results {
		message := pendingUpdateMessage(result)
		switch result.PolicyType {
		case "labels":
			require.Empty(t, message, "the adopted update reports nothing pending")
		case "rewrite":
			require.Contains(t, message, "rewrites the application spec")
		}
	}
}
