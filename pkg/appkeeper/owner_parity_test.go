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

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// What a resourcekeeper.ResourceKeeper must do when its owner is not an Application
// (design/vela-core/agent-dispatch.md, phase 1). Each scenario runs against every owner kind:
// the Application row proves the scenario is sound, the others that the keeper needs none.

// ownerPolicies are the apply-time policies a keeper honours; every owner kind passes
// them as data (appkeeper parses an Application's from its spec).
type ownerPolicies struct {
	takeOver *v1alpha1.TakeOverPolicySpec
}

// keeperFactory builds a keeper for one generation of an owner named name in namespace
// "default", whose manifests belong to the Application "shop".
type keeperFactory func(t *testing.T, cli client.Client, name string, generation int64, policies ownerPolicies) resourcekeeper.ResourceKeeper

var ownerKinds = map[string]keeperFactory{
	"Application": applicationKeeper,
	"Component":   componentKeeper,
}

func applicationKeeper(t *testing.T, cli client.Client, name string, generation int64, policies ownerPolicies) resourcekeeper.ResourceKeeper {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "default", UID: types.UID("uid-" + name), Generation: generation,
	}}
	rk, err := resourcekeeper.New(context.Background(), cli, NewAppResourceTracker(app), resourcekeeper.Policies{TakeOver: policies.takeOver}, resourcekeeper.Options{})
	require.NoError(t, err)
	return rk
}

// componentKeeper is the shape vela-agent needs: no Application, identity and
// apply-time policies passed in directly (Component.spec.apply).
func componentKeeper(t *testing.T, cli client.Client, name string, generation int64, policies ownerPolicies) resourcekeeper.ResourceKeeper {
	rk, err := resourcekeeper.New(context.Background(), cli, resourcetracker.NewBase(resourcetracker.Owner{Kind: "Component", Namespace: "default", Name: name, UID: types.UID("uid-" + name), Generation: generation, Deleting: false}),
		resourcekeeper.Policies{TakeOver: policies.takeOver}, resourcekeeper.Options{})
	require.NoError(t, err)
	return rk
}

func exists(t *testing.T, cli client.Client, name string) bool {
	cm := &corev1.ConfigMap{}
	err := cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, cm)
	if kerrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

func TestOwnerGarbageCollectsResourceDroppedBetweenGenerations(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0

	for kind, newKeeper := range ownerKinds {
		t.Run(kind, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

			gen1 := newKeeper(t, cli, "shop", 1, ownerPolicies{})
			r.NoError(gen1.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("cm-a"), renderedConfigMap("cm-b")}, nil))

			r.NoError(newKeeper(t, cli, "shop", 2, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("cm-b")}, nil))
			// A keeper loads its ResourceTrackers when built, so each pass gets a fresh
			// one, as each reconcile does.
			finished := false
			for i := 0; i < 5 && !finished; i++ {
				var err error
				finished, _, err = newKeeper(t, cli, "shop", 2, ownerPolicies{}).GarbageCollect(ctx)
				r.NoError(err)
			}
			r.True(finished, "garbage collection did not finish")

			r.False(exists(t, cli, "cm-a"), "cm-a was dropped in generation 2 and should be collected")
			r.True(exists(t, cli, "cm-b"), "cm-b is still in generation 2 and should be kept")
		})
	}
}

func TestOwnerTakeOverPolicyAppliesToUnmanagedResource(t *testing.T) {
	for kind, newKeeper := range ownerKinds {
		t.Run(kind, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: "default"}}
			cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(existing).Build()

			without := newKeeper(t, cli, "shop", 1, ownerPolicies{})
			err := without.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("legacy")}, nil)
			r.ErrorContains(err, "exists but not managed by any", "an unmanaged resource must not be adopted without take-over")

			with := newKeeper(t, cli, "shop", 1, ownerPolicies{takeOver: &v1alpha1.TakeOverPolicySpec{
				Rules: []v1alpha1.TakeOverPolicyRule{{Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}}}},
			}})
			r.NoError(with.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("legacy")}, nil))
		})
	}
}

// A Component and an Application may share a name and namespace. Today trackers are found
// by app name and namespace labels, so without a distinct owner identity they would
// adopt each other's trackers and collect each other's resources.
func TestComponentAndApplicationOfTheSameNameKeepSeparateTrackers(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	r.NoError(applicationKeeper(t, cli, "shop", 1, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("from-app")}, nil))
	r.NoError(componentKeeper(t, cli, "shop", 1, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("from-component")}, nil))

	// The Application drops everything in generation 2.
	r.NoError(applicationKeeper(t, cli, "shop", 2, ownerPolicies{}).Dispatch(ctx, nil, nil))
	finished := false
	for i := 0; i < 5 && !finished; i++ {
		var err error
		finished, _, err = applicationKeeper(t, cli, "shop", 2, ownerPolicies{}).GarbageCollect(ctx)
		r.NoError(err)
	}
	r.True(finished, "garbage collection did not finish")

	r.False(exists(t, cli, "from-app"), "the Application's own resource should be collected")
	r.True(exists(t, cli, "from-component"), "the Component's resource must not be collected by the Application")
}

// Hub-rendered manifests carry their Application's labels, so every Component of "shop"
// looks the same by app labels. Ownership has to be decided by the Component, or two
// Components could silently fight over one resource.
func TestComponentsOfOneApplicationCannotClaimTheSameResource(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	r.NoError(componentKeeper(t, cli, "backend", 1, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil))
	err := componentKeeper(t, cli, "frontend", 1, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil)
	r.ErrorContains(err, "is managed by other component", "a second Component must not take over a resource another Component owns")
}

// Dependency-ordered GC walks an Application's components. A Component owner has none,
// so a caller asking for dependency order must get plain GC, not a nil Application.
func TestComponentOwnerGarbageCollectsWhenDependencyOrderIsRequested(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	r.NoError(componentKeeper(t, cli, "backend", 1, ownerPolicies{}).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("cm-a")}, nil))
	r.NoError(componentKeeper(t, cli, "backend", 2, ownerPolicies{}).Dispatch(ctx, nil, nil))
	finished := false
	for i := 0; i < 5 && !finished; i++ {
		var err error
		finished, _, err = componentKeeper(t, cli, "backend", 2, ownerPolicies{}).GarbageCollect(ctx, resourcekeeper.DependencyGCOption{})
		r.NoError(err)
	}
	r.True(finished, "garbage collection did not finish")
	r.False(exists(t, cli, "cm-a"))
}

// The legacy clean-up step is supplied by the keeper's creator; the keeper decides when it
// runs: once the owner has a current tracker, or while the owner is being deleted, and
// never before (an owner not yet on current trackers may still need its legacy ones).
func TestLegacyGarbageCollectRunsOnlyOnceTheOwnerHasCurrentTrackers(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	ctx := context.Background()
	newKeeper := func(t *testing.T, cli client.Client, app *v1beta1.Application, calls *int) resourcekeeper.ResourceKeeper {
		rk, err := resourcekeeper.New(ctx, cli, NewAppResourceTracker(app), resourcekeeper.Policies{}, resourcekeeper.Options{
			LegacyGarbageCollect: func(context.Context) error { *calls++; return nil },
		})
		require.NoError(t, err)
		return rk
	}
	app := func() *v1beta1.Application {
		return &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "uid-shop", Generation: 1}}
	}

	t.Run("no current tracker", func(t *testing.T) {
		calls := 0
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		_, _, err := newKeeper(t, cli, app(), &calls).GarbageCollect(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, calls)
	})
	t.Run("current tracker", func(t *testing.T) {
		calls := 0
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		require.NoError(t, newKeeper(t, cli, app(), &calls).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("cm")}, nil))
		_, _, err := newKeeper(t, cli, app(), &calls).GarbageCollect(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, calls)
	})
	t.Run("owner being deleted", func(t *testing.T) {
		calls := 0
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		deleting := app()
		now := metav1.Now()
		deleting.DeletionTimestamp = &now
		_, _, err := newKeeper(t, cli, deleting, &calls).GarbageCollect(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, calls)
	})
	t.Run("disabled for this collection", func(t *testing.T) {
		calls := 0
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		require.NoError(t, newKeeper(t, cli, app(), &calls).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("cm")}, nil))
		_, _, err := newKeeper(t, cli, app(), &calls).GarbageCollect(ctx, resourcekeeper.DisableLegacyGCOption{})
		require.NoError(t, err)
		require.Equal(t, 0, calls)
	})
}

// Components can share a resource like Applications can: sharers are recorded by owner key,
// and when the controlling Component lets go, control passes to the next one; the resource
// goes only when its last sharer does.
func TestComponentsShareAResource(t *testing.T) {
	defer func(p float64) { resourcekeeper.MarkWithProbability = p }(resourcekeeper.MarkWithProbability)
	resourcekeeper.MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	shared := resourcekeeper.Policies{SharedResource: &v1alpha1.SharedResourcePolicySpec{
		Rules: []v1alpha1.SharedResourcePolicyRule{{Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}}}},
	}}
	keeper := func(name string, gen int64) resourcekeeper.ResourceKeeper {
		rk, err := resourcekeeper.New(ctx, cli, resourcetracker.NewBase(resourcetracker.Owner{Kind: "Component", Namespace: "default", Name: name, UID: types.UID("uid-" + name), Generation: gen, Deleting: false}), shared, resourcekeeper.Options{})
		r.NoError(err)
		return rk
	}
	gc := func(name string, gen int64) {
		finished := false
		for i := 0; i < 5 && !finished; i++ {
			var err error
			finished, _, err = keeper(name, gen).GarbageCollect(ctx)
			r.NoError(err)
		}
		r.True(finished)
	}
	get := func() *corev1.ConfigMap {
		cm := &corev1.ConfigMap{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "shared"}, cm); err != nil {
			r.True(kerrors.IsNotFound(err))
			return nil
		}
		return cm
	}

	r.NoError(keeper("backend", 1).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil))
	r.NoError(keeper("frontend", 1).Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("shared")}, nil))
	r.Equal("Component/default/backend,Component/default/frontend", get().Annotations[oam.AnnotationAppSharedBy])
	r.Equal("backend", get().Labels[oam.LabelOwnerName])

	// backend drops the resource: frontend takes over, the resource stays
	r.NoError(keeper("backend", 2).Dispatch(ctx, nil, nil))
	gc("backend", 2)
	r.NotNil(get())
	r.Equal("Component/default/frontend", get().Annotations[oam.AnnotationAppSharedBy])
	r.Equal("frontend", get().Labels[oam.LabelOwnerName])

	// frontend drops it too: it goes
	r.NoError(keeper("frontend", 2).Dispatch(ctx, nil, nil))
	gc("frontend", 2)
	r.Nil(get())
}
