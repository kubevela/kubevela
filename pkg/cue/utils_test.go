/*
 Copyright 2022. The KubeVela Authors.

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

package cue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestFillUnstructuredObject(t *testing.T) {
	testcases := map[string]struct {
		obj  *unstructured.Unstructured
		json string
	}{
		"test unstructured object with nil value": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"creationTimestamp": nil,
							},
						},
					},
				},
			},
			json: `{"object":{"apiVersion":"apps/v1","kind":"Deployment","spec":{"template":{"metadata":{"creationTimestamp":null}}}}}`,
		},
		"test unstructured object without nil value": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"creationTimestamp": "2022-05-25T12:07:02Z",
					},
				},
			},
			json: `{"object":{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"creationTimestamp":"2022-05-25T12:07:02Z"}}}`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			value, err := value.NewValue("", nil, "")
			assert.NoError(t, err)
			err = FillUnstructuredObject(value, testcase.obj, "object")
			assert.NoError(t, err)
			json, err := value.CueValue().MarshalJSON()
			assert.NoError(t, err)
			assert.Equal(t, testcase.json, string(json))
		})
	}
}
