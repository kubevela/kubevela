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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/addon"
)

// Filter is used to filter Unstructured objects. It is basically a func(unstructured.Unstructured) bool.
type Filter func(unstructured.Unstructured) bool

// Apply applies all the provided filters to a given object.
// Then returns if the object is filtered out or not.
// Returns true if this object is kept, otherwise false.
func Apply(obj unstructured.Unstructured, filters ...Filter) bool {
	// Apply all filters
	for _, filter := range filters {
		// If filtered out by one of the filters
		if !filter(obj) {
			return false
		}
	}

	// All filters have kept this item
	return true
}

// ApplyToList applies all the provided filters to a UnstructuredList.
// It only keeps items that pass all the filters.
func ApplyToList(list unstructured.UnstructuredList, filters ...Filter) unstructured.UnstructuredList {
	filteredList := unstructured.UnstructuredList{Object: list.Object}

	// Apply filters to each item in the list
	for _, u := range list.Items {
		kept := Apply(u, filters...)
		if !kept {
			continue
		}

		// Item that passed filters
		filteredList.Items = append(filteredList.Items, u)
	}

	return filteredList
}

// KeepAll returns a filter that keeps everything
func KeepAll() Filter {
	return func(unstructured.Unstructured) bool {
		return true
	}
}

// KeepNone returns a filter that filters out everything
func KeepNone() Filter {
	return func(unstructured.Unstructured) bool {
		return false
	}
}

// ByOwnerAddon returns a filter that filters out what does not belong to the owner addon.
// Empty addon name will keep everything.
func ByOwnerAddon(addonName string) Filter {
	if addonName == "" {
		// Empty addon name, just keep everything, no further action needed
		return KeepAll()
	}

	// Filter by which addon installed it by owner reference
	// only keep the ones that belong to the addon
	return func(obj unstructured.Unstructured) bool {
		ownerRefs := obj.GetOwnerReferences()
		isOwnedBy := false
		for _, ownerRef := range ownerRefs {
			if ownerRef.Name == addon.Addon2AppName(addonName) {
				isOwnedBy = true
				break
			}
		}
		return isOwnedBy
	}
}

// ByName returns a filter that matches the given name.
// Empty name will keep everything.
func ByName(name string) Filter {
	// Keep everything
	if name == "" {
		return KeepAll()
	}

	// Filter by name
	return func(obj unstructured.Unstructured) bool {
		return obj.GetName() == name
	}
}

// ByAppliedWorkload returns a filter that only keeps trait definitions that applies to the given workload.
// Empty workload name will keep everything.
func ByAppliedWorkload(workload string) Filter {
	// Keep everything
	if workload == "" {
		return KeepAll()
	}

	return func(obj unstructured.Unstructured) bool {
		traitDef := &v1beta1.TraitDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, traitDef); err != nil {
			return false
		}

		// Search for provided workload
		// If the trait definitions applies to the given workload, then it is kept.
		for _, w := range traitDef.Spec.AppliesToWorkloads {
			if w == workload || w == "*" {
				return true
			}
		}

		return false
	}
}
