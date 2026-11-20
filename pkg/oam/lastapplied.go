/*
Copyright 2026 The KubeVela Authors.

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

package oam

// AnnotationLastAppliedConfig normally holds a serialized copy of the object as it
// was last dispatched, used as the original side of a three-way merge. These two
// values are read instead as a sentinel meaning "do not record my configuration".
const (
	// SkipLastAppliedConfigDash is the "-" opt-out sentinel.
	SkipLastAppliedConfigDash = "-"
	// SkipLastAppliedConfigSkip is the "skip" opt-out sentinel.
	SkipLastAppliedConfigSkip = "skip"
)

// IsSkipLastAppliedConfig reports whether an AnnotationLastAppliedConfig value is
// the opt-out sentinel rather than a recorded configuration.
func IsSkipLastAppliedConfig(value string) bool {
	return value == SkipLastAppliedConfigDash || value == SkipLastAppliedConfigSkip
}
