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

import (
	"reflect"
	"sort"
)

// StringsContain strings contain
func StringsContain(items []string, source string) bool {
	for _, item := range items {
		if item == source {
			return true
		}
	}
	return false
}

// ThreeWaySliceCompare will compare two string slice, with the three return values: [both in A and B], [only in A], [only in B]
func ThreeWaySliceCompare(a []string, b []string) ([]string, []string, []string) {
	m := make(map[string]struct{})
	for _, k := range b {
		m[k] = struct{}{}
	}

	var AB, AO, BO []string
	for _, k := range a {
		_, ok := m[k]
		if !ok {
			AO = append(AO, k)
			continue
		}
		AB = append(AB, k)
		delete(m, k)
	}
	for k := range m {
		BO = append(BO, k)
	}
	sort.Strings(AB)
	sort.Strings(AO)
	sort.Strings(BO)
	return AB, AO, BO
}

// EqualSlice checks if two slice are equal
func EqualSlice(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}
