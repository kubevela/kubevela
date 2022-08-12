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
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
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

func TestApplyComponents(t *testing.T) {
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
	apply := func(comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		applyMap.Store(fmt.Sprintf("%s/%s", clusterName, comp.Name), true)
		return nil, nil, true, nil
	}
	healthCheck := func(comp apicommon.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, error) {
		_, found := applyMap.Load(fmt.Sprintf("%s/%s", clusterName, comp.Name))
		return found, nil
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

	healthy, _, err := applyComponents(apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.False(healthy)
	r.Equal(n*m, countMap())

	healthy, _, err = applyComponents(apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.False(healthy)
	r.Equal(2*n*m, countMap())

	healthy, _, err = applyComponents(apply, healthCheck, components, placements, parallelism)
	r.NoError(err)
	r.True(healthy)
	r.Equal(3*n*m, countMap())
}
