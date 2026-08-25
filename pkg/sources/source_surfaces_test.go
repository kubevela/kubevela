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

import (
	"slices"
	"testing"
)

// ConsumableSurfaces is derived from sourceReadingSurfaces rather than maintained
// beside it, because two hand-kept lists drift: a surface enabled for resolution
// while consumableFrom still refuses to let a definition name it leaves the
// definition unable to declare a capability the controller has.
//
// This pins the relationship rather than the contents, so adding a surface takes
// one edit and cannot reintroduce the gap.
func TestConsumableSurfacesDerivesFromResolvingSurfaces(t *testing.T) {
	for _, surface := range ConsumableSurfaces {
		if !SurfaceReadsSource(surface) {
			t.Errorf("%q is consumable but does not resolve; a definition could advertise "+
				"a capability the controller does not have", surface)
		}
		if surface == SurfaceSource {
			t.Error("source chaining is plumbing between sources, not a place an Application " +
				"consumes a value, so it must not be nameable in consumableFrom")
		}
	}

	// Every resolving surface except chaining must be nameable, or a definition
	// cannot restrict itself to a surface that genuinely works.
	for _, surface := range sourceReadingSurfaces {
		if surface == SurfaceSource {
			continue
		}
		if !slices.Contains(ConsumableSurfaces, surface) {
			t.Errorf("%q resolves but cannot be named in consumableFrom", surface)
		}
	}

	if len(ConsumableSurfaces) == 0 {
		t.Fatal("no surface is consumable, so a source cannot be read anywhere")
	}
}

// A surface outside the list must not resolve, or the two enforcement points
// would disagree about what is inert.
func TestUnknownSurfaceDoesNotResolve(t *testing.T) {
	for _, surface := range []string{"", "unknown", SurfacePolicy} {
		if SurfaceReadsSource(surface) {
			t.Errorf("%q must not resolve", surface)
		}
	}
}
