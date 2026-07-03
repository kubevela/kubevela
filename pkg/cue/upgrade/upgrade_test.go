/*
Copyright 2024 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Tests in this file cover KubeVela-specific wiring only:
//   - velaversion provider (GetCurrentVersion hook)
//   - EnableCUEVersionCompatibility local var syncing
//   - Prometheus metrics callbacks (OnRewrite, OnUpgradeDuration)
//
// Engine behaviour (list arithmetic, error field, cache) is tested in
// github.com/kubevela/pkg/cue/upgrade.
package upgrade_test

import (
	"context"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	pkgupgrade "github.com/kubevela/pkg/cue/upgrade"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
	velaversion "github.com/oam-dev/kubevela/version"
)

// TestVelaVersionProviderWiring verifies that GetCurrentVersion reads from velaversion.VelaVersion.
func TestVelaVersionProviderWiring(t *testing.T) {
	original := velaversion.VelaVersion
	defer func() { velaversion.VelaVersion = original }()

	cases := []struct {
		set     string
		wantErr bool
		wantVer string
	}{
		{"v1.11.2", false, "1.11"},
		{"1.12.0", false, "1.12"},
		{"v1.13.0-alpha.1+dev", false, "1.13"},
		{"UNKNOWN", false, ""}, // falls back to latest — just check no error
		{"", false, ""},        // same
		{"invalid-version", true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.set, func(t *testing.T) {
			velaversion.VelaVersion = tc.set
			got := pkgupgrade.GetCurrentVersion()
			if tc.wantErr {
				// Verify the engine propagates the error — call Upgrade without explicit version.
				_, err := upgrade.Upgrade("x: 1")
				if err == nil {
					t.Error("expected error from invalid version string, got nil")
				}
				return
			}
			if tc.wantVer != "" {
				v, err := upgrade.ParseVersion(got)
				if err != nil {
					t.Fatalf("ParseVersion(%q): %v", got, err)
				}
				want, _ := upgrade.ParseVersion(tc.wantVer)
				if v != want {
					t.Errorf("got version %v, want %v", v, want)
				}
			}
		})
	}
}

// TestEnableCUEVersionCompatibilitySyncs verifies that setting the local var to false
// causes EnsureCueVersionCompatibility to return the input unchanged.
func TestEnableCUEVersionCompatibilitySyncs(t *testing.T) {
	original := *upgrade.EnableCUEVersionCompatibility
	defer func() { *upgrade.EnableCUEVersionCompatibility = original }()
	*upgrade.EnableCUEVersionCompatibility = false

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	got, _ := upgrade.EnsureCueVersionCompatibility(input, "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
	if got != input {
		t.Errorf("expected input unchanged when disabled, got %q", got)
	}
}

// TestUpgradeWithUnknownVelaVersion verifies that UNKNOWN version falls back to latest
// and still applies all upgrades.
func TestUpgradeWithUnknownVelaVersion(t *testing.T) {
	original := velaversion.VelaVersion
	defer func() { velaversion.VelaVersion = original }()
	velaversion.VelaVersion = "UNKNOWN"

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	result, err := upgrade.Upgrade(input)
	if err != nil {
		t.Errorf("Upgrade() should not error on UNKNOWN version, got: %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Errorf("Upgrade() with UNKNOWN version should apply all upgrades, got: %v", result)
	}
}

// TestMetricsCallbackFired verifies that the OnRewrite Prometheus callback increments
// CUECompatRewriteTotal when a legacy template is upgraded.
func TestMetricsCallbackFired(t *testing.T) {
	// Flush the cache so the upgrade path actually runs.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pkgupgrade.InitCompatibilityCache(ctx, 512)
	upgrade.CUECompatRewriteTotal.Reset()

	input := `
list1: [1, 2, 3]
list2: [4, 5, 6]
combined: list1 + list2
`
	upgrade.EnsureCueVersionCompatibility(input, "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)

	mf, err := upgrade.CUECompatRewriteTotal.GetMetricWithLabelValues(
		"list-arithmetic", "1.11", string(upgrade.ComponentKind), string(upgrade.TemplateAreaMain),
	)
	if err != nil {
		t.Fatalf("failed to get metric: %v", err)
	}
	m := &dto.Metric{}
	if err := mf.Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Counter == nil || m.Counter.GetValue() < 1 {
		t.Errorf("expected counter >= 1 for list-arithmetic/1.11/Component, got %v", m)
	}
}
