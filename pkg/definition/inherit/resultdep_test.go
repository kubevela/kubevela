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

package inherit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Properties travel up the chain before results travel back down, so a `$super`
// block cannot be built from what the parent is about to produce. Caught where
// it was written, rather than surfacing later as an unresolved reference far
// from its cause.
func TestSupplyingFromTheParentsResultIsRefused(t *testing.T) {
	child := `
$super: properties: {
	image: $super.output.spec.template.spec.containers[0].image
}

parameter: {}
`
	_, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {}`, contextFile, ComponentSurface, SameCompiler(testCompile()))

	require.Error(t, err)
	require.Contains(t, err.Error(), "depends on what webservice produced")
	require.Contains(t, err.Error(), "may only reference `parameter` and `context`")
}

// An error in the block that is not about the parent's results is still fatal,
// and still attributed to the block holding it rather than to the parent.
func TestAnotherErrorInTheSuperBlockIsAttributedToIt(t *testing.T) {
	child := `
$super: properties: {
	image: parameter.nothingDeclaredHere
}

parameter: {}
`
	_, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {}`, contextFile, ComponentSurface, SameCompiler(testCompile()))

	require.Error(t, err)
	require.NotContains(t, err.Error(), "depends on what")
}

// The scan is for a qualified reference. A bare `output` turns up in plenty of
// errors that have nothing to do with reading a parent's result, and treating
// those as this mistake would send the author looking in the wrong place.
func TestOnlyAQualifiedReferenceCounts(t *testing.T) {
	require.True(t, mentionsResultField("some error about $super.output here"))
	require.True(t, mentionsResultField("and $super.patchOutputs for a trait"))

	require.False(t, mentionsResultField("output: incomplete value"))
	require.False(t, mentionsResultField("field patch not allowed"))
	require.False(t, mentionsResultField(""))
}

func TestEveryComponentAndTraitSurfaceIsScannedFor(t *testing.T) {
	fields := allResultFields()

	for _, f := range append(append([]string{}, ComponentSurface.Fields...), TraitSurface.Fields...) {
		require.Contains(t, fields, f)
	}
}
