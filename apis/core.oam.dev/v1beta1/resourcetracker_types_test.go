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

package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	v12 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v13 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/errors"
)

func TestManagedResource_DeepCopyEqual(t *testing.T) {
	r := require.New(t)
	mr := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
		OAMObjectReference:     common.OAMObjectReference{Component: "component"},
		Data:                   &runtime.RawExtension{Raw: []byte("data")},
	}
	r.True(mr.Equal(*mr.DeepCopy()))
}

func TestManagedResource_Equal(t *testing.T) {
	testCases := map[string]struct {
		input1 ManagedResource
		input2 ManagedResource
		equal  bool
	}{
		"equal": {
			input1: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
				OAMObjectReference:     common.OAMObjectReference{Component: "component"},
				Data:                   &runtime.RawExtension{Raw: []byte("data")},
			},
			input2: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
				OAMObjectReference:     common.OAMObjectReference{Component: "component"},
				Data:                   &runtime.RawExtension{Raw: []byte("data")},
			},
			equal: true,
		},
		"ClusterObjectReference not equal": {
			input1: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster"},
			},
			input2: ManagedResource{
				ClusterObjectReference: common.ClusterObjectReference{Cluster: "c"},
			},
			equal: false,
		},
		"OAMObjectReference not equal": {
			input1: ManagedResource{
				OAMObjectReference: common.OAMObjectReference{Component: "component"},
			},
			input2: ManagedResource{
				OAMObjectReference: common.OAMObjectReference{Component: "c"},
			},
			equal: false,
		},
		"Data content not equal": {
			input1: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("data")},
			},
			input2: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("d")},
			},
			equal: false,
		},
		"one data empty, one data not empty": {
			input1: ManagedResource{Data: nil},
			input2: ManagedResource{
				Data: &runtime.RawExtension{Raw: []byte("d")},
			},
			equal: false,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			r.Equal(tc.equal, tc.input1.Equal(tc.input2))
			r.Equal(tc.equal, tc.input2.Equal(tc.input1))
		})
	}
}

func TestManagedResourceKeys(t *testing.T) {
	r := require.New(t)
	input := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{
			Cluster: "cluster",
			ObjectReference: v1.ObjectReference{
				Namespace:  "namespace",
				Name:       "name",
				APIVersion: v12.SchemeGroupVersion.String(),
				Kind:       "Deployment",
			},
		},
		OAMObjectReference: common.OAMObjectReference{
			Env:       "env",
			Component: "component",
			Trait:     "trait",
		},
	}
	r.Equal("namespace/name", input.NamespacedName().String())
	r.Equal("apps/v1/Deployment/cluster/namespace/name", input.ResourceKey())
	r.Equal("env/component", input.ComponentKey())
	r.Equal("Deployment name (Cluster: cluster, Namespace: namespace)", input.DisplayName())
	var deploy1, deploy2 v12.Deployment
	deploy1.Spec.Replicas = pointer.Int32(5)
	bs, err := json.Marshal(deploy1)
	r.NoError(err)
	r.ErrorIs(input.UnmarshalTo(&deploy2), errors.ManagedResourceHasNoDataError{})
	_, err = input.ToUnstructuredWithData()
	r.ErrorIs(err, errors.ManagedResourceHasNoDataError{})
	input.Data = &runtime.RawExtension{Raw: bs}
	r.NoError(input.UnmarshalTo(&deploy2))
	r.Equal(deploy1, deploy2)
	obj := input.ToUnstructured()
	r.Equal("Deployment", obj.GetKind())
	r.Equal("apps/v1", obj.GetAPIVersion())
	r.Equal("name", obj.GetName())
	r.Equal("namespace", obj.GetNamespace())
	r.Equal("cluster", oam.GetCluster(obj))
	obj, err = input.ToUnstructuredWithData()
	r.NoError(err)
	val, correct, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	r.NoError(err)
	r.True(correct)
	r.Equal(int64(5), val)
}

func TestResourceTracker_ManagedResource(t *testing.T) {
	r := require.New(t)
	input := &ResourceTracker{}
	deploy1 := v12.Deployment{ObjectMeta: v13.ObjectMeta{Name: "deploy1"}}
	input.AddManagedResource(&deploy1, nil, true)
	r.Equal(1, len(input.Spec.ManagedResources))
	cm2 := v1.ConfigMap{ObjectMeta: v13.ObjectMeta{Name: "cm2"}}
	input.AddManagedResource(&cm2, nil, false)
	r.Equal(2, len(input.Spec.ManagedResources))
	pod3 := v1.Pod{ObjectMeta: v13.ObjectMeta{Name: "pod3"}}
	input.AddManagedResource(&pod3, nil, false)
	r.Equal(3, len(input.Spec.ManagedResources))
	deploy1.Spec.Replicas = pointer.Int32(5)
	input.AddManagedResource(&deploy1, nil, false)
	r.Equal(3, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&cm2, false)
	r.Equal(3, len(input.Spec.ManagedResources))
	r.True(input.Spec.ManagedResources[1].Deleted)
	input.DeleteManagedResource(&cm2, true)
	r.Equal(2, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&deploy1, true)
	r.Equal(1, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&pod3, true)
	r.Equal(0, len(input.Spec.ManagedResources))
	secret4 := v1.Secret{ObjectMeta: v13.ObjectMeta{Name: "secret4"}}
	input.DeleteManagedResource(&secret4, true)
	r.Equal(0, len(input.Spec.ManagedResources))
	input.DeleteManagedResource(&secret4, false)
	r.Equal(1, len(input.Spec.ManagedResources))
}
