/*
Copyright 2026 The KubeVela Authors.

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

import "testing"

func TestIsSkipLastAppliedConfig(t *testing.T) {
	cases := map[string]struct {
		value string
		want  bool
	}{
		"dash is the sentinel":         {value: "-", want: true},
		"skip is the sentinel":         {value: "skip", want: true},
		"empty is not":                 {value: "", want: false},
		"a recorded config is not":     {value: `{"apiVersion":"v1","kind":"ConfigMap"}`, want: false},
		"case matters":                 {value: "SKIP", want: false},
		"leading space is not trimmed": {value: " skip", want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSkipLastAppliedConfig(tc.value); got != tc.want {
				t.Errorf("IsSkipLastAppliedConfig(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
