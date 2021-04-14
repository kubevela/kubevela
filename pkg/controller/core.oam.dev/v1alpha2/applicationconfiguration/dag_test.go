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

package applicationconfiguration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFillDatainput(t *testing.T) {
	getObj1 := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"a": []interface{}{map[string]interface{}{
					"configMapRef": map[string]interface{}{
						"name":  "my-a",
						"value": "my-a",
					},
				}, map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": "my-c",
					},
				}},
			},
		}}
	}
	obj1 := getObj1()
	obj1Copy := getObj1()
	tests := map[string]struct {
		obj               *unstructured.Unstructured
		fs                []string
		val               interface{}
		strategyMergeKeys []string
		expObj            *unstructured.Unstructured
	}{
		"normal case: use string as element": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": interface{}("a-val"),
				},
			}},
			fs:  []string{"spec.a"},
			val: "a-val-b",
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": interface{}("a-val-b"),
				},
			}},
		},
		"slice case: append with target field not exist": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			}},
			fs:  []string{"spec.a"},
			val: []interface{}{"a-val-b"},
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{"a-val-b"},
				},
			}},
		},
		"slice case: append with no keys": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{"a-val"},
				},
			}},
			fs:  []string{"spec.a"},
			val: []interface{}{"a-val-b"},
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{"a-val", "a-val-b"},
				},
			}},
		},
		"slice case: append with keys match should update": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{map[string]interface{}{
						"name":  "my",
						"value": "a-val",
					}},
				},
			}},
			fs: []string{"spec.a"},
			val: []interface{}{map[string]interface{}{
				"name":  "my",
				"value": "a-val-b",
			}, map[string]interface{}{
				"name":  "my2",
				"value": "a-val-c",
			}},
			strategyMergeKeys: []string{"name"},
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{map[string]interface{}{
						"name":  "my",
						"value": "a-val-b",
					}, map[string]interface{}{
						"name":  "my2",
						"value": "a-val-c",
					}},
				},
			}},
		},
		"slice case: append with complex keys match should update": {
			obj: obj1,
			fs:  []string{"spec.a"},
			val: []interface{}{map[string]interface{}{
				"configMapRef": map[string]interface{}{
					"name":  "my-a",
					"value": "mm-a",
				},
			}, map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name": "my-b",
				},
			}},
			strategyMergeKeys: []string{"configMapRef.name", "secretRef.name"},
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{map[string]interface{}{
						"configMapRef": map[string]interface{}{
							"name":  "my-a",
							"value": "mm-a",
						},
					}, map[string]interface{}{
						"secretRef": map[string]interface{}{
							"name": "my-c",
						},
					}, map[string]interface{}{
						"secretRef": map[string]interface{}{
							"name": "my-b",
						},
					}},
				},
			}},
		},
		"slice case: no key match should just append": {
			obj: obj1Copy,
			fs:  []string{"spec.a"},
			val: []interface{}{map[string]interface{}{
				"configMapRef": map[string]interface{}{
					"name":  "my-a",
					"value": "mm-a",
				},
			}, map[string]interface{}{
				"secretRef": map[string]interface{}{
					"name": "my-b",
				},
			}},
			strategyMergeKeys: []string{"configMapRef.xx", "secretRef.yy"},
			expObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"a": []interface{}{map[string]interface{}{
						"configMapRef": map[string]interface{}{
							"name":  "my-a",
							"value": "my-a",
						},
					}, map[string]interface{}{
						"secretRef": map[string]interface{}{
							"name": "my-c",
						},
					}, map[string]interface{}{
						"configMapRef": map[string]interface{}{
							"name":  "my-a",
							"value": "mm-a",
						},
					}, map[string]interface{}{
						"secretRef": map[string]interface{}{
							"name": "my-b",
						},
					}},
				},
			}},
		},
	}
	for message, ti := range tests {
		err := fillDataInputValue(ti.obj, ti.fs, ti.val, ti.strategyMergeKeys)
		assert.NoError(t, err, message)
		assert.Equal(t, ti.expObj, ti.obj, message)
	}
}
