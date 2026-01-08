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
	"strings"

	"github.com/oam-dev/kubevela/version"
)

// UpgradeFunc represents a function that upgrades CUE code for version compatibility
type UpgradeFunc func(string) (string, error)

// upgradeRegistry holds upgrade functions for different CUE versions
var upgradeRegistry = make(map[string][]UpgradeFunc)

// RegisterUpgrade registers an upgrade function for a specific KubeVela version
// version should be in format "1.11", "1.12", etc.
func RegisterUpgrade(version string, upgradeFunc UpgradeFunc) {
	upgradeRegistry[version] = append(upgradeRegistry[version], upgradeFunc)
}

// getCurrentKubeVelaMinorVersion extracts the minor version (e.g., "1.11") from the full KubeVela version
func getCurrentKubeVelaMinorVersion() (string, error) {
	versionStr := version.VelaVersion
	if versionStr == "" || versionStr == "UNKNOWN" {
		return "", fmt.Errorf("unable to determine KubeVela version (got %q). Please specify the target version explicitly using --target-version=1.11", versionStr)
	}
	
	// Remove 'v' prefix if present
	versionStr = strings.TrimPrefix(versionStr, "v")
	
	// Use regex to extract major.minor version (e.g., "1.11.2" -> "1.11")
	re := regexp.MustCompile(`^(\d+\.\d+)`)
	matches := re.FindStringSubmatch(versionStr)
	if len(matches) >= 2 {
		return matches[1], nil
	}
	
	return "", fmt.Errorf("unable to parse KubeVela version %q. Please specify the target version explicitly using --target-version=1.11", versionStr)
}

// Upgrade applies all registered upgrades for KubeVela versions up to and including the target version
// targetVersion should be in format "1.11", "1.12", etc.
// If targetVersion is empty, applies upgrades for the current KubeVela CLI version
func Upgrade(cueStr string, targetVersion ...string) (string, error) {
	var version string
	var err error
	
	if len(targetVersion) > 0 && targetVersion[0] != "" {
		version = targetVersion[0]
	} else {
		version, err = getCurrentKubeVelaMinorVersion() // Default to current CLI version
		if err != nil {
			return "", err
		}
	}

	result := cueStr

	// Apply upgrades for all versions up to and including the target version
	// Currently we only support 1.11, but this can be extended
	supportedVersions := []string{"1.11"}
	
	for _, v := range supportedVersions {
		if shouldApplyUpgrade(v, version) {
			if upgrades, exists := upgradeRegistry[v]; exists {
				for _, upgrade := range upgrades {
					result, err = upgrade(result)
					if err != nil {
						return cueStr, fmt.Errorf("failed to apply upgrade for version %s: %w", v, err)
					}
				}
			}
		}
	}

	return result, nil
}

// shouldApplyUpgrade determines if upgrades for a given version should be applied
// based on the target version
func shouldApplyUpgrade(upgradeVersion, targetVersion string) bool {
	// For now, simple string comparison works since we only have 1.11
	// In the future, this could be enhanced with proper semantic version comparison
	return upgradeVersion <= targetVersion
}

// GetSupportedVersions returns a list of supported upgrade versions
func GetSupportedVersions() []string {
	versions := make([]string, 0, len(upgradeRegistry))
	for version := range upgradeRegistry {
		versions = append(versions, version)
	}
	return versions
}

// RequiresUpgrade checks if the CUE string requires upgrading to the target version
// Returns: (needsUpgrade bool, reasons []string, error)
// If targetVersion is empty, uses the current KubeVela CLI version
func RequiresUpgrade(cueStr string, targetVersion ...string) (bool, []string, error) {
	var version string
	var err error
	
	if len(targetVersion) > 0 && targetVersion[0] != "" {
		version = targetVersion[0]
	} else {
		version, err = getCurrentKubeVelaMinorVersion()
		if err != nil {
			return false, nil, err
		}
	}
	
	// For now, we only check for v1.11 upgrades
	// In the future, this can check against multiple version requirements
	if version >= "1.11" {
		return requires111Upgrade(cueStr)
	}
	
	return false, nil, nil
}