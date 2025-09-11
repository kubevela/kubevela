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

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrictUnmarshal(t *testing.T) {
	t.Parallel()

	type sampleStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	testCases := []struct {
		name       string
		json       string
		dest       interface{}
		expectErr  bool
		assertFunc func(*testing.T, interface{})
	}{
		{
			name:      "valid json",
			json:      `{"name": "test", "age": 10}`,
			dest:      &sampleStruct{},
			expectErr: false,
			assertFunc: func(t *testing.T, dest interface{}) {
				s := dest.(*sampleStruct)
				require.Equal(t, "test", s.Name)
				require.Equal(t, 10, s.Age)
			},
		},
		{
			name:      "unknown field",
			json:      `{"name": "test", "age": 10, "extra": "field"}`,
			dest:      &sampleStruct{},
			expectErr: true,
		},
		{
			name:      "invalid json",
			json:      `{"name": "test", "age": 10,}`,
			dest:      &sampleStruct{},
			expectErr: true,
		},
		{
			name:      "empty json",
			json:      ``,
			dest:      &sampleStruct{},
			expectErr: true, // EOF
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := StrictUnmarshal([]byte(tc.json), tc.dest)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.assertFunc != nil {
					tc.assertFunc(t, tc.dest)
				}
			}
		})
	}
}
