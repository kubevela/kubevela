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

package sources

import "strings"

// DefinitionError is a source definition refusing on its own terms, as
// opposed to failing to compile or fetch: the errors it raised through
// `errs`, or an output its `schema` does not admit.
type DefinitionError struct {
	// User are the definition's `errs` entries.
	User []string
	// Schema are the ways its output does not fit its schema, one per field.
	Schema []string

	message string
	cause   error
}

// Error is the resolver's message, or the entries joined when there is none.
func (e *DefinitionError) Error() string {
	if e.message != "" {
		return e.message
	}
	if entries := append(append([]string{}, e.User...), e.Schema...); len(entries) > 0 {
		return strings.Join(entries, "; ")
	}
	return "source definition refused"
}

func (e *DefinitionError) Unwrap() error { return e.cause }
