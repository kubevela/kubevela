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
// A definition that extends another is the parent with more on it, never
// something else. The child's `output`, `outputs` and, for traits, `patch`,
// `patchOutputs` and `processing` merge onto the parent's, so adding a label
// reads as if the parent were not there:
//
//	output: metadata: labels: "tenant.oam.dev/name": parameter.tenant
//
// A child adds fields the parent did not set and fills a field the parent left
// open. It does not change a field the parent set, even one from a defaulted
// parameter: that goes up through `$super.properties` instead.
//
// `$inherit` leaves one of the parent's outputs out, and the child may write its
// own of that name in its place:
//
//	$inherit: {outputs: {ingress: false}}
//
// It cannot turn the merge off wholly or for `output`, `patch` or the whole of
// `outputs`.
//
// What the parent produced is readable on `$super` as `output`, `outputs`,
// `patch` and `patchOutputs`.
//
// A definition declares the parameters it takes. It does not inherit its
// parent's: the parameters it publishes are the ones an application is
// validated against, and taking a parent's whole set publishes fields that
// reach nothing unless they are also forwarded.
//
// # What a parent marks
//
// A doc comment on a field of the parent's results guards it for every level
// below:
//
//	// +final       nothing extending this adds to the field, at any depth
//	// +required    nothing extending this leaves this output out; on the
//	//              `outputs` block it covers every output in it
//
// # The one constraint
//
// Properties travel up before results come back, so `$super.properties` is read
// before the parent renders and cannot depend on what it produced.
package inherit
