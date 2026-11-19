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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
)

func TestUseUpgrades(t *testing.T) {
	suites, err := Load("testdata/upgrade")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	c := suites[0].Cases[0]

	restore := UseUpgrades(false)
	t.Cleanup(restore)
	failures := c.Run()
	restore()
	require.Len(t, failures, 1, "a definition is tested as written")
	require.Contains(t, failures[0], "Addition of lists is superseded by list.Concat")

	restore = UseUpgrades(true)
	t.Cleanup(restore)
	failures = c.Run()
	restore()
	require.Empty(t, failures, "the upgrader rewrites list addition")

	restore = UseUpgrades(false)
	t.Cleanup(restore)
	require.NotEmpty(t, c.Run(), "switching back is not masked by a cached upgrade")
	restore()
}

func TestUseUpgradesRestores(t *testing.T) {
	before := []bool{*upgrade.EnableCUEVersionCompatibility, upgrade.EnableListConcatUpgrade, upgrade.EnableGenericDefaultGuardUpgrade}
	restore := UseUpgrades(true)
	t.Cleanup(restore)
	require.True(t, upgrade.EnableGenericDefaultGuardUpgrade, "--upgrade turns on every pass, not just the controller's defaults")
	restore()
	require.Equal(t, before, []bool{*upgrade.EnableCUEVersionCompatibility, upgrade.EnableListConcatUpgrade, upgrade.EnableGenericDefaultGuardUpgrade})
}

func TestEvaluateUpgradeMarkers(t *testing.T) {
	suites, err := Load("testdata/upgrade-marked")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	cases := map[string]*Case{}
	for _, c := range suites[0].Cases {
		cases[c.Name] = c
	}
	for name, tc := range map[string]struct {
		opts     RunOptions
		upgraded bool
		failure  string
	}{
		"needs the upgrader":        {upgraded: true},
		"broken even when upgraded": {failure: "output.spec.template.spec.containers[0].args: expected 1 elements, got 2"},
		"fixed but still marked":    {failure: "passes as written: remove @upgrade"},
		"unmarked and clean":        {},
	} {
		t.Run(name, func(t *testing.T) {
			out := cases[name].Evaluate(tc.opts)
			require.Equal(t, tc.upgraded, out.Upgraded)
			if tc.failure == "" {
				require.Empty(t, out.Failures)
			} else {
				require.Contains(t, out.Failures, tc.failure)
			}
		})
	}

	out := cases["needs the upgrader"].Evaluate(RunOptions{FailOnUpgrade: true})
	require.False(t, out.Upgraded)
	require.NotEmpty(t, out.Failures, "--fail-on-upgrade treats a marked case as unmarked")

	out = cases["fixed but still marked"].Evaluate(RunOptions{Upgrades: true})
	require.Empty(t, out.Failures, "under --upgrade a marker is not checked")
}

func TestUpgradeMarker(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", `import "vela/test"

@upgrade(reason="whole file")

"bare":  test.#ComponentRender & {definition: "d", expect: {}} @upgrade(reason="list addition")
"named": test.#ComponentRender & {definition: "d", expect: {}} @upgrade(generic-default-guard, bool-default-guard, reason="guards")
"file":  test.#ComponentRender & {definition: "d", expect: {}}
`)
	write(t, dir, "b_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", expect: {}} @upgrade(generic-default-gaurd)")
	suites, err := Load(dir)
	require.NoError(t, err)
	got := map[string]UpgradeMarker{}
	for _, c := range suites[0].Cases {
		require.NotNil(t, c.Upgrade, c.Name)
		require.Contains(t, c.Labels, "upgrade")
		got[c.Name] = *c.Upgrade
	}
	require.Equal(t, map[string]UpgradeMarker{
		"bare":  {Reason: "list addition"},
		"named": {Passes: []string{"generic-default-guard", "bool-default-guard"}, Reason: "guards"},
		"file":  {Reason: "whole file"},
	}, got)
	require.ErrorContains(t, suites[1].Err, `unknown upgrade pass "generic-default-gaurd"`)
}
