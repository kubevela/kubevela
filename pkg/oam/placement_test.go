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

package oam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePlacement(t *testing.T) {
	require.NoError(t, ValidatePlacement(PlacementHub))
	require.NoError(t, ValidatePlacement(PlacementRuntime))

	err := ValidatePlacement("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
	assert.Contains(t, err.Error(), PlacementHub)
	assert.Contains(t, err.Error(), PlacementRuntime)

	for _, bad := range []string{"spoke", "Hub", " hub", "runtime "} {
		err := ValidatePlacement(bad)
		require.Error(t, err, "%q", bad)
		assert.Contains(t, err.Error(), "invalid")
	}
}
