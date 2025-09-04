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

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLabelConflict(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "error contains LabelConflict",
			err:      fmt.Errorf("this is a LabelConflict error"),
			expected: true,
		},
		{
			name:     "error is exactly LabelConflict",
			err:      fmt.Errorf(LabelConflict),
			expected: true,
		},
		{
			name:     "error does not contain LabelConflict",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
		{
			name:     "error is nil",
			err:      nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsLabelConflict(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsCuePathNotFound(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "error contains both substrings",
			err:      fmt.Errorf("failed to lookup value: the path a.b.c does not exist"),
			expected: true,
		},
		{
			name:     "error contains only first substring",
			err:      fmt.Errorf("failed to lookup value"),
			expected: false,
		},
		{
			name:     "error contains only second substring",
			err:      fmt.Errorf("the path does not exist"),
			expected: false,
		},
		{
			name:     "error contains neither substring",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsCuePathNotFound(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
