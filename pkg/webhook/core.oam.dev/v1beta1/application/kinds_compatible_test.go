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
	celengine "github.com/kubevela/pkg/cel"
	"github.com/stretchr/testify/assert"
)

// TestLiteralKinds pins how a literal property value is judged against the
// parameter it feeds: by the same rule as an expression's result.
func TestLiteralKinds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		dst   cue.Kind
		want  bool
	}{
		{"string into string", "x", cue.StringKind, true},
		{"integral number into int", float64(3), cue.IntKind, true},
		{"integral number into float", float64(3), cue.FloatKind, true},
		{"integral number into number", float64(3), cue.NumberKind, true},
		{"fraction into int", 1.5, cue.IntKind, false},
		{"fraction into number", 1.5, cue.NumberKind, true},
		{"string into int", "three", cue.IntKind, false},
		{"bool into int", true, cue.IntKind, false},
		{"list into struct", []interface{}{}, cue.StructKind, false},
		{"struct into list", map[string]interface{}{}, cue.ListKind, false},
		{"unknown target", "x", cue.BottomKind, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, celengine.KindFits(jsonKind(tc.value), tc.dst))
		})
	}
}
