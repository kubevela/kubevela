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
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestResourceKeeperDispatchAndDelete(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := newAppKeeper(context.Background(), cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}, Policies{})
	r.NoError(err)
	rk := _rk.(*resourceKeeper)
	rk.policies.GarbageCollect = &v1alpha1.GarbageCollectPolicySpec{
		Rules: []v1alpha1.GarbageCollectPolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"versioned"}},
			Strategy: v1alpha1.GarbageCollectStrategyOnAppUpdate,
		}, {
			Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"life-long"}},
			Strategy: v1alpha1.GarbageCollectStrategyOnAppDelete,
		}, {
			Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"eternal"}},
			Strategy: v1alpha1.GarbageCollectStrategyNever,
		},
		}}
	rk.policies.ApplyOnce = &v1alpha1.ApplyOncePolicySpec{Enable: true}
	cm1 := &unstructured.Unstructured{}
	cm1.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm1.SetName("cm1")
	cm1.SetLabels(map[string]string{oam.TraitTypeLabel: "versioned"})
	cm2 := &unstructured.Unstructured{}
	cm2.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm2.SetName("cm2")
	cm2.SetLabels(map[string]string{oam.TraitTypeLabel: "life-long"})
	cm3 := &unstructured.Unstructured{}
	cm3.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm3.SetName("cm3")
	cm3.SetLabels(map[string]string{oam.TraitTypeLabel: "eternal"})

	r.NoError(rk.Dispatch(context.Background(), []*unstructured.Unstructured{cm1, cm2, cm3}, nil))
	r.NotNil(rk._rootRT)
	r.NotNil(rk._currentRT)
	r.Equal(2, len(rk._rootRT.Spec.ManagedResources))
	r.Equal(1, len(rk._currentRT.Spec.ManagedResources))
	r.NoError(rk.Delete(context.Background(), []*unstructured.Unstructured{cm1, cm2, cm3}))
	r.Equal(2, len(rk._rootRT.Spec.ManagedResources))
	r.Equal(1, len(rk._currentRT.Spec.ManagedResources))
}

func TestResourceKeeperAdmissionDispatchAndDelete(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := newAppKeeper(context.Background(), cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}, Policies{})
	r.NoError(err)
	rk := _rk.(*resourceKeeper)
	AllowCrossNamespaceResource = false
	defer func() {
		AllowCrossNamespaceResource = true
	}()
	objs := []*unstructured.Unstructured{{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "demo",
				"namespace": "demo",
			},
		},
	}}
	err = rk.Dispatch(context.Background(), objs, nil)
	r.NotNil(err)
	r.Contains(err.Error(), "forbidden")
	err = rk.Delete(context.Background(), objs)
	r.NotNil(err)
	r.Contains(err.Error(), "forbidden")
}

// TestApplyStrategiesNilReturnOnStateKeep verifies that ApplyStrategies returns nil
// when called with ApplyOnceStrategyOnAppStateKeep and the resource is not found.
// This is the precondition for the nil-guard in the dispatch path being correct:
// the dispatch path uses ApplyOnceStrategyOnAppUpdate, which never produces nil,
// so the guard there is purely defensive.
func TestApplyStrategiesNilReturnOnStateKeep(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	app := &v1beta1.Application{ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default"}}
	rk := &resourceKeeper{
		Client: cli,
		owner:  newAppOwner(app),
		policies: Policies{ApplyOnce: &v1alpha1.ApplyOncePolicySpec{
			Enable: true,
			Rules: []v1alpha1.ApplyOncePolicyRule{{
				Selector: v1alpha1.ResourcePolicyRuleSelector{
					CompNames: []string{"my-comp"},
				},
				Strategy: &v1alpha1.ApplyOnceStrategy{Path: []string{"*"}},
			}},
		}},
	}

	manifest := &unstructured.Unstructured{}
	manifest.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	manifest.SetName("nonexistent-cm")
	manifest.SetNamespace("default")
	manifest.SetLabels(map[string]string{oam.LabelAppComponent: "my-comp"})

	// For ApplyOnceStrategyOnAppStateKeep, a missing resource returns nil.
	result, err := applyStrategies(context.Background(), rk, manifest, v1alpha1.ApplyOnceStrategyOnAppStateKeep)
	r.NoError(err)
	r.Nil(result)

	// For ApplyOnceStrategyOnAppUpdate, a missing resource returns the original manifest (not nil).
	// This means the nil-guard in dispatch.go is defensive and cannot be triggered today.
	result, err = applyStrategies(context.Background(), rk, manifest, v1alpha1.ApplyOnceStrategyOnAppUpdate)
	r.NoError(err)
	r.NotNil(result)
}

// TestCleanupStaleEntriesUpdateError verifies that cleanupStaleEntries propagates
// errors from the underlying client Update call.
func TestCleanupStaleEntriesUpdateError(t *testing.T) {
	r := require.New(t)
	updateErr := fmt.Errorf("simulated update failure")
	cli := &test.MockClient{
		MockUpdate: test.NewMockUpdateFn(updateErr),
	}

	app := &v1beta1.Application{ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default"}}
	rk := &resourceKeeper{
		Client: cli,
		owner:  newAppOwner(app),
	}

	rt := &v1beta1.ResourceTracker{
		ObjectMeta: v12.ObjectMeta{Name: "test-rt", UID: "test-uid"},
	}
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName("stale-cm")
	cm.SetNamespace("default")

	mr := v1beta1.ManagedResource{}
	mr.APIVersion = v1.SchemeGroupVersion.String()
	mr.Kind = "ConfigMap"
	mr.Name = "stale-cm"
	mr.Namespace = "default"

	entries := []staleEntry{{mr: mr, rt: rt}}
	err := rk.cleanupStaleEntries(context.Background(), entries)
	r.Error(err)
	r.Contains(err.Error(), "failed to remove stale entries from resourcetracker test-rt")
}

// An apply-once rule with path "*" replaces the manifest with the live object, which drops
// the marks the keeper put on it. What is applied still has to say who owns it.
func TestApplyOnceKeepsTheOwnersMarks(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	existing.SetName("settings")
	existing.SetNamespace("default")
	r.NoError(cli.Create(ctx, existing)) // created by hand: nobody owns it, so take-over adopts it

	rk, err := New(ctx, cli, resourcetracker.NewBase(resourcetracker.Owner{
		Kind: "Component", Namespace: "default", Name: "backend", UID: "uid-backend", Generation: 1,
	}), Policies{ApplyOnce: &v1alpha1.ApplyOncePolicySpec{
		Enable: true,
		Rules: []v1alpha1.ApplyOncePolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}},
			Strategy: &v1alpha1.ApplyOnceStrategy{Path: []string{"*"}},
		}},
	}, TakeOver: &v1alpha1.TakeOverPolicySpec{Rules: []v1alpha1.TakeOverPolicyRule{{
		Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}},
	}}}}, Options{})
	r.NoError(err)

	rendered := existing.DeepCopy()
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{rendered}, nil))

	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	r.NoError(cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: "settings"}, live))
	r.Equal("Component", live.GetLabels()[oam.LabelOwnerKind], "apply-once keeps the live spec, not the ownership marks")
	r.Equal("backend", live.GetLabels()[oam.LabelOwnerName])
}

// TestResourceKeeperDeleteGivenOnlyTheResourceIdentity covers what a workflow kube.#Delete step does:
// it passes a manifest that names the resource but carries none of its annotations or labels. The
// protections live on the object in the cluster, so Delete has to read them from there.
func TestResourceKeeperDeleteGivenOnlyTheResourceIdentity(t *testing.T) {
	never := v1alpha1.GarbageCollectStrategyNever
	orphan := v1alpha1.GarbageCollectPropagation(v1alpha1.GarbageCollectPropagationOrphan)
	for name, tc := range map[string]struct {
		finalizers []string
		policy     *v1alpha1.GarbageCollectPolicySpec
		annotation map[string]string
		labels     map[string]string
		// kept reports that the resource must survive the delete.
		kept bool
		// sharedBy is the sharer list expected on a kept resource, "" when it should be unchanged.
		sharedBy string
	}{
		"a shared resource is only unshared": {
			annotation: map[string]string{oam.AnnotationAppSharedBy: "default/app,other-ns/other-app"},
			kept:       true,
			sharedBy:   "other-ns/other-app",
		},
		"a resource selected by an orphan propagation rule is released": {
			policy: &v1alpha1.GarbageCollectPolicySpec{Rules: []v1alpha1.GarbageCollectPolicyRule{{
				Selector:    v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"orphaned"}},
				Propagation: &orphan,
			}}},
			labels: map[string]string{oam.TraitTypeLabel: "orphaned"},
			kept:   true,
		},
		"a resource selected by a never strategy rule is released": {
			policy: &v1alpha1.GarbageCollectPolicySpec{Rules: []v1alpha1.GarbageCollectPolicyRule{{
				Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"kept"}},
				Strategy: never,
			}}},
			labels: map[string]string{oam.TraitTypeLabel: "kept"},
			kept:   true,
		},
		"every resource is released when the app is orphaning": {
			finalizers: []string{oam.FinalizerOrphanResource},
			kept:       true,
		},
		"an unprotected resource is deleted": {},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
			_rk, err := newAppKeeper(ctx, cli, &v1beta1.Application{
				ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1, Finalizers: tc.finalizers},
			}, Policies{GarbageCollect: tc.policy})
			r.NoError(err)
			rk := _rk.(*resourceKeeper)

			// The object in the cluster carries the protections and the app's marks.
			labels := map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"}
			for k, v := range resourcetracker.LabelsForKey("Application/default/app") {
				labels[k] = v
			}
			for k, v := range tc.labels {
				labels[k] = v
			}
			r.NoError(cli.Create(ctx, &v1.ConfigMap{ObjectMeta: v12.ObjectMeta{
				Name: "target", Namespace: "default", Labels: labels, Annotations: tc.annotation,
			}}))

			// What the workflow step hands over: the identity of the resource and nothing else.
			bare := &unstructured.Unstructured{}
			bare.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
			bare.SetName("target")
			bare.SetNamespace("default")
			r.NoError(rk.Delete(ctx, []*unstructured.Unstructured{bare}))

			got := &v1.ConfigMap{}
			err = cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: "target"}, got)
			if !tc.kept {
				r.True(kerrors.IsNotFound(err), "an unprotected resource should be deleted")
				return
			}
			r.NoError(err, "a protected resource must survive")
			if tc.sharedBy != "" {
				r.Equal(tc.sharedBy, got.Annotations[oam.AnnotationAppSharedBy])
				// The resource is handed to the next sharer, so it is marked as theirs and no longer ours.
				r.Equal("other-app", got.Labels[oam.LabelOwnerName])
				r.Equal("other-ns", got.Labels[oam.LabelOwnerNamespace])
				r.Equal("Application", got.Labels[oam.LabelOwnerKind])
				return
			}
			r.NotContains(got.Labels, oam.LabelAppName, "a released resource loses the app's marks")
			r.NotContains(got.Labels, oam.LabelAppNamespace)
			r.NotContains(got.Labels, oam.LabelOwnerName, "and no longer says the app owns it")
			r.NotContains(got.Labels, oam.LabelOwnerNamespace)
			r.NotContains(got.Labels, oam.LabelOwnerKind)
		})
	}
}

func TestResourceKeeperDeleteOfAMissingResource(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := newAppKeeper(ctx, cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}, Policies{})
	r.NoError(err)

	gone := &unstructured.Unstructured{}
	gone.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	gone.SetName("gone")
	gone.SetNamespace("default")
	r.NoError(_rk.Delete(ctx, []*unstructured.Unstructured{gone}))
}
