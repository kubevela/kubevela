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

package application

import (
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
)

// TestKindsCompatible pins the matrix that decides whether a source's type fits
// the parameter it feeds. It is the feature's central safety claim, and the
// permissive entries are choices rather than accidents - each is here so the
// next reader can tell a deliberate allowance from a hole.
func TestKindsCompatible(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  cue.Kind
		dst  cue.Kind
		want bool
		why  string
	}{
		{"string into string", cue.StringKind, cue.StringKind, true, "the ordinary case"},
		{"int into int", cue.IntKind, cue.IntKind, true, "the ordinary case"},

		{"string into int", cue.StringKind, cue.IntKind, false,
			"the mismatch this check exists to catch"},
		{"int into string", cue.IntKind, cue.StringKind, false, "and its reverse"},
		{"list into struct", cue.ListKind, cue.StructKind, false,
			"a collection is not an object"},
		{"struct into list", cue.StructKind, cue.ListKind, false, "nor the reverse"},
		{"bool into int", cue.BoolKind, cue.IntKind, false, "no numeric coercion from bool"},

		{"int into float", cue.IntKind, cue.FloatKind, true, "int is a subset of float"},
		{"int into number", cue.IntKind, cue.NumberKind, true, "and of number"},

		// Deliberate. CEL types arithmetic as double even when every operand is
		// integral, so an expression like source.cfg.port / 2 is a double feeding
		// an int. Whether the value is really fractional is not knowable before
		// it is fetched; CUE catches that at render.
		{"float into int", cue.FloatKind, cue.IntKind, true,
			"CEL types integral arithmetic as double; render enforces the rest"},
		{"number into int", cue.NumberKind, cue.IntKind, true, "same reason"},

		// Unknown on either side is accepted: an undecidable check must not make
		// an unparseable definition look like a broken Application.
		{"unknown source", cue.BottomKind, cue.IntKind, true, "nothing to compare against"},
		{"unknown target", cue.StringKind, cue.BottomKind, true, "nothing to compare against"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, kindsCompatible(tc.src, tc.dst), tc.why)
		})
	}
}

// TestKindsCompatibleIsSymmetricOnlyForNumbers guards the shape of the rule
// rather than each entry: kind intersection is symmetric, so any asymmetry is a
// numeric special case and should be visible as one.
func TestKindsCompatibleIsSymmetricOnlyForNumbers(t *testing.T) {
	kinds := []cue.Kind{cue.StringKind, cue.IntKind, cue.FloatKind, cue.BoolKind,
		cue.ListKind, cue.StructKind}
	for _, a := range kinds {
		for _, b := range kinds {
			assert.Equalf(t, kindsCompatible(a, b), kindsCompatible(b, a),
				"%v and %v disagree depending on direction", a, b)
		}
	}
}
