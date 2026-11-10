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

package oam

import "fmt"

const (
	// AnnotationPlacement marks where an object installs. The value is
	// PlacementHub or PlacementRuntime. An object with no annotation is not
	// marked.
	AnnotationPlacement = "resource.oam.dev/placement"

	// PlacementHub stays on the control plane.
	PlacementHub = "hub"
	// PlacementRuntime follows the installer's placement request.
	PlacementRuntime = "runtime"
)

// ValidatePlacement accepts only PlacementHub and PlacementRuntime.
// An empty value and any other value are errors.
func ValidatePlacement(value string) error {
	if value == PlacementHub || value == PlacementRuntime {
		return nil
	}
	if value == "" {
		return fmt.Errorf("placement is empty, must be %q or %q", PlacementHub, PlacementRuntime)
	}
	return fmt.Errorf("placement %q is invalid, must be %q or %q", value, PlacementHub, PlacementRuntime)
}
