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

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetType(t *testing.T) {
	svc1 := Service{}
	got := svc1.GetType()
	assert.Equal(t, DefaultWorkloadType, got)

	var workload2 = "W2"
	map2 := map[string]interface{}{
		"type": workload2,
		"cpu":  "0.5",
	}
	svc2 := Service(map2)
	got = svc2.GetType()
	assert.Equal(t, workload2, got)
}

func TestGetUserConfigName(t *testing.T) {
	t.Run("config name exists", func(t *testing.T) {
		svc := Service{"config": "my-config"}
		assert.Equal(t, "my-config", svc.GetUserConfigName())
	})

	t.Run("config name does not exist", func(t *testing.T) {
		svc := Service{"image": "nginx"}
		assert.Equal(t, "", svc.GetUserConfigName())
	})
}

func TestGetApplicationConfig(t *testing.T) {
	svc := Service{
		"image":  "nginx",
		"port":   80,
		"type":   "webservice",
		"build":  "./",
		"config": "my-config",
	}

	config := svc.GetApplicationConfig()

	assert.Contains(t, config, "image")
	assert.Contains(t, config, "port")
	assert.NotContains(t, config, "type")
	assert.NotContains(t, config, "build")
	assert.NotContains(t, config, "config")
	assert.Len(t, config, 2)
}

func TestToStringSlice(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "string",
			input:    "one",
			expected: []string{"one"},
		},
		{
			name:     "[]string",
			input:    []string{"one", "two"},
			expected: []string{"one", "two"},
		},
		{
			name:     "[]interface{} of strings",
			input:    []interface{}{"one", "two"},
			expected: []string{"one", "two"},
		},
		{
			name:     "[]interface{} of mixed types",
			input:    []interface{}{"one", 2, "three"},
			expected: []string{"one", "three"},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty []string",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "other type (int)",
			input:    123,
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := toStringSlice(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
