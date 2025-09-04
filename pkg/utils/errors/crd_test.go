/*
Copyright 2020-2022 The KubeVela Authors.

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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsCRDNotExists(t *testing.T) {
	noKindMatchErr := &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "testgroup", Kind: "testkind"},
	}
	wrappedErr := errors.Wrap(noKindMatchErr, "wrapped")
	otherErr := fmt.Errorf("some other error")

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "is a NoKindMatchError",
			err:      noKindMatchErr,
			expected: true,
		},
		{
			name:     "is a wrapped NoKindMatchError",
			err:      wrappedErr,
			expected: true,
		},
		{
			name:     "is another error",
			err:      otherErr,
			expected: false,
		},
		{
			name:     "is nil",
			err:      nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsCRDNotExists(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
