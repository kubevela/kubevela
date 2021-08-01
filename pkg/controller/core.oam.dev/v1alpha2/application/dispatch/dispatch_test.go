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
