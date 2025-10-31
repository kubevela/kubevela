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

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeFormat(t *testing.T) {
	testCases := []struct {
		name     string
		in       string
		expected string
	}{
		{
			name:     "1.5h",
			in:       "1.5h",
			expected: "1h30m0s",
		},
		{
			name:     "25h",
			in:       "25h",
			expected: "1d1h0m0s",
		},
		{
			name:     "0.1h",
			in:       "0.1h",
			expected: "6m0s",
		},
		{
			name:     "0.001h",
			in:       "0.001h",
			expected: "3s",
		},
		{
			name:     "0.00001h",
			in:       "0.00001h",
			expected: "36ms",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := time.ParseDuration(tc.in)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, TimeFormat(d))
		})
	}
}
