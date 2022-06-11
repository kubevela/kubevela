/*
Copyright 2021 The KubeVela Authors.

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

const addonSecPrefix = "addon-secret-"

// Convert2SecName returns the secret name that contains addon arguments
func Convert2SecName(name string) string {
	return addonSecPrefix + name
}

const addonAppPrefix = "addon-"

// Convert2AppName return the app name that represents the addon
func Convert2AppName(name string) string {
	return addonAppPrefix + name
}
