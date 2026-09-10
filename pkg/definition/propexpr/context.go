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

package propexpr

import (
	"sort"
	"sync"

	"cuelang.org/go/cue"
)

// ContextIdent is the second identifier an expression may reference, so an
// author can wire a render-context value straight into a parameter:
//
//	properties:
//	  cluster: '$(context.appLabels["cluster-name"])'
//
// KEP-2.16 lists this under Non-Goals: "OAM context fields needed in properties
// should be exposed via a SourceDefinition authored by the platform engineer,
// keeping the resolution model consistent". That rationale is worth re-examining
// rather than assuming, which is what this spike is for - see the devlog.
//
// The important structural difference from a SourceDefinition's context: a
// property expression feeds no cache. A source's context reads are restricted
// because they determine cache identity, and reading an unkeyed value would
// break sharing. Nothing is shared here - the expression is evaluated per render
// - so that constraint simply does not apply.
const ContextIdent = "context"

// ContextSchema is the readable context for one surface: which fields exist,
// with what type, and why the others do not.
//
// It is a view over the registry in context.cue, not a declaration of its own.
// The types are the CUE types written there, so what admission checks and what
// render supplies come from the same place.
type ContextSchema struct {
	// Surface names the schema in error messages.
	Surface string
	// key is the registry's own name for this surface.
	key string
	// value is the composed type for this surface. A zero value means an unknown
	// surface, which reports every field as unavailable.
	value cue.Value
	// excluded explains each field the render context carries that no surface
	// offers, so a new one has to be classified rather than silently ignored.
	excluded map[string]string
}

// ComponentContext is what a ComponentDefinition or TraitDefinition sees.
//
// Membership follows one rule: an expression sees what the definition it is
// feeding sees, at the moment it is rendered. A property expression is
// substituted immediately before the template runs, so the readable set is that
// template's context - not the cache-key rules, which are policy about a
// SourceDefinition's cache identity and curate a different set for a different
// purpose.
//
// That rule also settles context.name. In a SourceDefinition it is the binding
// entry (KEP amendment A4); here it is the component, because that is what a
// ComponentDefinition's context.name is.
var ComponentContext = surfaceSchema("component")

// TraitContext is what a TraitDefinition sees: its component's identity, plus
// its own type. A trait has no instance name in the API to expose alongside it.
var TraitContext = surfaceSchema("trait")

// WorkflowStepContext is what a workflow step's properties see. The step's
// properties are substituted before the engine receives them, from a context
// built the same way a component's is.
var WorkflowStepContext = surfaceSchema("workflowstep")

// PolicyContext is what a resource-rendering policy sees.
//
// Narrower than ScopedPolicyContext because the two policy paths run at
// different times against different data: this one substitutes while the appfile
// is built, before any render, from what the Appfile carries. There is no
// cluster yet and no policy revision metadata.
var PolicyContext = surfaceSchema("policy-default")

// RenderedPolicyContext is what a PolicyDefinition with a CUE template sees.
//
// It renders through the workload engine, so it gets the delivery context a
// component does and can resolve a source - unlike the built-in policies, whose
// properties are read straight off the appfile with no render behind them.
var RenderedPolicyContext = surfaceSchema("policy-rendered")

// ScopedPolicyContext is what an Application-scoped PolicyDefinition sees.
//
// It gets revision metadata and clusterVersion but no cluster: that render
// targets no cluster at all. context.name is omitted on both policy surfaces
// because it means the Application on one path and the policy on the other -
// expressions read appName or policyName, which say what they are.
var ScopedPolicyContext = surfaceSchema("policy-app")

// field returns the declared type of a context field on this surface.
func (c ContextSchema) field(name string) (cue.Value, bool) {
	if !c.value.Exists() {
		return cue.Value{}, false
	}
	v := c.value.LookupPath(cue.MakePath(cue.Str(name)))
	return v, v.Exists()
}

// Offers reports whether this surface makes a context field readable.
func (c ContextSchema) Offers(field string) bool {
	_, ok := c.field(field)
	return ok
}

// FieldValue returns the declared CUE type of a readable context field.
//
// Exported so another type system can be driven from the same registry: celexpr
// needs each surface's fields as CEL DeclTypes, and deriving them from this
// rather than restating them is the whole point of the registry.
func (c ContextSchema) FieldValue(name string) (cue.Value, bool) {
	return c.field(name)
}

// ReadableFields lists the context fields this surface offers, sorted.
//
// Exported so a render path can assert it actually supplies what its surface
// declares. Declaring a field the render omits - or supplies as a permanent
// empty string - type-checks at admission and means nothing at render, which is
// the failure this registry exists to make impossible.
func (c ContextSchema) ReadableFields() []string {
	return c.readable()
}

// readableFields memoises the enumeration below, keyed on the surface.
//
// Every schema comes from surfaceSchema, so a surface's composed type is fixed
// for the life of the process and cue.Value.Fields need only walk it once.
//
// The slice is copied out: a caller that sorts or appends must not reach into
// every later caller's answer.
var readableFields sync.Map // surface key -> []string

func (c ContextSchema) readable() []string {
	if !c.value.Exists() {
		return nil
	}
	if c.key != "" {
		if hit, ok := readableFields.Load(c.key); ok {
			return append([]string(nil), hit.([]string)...)
		}
	}
	iter, err := c.value.Fields()
	if err != nil {
		return nil
	}
	var out []string
	for iter.Next() {
		out = append(out, iter.Selector().Unquoted())
	}
	sort.Strings(out)
	if c.key != "" {
		readableFields.Store(c.key, out)
		return append([]string(nil), out...)
	}
	return out
}
