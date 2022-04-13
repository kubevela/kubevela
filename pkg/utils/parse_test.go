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

func TestByteCountIEC(t *testing.T) {
	testCases := map[string]struct {
		Input  int64
		Output string
	}{
		"1 B": {
			Input:  int64(1),
			Output: "1 B",
		},
		"1.1 KiB": {
			Input:  int64(1124),
			Output: "1.1 KiB",
		},
		"1.2 MiB": {
			Input:  int64(1258291),
			Output: "1.2 MiB",
		},
		"3.3 GiB": {
			Input:  int64(3543348020),
			Output: "3.3 GiB",
		},
	}
	r := require.New(t)
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r.Equal(tt.Output, ByteCountIEC(tt.Input))
		})
	}
}
