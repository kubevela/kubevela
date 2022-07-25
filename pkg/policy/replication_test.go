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
	baseComponents := []common.ApplicationComponent{
		{Name: "comp1"},
		{Name: "comp2"},
	}
	testCases := map[string]struct {
		Components []common.ApplicationComponent
		Selectors  []string
		Output     []string
		WantErr    error
	}{
		"nil selector, replicate all": {
			Components: baseComponents,
			Selectors:  nil,
			Output:     []string{"comp1", "comp2"},
		},
		"select all, replicate all": {
			Components: baseComponents,
			Selectors:  []string{"comp1", "comp2"},
			Output:     []string{"comp1", "comp2"},
		},
		"replicate part": {
			Components: baseComponents,
			Selectors:  []string{"comp1"},
			Output:     []string{"comp1"},
		},
		"part invalid selector": {
			Components: baseComponents,
			Selectors:  []string{"comp1", "comp3"},
			Output:     []string{"comp1"},
		},
		"no component selected": {
			Components: baseComponents,
			Selectors:  []string{"comp3"},
			Output:     []string{},
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
		Policies       []v1beta1.AppPolicy
		Components     []common.ApplicationComponent
		WantErr        error
		WantComps      []common.ApplicationComponent
		WantReplicaDes []v1alpha1.ReplicationDecision
	}{
		"no replication policy, filtered all components": {
			Policies: []v1beta1.AppPolicy{
				{
					Name:       PolicyName,
					Type:       "foo",
					Properties: nil,
				},
			},
			Components:     baseComps,
			WantComps:      baseComps,
			WantReplicaDes: nil,
		},
		"one replication policy, filter some components": {
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
				{Name: "comp1"},
			},
			WantReplicaDes: []v1alpha1.ReplicationDecision{
				{
					Keys:       []string{"replica-1", "replica-2"},
					Components: []string{"comp1"},
				},
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
				{Name: PolicyName,
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
			rds, comps, err := GetReplicationComponents(tc.Policies, tc.Components)
			if tc.WantErr != nil {
				assert2.Error(t, err)
				assert2.Contains(t, err.Error(), tc.WantErr.Error())
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, comps, tc.WantComps)
				assert.DeepEqual(t, rds, tc.WantReplicaDes)
			}
		})
	}
}
