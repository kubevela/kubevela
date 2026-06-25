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

package upgrade

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	cueast "cuelang.org/go/cue/ast"
	cueformat "cuelang.org/go/cue/format"
	cueparser "cuelang.org/go/cue/parser"
	"k8s.io/klog/v2"

	velaversion "github.com/oam-dev/kubevela/version"
)

// EnableCUEVersionCompatibility controls whether EnsureCueVersionCompatibility is applied at
// render time. Defaults to true. Can be disabled via --enable-cue-version-compatibility=false
var EnableCUEVersionCompatibility = true

// DefinitionKind identifies the type of definition for metrics, logs, and compatibility reports.
// It is an open string type — each consuming repo declares its own constants.
// Values should match the definition's Kind string for consistency, e.g. "Component", "Trait".
type DefinitionKind string

const (
	// ComponentKind represents a ComponentDefinition.
	ComponentKind DefinitionKind = "Component"
	// TraitKind represents a TraitDefinition.
	TraitKind DefinitionKind = "Trait"
	// PolicyKind represents a PolicyDefinition.
	PolicyKind DefinitionKind = "Policy"
	// WorkflowStepKind represents a WorkflowStepDefinition.
	WorkflowStepKind DefinitionKind = "WorkflowStep"
)

// TemplateArea identifies which part of a definition's CUE template a rewrite was applied to.
// It is an open string type — each consuming repo declares its own constants.
type TemplateArea string

const (
	// TemplateAreaMain is the primary output/parameter template block.
	TemplateAreaMain TemplateArea = "template"
	// TemplateAreaHealth is the health check CUE block.
	TemplateAreaHealth TemplateArea = "health"
	// TemplateAreaCustomStatus is the custom status CUE block.
	TemplateAreaCustomStatus TemplateArea = "custom_status"
	// TemplateAreaStatusDetail is the status detail CUE block.
	TemplateAreaStatusDetail TemplateArea = "status_detail"
)

// Version identifies a release version for upgrade ordering.
type Version struct {
	Major int
	Minor int
}

// String returns the canonical "Major.Minor" representation, e.g. "1.11".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// less reports whether v is strictly earlier than other.
func (v Version) less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	return v.Minor < other.Minor
}

// ParseVersion parses a "Major.Minor" string (with optional leading "v") into a Version.
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")
	re := regexp.MustCompile(`^(\d+)\.(\d+)(?:\.\d+)?(?:[+-].*)?$`)
	m := re.FindStringSubmatch(s)
	if len(m) < 3 {
		return Version{}, fmt.Errorf("cannot parse version %q: expected Major.Minor format", s)
	}
	var v Version
	fmt.Sscanf(m[1], "%d", &v.Major)
	fmt.Sscanf(m[2], "%d", &v.Minor)
	return v, nil
}

// upgradeEntry is the internal interface satisfied by all registered upgrade types.
// It is unexported; callers interact via the concrete public types.
type upgradeEntry interface {
	// id returns the stable machine-readable identifier, e.g. "list-arithmetic".
	id() string
	// reason returns the human-readable description for compatibility reports.
	reason() string
	// source identifies the origin of the incompatibility, e.g. "cue" or "kubevela".
	// Used in logs and compatibility reports to distinguish CUE language changes from
	// KubeVela template changes.
	source() string
	// versionLabel returns the version string used in metrics and reason output, e.g. "1.11" or "v0.11.0".
	versionLabel() string
	// appliesToTarget reports whether this fix should be applied given the explicit target KubeVela version.
	// For CUEUpgradeFunc this is determined by the runtime CUE language version, not the target.
	appliesToTarget(target Version) bool
	// precheck is an optional cheap heuristic; if set and returns false the fix is skipped.
	precheck() func(string) bool
	// upgrade applies the rewrite. Receives both raw string and pre-parsed AST.
	upgrade() func(string, *cueast.File) (string, error)
	// velaVersion returns the KubeVela Version associated with this entry, used for registry bucketing.
	// CUEUpgradeFunc entries are bucketed under their associated KubeVela version (for ordering).
	velaVersion() Version
}

// KubeVelaUpgradeFunc is a CUE compatibility fix triggered by a KubeVela version.
// It applies when the running (or target) KubeVela version is >= VelaVersion.
// Register with RegisterUpgrade.
//
// This type is intentionally self-contained: it can be constructed inline and
// passed directly to RegisterUpgrade without any additional wiring:
//
//	RegisterUpgrade(KubeVelaUpgradeFunc{
//	    ID:         "my-fix-1.12",
//	    VelaVersion: Version{1, 12},
//	    Reason:     "...",
//	    Upgrade:    myFixFunc,
//	})
type KubeVelaUpgradeFunc struct {
	ID          string // stable identifier, e.g. "list-arithmetic"
	VelaVersion Version
	Reason      string
	Precheck    func(cueStr string) bool
	Upgrade     func(cueStr string, file *cueast.File) (string, error)
}

func (u KubeVelaUpgradeFunc) id() string           { return u.ID }
func (u KubeVelaUpgradeFunc) reason() string       { return u.Reason }
func (u KubeVelaUpgradeFunc) source() string       { return "kubevela" }
func (u KubeVelaUpgradeFunc) versionLabel() string { return u.VelaVersion.String() }
func (u KubeVelaUpgradeFunc) velaVersion() Version { return u.VelaVersion }
func (u KubeVelaUpgradeFunc) precheck() func(string) bool {
	return u.Precheck
}
func (u KubeVelaUpgradeFunc) upgrade() func(string, *cueast.File) (string, error) {
	return u.Upgrade
}
func (u KubeVelaUpgradeFunc) appliesToTarget(target Version) bool {
	v := u.VelaVersion
	return v.less(target) || v == target
}

// CUEUpgradeFunc is a CUE compatibility fix triggered by the CUE language version.
// It applies when the running CUE language version (cue.LanguageVersion()) is >= CUEVersion,
// regardless of the KubeVela version. The AssociatedVelaVersion field is used only for
// registry ordering (fixes introduced at the same KubeVela release).
//
// Register with RegisterUpgrade:
//
//	RegisterUpgrade(CUEUpgradeFunc{
//	    ID:                    "error-field-label",
//	    CUEVersion:            Version{0, 14},
//	    AssociatedVelaVersion: Version{1, 11},
//	    Reason:                "...",
//	    Upgrade:               myFixFunc,
//	})
type CUEUpgradeFunc struct {
	ID                    string  // stable identifier
	CUEVersion            Version // minimum CUE language version that introduced the change, e.g. Version{0, 14}
	AssociatedVelaVersion Version // KubeVela version at which this fix was added (for ordering)
	Reason                string
	Precheck              func(cueStr string) bool
	Upgrade               func(cueStr string, file *cueast.File) (string, error)
}

func (u CUEUpgradeFunc) id() string           { return u.ID }
func (u CUEUpgradeFunc) reason() string       { return u.Reason }
func (u CUEUpgradeFunc) source() string       { return "cue" }
func (u CUEUpgradeFunc) versionLabel() string { return u.CUEVersion.String() }
func (u CUEUpgradeFunc) velaVersion() Version { return u.AssociatedVelaVersion }
func (u CUEUpgradeFunc) precheck() func(string) bool {
	return u.Precheck
}
func (u CUEUpgradeFunc) upgrade() func(string, *cueast.File) (string, error) {
	return u.Upgrade
}
func (u CUEUpgradeFunc) appliesToTarget(_ Version) bool {
	// CUE-language fixes apply whenever the running CUE library is >= CUEVersion.
	// The target KubeVela version is irrelevant.
	current, err := ParseVersion(cue.LanguageVersion())
	if err != nil {
		return true // fail-open: apply the fix
	}
	return u.CUEVersion.less(current) || u.CUEVersion == current
}

// UpgradeFunc is a type alias for KubeVelaUpgradeFunc, preserved for backward compatibility.
// New code should use KubeVelaUpgradeFunc or CUEUpgradeFunc directly.
type UpgradeFunc = KubeVelaUpgradeFunc

// upgradeRegistry holds all registered upgrade entries, keyed by their associated KubeVela Version.
var upgradeRegistry = make(map[Version][]upgradeEntry)

// RegisterUpgrade registers an upgrade entry. Both KubeVelaUpgradeFunc and CUEUpgradeFunc
// satisfy the internal upgradeEntry interface and can be passed directly.
func RegisterUpgrade(u upgradeEntry) {
	if u.id() == "" {
		panic(fmt.Sprintf("upgrade.RegisterUpgrade: upgrade entry for version %s has empty ID", u.velaVersion()))
	}
	v := u.velaVersion()
	upgradeRegistry[v] = append(upgradeRegistry[v], u)
}

// sortedVersions returns all registered KubeVela versions in ascending order.
func sortedVersions() []Version {
	versions := make([]Version, 0, len(upgradeRegistry))
	for v := range upgradeRegistry {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].less(versions[j])
	})
	return versions
}

// latestSupportedVersion returns the highest registered KubeVela version.
func latestSupportedVersion() Version {
	vs := sortedVersions()
	return vs[len(vs)-1]
}

// getCurrentKubeVelaVersion parses the running KubeVela version into a Version.
// If the version is unknown it returns the latest registered version.
func getCurrentKubeVelaVersion() (Version, error) {
	versionStr := velaversion.VelaVersion
	if versionStr == "" || versionStr == "UNKNOWN" {
		latest := latestSupportedVersion()
		klog.InfoS("cue/upgrade: KubeVela version is unknown (dev build), applying all upgrades", "assumedVersion", latest)
		return latest, nil
	}
	v, err := ParseVersion(versionStr)
	if err != nil {
		return Version{}, fmt.Errorf("unable to parse KubeVela version: %w. Please specify the target version explicitly using --target-version=1.11", err)
	}
	return v, nil
}

// Upgrade applies all registered upgrades that apply to the given target version.
// If no targetVersion is provided, uses the current KubeVela CLI version.
func Upgrade(cueStr string, targetVersion ...Version) (string, error) {
	var target Version
	var err error

	if len(targetVersion) > 0 {
		target = targetVersion[0]
	} else {
		target, err = getCurrentKubeVelaVersion()
		if err != nil {
			return "", err
		}
	}

	// Normalise whitespace before comparing so that formatting differences
	// don't cause a template to appear as needing upgrade when it doesn't.
	normalized, err := normalizeCUEWhitespace(cueStr)
	if err == nil {
		cueStr = normalized
	}

	result := cueStr

	for _, v := range sortedVersions() {
		for _, u := range upgradeRegistry[v] {
			if !u.appliesToTarget(target) {
				continue
			}
			pc := u.precheck()
			if pc != nil && !pc(result) {
				continue
			}
			// Parse once per fix, after Precheck passes.
			file, parseErr := cueparser.ParseFile("", result, cueparser.ParseComments)
			if parseErr != nil {
				return cueStr, fmt.Errorf("failed to parse CUE for upgrade %s: %w", v, parseErr)
			}
			result, err = u.upgrade()(result, file)
			if err != nil {
				return cueStr, fmt.Errorf("failed to apply upgrade for version %s: %w", v, err)
			}
		}
	}

	// Normalise the result so both input and output are in canonical form.
	if normalizedResult, err := normalizeCUEWhitespace(result); err == nil {
		result = normalizedResult
	}

	return result, nil
}

// GetSupportedVersions returns all registered KubeVela versions in ascending order.
func GetSupportedVersions() []Version {
	return sortedVersions()
}

// normalizeCUEWhitespace parses and reformats a CUE string to produce a canonical
// representation, so that whitespace-only differences don't appear as upgrades.
func normalizeCUEWhitespace(cueStr string) (string, error) {
	f, err := cueparser.ParseFile("", cueStr, cueparser.ParseComments)
	if err != nil {
		return cueStr, err
	}
	b, err := cueformat.Node(f)
	if err != nil {
		return cueStr, err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// appliedFix records a single upgrade fix that changed the template.
type appliedFix struct {
	id      string
	version string
}

// upgradeWithIDs applies all registered upgrades and returns the rewritten CUE string along with
// metadata for every fix that changed the template. Used internally to drive metrics.
func upgradeWithIDs(cueStr string) (result string, applied []appliedFix, err error) {
	target, err := getCurrentKubeVelaVersion()
	if err != nil {
		return cueStr, nil, err
	}

	normalized, normErr := normalizeCUEWhitespace(cueStr)
	if normErr == nil {
		cueStr = normalized
	}

	result = cueStr
	for _, v := range sortedVersions() {
		for _, u := range upgradeRegistry[v] {
			if !u.appliesToTarget(target) {
				continue
			}
			pc := u.precheck()
			if pc != nil && !pc(result) {
				continue
			}
			file, parseErr := cueparser.ParseFile("", result, cueparser.ParseComments)
			if parseErr != nil {
				return cueStr, nil, fmt.Errorf("failed to parse CUE for upgrade %s: %w", v, parseErr)
			}
			rewritten, upgradeErr := u.upgrade()(result, file)
			if upgradeErr != nil {
				return cueStr, nil, fmt.Errorf("failed to apply upgrade for version %s: %w", v, upgradeErr)
			}
			if rewritten != result {
				applied = append(applied, appliedFix{id: u.id(), version: v.String()})
			}
			result = rewritten
		}
	}

	if normalizedResult, normErr := normalizeCUEWhitespace(result); normErr == nil {
		result = normalizedResult
	}
	return result, applied, nil
}

// EnsureCueVersionCompatibility applies all upgrades for the current KubeVela version to the
// provided CUE string, ensuring backward compatibility with legacy CUE syntax.
// Returns the upgraded template and whether any semantic upgrades were actually applied
// (false for formatting-only normalization or already-compatible templates).
// Returns the original string unchanged if EnableCUEVersionCompatibility is false.
func EnsureCueVersionCompatibility(cueStr, defName string, defKind DefinitionKind, area TemplateArea) (string, bool) {
	if !EnableCUEVersionCompatibility {
		return cueStr, false
	}

	key := templateHash(cueStr)
	start := time.Now()

	cache := compatCache.Load()
	if entry, ok := cache.get(key); ok {
		if !entry.requiresUpgrade {
			klog.V(4).InfoS("cue/upgrade: skip (already compatible)", "definition", defName)
			return cueStr, false
		}
		klog.V(4).InfoS("cue/upgrade: cache hit (upgraded template)", "definition", defName)
		return entry.upgraded, true
	}

	upgraded, applied, err := upgradeWithIDs(cueStr)
	elapsed := time.Since(start)
	CUECompatUpgradeDuration.WithLabelValues(string(defKind)).Observe(elapsed.Seconds())

	if err != nil {
		klog.InfoS("cue/upgrade: skipping compatibility upgrade (fail-open)", "definition", defName, "err", err, "elapsed", elapsed)
		return cueStr, false
	}

	wasUpgraded := len(applied) > 0
	if wasUpgraded {
		cache.put(key, compatEntry{requiresUpgrade: true, upgraded: upgraded})
		for _, fix := range applied {
			CUECompatRewriteTotal.WithLabelValues(fix.id, fix.version, string(defKind), string(area)).Inc()
		}
		klog.InfoS("cue/upgrade: applied CUE version compatibility fixes", "definition", defName, "elapsed", elapsed)
	} else {
		cache.put(key, compatEntry{requiresUpgrade: false})
		klog.V(4).InfoS("cue/upgrade: no compatibility fixes needed", "definition", defName, "elapsed", elapsed)
	}

	return upgraded, wasUpgraded
}

// RequiresUpgrade checks if the CUE string requires upgrading to the target version.
// If no targetVersion is provided, uses the current KubeVela CLI version.
func RequiresUpgrade(cueStr string, targetVersion ...Version) (bool, []string, error) {
	var target Version
	var err error

	if len(targetVersion) > 0 {
		target = targetVersion[0]
	} else {
		target, err = getCurrentKubeVelaVersion()
		if err != nil {
			return false, nil, err
		}
	}

	// Normalise whitespace so formatting-only differences don't appear as upgrades.
	normalized, err := normalizeCUEWhitespace(cueStr)
	if err == nil {
		cueStr = normalized
	}

	var allReasons []string

	for _, v := range sortedVersions() {
		for _, u := range upgradeRegistry[v] {
			if !u.appliesToTarget(target) {
				continue
			}
			pc := u.precheck()
			if pc != nil && !pc(cueStr) {
				continue
			}
			file, parseErr := cueparser.ParseFile("", cueStr, cueparser.ParseComments)
			if parseErr != nil {
				return false, nil, fmt.Errorf("failed to parse CUE for version %s: %w", v, parseErr)
			}
			result, err := u.upgrade()(cueStr, file)
			if err != nil {
				return false, nil, fmt.Errorf("failed to check upgrade for version %s: %w", v, err)
			}
			if result != cueStr {
				allReasons = append(allReasons, fmt.Sprintf("[%s@%s] [%s] %s", u.source(), u.versionLabel(), u.id(), u.reason()))
			}
		}
	}

	return len(allReasons) > 0, allReasons, nil
}
