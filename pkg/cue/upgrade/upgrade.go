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

// Package upgrade provides CUE version-compatibility helpers for KubeVela.
// The core engine lives in github.com/kubevela/pkg/cue/upgrade; this package
// wires KubeVela-specific concerns (version provider, Prometheus metrics) and
// re-exports the public API so existing call sites need no import change.
package upgrade

import (
	"context"
	"time"

	pkgupgrade "github.com/kubevela/pkg/cue/upgrade"

	velaversion "github.com/oam-dev/kubevela/version"
)

// Version is a release version for upgrade ordering.
type Version = pkgupgrade.Version

// DefinitionKind identifies the type of definition for metrics and compatibility reports.
type DefinitionKind = pkgupgrade.DefinitionKind

// TemplateArea identifies which part of a definition's CUE template a rewrite was applied to.
type TemplateArea = pkgupgrade.TemplateArea

// KubeVelaUpgradeFunc is a CUE compatibility fix triggered by a KubeVela version.
type KubeVelaUpgradeFunc = pkgupgrade.KubeVelaUpgradeFunc

// CUEUpgradeFunc is a CUE compatibility fix triggered by the CUE language version.
type CUEUpgradeFunc = pkgupgrade.CUEUpgradeFunc

// UpgradeFunc is a backward-compatible alias for KubeVelaUpgradeFunc.
type UpgradeFunc = pkgupgrade.UpgradeFunc //nolint:revive

// KubeVela-specific DefinitionKind constants.
const (
	ComponentKind    DefinitionKind = "Component"
	TraitKind        DefinitionKind = "Trait"
	PolicyKind       DefinitionKind = "Policy"
	WorkflowStepKind DefinitionKind = "WorkflowStep"
)

// KubeVela-specific TemplateArea constants.
const (
	TemplateAreaMain         TemplateArea = "template"
	TemplateAreaHealth       TemplateArea = "health"
	TemplateAreaCustomStatus TemplateArea = "custom_status"
	TemplateAreaStatusDetail TemplateArea = "status_detail"
)

// EnableCUEVersionCompatibility is a pointer alias for pkgupgrade.EnableCUEVersionCompatibility.
// Writing to it (via dereference) updates the engine directly with no additional synchronization.
var EnableCUEVersionCompatibility = &pkgupgrade.EnableCUEVersionCompatibility

// CompatibilityCacheSize mirrors pkgupgrade.CompatibilityCacheSize.
var CompatibilityCacheSize = pkgupgrade.CompatibilityCacheSize

// Re-export functions.
var (
	ParseVersion         = pkgupgrade.ParseVersion
	RegisterUpgrade      = pkgupgrade.RegisterUpgrade
	GetSupportedVersions = pkgupgrade.GetSupportedVersions
)

// SetCacheEntryTTL sets how long an unaccessed cache entry lives before eviction.
// Must be called before InitCompatibilityCache to take effect.
func SetCacheEntryTTL(d time.Duration) {
	pkgupgrade.CacheEntryTTL = d
}

// Upgrade applies all registered upgrades to cueStr.
func Upgrade(cueStr string, targetVersion ...Version) (string, error) {
	return pkgupgrade.Upgrade(cueStr, targetVersion...)
}

// RequiresUpgrade checks whether cueStr needs upgrading.
func RequiresUpgrade(cueStr string, targetVersion ...Version) (bool, []string, error) {
	return pkgupgrade.RequiresUpgrade(cueStr, targetVersion...)
}

// EnsureCueVersionCompatibility applies all upgrades for the running KubeVela version.
func EnsureCueVersionCompatibility(cueStr, defName string, defKind DefinitionKind, area TemplateArea) (string, bool) {
	return pkgupgrade.EnsureCueVersionCompatibility(cueStr, defName, defKind, area)
}

// InitCompatibilityCache reinitialises the LRU cache.
func InitCompatibilityCache(ctx context.Context, size int) {
	pkgupgrade.InitCompatibilityCache(ctx, size)
}

func init() {
	// Wire the KubeVela version provider.
	pkgupgrade.GetCurrentVersion = func() string {
		return velaversion.VelaVersion
	}

	// Wire Prometheus metrics callbacks into the engine hooks.
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
