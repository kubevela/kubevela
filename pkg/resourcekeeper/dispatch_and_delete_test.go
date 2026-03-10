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
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestResourceKeeperDispatchAndDelete(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := NewResourceKeeper(context.Background(), cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	})
	r.NoError(err)
	rk := _rk.(*resourceKeeper)
	rk.garbageCollectPolicy = &v1alpha1.GarbageCollectPolicySpec{
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
	rk.applyOncePolicy = &v1alpha1.ApplyOncePolicySpec{Enable: true}
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
	_rk, err := NewResourceKeeper(context.Background(), cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	})
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
		app:    app,
		applyOncePolicy: &v1alpha1.ApplyOncePolicySpec{
			Enable: true,
			Rules: []v1alpha1.ApplyOncePolicyRule{{
				Selector: v1alpha1.ResourcePolicyRuleSelector{
					CompNames: []string{"my-comp"},
				},
				Strategy: &v1alpha1.ApplyOnceStrategy{Path: []string{"*"}},
			}},
		},
	}

	manifest := &unstructured.Unstructured{}
	manifest.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	manifest.SetName("nonexistent-cm")
	manifest.SetNamespace("default")
	manifest.SetLabels(map[string]string{oam.LabelAppComponent: "my-comp"})

	// For ApplyOnceStrategyOnAppStateKeep, a missing resource returns nil.
	result, err := ApplyStrategies(context.Background(), rk, manifest, v1alpha1.ApplyOnceStrategyOnAppStateKeep)
	r.NoError(err)
	r.Nil(result)

	// For ApplyOnceStrategyOnAppUpdate, a missing resource returns the original manifest (not nil).
	// This means the nil-guard in dispatch.go is defensive and cannot be triggered today.
	result, err = ApplyStrategies(context.Background(), rk, manifest, v1alpha1.ApplyOnceStrategyOnAppUpdate)
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
		app:    app,
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
