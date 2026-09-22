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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSurfaceNamesAreDeclaredAndSorted(t *testing.T) {
	names := SurfaceNames()
	require.NotEmpty(t, names, "a registry with no surfaces would offer nothing anywhere")
	require.IsIncreasing(t, names, "sorted, because error messages list them")
	for _, n := range names {
		require.True(t, SurfaceDeclared(n), "%s is listed, so it must be declared", n)
	}
	require.False(t, SurfaceDeclared("not-a-surface"))
	require.False(t, SurfaceDeclared(""))
}

// Failing open matters here: a render path not yet taught to name its surface
// should behave as it did before surfaces existed, not lose its context and
// start rejecting valid expressions.
func TestContextForFallsBackToTheComponentContext(t *testing.T) {
	require.Equal(t, ComponentContext, ContextFor("not-a-surface"))
	require.Equal(t, ComponentContext, ContextFor(""))

	for _, surface := range SurfaceNames() {
		got := ContextFor(surface)
		require.NotEmpty(t, got.ReadableFields(), "%s offers no fields at all", surface)
	}
}

func TestSurfaceOffersAgreesWithTheSurfaceSchema(t *testing.T) {
	require.False(t, SurfaceOffers("not-a-surface", "cluster"),
		"an unknown surface offers nothing rather than everything")

	for _, surface := range SurfaceNames() {
		schema := ContextFor(surface)
		for _, field := range schema.ReadableFields() {
			require.True(t, SurfaceOffers(surface, field),
				"%s lists %s as readable, so SurfaceOffers must agree", surface, field)
			_, ok := schema.FieldValue(field)
			require.True(t, ok, "%s.%s is readable, so it must have a declared type", surface, field)
		}
		require.False(t, SurfaceOffers(surface, "noSuchContextField"))
		_, ok := schema.FieldValue("noSuchContextField")
		require.False(t, ok)
	}
}

// The message reads "unavailable in workflow steps", so every surface needs a
// plural and an unknown one has to render as something rather than blank.
func TestSurfacePlural(t *testing.T) {
	for _, surface := range SurfaceNames() {
		require.NotEmpty(t, SurfacePlural(surface))
	}
	require.Equal(t, "not-a-surface", SurfacePlural("not-a-surface"),
		"an unknown surface renders as itself rather than as empty text")
}

// SurfacesOffering is what tells an author where a refused read would work, so
// it has to agree with SurfaceOffers rather than be maintained beside it.
func TestSurfacesOfferingAgreesWithSurfaceOffers(t *testing.T) {
	for _, field := range ComponentContext.ReadableFields() {
		offering := SurfacesOffering(field)
		require.IsIncreasing(t, offering, "%s: sorted for a stable message", field)

		var want []string
		for _, surface := range SurfaceNames() {
			if SurfaceOffers(surface, field) {
				want = append(want, SurfacePlural(surface))
			}
		}
		sort.Strings(want)
		require.Equal(t, want, offering, "%s", field)
	}
	require.Empty(t, SurfacesOffering("noSuchContextField"))
}

// Every field in the registry is either offered somewhere or explicitly
// excluded with a reason. An unclassified field is how a new upstream context
// value silently becomes unreadable with nothing saying why.
func TestKnownFieldCoversEveryDeclaredField(t *testing.T) {
	for _, surface := range SurfaceNames() {
		for _, field := range ContextFor(surface).ReadableFields() {
			require.True(t, knownField(field), "%s is offered but not classified", field)
		}
	}
	require.False(t, knownField("noSuchContextField"))
}
