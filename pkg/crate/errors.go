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

package crate

import "errors"

// Distinct, typed errors for the crate fetch/resolve path. Callers such as the
// type: crate render service match these with errors.Is to set a meaningful
// Application condition (a mis-named registry is an operator config problem; a
// missing crate is a publish/naming problem) instead of parsing message text.
var (
	// ErrRegistryNotFound marks an unknown crate registry name. Resolution
	// wraps it (see notFoundError) so both fetch and the CLI report and detect
	// unknown names the same way.
	ErrRegistryNotFound = errors.New("crate registry not found")

	// ErrCrateNotFound marks a crate that is absent from an otherwise valid
	// registry. The fetch wraps it when the registry has no such crate.
	ErrCrateNotFound = errors.New("crate not found in registry")
)
