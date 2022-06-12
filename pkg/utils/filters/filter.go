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

package filters

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/utils/addon"
)

// KeepAll returns a filter that keeps everything
func KeepAll() func(unstructured unstructured.Unstructured) bool {
	return func(unstructured unstructured.Unstructured) bool {
		return true
	}
}

// KeepNone returns a filter that filters out everything
func KeepNone() func(unstructured unstructured.Unstructured) bool {
	return func(unstructured unstructured.Unstructured) bool {
		return false
	}
}

// ByOwnerAddon returns a filter that filters out what does not belong to the owner addon
func ByOwnerAddon(addonName string) func(unstructured unstructured.Unstructured) bool {
	if addonName == "" {
		// Empty addon name, just keep everything, no further action needed
		return KeepAll()
	}

	// Filter by which addon installed it by owner reference
	// only keep the ones that belong to the addon
	return func(unstructured unstructured.Unstructured) bool {
		ownerRefs := unstructured.GetOwnerReferences()
		isOwnedBy := false
		for _, ownerRef := range ownerRefs {
			if ownerRef.Name == addon.Convert2AppName(addonName) {
				isOwnedBy = true
				break
			}
		}
		return isOwnedBy
	}
}

// ByName returns a filter that matches the given name
func ByName(name string) func(unstructured unstructured.Unstructured) bool {
	// Keep everything
	if name == "" {
		return KeepAll()
	}

	// Filter by name
	return func(unstructured unstructured.Unstructured) bool {
		return unstructured.GetName() == name
	}
}
