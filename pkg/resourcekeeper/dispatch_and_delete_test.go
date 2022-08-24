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
	"testing"

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
