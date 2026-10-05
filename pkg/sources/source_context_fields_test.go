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

package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextFieldsFollowTheConsumingSurface(t *testing.T) {
	component, err := ContextFields(SurfaceComponent)
	require.NoError(t, err)
	require.Contains(t, component, "name")
	require.Contains(t, component, "namespace")
	require.Contains(t, component, "componentName")
	require.NotContains(t, component, "stepName")

	step, err := ContextFields("workflowstep")
	require.NoError(t, err)
	require.Contains(t, step, "stepName")
	require.NotContains(t, step, "componentName")

	_, err = ContextFields("nonsense")
	require.Error(t, err)
}
