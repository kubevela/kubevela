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

// Package inherit renders a definition that extends another.
//
// Each level compiles on its own, with its own imports, and hands its result
// down as a cue.Value. Passing values rather than data lets a child override a
// field its parent derived from a defaulted parameter, which arrives still a
// disjunction.
//
// # What a child writes
//
// `$super.properties` is what the parent receives, checked against its schema and
// named for the Application's own `properties`:
//
//	$super: properties: {image: parameter.image}
//
// The child's `output`, `outputs` and, for traits, `patch`, `patchOutputs` and
// `processing` merge onto the parent's, so adding a label reads as if the parent
// were not there:
//
//	output: metadata: labels: "tenant.oam.dev/name": parameter.tenant
//
// `$inherit` turns merging off, wholly (`$inherit: false`) or per field
// (`$inherit: {output: false}`).
//
// What the parent produced is readable on `$super` as `output`, `outputs`,
// `patch` and `patchOutputs`, and its schema as `$super.parameter`.
//
// # The one constraint
//
// Properties travel up before results come back, so `$super.properties` is read
// before the parent renders and cannot depend on what it produced.
package inherit
