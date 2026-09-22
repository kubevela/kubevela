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
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// A list-valued parameter written with a default is the ordinary way to declare
// one - `paths: [...string] | *["app.yaml"]` - and admission was rejecting every
// Application that supplied more than the default:
//
//	"spec.sources[1].properties.paths[0]": property "paths.0" is not declared in
//	the parameter schema of SourceDefinition "app-config"
//
// Properties are flattened to dotted leaves, so a two-element list is checked as
// paths.0 and paths.1. Resolving those against a plain [...string] already
// worked; a defaulted disjunction did not, because neither Index nor AnyIndex
// resolves through one.
func TestListParameterWithDefaultAcceptsIndexedLeaves(t *testing.T) {
	for _, tc := range []struct {
		name, schema string
		indices      []string
	}{
		{"plain open list", `parameter: { paths: [...string] }`, []string{"paths.0", "paths.1"}},
		{"open list with default", `parameter: { paths: [...string] | *["app.yaml"] }`, []string{"paths.0", "paths.1"}},
		{"default written first", `parameter: { paths: *["app.yaml"] | [...string] }`, []string{"paths.0", "paths.1"}},
		{"closed list", `parameter: { paths: [string, string] }`, []string{"paths.0", "paths.1"}},
		{"list of structs with default", `parameter: { items: [...{name: string}] | *[] }`, []string{"items.0.name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			v := cuecontext.New().CompileString(tc.schema)
			r.NoError(v.Err())
			cs := &cueStruct{root: v.LookupPath(cue.ParsePath("parameter"))}

			for _, path := range tc.indices {
				kind, declared := cs.kindAt(segs(path))
				r.True(declared, "%s must be declared; admission rejects the Application otherwise", path)
				r.Equal(cue.StringKind, kind, "%s should type as the element type", path)
			}
		})
	}
}

// An index genuinely outside the declared shape still has to be refused, or the
// fix would accept anything with a number in the path.
func TestClosedListRejectsAnIndexPastItsLength(t *testing.T) {
	r := require.New(t)
	v := cuecontext.New().CompileString(`parameter: { pair: [string, string] }`)
	r.NoError(v.Err())
	cs := &cueStruct{root: v.LookupPath(cue.ParsePath("parameter"))}

	_, declared := cs.kindAt(segs("pair.2"))
	r.False(declared, "a closed two-element list has no third element")
}
