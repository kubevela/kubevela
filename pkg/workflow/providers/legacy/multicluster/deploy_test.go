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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
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

func setupHandlers(ctx context.Context, t *testing.T) (client.Client, *appfile.Appfile, *application.AppHandler) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).Build()
	singleton.KubeClient.Set(cli)
	p := appfile.NewApplicationParser(cli)
	handler, err := application.NewAppHandler(ctx, &application.Reconciler{
		Client: cli,
	}, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}, p)
	r.NoError(err)
	appfile := &appfile.Appfile{
		Name: "test",
		AppRevision: &v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
						"test": {
							ObjectMeta: metav1.ObjectMeta{Name: "test"},
							Spec: v1beta1.ComponentDefinitionSpec{
								// Status: &common.Status{
								// 	HealthPolicy: `isHealth: false`,
								// },
								Schematic: &apicommon.Schematic{
									CUE: &apicommon.CUE{
										Template: `output: {
	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		labels: {
			app: "web"
		}
		name: "test-\(parameter.idx)"
	}
}
`,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	handler.PrepareCurrentAppRevision(ctx, appfile)
	return cli, appfile, handler
}

func TestApplyComponentsDepends(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	_, appfile, handler := setupHandlers(ctx, t)
	const n, m = 50, 5
	var components []apicommon.ApplicationComponent
	var placements []v1alpha1.PlacementDecision
	for i := 0; i < n*3; i++ {
		comp := apicommon.ApplicationComponent{Name: fmt.Sprintf("comp-%d", i), Type: "test", Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"idx": %d}`, i))}}
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
	parallelism := 10

	countMap := func() int {
		cnt := 0
		applyMap.Range(func(key, value interface{}) bool {
			cnt++
			return true
		})
		return cnt
	}
	appfile.Components = components
	healthy, _, err := applyComponents(ctx, appfile, handler, components, placements, parallelism)
	fmt.Println("======", err.Error())
	r.NoError(err)
	r.False(healthy)
	r.Equal(n*m, countMap())

	healthy, _, err = applyComponents(ctx, appfile, handler, components, placements, parallelism)
	r.NoError(err)
	r.False(healthy)
	r.Equal(2*n*m, countMap())

	healthy, _, err = applyComponents(ctx, appfile, handler, components, placements, parallelism)
	r.NoError(err)
	r.True(healthy)
	r.Equal(3*n*m, countMap())
}

func TestApplyComponentsIO(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	scheme := runtime.NewScheme()
	r.NoError(v1beta1.AddToScheme(scheme))
	r.NoError(appsv1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	singleton.KubeClient.Set(cli)
	p := appfile.NewApplicationParser(cli)
	handler, err := application.NewAppHandler(ctx, &application.Reconciler{
		Client: cli,
	}, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}, p)
	r.NoError(err)
	appfile := &appfile.Appfile{
		AppRevision: &v1beta1.ApplicationRevision{},
	}
	handler.PrepareCurrentAppRevision(ctx, appfile)

	var (
		parallelism = 10
		applyMap    = new(sync.Map)
	)

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
			healthy, _, err := applyComponents(ctx, appfile, handler, components, placements, parallelism)
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
		healthy, _, err := applyComponents(ctx, appfile, handler, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)
		healthy, _, err = applyComponents(ctx, appfile, handler, components, placements, parallelism)
		r.ErrorContains(err, "failed to lookup value")
		r.False(healthy)
	})

	t.Run("apply components with io and replication", func(t *testing.T) {
		// comp-0 ---> comp1-beijing  --> comp2-beijing
		// 		   |-> comp1-shanghai --> comp2-shanghai
		resetStore()

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
		healthy, _, err := applyComponents(ctx, appfile, handler, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)

		healthy, _, err = applyComponents(ctx, appfile, handler, components, placements, parallelism)
		r.NoError(err)
		r.False(healthy)

		healthy, _, err = applyComponents(ctx, appfile, handler, components, placements, parallelism)
		r.NoError(err)
		r.True(healthy)

	})
}
