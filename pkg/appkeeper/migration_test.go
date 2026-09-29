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

package appkeeper

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Resources and trackers written before owner.oam.dev/* labels existed carry only
// app.oam.dev/* labels. After the upgrade the Application must still own them: re-apply
// them (gaining owner labels), and collect them.

func shopApp(gen int64) *v1beta1.Application {
	return &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "uid-shop", Generation: gen}}
}

// preUpgrade writes a ConfigMap and a versioned tracker recording it, as the controller did
// before owner labels: app labels only.
func preUpgrade(t *testing.T, cli client.Client, cmName string, gen int64) {
	ctx := context.Background()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: "default", Labels: map[string]string{
		oam.LabelAppName: "shop", oam.LabelAppNamespace: "default",
	}}}
	require.NoError(t, cli.Create(ctx, cm))
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "shop-v1-default", Labels: map[string]string{
		oam.LabelAppName: "shop", oam.LabelAppNamespace: "default", oam.LabelAppUID: "uid-shop",
	}}}
	meta.AddFinalizer(rt, resourcetracker.Finalizer)
	rt.Spec.Type = v1beta1.ResourceTrackerTypeVersioned
	rt.Spec.ApplicationGeneration = gen
	require.NoError(t, cli.Create(ctx, rt))
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	obj.SetName(cmName)
	obj.SetNamespace("default")
	require.NoError(t, resourcetracker.RecordManifestsInResourceTracker(ctx, cli, rt, []*unstructured.Unstructured{obj}, true, false, ""))
}

func renderedConfigMap(name string) *unstructured.Unstructured {
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName(name)
	cm.SetNamespace("default")
	cm.SetLabels(map[string]string{oam.LabelAppName: "shop", oam.LabelAppNamespace: "default"}) // as the renderer labels it
	return cm
}

func TestUpgradedApplicationReappliesAResourceWrittenBeforeOwnerLabels(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	preUpgrade(t, cli, "settings", 1)

	rk, err := New(ctx, cli, shopApp(2))
	r.NoError(err)
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("settings")}, nil), "still owned through its app labels")

	cm := &corev1.ConfigMap{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "settings"}, cm))
	r.Equal("Application", cm.Labels[oam.LabelOwnerKind], "re-applying adds owner labels")
	r.Equal("shop", cm.Labels[oam.LabelOwnerName])
	r.Equal("shop", cm.Labels[oam.LabelAppName], "app labels are kept")

	rts := &v1beta1.ResourceTrackerList{}
	r.NoError(cli.List(ctx, rts, client.MatchingLabels{oam.LabelOwnerKind: "Application", oam.LabelOwnerName: "shop"}))
	r.Len(rts.Items, 2, "the old tracker (backfilled on load) and the new one both carry owner labels")
	for _, rt := range rts.Items {
		r.Equal("shop", rt.Labels[oam.LabelAppName], "and keep their app labels")
		r.Equal("uid-shop", rt.Labels[oam.LabelOwnerUID])
	}
}

func TestUpgradedApplicationCollectsAResourceWrittenBeforeOwnerLabels(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	preUpgrade(t, cli, "stale", 1)

	rk, err := New(ctx, cli, shopApp(2))
	r.NoError(err)
	r.NoError(rk.Dispatch(ctx, nil, nil)) // generation 2 no longer renders it
	finished := false
	for i := 0; i < 5 && !finished; i++ {
		rk, err = New(ctx, cli, shopApp(2))
		r.NoError(err)
		finished, _, err = rk.GarbageCollect(ctx, resourcekeeper.DisableLegacyGCOption{})
		r.NoError(err)
	}
	r.True(finished)
	err = cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "stale"}, &corev1.ConfigMap{})
	r.True(kerrors.IsNotFound(err), "a resource the Application owned before the upgrade is still collected")
}

func TestAComponentCannotTakeAnApplicationsResource(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	app, err := New(ctx, cli, shopApp(1))
	r.NoError(err)
	r.NoError(app.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil))

	component, err := resourcekeeper.New(ctx, cli, resourcetracker.NewBase(resourcetracker.Owner{Kind: "Component", Namespace: "default", Name: "backend", UID: "uid-backend", Generation: 1, Deleting: false}), resourcekeeper.Policies{}, resourcekeeper.Options{})
	r.NoError(err)
	err = component.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil)
	r.ErrorContains(err, "is managed by other component Application/default/shop", "the conflict names the Application")
}

// A resource written before owner labels carries app labels only. Until its Application
// re-applies it, another kind must still see it as the Application's: take-over adopts
// resources nobody owns, and this one is owned.
func TestAnotherKindCannotTakeAnApplicationsResourceWrittenBeforeOwnerLabels(t *testing.T) {
	for _, takeOver := range []bool{false, true} {
		r := require.New(t)
		ctx := context.Background()
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		preUpgrade(t, cli, "settings", 1)

		policies := resourcekeeper.Policies{}
		if takeOver {
			policies.TakeOver = &v1alpha1.TakeOverPolicySpec{Rules: []v1alpha1.TakeOverPolicyRule{{
				Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}},
			}}}
		}
		component, err := resourcekeeper.New(ctx, cli, resourcetracker.NewBase(resourcetracker.Owner{Kind: "Component", Namespace: "default", Name: "backend", UID: "uid-backend", Generation: 1, Deleting: false}), policies, resourcekeeper.Options{})
		r.NoError(err)
		err = component.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("settings")}, nil)
		r.ErrorContains(err, "Application/default/shop", "take-over=%v: refused, naming the Application", takeOver)

		cm := &corev1.ConfigMap{}
		r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "settings"}, cm))
		r.Empty(cm.Labels[oam.LabelOwnerKind], "take-over=%v: the Component did not stamp it", takeOver)
	}
}

func TestOrphaningStripsBothLabelSets(t *testing.T) {
	obj := renderedConfigMap("kept")
	tracked := NewAppResourceTracker(shopApp(1))
	tracked.Stamp(obj)
	r := require.New(t)
	r.Equal("default/shop", tracked.ControlledBy(obj))
	tracked.Release(obj)
	r.Empty(tracked.ControlledBy(obj), "released: no owner under either scheme")
	for _, k := range []string{oam.LabelAppName, oam.LabelAppNamespace, oam.LabelOwnerKind, oam.LabelOwnerName, oam.LabelOwnerNamespace} {
		r.NotContains(obj.GetLabels(), k)
	}
}

// An idle Application never dispatches again: state-keep re-applies its resources from its
// tracker records, which may carry app labels only. So state-keep has to add the owner
// labels, or an idle Application never gains them.
func TestStateKeepAddsOwnerLabelsToAnIdleApplicationsResources(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	cm := renderedConfigMap("settings") // app labels only, as the controller wrote it before the upgrade
	r.NoError(cli.Create(ctx, cm.DeepCopy()))
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "shop-v1-default", Labels: map[string]string{
		oam.LabelAppName: "shop", oam.LabelAppNamespace: "default", oam.LabelAppUID: "uid-shop",
	}}}
	meta.AddFinalizer(rt, resourcetracker.Finalizer)
	rt.Spec.Type = v1beta1.ResourceTrackerTypeVersioned
	rt.Spec.ApplicationGeneration = 1
	r.NoError(cli.Create(ctx, rt))
	r.NoError(resourcetracker.RecordManifestsInResourceTracker(ctx, cli, rt, []*unstructured.Unstructured{cm}, false, false, "")) // full record, as for state-keep

	rk, err := New(ctx, cli, shopApp(1)) // same generation: nothing to dispatch
	r.NoError(err)
	r.NoError(rk.StateKeep(ctx))

	live := &corev1.ConfigMap{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "settings"}, live))
	r.Equal("Application", live.Labels[oam.LabelOwnerKind], "state-keep adds the owner labels")
	r.Equal("shop", live.Labels[oam.LabelOwnerName])
	r.Equal("default", live.Labels[oam.LabelOwnerNamespace])
	r.Equal("shop", live.Labels[oam.LabelAppName], "app labels are kept")
}

// Trackers are rewritten only when what they record changes, so an idle Application's
// trackers (its root tracker above all) would never gain owner labels on their own. The
// keeper adds any missing when it loads them, once.
func TestLoadingAddsOwnerLabelsToTrackersWrittenBeforeThem(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	preUpgrade(t, cli, "settings", 2) // current tracker for generation 2, app labels only

	_, err := New(ctx, cli, shopApp(2))
	r.NoError(err)
	rt := &v1beta1.ResourceTracker{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Name: "shop-v1-default"}, rt))
	r.Equal("Application", rt.Labels[oam.LabelOwnerKind])
	r.Equal("shop", rt.Labels[oam.LabelOwnerName])
	r.Equal("default", rt.Labels[oam.LabelOwnerNamespace])
	r.Equal("uid-shop", rt.Labels[oam.LabelOwnerUID])
	r.Equal("shop", rt.Labels[oam.LabelAppName], "app labels are kept")

	version := rt.ResourceVersion
	_, err = New(ctx, cli, shopApp(2))
	r.NoError(err)
	r.NoError(cli.Get(ctx, client.ObjectKey{Name: "shop-v1-default"}, rt))
	r.Equal(version, rt.ResourceVersion, "a tracker that has them is not written again")
}

// An Application carrying the orphan finalizer leaves its resources behind instead of
// deleting them: garbage collection releases them, stripping both label sets.
func TestOrphaningApplicationReleasesItsResourcesInsteadOfDeletingThem(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	app := shopApp(1)
	rk, err := New(ctx, cli, app)
	r.NoError(err)
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("kept")}, nil))

	orphaning := shopApp(2)
	orphaning.SetFinalizers([]string{oam.FinalizerOrphanResource})
	finished := false
	for i := 0; i < 5 && !finished; i++ {
		rk, err = New(ctx, cli, orphaning) // generation 2 renders nothing
		r.NoError(err)
		if i == 0 {
			r.NoError(rk.Dispatch(ctx, nil, nil))
			continue
		}
		finished, _, err = rk.GarbageCollect(ctx, resourcekeeper.DisableLegacyGCOption{})
		r.NoError(err)
	}
	r.True(finished)

	cm := &corev1.ConfigMap{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "kept"}, cm), "the resource is kept, not deleted")
	for _, k := range []string{oam.LabelOwnerKind, oam.LabelOwnerName, oam.LabelOwnerNamespace, oam.LabelAppName, oam.LabelAppNamespace} {
		r.NotContains(cm.Labels, k, "and released under both label schemes")
	}
}

// failingUpdates refuses writes while switched on.
type failingUpdates struct {
	client.Client
	on bool
}

func (c *failingUpdates) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.on {
		return kerrors.NewForbidden(schema.GroupResource{Resource: "resourcetrackers"}, obj.GetName(), nil)
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *failingUpdates) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.on {
		return kerrors.NewForbidden(schema.GroupResource{Resource: "resourcetrackers"}, obj.GetName(), nil)
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// Labelling existing trackers is best-effort: a write that fails must not fail the reconcile,
// and the labels must arrive later rather than being lost.
func TestATrackerLabelBackfillThatFailsIsRetried(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := &failingUpdates{Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build()}
	preUpgrade(t, cli.Client, "settings", 2)

	cli.on = true
	_, err := New(ctx, cli, shopApp(2))
	r.NoError(err, "a failed backfill does not fail the caller")
	rt := &v1beta1.ResourceTracker{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Name: "shop-v1-default"}, rt))
	r.Empty(rt.Labels[oam.LabelOwnerKind], "nothing was written")

	cli.on = false
	rk, err := New(ctx, cli, shopApp(2))
	r.NoError(err)
	r.NoError(cli.Get(ctx, client.ObjectKey{Name: "shop-v1-default"}, rt))
	r.Equal("Application", rt.Labels[oam.LabelOwnerKind], "the next load labels it")
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("settings")}, nil))
}
