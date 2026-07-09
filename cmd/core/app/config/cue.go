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

package config

import (
	"context"

	"github.com/kubevela/pkg/cue/cuex"
	wfupgrade "github.com/kubevela/workflow/pkg/cue/upgrade"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
)

// CUEConfig contains CUE language configuration.
type CUEConfig struct {
	EnableExternalPackage         bool
	EnableExternalPackageWatch    bool
	EnableCUEVersionCompatibility bool
	CUECompatibilityCacheSize     int
	CUEUpgradeListConcatEnabled   bool
	CUEUpgradeErrorFieldEnabled   bool
	CUEUpgradeBoolDefaultGuard    bool
	CUEUpgradeGenericDefaultGuard bool
	CUEUpgradeKeepValidators      bool
	CUEUpgradeEvalv3SelfRefGuard  bool
}

// NewCUEConfig creates a new CUEConfig with defaults.
func NewCUEConfig() *CUEConfig {
	return &CUEConfig{
		EnableExternalPackage:         cuex.EnableExternalPackageForDefaultCompiler,
		EnableExternalPackageWatch:    cuex.EnableExternalPackageWatchForDefaultCompiler,
		EnableCUEVersionCompatibility: *upgrade.EnableCUEVersionCompatibility,
		CUECompatibilityCacheSize:     upgrade.CompatibilityCacheSize,
		CUEUpgradeListConcatEnabled:   upgrade.EnableListConcatUpgrade,
		CUEUpgradeErrorFieldEnabled:   upgrade.EnableErrorFieldLabelUpgrade,
		CUEUpgradeBoolDefaultGuard:    upgrade.EnableBoolDefaultGuardUpgrade,
		CUEUpgradeGenericDefaultGuard: upgrade.EnableGenericDefaultGuardUpgrade,
		CUEUpgradeKeepValidators:      upgrade.EnableKeepValidatorsSingletonUpgrade,
		CUEUpgradeEvalv3SelfRefGuard:  upgrade.EnableEvalv3SelfRefGuardUpgrade,
	}
}

// AddFlags registers CUE configuration flags.
func (c *CUEConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.EnableExternalPackage,
		"enable-external-package-for-default-compiler",
		c.EnableExternalPackage,
		"Enable loading third-party CUE packages into the default CUE compiler. When enabled, external CUE packages can be imported and used in CUE templates.")
	fs.BoolVar(&c.EnableExternalPackageWatch,
		"enable-external-package-watch-for-default-compiler",
		c.EnableExternalPackageWatch,
		"Enable watching for changes in external CUE packages and automatically reload them when modified. Requires enable-external-package-for-default-compiler to be enabled.")
	fs.BoolVar(&c.EnableCUEVersionCompatibility,
		"enable-cue-version-compatibility",
		c.EnableCUEVersionCompatibility,
		"Automatically rewrite legacy CUE syntax in stored definitions at render time.")
	fs.IntVar(&c.CUECompatibilityCacheSize,
		"cue-compatibility-cache-size",
		c.CUECompatibilityCacheSize,
		"Maximum number of CUE templates to cache after version compatibility rewriting. Set to 0 to disable caching.")
	fs.BoolVar(&c.CUEUpgradeListConcatEnabled,
		"cue-upgrade-list-concat-enabled",
		c.CUEUpgradeListConcatEnabled,
		"Enable list concat compatibility rewrite pass.")
	fs.BoolVar(&c.CUEUpgradeErrorFieldEnabled,
		"cue-upgrade-error-field-label-enabled",
		c.CUEUpgradeErrorFieldEnabled,
		"Enable error field-label compatibility rewrite pass.")
	fs.BoolVar(&c.CUEUpgradeBoolDefaultGuard,
		"cue-upgrade-bool-default-guard-enabled",
		c.CUEUpgradeBoolDefaultGuard,
		"Enable bool default-guard hazard compatibility rewrite pass.")
	fs.BoolVar(&c.CUEUpgradeGenericDefaultGuard,
		"cue-upgrade-generic-default-guard-enabled",
		c.CUEUpgradeGenericDefaultGuard,
		"Enable generic default-guard hazard compatibility rewrite pass.")
	fs.BoolVar(&c.CUEUpgradeKeepValidators,
		"cue-upgrade-keepvalidators-singleton-enabled",
		c.CUEUpgradeKeepValidators,
		"Enable keepvalidators singleton concretization compatibility pass.")
	fs.BoolVar(&c.CUEUpgradeEvalv3SelfRefGuard,
		"cue-upgrade-evalv3-selfref-guard-enabled",
		c.CUEUpgradeEvalv3SelfRefGuard,
		"Enable evalv3 self-reference default-guard compatibility rewrite pass.")
}

// SyncToCUEGlobals syncs the parsed configuration values to CUE package global variables.
// This should be called after flag parsing to ensure the CUE compiler uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the cuex package. Ideally, the CUE compiler configuration should be injected
// rather than relying on globals.
//
// The flow is: CLI flags -> CUEConfig struct fields -> cuex/upgrade globals (via this method)
// ctx should be the controller's root context so cache eviction goroutines are tied to its lifetime.
func (c *CUEConfig) SyncToCUEGlobals(ctx context.Context) {
	cuex.EnableExternalPackageForDefaultCompiler = c.EnableExternalPackage
	cuex.EnableExternalPackageWatchForDefaultCompiler = c.EnableExternalPackageWatch
	*upgrade.EnableCUEVersionCompatibility = c.EnableCUEVersionCompatibility
	*wfupgrade.EnableCUEVersionCompatibility = c.EnableCUEVersionCompatibility
	upgrade.EnableListConcatUpgrade = c.CUEUpgradeListConcatEnabled
	upgrade.EnableErrorFieldLabelUpgrade = c.CUEUpgradeErrorFieldEnabled
	upgrade.EnableBoolDefaultGuardUpgrade = c.CUEUpgradeBoolDefaultGuard
	upgrade.EnableGenericDefaultGuardUpgrade = c.CUEUpgradeGenericDefaultGuard
	upgrade.EnableKeepValidatorsSingletonUpgrade = c.CUEUpgradeKeepValidators
	upgrade.EnableEvalv3SelfRefGuardUpgrade = c.CUEUpgradeEvalv3SelfRefGuard
	wfupgrade.EnableListConcatUpgrade = c.CUEUpgradeListConcatEnabled
	wfupgrade.EnableErrorFieldLabelUpgrade = c.CUEUpgradeErrorFieldEnabled
	wfupgrade.EnableBoolDefaultGuardUpgrade = c.CUEUpgradeBoolDefaultGuard
	wfupgrade.EnableGenericDefaultGuardUpgrade = c.CUEUpgradeGenericDefaultGuard
	wfupgrade.EnableKeepValidatorsSingletonUpgrade = c.CUEUpgradeKeepValidators
	wfupgrade.EnableEvalv3SelfRefGuardUpgrade = c.CUEUpgradeEvalv3SelfRefGuard
	if c.CUECompatibilityCacheSize < 0 {
		klog.Warningf("cue-compatibility-cache-size %d is invalid (must be >= 0); caching disabled", c.CUECompatibilityCacheSize)
		c.CUECompatibilityCacheSize = 0
	}
	upgrade.InitCompatibilityCache(ctx, c.CUECompatibilityCacheSize)
	wfupgrade.InitCompatibilityCache(ctx, c.CUECompatibilityCacheSize)
}
