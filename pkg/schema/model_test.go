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

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateEveryPattern(t *testing.T) {
	ps := generate(t, `parameter: name: string & =~"^[a-z]+$" & =~"^.{0,5}$" & !~"^x" & !~"y$"`)
	name := ps.OpenAPI.Properties["name"].Value
	for value, ok := range map[string]bool{
		"abc":     true,
		"abcdefg": false,
		"Abc":     false,
		"xab":     false,
		"aby":     false,
	} {
		err := name.VisitJSON(value)
		if ok {
			assert.NoError(t, err, value)
		} else {
			assert.Error(t, err, value)
		}
	}
	assert.Equal(t, `^(?=[\s\S]*(?:^[a-z]+$))(?=[\s\S]*(?:^.{0,5}$))`, uiParam(t, ps.UI, "name").Validate.Pattern)
}

func TestGenerateInt32Bounds(t *testing.T) {
	ps := generate(t, `parameter: {
	plain: int32
	upper: int32 & <=10
	lower: int32 & >=0
}`)
	bounds := func(key string) (*float64, *float64) {
		s := ps.OpenAPI.Properties[key].Value
		return s.Min, s.Max
	}
	lo, hi := bounds("plain")
	assert.Nil(t, lo)
	assert.Nil(t, hi)
	lo, hi = bounds("upper")
	assert.Nil(t, lo)
	require.NotNil(t, hi)
	assert.Equal(t, 10.0, *hi)
	lo, hi = bounds("lower")
	require.NotNil(t, lo)
	assert.Equal(t, 0.0, *lo)
	assert.Nil(t, hi)
}
