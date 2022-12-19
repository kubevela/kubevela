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

package multicluster

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestOverrideConfiguration(t *testing.T) {
	testCases := map[string]struct {
		Policies   []v1beta1.AppPolicy
		Components []apicommon.ApplicationComponent
		Outputs    []apicommon.ApplicationComponent
		Error      string
	}{
		"invalid-policies": {
			Policies: []v1beta1.AppPolicy{{
				Name:       "override-policy",
				Type:       "override",
				Properties: &runtime.RawExtension{Raw: []byte(`bad value`)},
			}},
			Error: "failed to parse override policy",
		},
		"empty-policy": {
			Policies: []v1beta1.AppPolicy{{
				Name:       "override-policy",
				Type:       "override",
				Properties: nil,
			}},
			Error: "empty properties",
		},
		"normal": {
			Policies: []v1beta1.AppPolicy{{
				Name:       "override-policy",
				Type:       "override",
				Properties: &runtime.RawExtension{Raw: []byte(`{"components":[{"name":"comp","properties":{"x":5}}]}`)},
			}},
			Components: []apicommon.ApplicationComponent{{
				Name:       "comp",
				Traits:     []apicommon.ApplicationTrait{},
				Properties: &runtime.RawExtension{Raw: []byte(`{"x":1}`)},
			}},
			Outputs: []apicommon.ApplicationComponent{{
				Name:       "comp",
				Traits:     []apicommon.ApplicationTrait{},
				Properties: &runtime.RawExtension{Raw: []byte(`{"x":5}`)},
			}},
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			comps, err := overrideConfiguration(tt.Policies, tt.Components)
			if tt.Error != "" {
				r.NotNil(err)
				r.Contains(err.Error(), tt.Error)
			} else {
				r.NoError(err)
				r.Equal(tt.Outputs, comps)
			}
		})
	}
}

func TestApplyComponentsDepends(t *testing.T) {
	r := require.New(t)
	const n, m = 50, 5
	var components []apicommon.ApplicationComponent
	var placements []v1alpha1.PlacementDecision
	for i := 0; i < n*3; i++ {
		comp := apicommon.ApplicationComponent{Name: fmt.Sprintf("comp-%d", i)}
		if i%3 != 0 {
			comp.DependsOn = append(comp.DependsOn, fmt.Sprintf("comp-%d", i-1))
		}
		if i%3 == 2 {
			comp.DependsOn = append(comp.DependsOn, fmt.Sprintf("comp-%d", i-1))
		}
		components = append(components, comp)
	}
	for i := 0; i < m; i++ {
		placements = append(placements, v1alpha1.PlacementDecision{Cluster: fmt.Sprintf("cluster-%d", i)})
	}

	applyMap := &sync.Map{}
	apply := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		applyMap.Store(fmt.Sprintf("%s/%s", clusterName, comp.Name), true)
		return nil, nil, true, nil
	}
	healthCheck := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
		_, found := applyMap.Load(fmt.Sprintf("%s/%s", clusterName, comp.Name))
		return found, nil, nil, nil
	}
	parallelism := 10

	countMap := func() int {
		cnt := 0
		applyMap.Range(func(key, value interface{}) bool {
			cnt++
			return true
		})
		return cnt
	}
	ctx := context.Background()
	healthy, _, err := applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.False(healthy)
	r.Equal(n*m, countMap())

	healthy, _, err = applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.False(healthy)
	r.Equal(2*n*m, countMap())

	healthy, _, err = applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.True(healthy)
	r.Equal(3*n*m, countMap())
}

func TestApplyComponentsIO(t *testing.T) {
	r := require.New(t)

	var (
		parallelism = 10
		applyMap    = new(sync.Map)
		ctx         = context.Background()
	)
	apply := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		applyMap.Store(fmt.Sprintf("%s/%s", clusterName, comp.Name), true)
		return nil, nil, true, nil
	}
	healthCheck := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
		_, found := applyMap.Load(fmt.Sprintf("%s/%s", clusterName, comp.Name))
		return found, &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"path": fmt.Sprintf("%s/%s", clusterName, comp.Name),
				},
			}}, []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								oam.TraitResource: "obj",
							},
						},
						"spec": map[string]interface{}{
							"path": fmt.Sprintf("%s/%s", clusterName, comp.Name),
						},
					},
				},
			}, nil
	}

	resetStore := func() {
		applyMap = &sync.Map{}
	}
	countMap := func() int {
		cnt := 0
		applyMap.Range(func(key, value interface{}) bool {
			cnt++
			return true
		})
		return cnt
	}

	t.Run("apply components with io successfully", func(t *testing.T) {
		resetStore()
		const n, m = 10, 5
		var components []apicommon.ApplicationComponent
		var placements []v1alpha1.PlacementDecision
		for i := 0; i < n; i++ {
			comp := apicommon.ApplicationComponent{
				Name:       fmt.Sprintf("comp-%d", i),
				Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"placeholder":%d}`, i))},
			}
			if i != 0 {
				comp.Inputs = workflowv1alpha1.StepInputs{
					{
						ParameterKey: "input_slot_1",
						From:         fmt.Sprintf("var-output-%d", i-1),
					},
					{
						ParameterKey: "input_slot_2",
						From:         fmt.Sprintf("var-outputs-%d", i-1),
					},
				}
			}
			if i != n-1 {
				comp.Outputs = workflowv1alpha1.StepOutputs{
					{
						ValueFrom: "output.spec.path",
						Name:      fmt.Sprintf("var-output-%d", i),
					},
					{
						ValueFrom: "outputs.obj.spec.path",
						Name:      fmt.Sprintf("var-outputs-%d", i),
					},
				}
			}
			components = append(components, comp)
		}
		for i := 0; i < m; i++ {
			placements = append(placements, v1alpha1.PlacementDecision{Cluster: fmt.Sprintf("cluster-%d", i)})
		}

		for i := 0; i < n; i++ {
			healthy, _, err := applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
			r.NoError(err)
			r.Equal((i+1)*m, countMap())
			if i == n-1 {
				r.True(healthy)
			} else {
				r.False(healthy)
			}
		}
	})

	t.Run("apply components with io failed", func(t *testing.T) {
		resetStore()
		components := []apicommon.ApplicationComponent{
			{
				Name: "comp-0",
				Outputs: workflowv1alpha1.StepOutputs{
					{
						ValueFrom: "output.spec.error_path",
						Name:      "var1",
					},
				},
			},
			{
				Name: "comp-1",
				Inputs: workflowv1alpha1.StepInputs{
					{
						ParameterKey: "input_slot_1",
						From:         "var1",
					},
				},
			},
		}
		placements := []v1alpha1.PlacementDecision{
			{Cluster: "cluster-0"},
		}
		healthy, _, err := applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)
		healthy, _, err = applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
		r.ErrorContains(err, "failed to lookup value")
		r.False(healthy)
	})

	t.Run("apply components with io and replication", func(t *testing.T) {
		// comp-0 ---> comp1-beijing  --> comp2-beijing
		// 		   |-> comp1-shanghai --> comp2-shanghai
		resetStore()
		storeKey := func(clusterName string, comp apicommon.ApplicationComponent) string {
			return fmt.Sprintf("%s/%s/%s", clusterName, comp.Name, comp.ReplicaKey)
		}
		type applyResult struct {
			output  *unstructured.Unstructured
			outputs []*unstructured.Unstructured
		}
		apply := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
			time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
			key := storeKey(clusterName, comp)
			result := applyResult{
				output: &unstructured.Unstructured{Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"path":        key,
						"anotherPath": key,
					},
				}}, outputs: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									oam.TraitResource: "obj",
								},
							},
							"spec": map[string]interface{}{
								"path": key,
							},
						},
					},
				},
			}
			applyMap.Store(storeKey(clusterName, comp), result)
			return nil, nil, true, nil
		}
		healthCheck := func(_ context.Context, comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
			key := storeKey(clusterName, comp)
			r, found := applyMap.Load(key)
			result, _ := r.(applyResult)
			return found, result.output, result.outputs, nil
		}

		inputSlot := "input_slot"
		components := []apicommon.ApplicationComponent{
			{
				Name: "comp-0",
				Outputs: workflowv1alpha1.StepOutputs{
					{
						ValueFrom: "output.spec.path",
						Name:      "var1",
					},
				},
			},
			{
				Name: "comp-1",
				Inputs: workflowv1alpha1.StepInputs{
					{
						ParameterKey: inputSlot,
						From:         "var1",
					},
				},
				Outputs: workflowv1alpha1.StepOutputs{
					{
						ValueFrom: "output.spec.anotherPath",
						Name:      "var2",
					},
				},
				ReplicaKey: "beijing",
			},
			{
				Name: "comp-1",
				Inputs: workflowv1alpha1.StepInputs{
					{
						ParameterKey: inputSlot,
						From:         "var1",
					},
				},
				Outputs: workflowv1alpha1.StepOutputs{
					{
						ValueFrom: "output.spec.anotherPath",
						Name:      "var2",
					},
				},
				ReplicaKey: "shanghai",
			},
			{
				Name: "comp-2",
				Inputs: workflowv1alpha1.StepInputs{
					{
						ParameterKey: inputSlot,
						From:         "var2",
					},
				},
				ReplicaKey: "beijing",
			},
			{
				Name: "comp-2",
				Inputs: workflowv1alpha1.StepInputs{
					{
						ParameterKey: inputSlot,
						From:         "var2",
					},
				},
				ReplicaKey: "shanghai",
			},
		}
		placements := []v1alpha1.PlacementDecision{
			{Cluster: "cluster-0"},
		}
		healthy, _, err := applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)

		healthy, _, err = applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)

		healthy, _, err = applyComponents(ctx, apply, healthCheck, components, placements, parallelism)
		r.NoError(err)
		r.True(healthy)

	})
}
