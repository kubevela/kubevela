/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package upgrade provides CUE version-compatibility helpers for KubeVela.
// The core engine lives in github.com/kubevela/pkg/cue/upgrade; this package
// wires KubeVela-specific concerns (version provider, Prometheus metrics) and
// re-exports the public API so existing call sites need no import change.
package upgrade

import (
	"context"
	"sync"
	"time"

	pkgupgrade "github.com/kubevela/pkg/cue/upgrade"

	velaversion "github.com/oam-dev/kubevela/version"
)

type Version = pkgupgrade.Version
type DefinitionKind = pkgupgrade.DefinitionKind
type TemplateArea = pkgupgrade.TemplateArea
type KubeVelaUpgradeFunc = pkgupgrade.KubeVelaUpgradeFunc
type CUEUpgradeFunc = pkgupgrade.CUEUpgradeFunc
type UpgradeFunc = pkgupgrade.UpgradeFunc //nolint:revive

const (
	ComponentKind    DefinitionKind = "Component"
	TraitKind        DefinitionKind = "Trait"
	PolicyKind       DefinitionKind = "Policy"
	WorkflowStepKind DefinitionKind = "WorkflowStep"
)

const (
	TemplateAreaMain         TemplateArea = "template"
	TemplateAreaHealth       TemplateArea = "health"
	TemplateAreaCustomStatus TemplateArea = "custom_status"
	TemplateAreaStatusDetail TemplateArea = "status_detail"
)

var EnableCUEVersionCompatibility = &pkgupgrade.EnableCUEVersionCompatibility
var CompatibilityCacheSize = pkgupgrade.CompatibilityCacheSize

var (
	// EnableListConcatUpgrade controls the list-arithmetic compatibility rewrite pass.
	EnableListConcatUpgrade = true
	// EnableErrorFieldLabelUpgrade controls quoting of legacy unquoted error labels.
	EnableErrorFieldLabelUpgrade = true
	// EnableBoolDefaultGuardUpgrade controls the bool default-guard hazard rewrite pass.
	EnableBoolDefaultGuardUpgrade = false
	// EnableGenericDefaultGuardUpgrade controls generic (non-bool) default-guard hazard rewrites.
	EnableGenericDefaultGuardUpgrade = false
	// EnableKeepValidatorsSingletonUpgrade controls singleton keepvalidators concretization rewrites.
	EnableKeepValidatorsSingletonUpgrade = false
	// EnableEvalv3SelfRefGuardUpgrade controls evalv3 self-reference default-guard rewrites.
	EnableEvalv3SelfRefGuardUpgrade = false
)

var syncLocalFlagsMu sync.Mutex

var (
	ParseVersion         = pkgupgrade.ParseVersion
	RegisterUpgrade      = pkgupgrade.RegisterUpgrade
	GetSupportedVersions = pkgupgrade.GetSupportedVersions
)

func SetCacheEntryTTL(d time.Duration) {
	pkgupgrade.CacheEntryTTL = d
}

func Upgrade(cueStr string, targetVersion ...Version) (string, error) {
	syncLocalFlagsMu.Lock()
	defer syncLocalFlagsMu.Unlock()
	syncLocalFlagsLocked()
	return pkgupgrade.Upgrade(cueStr, targetVersion...)
}

func RequiresUpgrade(cueStr string, targetVersion ...Version) (bool, []string, error) {
	syncLocalFlagsMu.Lock()
	defer syncLocalFlagsMu.Unlock()
	syncLocalFlagsLocked()
	return pkgupgrade.RequiresUpgrade(cueStr, targetVersion...)
}

func EnsureCueVersionCompatibility(cueStr, defName string, defKind DefinitionKind, area TemplateArea) (string, bool) {
	syncLocalFlagsMu.Lock()
	defer syncLocalFlagsMu.Unlock()
	syncLocalFlagsLocked()
	return pkgupgrade.EnsureCueVersionCompatibility(cueStr, defName, defKind, area)
}

func InitCompatibilityCache(ctx context.Context, size int) {
	pkgupgrade.InitCompatibilityCache(ctx, size)
}

func init() {
	syncLocalFlags()
	pkgupgrade.GetCurrentVersion = func() string {
		return velaversion.VelaVersion
	}
	pkgupgrade.OnRewrite = func(fixID, fixVersion string, defKind pkgupgrade.DefinitionKind, area pkgupgrade.TemplateArea) {
		CUECompatRewriteTotal.WithLabelValues(fixID, fixVersion, string(defKind), string(area)).Inc()
	}
	pkgupgrade.OnUpgradeDuration = func(defKind pkgupgrade.DefinitionKind, elapsed time.Duration) {
		CUECompatUpgradeDuration.WithLabelValues(string(defKind)).Observe(elapsed.Seconds())
	}
	pkgupgrade.OnCacheEviction = func(reason string) {
		CUECompatCacheEvictionsTotal.WithLabelValues(reason).Inc()
	}
}

func syncLocalFlags() {
	syncLocalFlagsMu.Lock()
	defer syncLocalFlagsMu.Unlock()
	syncLocalFlagsLocked()
}

func syncLocalFlagsLocked() {
	pkgupgrade.EnableListArithmeticUpgrade = EnableListConcatUpgrade
	pkgupgrade.EnableErrorFieldLabelUpgrade = EnableErrorFieldLabelUpgrade
	pkgupgrade.EnableBoolDefaultNegationUpgrade = EnableBoolDefaultGuardUpgrade
	pkgupgrade.EnableGenericDefaultGuardUpgrade = EnableGenericDefaultGuardUpgrade
	pkgupgrade.EnableKeepValidatorsSingletonUpgrade = EnableKeepValidatorsSingletonUpgrade
	pkgupgrade.EnableEvalv3SelfRefGuardUpgrade = EnableEvalv3SelfRefGuardUpgrade
}
