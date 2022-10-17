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

package writer

import (
	"testing"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/stretchr/testify/require"
)

func TestConvertMap2KV(t *testing.T) {
	r := require.New(t)
	re := map[string]string{}
	err := convertMap2PropertiesKV("", map[string]interface{}{
		"s":  "s",
		"n":  1,
		"nn": 1.5,
		"b":  true,
		"m": map[string]interface{}{
			"s": "s",
			"b": false,
		},
		"aa": []string{"a", "a"},
		"ai": []int64{1, 2},
		"ar": []map[string]interface{}{{
			"s2": "s2",
		}},
	}, re)
	r.Equal(err, nil)
	r.Equal(re, map[string]string{
		"s":       "s",
		"n":       "1",
		"nn":      "1.5",
		"b":       "true",
		"m.s":     "s",
		"m.b":     "false",
		"aa":      "a,a",
		"ai":      "1,2",
		"ar.0.s2": "s2",
	})
}

func TestEncodingOutput(t *testing.T) {
	r := require.New(t)
	testValue := `
		context: {
			key1: "hello"
			key2: 2
			key3: true
			key4: 4.4
			key5: ["hello"]
			key6: [{"hello": 1}]
			key7: [1, 2]
			key8: [1.2, 1]
			key9: {key10: [{"wang": true}]}
		}
	`
	v, err := value.NewValue(testValue, nil, "")
	r.Equal(err, nil)

	_, err = encodingOutput(v, "yaml")
	r.Equal(err, nil)

	_, err = encodingOutput(v, "properties")
	r.Equal(err, nil)

	_, err = encodingOutput(v, "toml")
	r.Equal(err, nil)

	json, err := encodingOutput(v, "json")
	r.Equal(err, nil)
	r.Equal(string(json), `{"context":{"key1":"hello","key2":2,"key3":true,"key4":4.4,"key5":["hello"],"key6":[{"hello":1}],"key7":[1,2],"key8":[1.2,1],"key9":{"key10":[{"wang":true}]}}}`)
}
