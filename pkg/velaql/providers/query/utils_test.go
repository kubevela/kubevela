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

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubevela/workflow/pkg/cue/model/value"
)

func TestFillQueryResult(t *testing.T) {
	testcases := map[string]struct {
		queryRes interface{}
		json     string
	}{
		"test fill query result which contains *unstructured.Unstructured": {
			queryRes: []Resource{
				{
					Cluster:   "local",
					Component: "web",
					Revision:  "v1",
					Object: &unstructured.Unstructured{
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
				},
				{
					Cluster:   "ap-southeast-1",
					Component: "web",
					Revision:  "v2",
					Object: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"creationTimestamp": "2022-05-25T12:07:02Z",
							},
						},
					},
				},
			},
			json: `{"list":[{"cluster":"local","component":"web","revision":"v1","object":{"apiVersion":"apps/v1","kind":"Deployment","spec":{"template":{"metadata":{"creationTimestamp":null}}}}},{"cluster":"ap-southeast-1","component":"web","revision":"v2","object":{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"creationTimestamp":"2022-05-25T12:07:02Z"}}}]}`,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			value, err := value.NewValue("", nil, "")
			assert.NoError(t, err)
			err = fillQueryResult(value, testcase.queryRes, "list")
			assert.NoError(t, err)
			json, err := value.CueValue().MarshalJSON()
			assert.NoError(t, err)
			assert.Equal(t, testcase.json, string(json))
		})
	}
}
