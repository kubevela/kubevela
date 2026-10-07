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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderAddonCases(t *testing.T) {
	suites, err := Load("testdata/addons/shop/shop_test.cue")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	for _, c := range suites[0].Cases {
		require.Empty(t, c.Run(), c.Name)
		require.Equal(t, "shop", c.Subject.Name)
		require.Contains(t, c.Labels, "addon")
	}
}

// A definition an addon ships is tested beside it, like any other, and the
// test is not part of the addon.
func TestAddonShippedDefinition(t *testing.T) {
	suites, err := Load("testdata/addons/shop")
	require.NoError(t, err)
	require.Len(t, suites, 2)
	for _, s := range suites {
		require.NoError(t, s.Err, s.File)
		for _, c := range s.Cases {
			require.Empty(t, c.Run(), c.Name)
		}
	}
	rendered, err := RenderAddon("testdata/addons/shop", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"shop-web"}, sortedKeys(rendered.Definitions["ComponentDefinition"]), "the test beside the definition is not installed with it")
}

func TestRenderAddonMissing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#AddonRender & {addon: \"nope\", expect: {}}")
	suites, err := Load(dir)
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, `addon "nope": no metadata.yaml in`)
}

// An addon vela addon enable refuses is refused here too.
func TestRenderAddonValidatesAsEnableDoes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	require.NoError(t, os.CopyFS(dir, os.DirFS("testdata/addons/shop")))
	meta, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	require.NoError(t, err)
	var kept []string
	for _, line := range strings.Split(string(meta), "\n") {
		if !strings.HasPrefix(line, "version:") {
			kept = append(kept, line)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(strings.Join(kept, "\n")), 0o600))
	_, err = RenderAddon(dir, nil)
	require.ErrorContains(t, err, "must define the version of addon")
}

// An @upgrade addon case is labelled upgrade, as every other kind's is.
func TestAddonUpgradeCaseIsLabelled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(dir, "shop"), os.DirFS("testdata/addons/shop")))
	file := filepath.Join(dir, "shop_test.cue")
	require.NoError(t, os.WriteFile(file, []byte(`import "vela/test"

"renders": test.#AddonRender & {addon: "shop"} @upgrade(reason="waiting on the upgrader")
`), 0o600))
	s := loadOne(t, file)
	require.Contains(t, s.Cases[0].Labels, "upgrade")
}

// Addon templates compile without providers, so a mock could answer nothing.
func TestAddonCaseRefusesMocks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(dir, "shop"), os.DirFS("testdata/addons/shop")))
	file := filepath.Join(dir, "shop_test.cue")
	require.NoError(t, os.WriteFile(file, []byte(`import "vela/test"

"renders": test.#AddonRender & {
	addon: "shop"
	mocks: "vela/kube": "#Get": $returns: {}
}
`), 0o600))
	suites, err := Load(file)
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, "addon templates compile without providers, so there is nothing to mock")
}
