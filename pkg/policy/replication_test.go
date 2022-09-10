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

package policy

import (
	"fmt"
	"testing"

	assert2 "github.com/stretchr/testify/assert"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestReplicateComponents(t *testing.T) {
	comp1 := common.ApplicationComponent{Name: "comp1"}
	comp2 := common.ApplicationComponent{Name: "comp2"}
	baseComponents := []common.ApplicationComponent{
		comp1,
		comp2,
	}
	testCases := map[string]struct {
		Components []common.ApplicationComponent
		Selectors  []string
		Output     []common.ApplicationComponent
		WantErr    error
	}{
		"nil selector, don't replicate": {
			Components: baseComponents,
			Selectors:  nil,
			Output:     nil,
			WantErr:    fmt.Errorf("no component selected to replicate"),
		},
		"select all, replicate all": {
			Components: baseComponents,
			Selectors:  []string{"comp1", "comp2"},
			Output:     baseComponents,
		},
		"replicate part": {
			Components: baseComponents,
			Selectors:  []string{"comp1"},
			Output:     []common.ApplicationComponent{comp1},
		},
		"part invalid selector": {
			Components: baseComponents,
			Selectors:  []string{"comp1", "comp3"},
			Output:     []common.ApplicationComponent{comp1},
		},
		"no component selected": {
			Components: baseComponents,
			Selectors:  []string{"comp3"},
			Output:     []common.ApplicationComponent{},
			WantErr:    fmt.Errorf("no component selected for replicate"),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result, err := selectReplicateComponents(tc.Components, tc.Selectors)
			if tc.WantErr != nil {
				assert2.Error(t, err)
			} else {
				assert.NilError(t, err)
				assert.Equal(t, len(result), len(tc.Output))
				assert.DeepEqual(t, result, tc.Output)
			}
		})
	}
}

func TestGetReplicationComponents(t *testing.T) {
	baseComps := []common.ApplicationComponent{
		{Name: "comp1"},
		{Name: "comp2"},
	}
	PolicyName := "test-policy"
	testCases := map[string]struct {
		Policies   []v1beta1.AppPolicy
		Components []common.ApplicationComponent
		WantErr    error
		WantComps  []common.ApplicationComponent
	}{
		"no replication policy, all components remain unchanged": {
			Policies: []v1beta1.AppPolicy{
				{
					Name:       PolicyName,
					Type:       "foo",
					Properties: nil,
				},
			},
			Components: baseComps,
			WantComps:  baseComps,
		},
		"one replication policy, replicate those components": {
			Policies: []v1beta1.AppPolicy{
				{
					Name: PolicyName,
					Type: "replication",
					Properties: util.Object2RawExtension(v1alpha1.ReplicationPolicySpec{
						Keys:     []string{"replica-1", "replica-2"},
						Selector: []string{"comp1"},
					}),
				},
			},
			Components: baseComps,
			WantComps: []common.ApplicationComponent{
				{Name: "comp2"},
				{Name: "comp1", ReplicaKey: "replica-1"},
				{Name: "comp1", ReplicaKey: "replica-2"},
			},
		},
		"replicate non-exist component": {
			Policies: []v1beta1.AppPolicy{
				{
					Name: PolicyName,
					Type: "replication",
					Properties: util.Object2RawExtension(v1alpha1.ReplicationPolicySpec{
						Keys:     []string{"replica-1", "replica-2"},
						Selector: []string{"comp-non-exist"},
					}),
				},
			},
			Components: baseComps,
			WantErr:    fmt.Errorf("failed to apply replicate policy %s", PolicyName),
		},
		"invalid-override-policy": {
			Policies: []v1beta1.AppPolicy{
				{
					Name:       PolicyName,
					Type:       "replication",
					Properties: &runtime.RawExtension{Raw: []byte(`{bad value}`)},
				},
			},
			Components: baseComps,
			WantErr:    fmt.Errorf("failed to parse replicate policy %s", PolicyName),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			comps, err := ReplicateComponents(tc.Policies, tc.Components)
			if tc.WantErr != nil {
				assert2.Error(t, err)
				assert2.Contains(t, err.Error(), tc.WantErr.Error())
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, comps, tc.WantComps)
			}
		})
	}
}
