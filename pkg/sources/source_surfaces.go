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
)

// Surfaces an Application can carry a property expression on.
//
// These live here, next to the resolver, because which of them can read a
// `source` is a property of this package: resolution is wired into
// workloadDef.Complete and traitDef.Complete and nowhere else. The admission
// webhook and the appfile parser both enforce that rule, so it is stated once
// rather than restated per enforcement point - which is exactly the divergence
// that let admission accept `$(context...)` in every policy while only one kind
// of policy substituted it.
const (
	// SurfaceComponent is an expression in a component's properties.
	SurfaceComponent = "component"
	// SurfaceTrait is an expression in a trait's properties.
	SurfaceTrait = "trait"
	// SurfacePolicy is an expression in a built-in policy's properties - topology,
	// override and the rest. It may read `context` but not `source`: those
	// properties are consumed straight off af.Policies by a provider, so nothing
	// renders them and there is no resolver to reach a source through.
	SurfacePolicy = "policy-default"
	// SurfacePolicyRendered is an expression in a PolicyDefinition that has a CUE
	// template. It renders through the same engine a component does, so a source
	// resolves there exactly as it would in a component.
	SurfacePolicyRendered = "policy-rendered"
	// SurfacePolicyApp is an expression in an Application-scoped PolicyDefinition.
	// It renders before the appfile is built, so there is no spec.sources[] to
	// resolve against - context only, substituted by the appfile-time pass.
	SurfacePolicyApp = "policy-app"
	// SurfaceWorkflowStep is an expression in a workflow step's (or sub-step's) properties.
	SurfaceWorkflowStep = "workflowstep"
	// SurfaceSource is an expression in a later spec.sources[] entry's properties,
	// i.e. source chaining. It resolves because the chain is walked during a
	// component render.
	SurfaceSource = "source"
)

// sourceReadingSurfaces is the one list of surfaces where an expression may read
// `source`. Everything else about surfaces derives from it.
//
// It is a single list because the alternative was tried and drifted: enabling a
// surface here left ConsumableSurfaces still refusing to let a definition name
// it, so a definition could not declare a capability the controller had. Deriving
// the second list makes that state unreachable rather than merely fixed.
var sourceReadingSurfaces = []string{
	SurfaceComponent,
	SurfaceTrait,
	SurfaceSource,
	SurfaceWorkflowStep,
	SurfacePolicyRendered,
}

// ConsumableSurfaces are the surfaces a SourceDefinition may name in
// consumableFrom: the places an Application actually consumes a resolved value.
//
// Derived from sourceReadingSurfaces, less source chaining - that is plumbing
// between sources, not a place an Application consumes a value, so a definition
// has no reason to name it.
var ConsumableSurfaces = consumableSurfaces()

func consumableSurfaces() []string {
	out := make([]string, 0, len(sourceReadingSurfaces))
	for _, surface := range sourceReadingSurfaces {
		if surface == SurfaceSource {
			continue
		}
		out = append(out, surface)
	}
	return out
}

// SurfaceReadsSource reports whether an expression on this surface may read a
// `source`.
//
// Anywhere else the read cannot be honoured: the value would survive into the
// consumer as the literal text of the expression, which either fails CUE
// type-checking with a confusing message or, where the target parameter is open,
// is silently accepted as a value. Both enforcement points reject it instead.
func SurfaceReadsSource(surface string) bool {
	return slices.Contains(sourceReadingSurfaces, surface)
}
