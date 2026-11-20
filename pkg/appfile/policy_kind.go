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

package appfile

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// Policies fall into three kinds, and property expressions behave differently in
// each. Naming the distinction keeps each kind's rules its own, rather than
// applying the narrowest of the three to all of them.
//
//   - built-in: topology, override, garbage-collect and friends. Their properties
//     are read straight off af.Policies by a provider; nothing renders them, so
//     there is no resolver and no source. Expressions are substituted while the
//     appfile is built.
//   - rendered: any other type, which is a PolicyDefinition with a CUE template.
//     It renders through NewWorkloadAbstractEngine exactly as a component does,
//     so a source resolves there.
//   - application-scoped: a PolicyDefinition whose spec.scope is not the default.
//     It renders before the appfile exists, so there is no spec.sources[] to
//     resolve against at all, and its context expressions are substituted by the
//     appfile-time pass like a built-in policy's. Only spec.scope tells it apart
//     from a rendered one - the type name looks identical.

// builtinPolicyTypes are the policy types KubeVela consumes directly rather than
// rendering.
//
// Taken from the switch in parsePolicies. Note that parsePoliciesFromRevision
// carries a *different* list - it omits replication - so an Application parsed
// from a revision classifies that one policy differently from the same
// Application parsed fresh. That inconsistency predates this file and is not
// resolved here; the fresh-parse list is used because it is the one that governs
// admission, which is where the expression rules are enforced. TestBuiltinPolicyTypesMatchParser
// pins the agreement so the two cannot drift further apart unnoticed.
var builtinPolicyTypes = map[string]bool{
	v1alpha1.GarbageCollectPolicyType: true,
	v1alpha1.ApplyOncePolicyType:      true,
	v1alpha1.SharedResourcePolicyType: true,
	v1alpha1.TakeOverPolicyType:       true,
	v1alpha1.ReadOnlyPolicyType:       true,
	v1alpha1.ResourceUpdatePolicyType: true,
	v1alpha1.EnvBindingPolicyType:     true,
	v1alpha1.TopologyPolicyType:       true,
	v1alpha1.OverridePolicyType:       true,
	v1alpha1.DebugPolicyType:          true,
	v1alpha1.ReplicationPolicyType:    true,
}

// PolicySurface names the surface a policy's property expressions live on.
//
// There are three kinds, not two, and the middle one is easy to miss: an
// Application-scoped PolicyDefinition is not built-in, but it does not render
// through the workload engine either. Classifying it as rendered skips the
// parse-time substitution it depends on, and its expressions reach the scoped
// render as literal text - which is exactly what happened, and what the e2e spec
// for scoped policies caught.
//
// appScoped cannot be derived from the type alone: it is spec.scope on the
// PolicyDefinition, so every caller must look it up. Built-in types never have a
// definition, so they are settled before the question arises.
//
// Five places need this answer - the parser's surface check and substitution
// pass, and admission's three - and they must all give the same one, or a policy
// is accepted by admission and refused by the parser.
func PolicySurface(policyType string, appScoped bool) string {
	switch {
	case IsBuiltinPolicyType(policyType):
		return sources.SurfacePolicy
	case appScoped:
		return sources.SurfacePolicyApp
	default:
		return sources.SurfacePolicyRendered
	}
}

// PolicyContextSchema is the context a policy is evaluated against.
//
// Paired with PolicySurface for the same reason: the surface and the schema
// describe one call site, and picking them independently is how they came to
// disagree before.
func PolicyContextSchema(policyType string, appScoped bool) propexpr.ContextSchema {
	return propexpr.ContextFor(PolicySurface(policyType, appScoped))
}

// IsBuiltinPolicyType reports a policy KubeVela consumes rather than renders.
//
// Such a policy has no render, so its properties are substituted while the
// appfile is built, from what the Appfile carries - and it can never read a
// source, because there is no resolver on that path.
func IsBuiltinPolicyType(policyType string) bool {
	return builtinPolicyTypes[policyType]
}
