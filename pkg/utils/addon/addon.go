/*
Copyright 2022 The KubeVela Authors.

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

package addon

import "strings"

// AddonSecPrefix is the prefix for secret of addons
const AddonSecPrefix = "addon-secret-"

// Addon2SecName returns the secret name that contains addon arguments
func Addon2SecName(addonName string) string {
	if addonName == "" {
		return ""
	}

	return AddonSecPrefix + addonName
}

// AddonAppPrefix is the prefix for corresponding Application of an addon
const AddonAppPrefix = "addon-"

// Addon2AppName return the app name that represents the addon
func Addon2AppName(addonName string) string {
	if addonName == "" {
		return ""
	}

	return AddonAppPrefix + addonName
}

// AppName2Addon converts an addon app name to the actual addon name
// If it is not a addon app name, empty string is returned.
func AppName2Addon(appName string) string {
	if !strings.HasPrefix(appName, AddonAppPrefix) {
		return ""
	}

	return appName[len(AddonAppPrefix):]
}
