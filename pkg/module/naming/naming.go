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

// Package naming holds the module naming rules shared by the side that writes
// definition names and the side that reads them back: the render service
// derives {module}-{apiVersion}-{name} when installing a module, and type
// resolution derives the same string when an Application refers to a module
// capability. Both must agree exactly, so the derivation lives in one place.
//
// It deliberately imports nothing outside the standard library. pkg/module
// reaches pkg/appfile transitively (through pkg/definition and
// pkg/workflow/providers), so pkg/appfile cannot import pkg/module — the same
// constraint that pkg/module/service/api solves for the renderer interface.
// A stdlib-only leaf package is importable from either side with no cycle.
package naming

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

const (
	// MaxObjectNameLen is the Kubernetes limit for a metadata.name.
	MaxObjectNameLen = 253
	// MaxLabelValueLen is the Kubernetes limit for a label value.
	MaxLabelValueLen = 63
)

// apiVersionPattern is the required format for a module line's apiVersion,
// following the Kubernetes API stability level convention (e.g. v1, v2,
// v1beta1, v1alpha2).
var apiVersionPattern = regexp.MustCompile(`^v\d+(alpha\d+|beta\d+)?$`)

// APIVersionPattern returns the source of the apiVersion pattern, for error
// messages that quote the expected format.
func APIVersionPattern() string {
	return apiVersionPattern.String()
}

// IsValidAPIVersion reports whether s is a well-formed module API line version.
func IsValidAPIVersion(s string) bool {
	return apiVersionPattern.MatchString(s)
}

// DefinitionName derives the installed object name for a module capability.
// This is the one derivation both the render service and type resolution must
// agree on: change it here or neither side can find what the other wrote.
func DefinitionName(module, apiVersion, name string) string {
	return TruncateName(module + "-" + apiVersion + "-" + name)
}

// OwnedApplicationName derives the name of the Application the render service
// creates for a module. Like DefinitionName this is a shared derivation: the
// render service writes the name and vela module deploy reads it back, so the
// two must agree exactly.
//
// The owned Application always lives in systemNamespace whatever namespace its
// definitions target, so the target namespace is part of the name. Without it,
// installing one module for two namespaces would render two Applications
// sharing a single name, and each install would claim the other's object.
// A definitionNamespace equal to systemNamespace adds nothing to disambiguate
// and keeps the bare module-{name} form.
func OwnedApplicationName(module, definitionNamespace, systemNamespace string) string {
	if definitionNamespace == "" || definitionNamespace == systemNamespace {
		return TruncateName("module-" + module)
	}
	return TruncateName("module-" + module + "-" + definitionNamespace)
}

// TruncateName keeps a derived definition name within the Kubernetes
// object-name limit.
func TruncateName(name string) string {
	return TruncateWithHash(name, MaxObjectNameLen)
}

// TruncateWithHash keeps s within max bytes, appending a stable 8-char digest of
// the full value so two long values sharing a prefix stay distinct.
func TruncateWithHash(s string, max int) string {
	if len(s) <= max {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	suffix := "-" + hex.EncodeToString(sum[:])[:8]
	return s[:max-len(suffix)] + suffix
}
