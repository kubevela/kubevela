/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

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

func TestVelaVersionProviderWiring(t *testing.T) {
	original := velaversion.VelaVersion
	defer func() { velaversion.VelaVersion = original }()
	velaversion.VelaVersion = "v1.11.2"
	got := pkgupgrade.GetCurrentVersion()
	if got != "v1.11.2" {
		t.Fatalf("GetCurrentVersion()=%q", got)
	}
}

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

func TestPerUpgradeFlagsAffectRewrite(t *testing.T) {
	origList := upgrade.EnableListConcatUpgrade
	origBool := upgrade.EnableBoolDefaultGuardUpgrade
	t.Cleanup(func() {
		upgrade.EnableListConcatUpgrade = origList
		upgrade.EnableBoolDefaultGuardUpgrade = origBool
	})

	listInput := `
list1: [1, 2]
list2: [3, 4]
combined: list1 + list2
`
	upgrade.EnableListConcatUpgrade = false
	got, _ := upgrade.EnsureCueVersionCompatibility(listInput, "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
	if strings.Contains(got, "list.Concat") {
		t.Fatalf("expected list arithmetic unchanged when disabled, got: %s", got)
	}
	upgrade.EnableListConcatUpgrade = true
	got, _ = upgrade.EnsureCueVersionCompatibility(listInput+"\n", "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
	if !strings.Contains(got, "list.Concat") {
		t.Fatalf("expected list.Concat rewrite when enabled, got: %s", got)
	}

	boolInput := `
_flag: bool | *false
if cond {
	_flag: true
}

if !_flag {
	_error: 0 & "required"
}
`
	upgrade.EnableBoolDefaultGuardUpgrade = false
	got, _ = upgrade.EnsureCueVersionCompatibility(boolInput, "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
	if !strings.Contains(got, "bool | *false") {
		t.Fatalf("expected bool default guard unchanged when disabled, got: %s", got)
	}
	upgrade.EnableBoolDefaultGuardUpgrade = true
	got, _ = upgrade.EnsureCueVersionCompatibility(boolInput+"\n", "test-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
	if strings.Contains(got, "bool | *false") {
		t.Fatalf("expected bool default guard rewritten when enabled, got: %s", got)
	}
}

func TestComplexTemplatePerUpgradeFlags(t *testing.T) {
	origList := upgrade.EnableListConcatUpgrade
	origError := upgrade.EnableErrorFieldLabelUpgrade
	origBool := upgrade.EnableBoolDefaultGuardUpgrade
	origGeneric := upgrade.EnableGenericDefaultGuardUpgrade
	origKeep := upgrade.EnableKeepValidatorsSingletonUpgrade
	origEval := upgrade.EnableEvalv3SelfRefGuardUpgrade
	t.Cleanup(func() {
		upgrade.EnableListConcatUpgrade = origList
		upgrade.EnableErrorFieldLabelUpgrade = origError
		upgrade.EnableBoolDefaultGuardUpgrade = origBool
		upgrade.EnableGenericDefaultGuardUpgrade = origGeneric
		upgrade.EnableKeepValidatorsSingletonUpgrade = origKeep
		upgrade.EnableEvalv3SelfRefGuardUpgrade = origEval
	})

	const input = `
import "strings"

left: ["a"]
right: ["b"]
both: left + right
combined: strings.Join(left + right, "-")

error: "legacy error label"

_flag: bool | *false
if cond {
	_flag: true
}
if !_flag {
	_error: 0 & "required"
}

_mode: string | *""
if cond { _mode: "x" }
if _mode == "x" { out: true }

x: >=1 & <=1
y: x + 1

z: *45 | int & {
	if z < 1 { _|_ & {errorMessage: "z must be >= 1"} }
}
`

	tests := []struct {
		name   string
		flags  func()
		expect []string
		avoid  []string
	}{
		{
			name: "defaults-only-list-and-error",
			flags: func() {
				upgrade.EnableListConcatUpgrade = true
				upgrade.EnableErrorFieldLabelUpgrade = true
				upgrade.EnableBoolDefaultGuardUpgrade = false
				upgrade.EnableGenericDefaultGuardUpgrade = false
				upgrade.EnableKeepValidatorsSingletonUpgrade = false
				upgrade.EnableEvalv3SelfRefGuardUpgrade = false
			},
			expect: []string{
				"strings.Join(list.Concat([",
				`"error": "legacy error label"`,
				"_flag: bool | *false",
				"_mode: string | *\"\"",
				"x: >=1 & <=1",
				"z: *45 | int & {",
			},
			avoid: []string{
				"_modeVal:",
				"x: 1",
			},
		},
		{
			name: "all-enabled",
			flags: func() {
				upgrade.EnableListConcatUpgrade = true
				upgrade.EnableErrorFieldLabelUpgrade = true
				upgrade.EnableBoolDefaultGuardUpgrade = true
				upgrade.EnableGenericDefaultGuardUpgrade = true
				upgrade.EnableKeepValidatorsSingletonUpgrade = true
				upgrade.EnableEvalv3SelfRefGuardUpgrade = true
			},
			expect: []string{
				"strings.Join(list.Concat([",
				`"error": "legacy error label"`,
				"_modeVal: string | *\"\"",
				"x: 1",
			},
			avoid: []string{
				"_flag: bool | *false",
				"_isSecondary: bool | *false",
				"_mode: string | *\"\"",
				"x: >=1 & <=1",
				"z: *45 | int & {",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.flags()
			got, ok := upgrade.EnsureCueVersionCompatibility(input+"\n// "+tc.name+"\n", "complex-def", upgrade.ComponentKind, upgrade.TemplateAreaMain)
			if !ok {
				t.Fatalf("expected rewrite to report ok=true, got false")
			}
			for _, want := range tc.expect {
				if !strings.Contains(got, want) {
					t.Fatalf("expected output to contain %q, got:\n%s", want, got)
				}
			}
			for _, bad := range tc.avoid {
				if strings.Contains(got, bad) {
					t.Fatalf("expected output NOT to contain %q, got:\n%s", bad, got)
				}
			}
		})
	}
}

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
		t.Fatalf("Upgrade() error: %v", err)
	}
	if !strings.Contains(result, "list.Concat") {
		t.Fatalf("expected list.Concat rewrite, got: %s", result)
	}
}

func TestMetricsCallbackFired(t *testing.T) {
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
		t.Fatalf("expected counter >= 1, got %v", m)
	}
}
