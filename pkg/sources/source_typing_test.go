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

// A schema's optional field is not a guarantee, so the shape must not claim it.
// The required-field check flattens this map and treats every key it finds as
// provided, so claiming an optional one lets a component demand something the
// source may never supply.
func TestShapeOfDoesNotClaimOptionalFields(t *testing.T) {
	ctx := cuecontext.New()
	typer := &paramTyper{cuectx: ctx, compiled: map[string]cue.Value{}}

	schema := ctx.CompileString(`{meta: {region?: string, zone: string}}`)
	require.NoError(t, schema.Err())

	shape, ok := typer.shapeOf(schema.LookupPath(cue.MakePath(cue.Str("meta")))).(map[string]any)
	require.True(t, ok, "a struct field keeps its shape")

	require.Contains(t, shape, "zone", "a guaranteed field is carried")
	require.NotContains(t, shape, "region", "an optional field is not a guarantee")
}

// Reading a whole binding is still a bare read, so it is typed from the
// binding's own schema rather than falling through to an opaque CEL type.
func TestFromSchemaTypesAWholeBindingRead(t *testing.T) {
	ctx := cuecontext.New()
	typer := &paramTyper{
		cuectx:   ctx,
		compiled: map[string]cue.Value{},
		schemas:  map[string]string{"cfg": `{host: string, port: int}`},
		ready:    true,
	}

	shaped, ok := typer.fromSchema("source.cfg")
	require.True(t, ok, "a whole-binding read has a schema to judge by")

	shape, ok := shaped.(map[string]any)
	require.True(t, ok, "the binding's schema is a struct, so the shape is one too")
	require.Equal(t, CUEType("string"), shape["host"])
	require.Equal(t, CUEType("int"), shape["port"])
}

// The workload output becomes the base a trait patches against. A leaf the
// source feeds is not knowable here, and a zero value in its place conflicts
// with every literal a trait might patch in, refusing an Application that
// renders fine. Leaving it out unifies with anything.
func TestConcreteForValidationLeavesRoomForATraitPatch(t *testing.T) {
	cuectx := cuecontext.New()
	v := cuectx.CompileString(`{
	spec: {
		replicas: 2
		template: spec: containers: [{name: "c", image: string}]
	}
}`)
	require.NoError(t, v.Err())

	out, changed := ConcreteForValidation(v)
	require.True(t, changed)

	image := out.LookupPath(cue.ParsePath("spec.template.spec.containers[0].image"))
	require.False(t, image.Exists(), "an unknowable leaf is left out, not zeroed")

	replicas, err := out.LookupPath(cue.ParsePath("spec.replicas")).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 2, replicas, "a known value is kept")

	// What the trait render actually does with the base.
	patched := out.Unify(cuectx.CompileString(`{
	spec: template: spec: containers: [{name: "c", image: "nginx"}]
}`))
	require.NoError(t, patched.Err(), "a trait must be able to patch the field")
	require.NoError(t, patched.Validate(cue.Concrete(true)))

	// The engine hands the base to the next template as JSON.
	_, err = out.MarshalJSON()
	require.NoError(t, err, "the base must still marshal")
}
