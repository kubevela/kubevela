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

package common

import (
	"encoding/json"
	"reflect"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestAddSourceIntoDefinition(t *testing.T) {
	caseJson := []byte(`{"template":""}`)
	wantJson := []byte(`{"source":{"repoName":"foo"},"template":""}`)
	source := types.Source{RepoName: "foo"}
	testcase := runtime.RawExtension{Raw: caseJson}
	err := addSourceIntoExtension(&testcase, &source)
	if err != nil {
		t.Error("meet an error ", err)
		return
	}
	var result, want map[string]interface{}
	err = json.Unmarshal(testcase.Raw, &result)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	err = json.Unmarshal(wantJson, &want)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("error result want %s, got %s", result, testcase)
	}
}

func TestCheckLabelExistence(t *testing.T) {
	cases := map[string]struct {
		labels  map[string]string
		label   string
		existed bool
	}{
		"label exists": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "env=prod",
			existed: true,
		},

		"label's key matches": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "env=dev",
			existed: false,
		},
		"label's key doesn't match": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "type=terraform",
			existed: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := CheckLabelExistence(tc.labels, tc.label)
			assert.Equal(t, result, tc.existed)
		})
	}
}
