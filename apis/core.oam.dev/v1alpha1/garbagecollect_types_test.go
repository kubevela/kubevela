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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestGarbageCollectPolicySpec_FindStrategy(t *testing.T) {
	testCases := map[string]struct {
		rules          []GarbageCollectPolicyRule
		input          *unstructured.Unstructured
		notFound       bool
		expectStrategy GarbageCollectStrategy
	}{
		"trait type rule match": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{TraitTypes: []string{"a"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.TraitTypeLabel: "a"},
				},
			}},
			expectStrategy: GarbageCollectStrategyNever,
		},
		"trait type rule mismatch": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{TraitTypes: []string{"a"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input:    &unstructured.Unstructured{Object: map[string]interface{}{}},
			notFound: true,
		},
		"trait type rule multiple match": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{TraitTypes: []string{"a"}},
				Strategy: GarbageCollectStrategyOnAppDelete,
			}, {
				Selector: ResourcePolicyRuleSelector{TraitTypes: []string{"a"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.TraitTypeLabel: "a"},
				},
			}},
			expectStrategy: GarbageCollectStrategyOnAppDelete,
		},
		"component type rule match": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{CompTypes: []string{"comp"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.WorkloadTypeLabel: "comp"},
				},
			}},
			expectStrategy: GarbageCollectStrategyNever,
		},
		"rule match both component type and trait type, component type first": {
			rules: []GarbageCollectPolicyRule{
				{
					Selector: ResourcePolicyRuleSelector{CompTypes: []string{"comp"}},
					Strategy: GarbageCollectStrategyNever,
				},
				{
					Selector: ResourcePolicyRuleSelector{TraitTypes: []string{"trait"}},
					Strategy: GarbageCollectStrategyOnAppDelete,
				},
			},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.WorkloadTypeLabel: "comp", oam.TraitTypeLabel: "trait"},
				},
			}},
			expectStrategy: GarbageCollectStrategyNever,
		},
		"component name rule match": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{CompNames: []string{"comp-name"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.LabelAppComponent: "comp-name"},
				},
			}},
			expectStrategy: GarbageCollectStrategyNever,
		},
		"resource type rule match": {
			rules: []GarbageCollectPolicyRule{{
				Selector: ResourcePolicyRuleSelector{OAMResourceTypes: []string{"TRAIT"}},
				Strategy: GarbageCollectStrategyNever,
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{oam.LabelOAMResourceType: "TRAIT"},
				},
			}},
			expectStrategy: GarbageCollectStrategyNever,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			spec := GarbageCollectPolicySpec{Rules: tc.rules}
			strategy := spec.FindStrategy(tc.input)
			if tc.notFound {
				r.Nil(strategy)
			} else {
				r.Equal(tc.expectStrategy, *strategy)
			}
		})
	}
}
