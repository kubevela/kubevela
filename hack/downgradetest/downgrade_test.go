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

// Package downgradetest runs hack/downgrade-from-1.12.sh against stub helm and kubectl commands.
package downgradetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubHelm answers the helm calls the script makes in a dry run. `helm show chart` reports the version
// asked for, or STUB_CHART_VERSION when set, as Helm does for a local chart whatever --version says.
// `helm template` renders one builtin definition under the chart layout named by STUB_LAYOUT: 1.11.0 kept
// them in templates/defwithtemplate, 1.11.1 and later ship them as ConfigMaps a hook Job applies
// (templates/builtin-definitions).
const stubHelm = `#!/usr/bin/env bash
case "$1" in
show)
	version="$STUB_CHART_VERSION"
	while [ $# -gt 0 ]; do [ "$1" = "--version" ] && [ -z "$version" ] && version="$2"; shift; done
	echo "name: vela-core"
	echo "version: $version"
	;;
history) echo '[{"chart":"vela-core-1.12.0-alpha.2","status":"deployed"}]' ;;
version) echo "v3.14.4+gabcdef" ;;
get) echo "{}" ;;
template)
	while [ $# -gt 0 ]; do [ "$1" = "--output-dir" ] && out="$2"; shift; done
	mkdir -p "$out/vela-core/templates"
	[ -n "$STUB_LAYOUT" ] || exit 0
	mkdir -p "$out/vela-core/templates/$STUB_LAYOUT"
	echo "kind: ComponentDefinition" >"$out/vela-core/templates/$STUB_LAYOUT/webservice.yaml"
	;;
*) exit 0 ;;
esac
`

// stubKubectl reports one existing builtin and no webhook configurations.
const stubKubectl = `#!/usr/bin/env bash
case "$*" in
*"--dry-run=client"*) echo "componentdefinition.core.oam.dev/webservice" ;;
"get validatingwebhookconfigurations -o name") ;;
*) exit 0 ;;
esac
`

func runDowngrade(t *testing.T, layout string, args ...string) (string, error) {
	t.Helper()
	return runDowngradeWithChart(t, layout, "", args...)
}

func runDowngradeWithChart(t *testing.T, layout, chartVersion string, args ...string) (string, error) {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(here), "..", "downgrade-from-1.12.sh")
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "helm"), []byte(stubHelm), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "kubectl"), []byte(stubKubectl), 0o755))
	cmd := exec.Command("bash", append([]string{script, "--dry-run"}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "STUB_LAYOUT="+layout, "STUB_CHART_VERSION="+chartVersion)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestDowngradeToHookJobChart(t *testing.T) {
	// 1.11.1 and 1.11.2 apply their builtins from a post-upgrade hook Job, as 1.12 does, so the
	// downgrade is the upgrade itself: nothing is handed to Helm first.
	for _, version := range []string{"1.11.1", "1.11.2"} {
		t.Run(version, func(t *testing.T) {
			out, err := runDowngrade(t, "builtin-definitions", "--version", version)
			require.NoError(t, err, out)
			require.Contains(t, out, "would: helm upgrade kubevela kubevela/vela-core --version "+version)
			require.NotContains(t, out, "would: kubectl label", "a hook Job chart does not own its builtins: nothing to adopt")
			require.NotContains(t, out, "would: kubectl apply", "the chart's own Job re-applies the builtins")
		})
	}
}

func TestDowngradeDefaultsToHookJobChart(t *testing.T) {
	out, err := runDowngrade(t, "builtin-definitions")
	require.NoError(t, err, out)
	require.Contains(t, out, "--version 1.11.2")
}

func TestDowngradeToReleaseOwnedBuiltins(t *testing.T) {
	// 1.11.0 renders its builtins into the release, so they are re-applied and handed to Helm first.
	out, err := runDowngrade(t, "defwithtemplate", "--version", "1.11.0")
	require.NoError(t, err, out)
	require.Contains(t, out, "would: kubectl apply --server-side --force-conflicts --field-manager=helm")
	require.Contains(t, out, "would: kubectl label componentdefinition.core.oam.dev/webservice")
	upgrade := strings.Index(out, "would: helm upgrade")
	require.Greater(t, upgrade, strings.Index(out, "would: kubectl label"), "the builtins are adopted before the upgrade")
}

func TestDowngradeRefusesChartWithoutBuiltins(t *testing.T) {
	out, err := runDowngrade(t, "", "--version", "1.11.2")
	require.Error(t, err)
	require.Contains(t, out, "rendered no builtin definitions")
}

func TestDowngradeRefusesChartOfAnotherVersion(t *testing.T) {
	// Helm ignores --version for a local chart path, so a 1.12 checkout would be "downgraded" to itself.
	out, err := runDowngradeWithChart(t, "builtin-definitions", "1.12.0", "--chart", "./charts/vela-core", "--version", "1.11.2")
	require.Error(t, err)
	require.Contains(t, out, "is version 1.12.0, not 1.11.2")
	require.NotContains(t, out, "would: helm upgrade")
}

func TestDowngradeAcceptsLocalChartOfTheVersion(t *testing.T) {
	out, err := runDowngradeWithChart(t, "builtin-definitions", "1.11.2", "--chart", "./vela-core-1.11.2", "--version", "1.11.2")
	require.NoError(t, err, out)
	require.Contains(t, out, "would: helm upgrade kubevela ./vela-core-1.11.2 --version 1.11.2")
}
