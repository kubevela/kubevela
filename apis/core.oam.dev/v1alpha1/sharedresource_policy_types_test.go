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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSharedResourcePolicySpec_FindStrategy(t *testing.T) {
	testCases := map[string]struct {
		rules   []SharedResourcePolicyRule
		input   *unstructured.Unstructured
		matched bool
	}{
		"shared resource rule resourceName match": {
			rules: []SharedResourcePolicyRule{{
				Selector: ResourcePolicyRuleSelector{ResourceNames: []string{"example"}},
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "example",
				},
			}},
			matched: true,
		},
		"shared resource rule resourceType match": {
			rules: []SharedResourcePolicyRule{{
				Selector: ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap", "Namespace"}},
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind": "Namespace",
			}},
			matched: true,
		},
		"shared resource rule mismatch": {
			rules: []SharedResourcePolicyRule{{
				Selector: ResourcePolicyRuleSelector{ResourceNames: []string{"mismatch"}},
			}},
			input: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind": "Namespace",
			}},
			matched: false,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			spec := SharedResourcePolicySpec{Rules: tc.rules}
			r.Equal(tc.matched, spec.FindStrategy(tc.input))
		})
	}
}
