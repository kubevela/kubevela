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

package cue

import "cuelang.org/go/cue"

// GetSelectorLabel safely extracts a label from a CUE selector.
// It uses String() by default to avoid panics on pattern parameter selectors,
// and only uses Unquoted() when it's safe (i.e., for StringLabel with concrete names).
// This prevents panics that would occur when calling Unquoted() on pattern constraints like [string]: T.
func GetSelectorLabel(selector cue.Selector) string {
	// Use String() as a safe default
	label := selector.String()
	// If it's a quoted string, unquote it safely
	if selector.IsString() && selector.LabelType() == cue.StringLabel {
		label = selector.Unquoted()
	}
	return label
}
