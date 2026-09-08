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

// Package module implements the on-disk module package format: a parser
// that reads a module source tree into a validated Module model, shared
// by vela module publish, the registry fetch, and vela module deploy.
package module

// Line is one API line of a module: its version identity, the auxiliary objects
// backing it, and its rendered ComponentDefinitions.
type Line struct {
	// APIVersion is the API line identifier, e.g. "v1", "v1beta1".
	APIVersion string
	// Enabled is the module author's install switch for this line, read from
	// v<N>/_version.cue's top-level enabled field. Absent means true: a line
	// ships installable unless its author opts out. The render service
	// installs every enabled line and skips the rest.
	Enabled bool
	// Auxiliary holds the objects read from v<N>/auxiliary/, in filename order
	// (and document order within a multi-document file).
	// Empty for a line that ships no auxiliary/.
	Auxiliary []map[string]interface{}
	// Definitions are the rendered ComponentDefinitions (or other
	// X-Definitions) under v<N>/definitions/, in sorted filename order.
	Definitions []map[string]interface{}
}

// Module is a parsed module source tree: its identity, module-wide auxiliary
// objects, and its API lines keyed by apiVersion.
type Module struct {
	// Name is the module identity, read from _module.cue's top-level module
	// field.
	Name string
	// Version is the module's own semver, read from _module.cue's top-level
	// version field. It is the tag vela module publish uses for the artifact.
	Version string
	// Auxiliary holds the objects read from the module-level auxiliary/, in
	// filename order (and document order within a multi-document file). Empty
	//for infra-less module that ships no module-level auxiliary/.
	Auxiliary []map[string]interface{}
	// Lines are the module's API lines, keyed by APIVersion.
	Lines map[string]Line
}
