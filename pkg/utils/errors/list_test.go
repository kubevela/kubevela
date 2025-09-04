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

func TestErrorList(t *testing.T) {
	t.Run("HasError", func(t *testing.T) {
		var nilList ErrorList
		assert.False(t, nilList.HasError())

		emptyList := ErrorList{}
		assert.False(t, emptyList.HasError())

		listWithErr := ErrorList{fmt.Errorf("err1")}
		assert.True(t, listWithErr.HasError())
	})

	t.Run("Error", func(t *testing.T) {
		var nilList ErrorList
		assert.Equal(t, "", nilList.Error())

		emptyList := ErrorList{}
		assert.Equal(t, "", emptyList.Error())

		listWithOneErr := ErrorList{fmt.Errorf("err1")}
		assert.Equal(t, "Found 1 errors. [(err1)]", listWithOneErr.Error())

		listWithTwoErrs := ErrorList{fmt.Errorf("err1"), fmt.Errorf("err2")}
		assert.Equal(t, "Found 2 errors. [(err1), (err2)]", listWithTwoErrs.Error())
	})
}

func TestAggregateErrors(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")

	testCases := []struct {
		name     string
		errs     []error
		expected error
	}{
		{
			name:     "multiple non-nil errors",
			errs:     []error{err1, err2},
			expected: ErrorList{err1, err2},
		},
		{
			name:     "some nil errors",
			errs:     []error{nil, err1, nil, err2, nil},
			expected: ErrorList{err1, err2},
		},
		{
			name:     "only nil errors",
			errs:     []error{nil, nil, nil},
			expected: nil,
		},
		{
			name:     "empty slice",
			errs:     []error{},
			expected: nil,
		},
		{
			name:     "nil slice",
			errs:     nil,
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AggregateErrors(tc.errs)
			assert.Equal(t, tc.expected, result)
		})
	}
}
