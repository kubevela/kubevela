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

// Package crate implements the on-disk crate package format: a parser
// that reads a crate source tree into a validated Crate model, shared
// by vela crate publish, the registry fetch, and vela crate deploy.
package crate

// Line is one API line of a crate: its version identity, the auxiliary objects
// backing it, and its rendered ComponentDefinitions.
type Line struct {
	// APIVersion is the API line identifier, e.g. "v1", "v1beta1".
	APIVersion string
	// Enabled is the crate author's install switch for this line, read from
	// v<N>/_version.cue's top-level enabled field. Absent means true: a line
	// ships installable unless its author opts out. The render service
	// installs every enabled line and skips the rest.
	Enabled bool
	// Placement is the line default read from the placement field in
	// v<N>/_version.cue. The value is hub or runtime. Empty means the line
	// sets no default. An object annotation resource.oam.dev/placement wins
	// over it. A file in v<N>/auxiliary/hub/ is stamped hub when it has no
	// annotation of its own, so the folder beats this default. It applies
	// to the line's auxiliary objects. Definitions stay on the hub.
	Placement string
	// Auxiliary holds the objects read from v<N>/auxiliary/, in filename order
	// (and document order within a multi-document file).
	// Empty for a line that ships no auxiliary/.
	Auxiliary []map[string]interface{}
	// Definitions are the rendered ComponentDefinitions (or other
	// X-Definitions) under v<N>/definitions/, in sorted filename order.
	Definitions []map[string]interface{}
}

// Crate is a parsed crate source tree: its identity, crate-wide auxiliary
// objects, and its API lines keyed by apiVersion.
type Crate struct {
	// Name is the crate identity, read from _crate.cue's top-level crate
	// field.
	Name string
	// Version is the crate's own semver, read from _crate.cue's top-level
	// version field. It is the tag vela crate publish uses for the artifact.
	Version string
	// Placement is the crate default read from the placement field in
	// _crate.cue. The value is hub or runtime. Empty means the crate sets
	// no default. A line default wins over it, and an object annotation
	// resource.oam.dev/placement wins over both. A file in auxiliary/hub/
	// is stamped hub when it has no annotation of its own, so the folder
	// beats this default. Absent means hub.
	Placement string
	// Warnings are non-fatal findings from the source tree, such as an
	// unrecognized folder under auxiliary/ or an object in hub/ whose own
	// annotation says runtime. vela crate publish prints them. An
	// unrecognized folder is skipped. The object's annotation wins over the
	// folder.
	Warnings []string
	// Auxiliary holds the objects read from the crate-level auxiliary/, in
	// filename order (and document order within a multi-document file). Empty
	// for infra-less crate that ships no crate-level auxiliary/.
	Auxiliary []map[string]interface{}
	// Lines are the crate's API lines, keyed by APIVersion.
	Lines map[string]Line
}
