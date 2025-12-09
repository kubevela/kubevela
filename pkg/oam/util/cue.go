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

package util

import "cuelang.org/go/cue"

// GetIteratorLabel returns the label string from a CUE iterator using the
// non-deprecated Selector API. It handles both string labels and other label types.
func GetIteratorLabel(iter cue.Iterator) string {
	selector := iter.Selector()

	// If it's a quoted string, unquote it safely
	if selector.IsString() && selector.LabelType() == cue.StringLabel {
		return selector.Unquoted()
	}

	// For other label types, use the string representation
	return selector.String()
}
