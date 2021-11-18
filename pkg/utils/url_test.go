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

func TestParseEndpoint(t *testing.T) {
	testCases := []struct {
		Input    string
		Output   string
		HasError bool
	}{{
		Input:  "127.0.0.1",
		Output: "https://127.0.0.1:443",
	}, {
		Input:  "http://127.0.0.1",
		Output: "http://127.0.0.1:80",
	}, {
		Input:  "127.0.0.1:6443",
		Output: "https://127.0.0.1:6443",
	}, {
		Input:  "127.0.0.1:80",
		Output: "http://127.0.0.1:80",
	}, {
		Input:  "localhost",
		Output: "https://localhost:443",
	}, {
		Input:  "https://worker-control-plane:6443",
		Output: "https://worker-control-plane:6443",
	}, {
		Input:    "invalid url",
		HasError: true,
	}}
	r := require.New(t)
	for _, testCase := range testCases {
		output, err := ParseAPIServerEndpoint(testCase.Input)
		if testCase.HasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		r.Equal(testCase.Output, output)
	}
}
