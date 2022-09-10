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

package utils

// FilterComponents select the components using the selectors
func FilterComponents(components []string, selector []string) []string {
	if selector != nil {
		filter := map[string]bool{}
		for _, compName := range selector {
			filter[compName] = true
		}
		var _comps []string
		for _, compName := range components {
			if _, ok := filter[compName]; ok {
				_comps = append(_comps, compName)
			}
		}
		return _comps
	}
	return components
}
