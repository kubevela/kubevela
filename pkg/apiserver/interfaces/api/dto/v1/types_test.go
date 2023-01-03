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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProperties(t *testing.T) {
	r := require.New(t)
	type p struct {
		Properties Properties `json:"properties"`
	}

	testJsonString := `{"properties": "{\"hello\": 1}"}`

	var a p
	if err := json.Unmarshal([]byte(testJsonString), &a); err != nil {
		t.Fatal(err)
	}

	r.Equal(a.Properties["hello"], float64(1))

	testJson := `{"properties": {"hello": 2}}`
	var b p
	if err := json.Unmarshal([]byte(testJson), &b); err != nil {
		t.Fatal(err)
	}

	r.Equal(b.Properties["hello"], float64(2))
}
