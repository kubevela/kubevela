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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// A constraint expressed in terms of another field means nothing lifted out of
// its schema: `int & <max` alone either fails to compile or binds to whatever
// the consuming template calls max.
func TestTypeExprFallsBackWhenAConstraintIsNotSelfContained(t *testing.T) {
	ctx := cuecontext.New()
	typer := &paramTyper{cuectx: ctx, compiled: map[string]cue.Value{}}

	schema := ctx.CompileString(`{
	max:    int
	min:    int & <max
	plain:  >0 & int
	choice: "a" | "b"
	simple: string
}`)
	require.NoError(t, schema.Err())

	field := func(name string) cue.Value {
		return schema.LookupPath(cue.MakePath(cue.Str(name)))
	}

	// Self-contained constraints are carried. CUE normalises the ordering, so
	// the text is its own rather than the author's.
	require.Equal(t, "int & >0", typer.typeExprFor(field("plain")))
	require.Equal(t, `"a" | "b"`, typer.typeExprFor(field("choice")))
	require.Equal(t, "string", typer.typeExprFor(field("simple")))

	// One referencing a sibling cannot be evaluated, leaving no kind to fall
	// back to either, so the answer is the permissive one.
	got := typer.typeExprFor(field("min"))
	require.NotContains(t, got, "max", "a cross-field reference must not escape its schema")
	require.Equal(t, "_", got, "unknowable means unconstrained, never falsely refused")
}

// A rendered output carries label and annotation keys that are not bare CUE
// identifiers, and those keys are what a trait patches against. Filling the
// non-concrete leaves must not rewrite them.
func TestConcreteForValidationKeepsQuotedKeys(t *testing.T) {
	cuectx := cuecontext.New()
	v := cuectx.CompileString(`{
	metadata: labels: {
		"app.oam.dev/name": "web"
		plain:              "kept"
	}
	spec: replicas: int
}`)
	require.NoError(t, v.Err())

	out, changed := ConcreteForValidation(v)
	require.True(t, changed, "a non-concrete replicas must be filled")

	labels := out.LookupPath(cue.ParsePath("metadata.labels"))
	require.NoError(t, labels.Err())

	name := labels.LookupPath(cue.MakePath(cue.Str("app.oam.dev/name")))
	require.True(t, name.Exists(), "a dotted label key must survive the fill")
	got, err := name.String()
	require.NoError(t, err)
	require.Equal(t, "web", got)
}
