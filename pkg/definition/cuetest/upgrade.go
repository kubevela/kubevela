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
	"context"

	pkgupgrade "github.com/kubevela/pkg/cue/upgrade"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
)

// upgradePasses are the upgrader's optional rewrites, named as the
// controller's --cue-upgrade-<name>-enabled flags name them.
var upgradePasses = []struct {
	name string
	on   *bool
}{
	{"list-concat", &upgrade.EnableListConcatUpgrade},
	{"error-field-label", &upgrade.EnableErrorFieldLabelUpgrade},
	{"bool-default-guard", &upgrade.EnableBoolDefaultGuardUpgrade},
	{"generic-default-guard", &upgrade.EnableGenericDefaultGuardUpgrade},
	{"keepvalidators-singleton", &upgrade.EnableKeepValidatorsSingletonUpgrade},
	{"evalv3-selfref-guard", &upgrade.EnableEvalv3SelfRefGuardUpgrade},
}

// controllerDefaults are the passes a controller runs unless configured
// otherwise.
var controllerDefaults = func() map[string]bool {
	on := map[string]bool{}
	for _, p := range upgradePasses {
		on[p.name] = *p.on
	}
	return on
}()

// UpgradeMarker is a case's @upgrade: a known issue, where the definition
// only works once the upgrader rewrites it. It runs with the controller's
// default passes plus any it names.
type UpgradeMarker struct {
	Passes []string
	Reason string
}

// RunOptions control how Evaluate runs a case.
type RunOptions struct {
	// Upgrades runs every case with every upgrade pass on.
	Upgrades bool
	// FailOnUpgrade treats @upgrade cases as unmarked.
	FailOnUpgrade bool
}

// Outcome is a case's result under RunOptions.
type Outcome struct {
	Failures []string
	// Upgraded is set when an @upgrade case failed as written but passed
	// once upgraded: a known issue, not a failure.
	Upgraded bool
}

// Evaluate runs the case as written, unless opts.Upgrades says otherwise. An
// @upgrade case that fails as written runs again with its passes; one that
// passes as written fails, so the marker goes with the fix.
func (c *Case) Evaluate(opts RunOptions) Outcome {
	if opts.Upgrades {
		defer UseUpgrades(true)()
		return Outcome{Failures: c.Run()}
	}
	failures := c.runAsWritten()
	if c.Upgrade == nil || opts.FailOnUpgrade {
		return Outcome{Failures: failures}
	}
	if len(failures) == 0 {
		return Outcome{Failures: []string{"passes as written: remove @upgrade"}}
	}
	defer useUpgradePasses(true, c.Upgrade.passSet())()
	if failures = c.Run(); len(failures) > 0 {
		return Outcome{Failures: failures}
	}
	return Outcome{Upgraded: true}
}

// runAsWritten runs the case with the upgrader off, then puts it back.
func (c *Case) runAsWritten() []string {
	return withoutUpgrades(c.Run)
}

// withoutUpgrades calls run with the upgrader off, and puts it back however
// run returns.
func withoutUpgrades(run func() []string) []string {
	defer UseUpgrades(false)()
	return run()
}

// passSet is the controller's default passes plus the marker's.
func (m *UpgradeMarker) passSet() map[string]bool {
	on := map[string]bool{}
	for name, def := range controllerDefaults {
		on[name] = def
	}
	for _, name := range m.Passes {
		on[name] = true
	}
	return on
}

// UseUpgrades sets whether definitions are run through KubeVela's CUE
// upgrader, with every rewrite pass on, or evaluated as written. Tests default
// to as written, so a definition that only works once rewritten fails until it
// is updated. The switches are process-wide, so call this from an entry point
// and call the returned function to put them back.
func UseUpgrades(on bool) (restore func()) {
	all := map[string]bool{}
	for _, p := range upgradePasses {
		all[p.name] = on
	}
	return useUpgradePasses(on, all)
}

// useUpgradePasses sets the upgrader on or off and each pass as given, and
// runs it uncached: its cache is keyed on the template alone, so a result
// from other settings would be served back.
func useUpgradePasses(enabled bool, on map[string]bool) (restore func()) {
	savedEnabled := *upgrade.EnableCUEVersionCompatibility
	*upgrade.EnableCUEVersionCompatibility = enabled
	saved := make([]bool, len(upgradePasses))
	for i, p := range upgradePasses {
		saved[i], *p.on = *p.on, on[p.name]
	}
	cacheSize := pkgupgrade.CompatibilityCacheSize
	pkgupgrade.InitCompatibilityCache(context.Background(), 0)
	return func() {
		*upgrade.EnableCUEVersionCompatibility = savedEnabled
		for i, p := range upgradePasses {
			*p.on = saved[i]
		}
		pkgupgrade.InitCompatibilityCache(context.Background(), cacheSize)
	}
}
