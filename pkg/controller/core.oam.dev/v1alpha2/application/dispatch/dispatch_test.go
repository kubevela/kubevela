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

package dispatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestSetOAMOwner(t *testing.T) {
	tests := map[string]struct {
		OO       ObjectOwner
		CO       v1.OwnerReference
		ExpOwner []v1.OwnerReference
	}{
		"test empty origin owner": {
			OO: &unstructured.Unstructured{},
			CO: v1.OwnerReference{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			},
			ExpOwner: []v1.OwnerReference{{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			}},
		},
		"test remove old resourceTracker owner": {
			OO: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "core.oam.dev/v1beta1",
								"kind":       "ResourceTracker",
							},
							map[string]interface{}{
								"apiVersion": "core.oam.dev/v1alpha2",
								"kind":       "ApplicationContext",
							},
						},
					},
				},
			},
			CO: v1.OwnerReference{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			},
			ExpOwner: []v1.OwnerReference{{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			}},
		},
		"test other owner not removed": {
			OO: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "core.oam.dev/v1alpha1",
								"kind":       "Rollout",
								"name":       "xxx",
							},
						},
					},
				},
			},
			CO: v1.OwnerReference{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			},
			ExpOwner: []v1.OwnerReference{{
				APIVersion: "core.oam.dev/v1beta1",
				Kind:       "ResourceTracker",
				Name:       "myapp",
			},
				{
					APIVersion: "core.oam.dev/v1alpha1",
					Kind:       "Rollout",
					Name:       "xxx",
				}},
		},
	}
	for name, ti := range tests {
		setOrOverrideOAMControllerOwner(ti.OO, ti.CO)
		assert.Equal(t, ti.ExpOwner, ti.OO.GetOwnerReferences(), name)
	}
}

func TestCheckComponentDeleted(t *testing.T) {
	wl_1 := unstructured.Unstructured{}
	wl_1.SetLabels(map[string]string{oam.LabelAppComponent: "comp-1"})

	wl_2 := unstructured.Unstructured{}

	wl_3 := unstructured.Unstructured{}
	wl_3.SetLabels(map[string]string{oam.LabelAppComponent: "comp-3"})

	components := []common.ApplicationComponent{{Name: "comp-1"}}

	testCase := map[string]struct {
		u   unstructured.Unstructured
		res bool
	}{
		"exsit comp":       {wl_1, false},
		"no label deleted": {wl_2, true},
		"not exsit comp":   {wl_3, true},
	}

	for caseName, s := range testCase {
		b := checkResourceRelatedCompDeleted(s.u, components)
		if b != s.res {
			t.Errorf("check comp deleted func meet error: %s want %v got %v", caseName, s.res, b)
		}
	}
}
