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

// Package appscope holds what the controller and admission must agree on
// about the Application-scoped policies that apply to an Application: the order
// they run in, and whether it asks to skip the globals. Admission judges an
// Application by the context the policies supply, and the last to set a key
// wins, so the two may not differ.
package appscope

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Ref places an Application-scoped policy among those applying to an
// Application.
type Ref struct {
	Name     string
	Priority int32
	Global   bool
	// SpecOrder is an explicit policy's index in spec.policies.
	SpecOrder int
}

// Less orders policies as they run: lower priority first; at a priority,
// globals before explicit policies, globals by name, explicit policies in the
// order the spec lists them. An explicit policy has priority 0.
func Less(a, b Ref) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.Global != b.Global {
		return a.Global
	}
	if a.Global {
		return a.Name < b.Name
	}
	return a.SpecOrder < b.SpecOrder
}

// AsksToSkipGlobals reports whether an Application carries the skip-global
// annotation, whether or not skips are permitted.
func AsksToSkipGlobals(app *v1beta1.Application) bool {
	return app.Annotations[oam.AnnotationSkipGlobalPolicies] == "true"
}
