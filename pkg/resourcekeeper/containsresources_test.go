/*
Copyright 2022 The KubeVela Authors.

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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// newUnstructured is a helper to create a simple unstructured object for testing.
func newUnstructured(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

func TestContainsResources(t *testing.T) {
	res1 := newUnstructured("res-1")
	res2 := newUnstructured("res-2")
	res3 := newUnstructured("res-3")
	res4Missing := newUnstructured("res-4-missing")

	baseKeeperSetup := func() *resourceKeeper {
		return &resourceKeeper{
			Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			_currentRT: &v1beta1.ResourceTracker{},
			_rootRT:    &v1beta1.ResourceTracker{},
		}
	}

	testCases := []struct {
		name     string
		keeper   *resourceKeeper
		input    []*unstructured.Unstructured
		expected bool
	}{
		{
			name: "all resources exist across both trackers",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				k._rootRT.AddManagedResource(res2, true, false, "")
				k._rootRT.AddManagedResource(res3, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1, res2, res3},
			expected: true,
		},
		{
			name: "one resource exists in currentRT",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1},
			expected: true,
		},
		{
			name: "one resource exists in rootRT",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._rootRT.AddManagedResource(res2, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res2},
			expected: true,
		},
		{
			name: "one resource is missing",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1, res4Missing},
			expected: false,
		},
		{
			name: "empty input slice should return true",
			keeper: &resourceKeeper{
				Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			},
			input:    []*unstructured.Unstructured{},
			expected: true,
		},
		{
			name: "trackers are nil",
			keeper: &resourceKeeper{
				Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			},
			input:    []*unstructured.Unstructured{res1},
			expected: false,
		},
		{
			name: "only rootRT is nil, resource in currentRT",
			keeper: func() *resourceKeeper {
				k := &resourceKeeper{
					Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
					_currentRT: &v1beta1.ResourceTracker{},
				}
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1},
			expected: true,
		},
		{
			name: "only rootRT is nil, resource not in currentRT",
			keeper: func() *resourceKeeper {
				k := &resourceKeeper{
					Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
					_currentRT: &v1beta1.ResourceTracker{},
				}
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res2},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			result := tc.keeper.ContainsResources(tc.input)
			r.Equal(tc.expected, result)
		})
	}
}
