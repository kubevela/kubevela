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

package writer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertMap2KV(t *testing.T) {
	r := require.New(t)
	re := map[string]string{}
	err := convertMap2KV("", map[string]interface{}{
		"s":  "s",
		"n":  1,
		"nn": 1.5,
		"b":  true,
		"m": map[string]interface{}{
			"s": "s",
			"b": false,
		},
	}, re)
	r.Equal(err, nil)
	r.Equal(re, map[string]string{
		"s":   "s",
		"n":   "1",
		"nn":  "1.5",
		"b":   "true",
		"m.s": "s",
		"m.b": "false",
	})
}
