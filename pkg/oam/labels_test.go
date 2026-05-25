/*
Copyright 2024 The KubeVela Authors.

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

package oam

import (
	"testing"
)

func TestFilterInternalMetadata(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map returns nil",
			input:    map[string]string{},
			expected: nil,
		},
		{
			name: "all internal keys returns nil",
			input: map[string]string{
				"app.oam.dev/revision":     "filter",
				"kubernetes.io/managed-by": "filter",
			},
			expected: nil,
		},
		{
			name: "keys without prefixes are kept",
			input: map[string]string{
				"simple-key": "keep",
				"another":    "keep",
			},
			expected: map[string]string{
				"simple-key": "keep",
				"another":    "keep",
			},
		},
		{
			name: "filters all seven internal prefixes",
			input: map[string]string{
				"app.oam.dev/name":                   "filter",
				"oam.dev/tracker":                    "filter",
				"kubectl.kubernetes.io/last-applied": "filter",
				"kubernetes.io/service-account":      "filter",
				"k8s.io/cluster-service":             "filter",
				"helm.sh/chart":                      "filter",
				"app.kubernetes.io/managed-by":       "filter",
				"user.custom/annotation":             "keep",
			},
			expected: map[string]string{
				"user.custom/annotation": "keep",
			},
		},
		{
			name: "mixed keys keeps only external",
			input: map[string]string{
				"user.custom/annotation":   "keep",
				"my-label":                 "keep",
				"team":                     "platform",
				"custom.guidewire.dev/foo": "keep",
				"app.oam.dev/revision":     "filter",
				"helm.sh/chart":            "filter",
			},
			expected: map[string]string{
				"user.custom/annotation":   "keep",
				"my-label":                 "keep",
				"team":                     "platform",
				"custom.guidewire.dev/foo": "keep",
			},
		},
		{
			name: "external prefix not in internal list is kept",
			input: map[string]string{
				"custom.guidewire.dev/foo": "keep",
				"app.oam.dev/name":         "filter",
			},
			expected: map[string]string{
				"custom.guidewire.dev/foo": "keep",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterInternalMetadata(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d", len(tt.expected), len(got))
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("key %q: expected %q, got %q", k, v, got[k])
				}
			}
		})
	}
}
