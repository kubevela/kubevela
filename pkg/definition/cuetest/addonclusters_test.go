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

package cuetest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderAddonRuntimeClusters(t *testing.T) {
	s := loadOne(t, "testdata/addons/fleet/fleet_test.cue")
	require.Len(t, s.Cases, 3)
	for _, c := range s.Cases {
		require.Empty(t, c.Run(), c.Name)
	}
}

func TestRenderAddonDefinitionNameClash(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fleet")
	require.NoError(t, os.CopyFS(dir, os.DirFS("testdata/addons/fleet")))
	trait, err := os.ReadFile(filepath.Join(dir, "definitions", "agent-component.cue"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "definitions", "agent-copy.cue"), trait, 0o600))
	_, err = RenderAddon(dir, nil)
	require.ErrorContains(t, err, `two ComponentDefinitions named "fleet-agent"`)
}
