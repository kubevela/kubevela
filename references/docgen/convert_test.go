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

package docgen

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestParseCapabilityFromUnstructured(t *testing.T) {
	testCases := []struct {
		name       string
		obj        unstructured.Unstructured
		wantCap    types.Capability
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "trait definition",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "TraitDefinition",
					"metadata": map[string]interface{}{
						"name": "my-trait",
					},
					"spec": map[string]interface{}{
						"appliesToWorkloads": []interface{}{"webservice", "worker"},
						"schematic": map[string]interface{}{
							"cue": map[string]interface{}{
								"template": "parameter: {}",
							},
						},
					},
				},
			},
			wantCap: types.Capability{
				Name:      "my-trait",
				Type:      types.TypeTrait,
				AppliesTo: []string{"webservice", "worker"},
			},
			wantErr: false,
		},
		{
			name: "component definition",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "ComponentDefinition",
					"metadata": map[string]interface{}{
						"name": "my-comp",
					},
					"spec": map[string]interface{}{
						"workload": map[string]interface{}{
							"type": "worker",
						},
						"schematic": map[string]interface{}{
							"cue": map[string]interface{}{
								"template": "parameter: {}",
							},
						},
					},
				},
			}, wantCap: types.Capability{
				Name: "my-comp",
				Type: types.TypeComponentDefinition,
			},
			wantErr: false,
		},
		{
			name: "policy definition",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "PolicyDefinition",
					"metadata": map[string]interface{}{
						"name": "my-policy",
					},
					"spec": map[string]interface{}{
						"schematic": map[string]interface{}{
							"cue": map[string]interface{}{
								"template": "parameter: {}",
							},
						},
					},
				},
			},
			wantCap: types.Capability{
				Name: "my-policy",
				Type: types.TypePolicy,
			},
			wantErr: false,
		},
		{
			name: "workflow step definition",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "WorkflowStepDefinition",
					"metadata": map[string]interface{}{
						"name": "my-step",
					},
					"spec": map[string]interface{}{
						"schematic": map[string]interface{}{
							"cue": map[string]interface{}{
								"template": "parameter: {}",
							},
						},
					},
				},
			},
			wantCap: types.Capability{
				Name: "my-step",
				Type: types.TypeWorkflowStep,
			},
			wantErr: false,
		},
		{
			name: "unknown kind",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "UnknownKind",
					"metadata": map[string]interface{}{
						"name": "my-unknown",
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "unknown definition Type UnknownKind",
		},
		{
			name: "malformed spec",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1beta1",
					"kind":       "TraitDefinition",
					"metadata": map[string]interface{}{
						"name": "my-trait",
					},
					"spec": "this-should-be-a-map",
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// The mapper is nil for these cases as they don't rely on it.
			// A separate test would be needed for the mapper-dependent path.
			cap, err := ParseCapabilityFromUnstructured(nil, tc.obj)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrMsg != "" {
					require.Contains(t, err.Error(), tc.wantErrMsg)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantCap.Name, cap.Name)
			require.Equal(t, tc.wantCap.Type, cap.Type)
			require.Equal(t, tc.wantCap.AppliesTo, cap.AppliesTo)
		})
	}
}
