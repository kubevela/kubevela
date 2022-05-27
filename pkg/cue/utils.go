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

package cue

import (
	"bytes"

	"cuelang.org/go/pkg/encoding/json"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

// int data can evaluate with number in CUE, so it's OK if we convert the original float type data to int
func isIntegral(val float64) bool {
	return val == float64(int(val))
}

// IntifyValues will make values to int.
// JSON marshalling of user values will put integer into float,
// we have to change it back so that CUE check will succeed.
func IntifyValues(raw interface{}) interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		return intifyMap(v)
	case []interface{}:
		return intifyList(v)
	case float64:
		if isIntegral(v) {
			return int(v)
		}
		return v
	default:
		return raw
	}
}

func intifyList(l []interface{}) interface{} {
	l2 := make([]interface{}, 0, len(l))
	for _, v := range l {
		l2 = append(l2, IntifyValues(v))
	}
	return l2
}

func intifyMap(m map[string]interface{}) interface{} {
	m2 := make(map[string]interface{}, len(m))
	for k, v := range m {
		m2[k] = IntifyValues(v)
	}
	return m2
}

// FillUnstructuredObject fill runtime.Unstructured to *value.Value
func FillUnstructuredObject(v *value.Value, obj runtime.Unstructured, paths ...string) error {
	var buf bytes.Buffer
	if err := unstructured.UnstructuredJSONScheme.Encode(obj, &buf); err != nil {
		return v.FillObject(err.Error(), "err")
	}
	expr, err := json.Unmarshal(buf.Bytes())
	if err != nil {
		return v.FillObject(err.Error(), "err")
	}
	return v.FillObject(expr, paths...)
}
