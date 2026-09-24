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
	"context"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const typingTemplate = `
schema: {host: string, port: int, meta: {region: string, zone?: string}, tags: [...string]}
$internal: {key: "cfg", keyInputs: []}
output: {host: "example.com", port: 8080, meta: {region: "eu-west-1"}, tags: ["a"]}
`

func typingContext(t *testing.T) wfprocess.Context {
	t.Helper()
	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{"cfg": {}})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"cfg": "demo"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"demo": typingTemplate})
	return pCtx
}

func TestTypeOnlyMarker(t *testing.T) {
	require.False(t, TypeOnly(context.Background()), "a real render is not a validation")
	var noCtx context.Context
	require.False(t, TypeOnly(noCtx), "no context is not a validation")
	require.True(t, TypeOnly(WithTypeOnly(context.Background())))
}

// Each expression becomes the type its schema declares; everything else is
// carried through untouched.
func TestTypedParamsTypesExpressionsAndLeavesTheRestAlone(t *testing.T) {
	ctx := typingContext(t)

	out, err := TypedParams(ctx, map[string]any{
		"host":     "$(source.cfg.host)",
		"port":     "$(source.cfg.port)",
		"meta":     "$(source.cfg.meta)",
		"tags":     "$(source.cfg.tags)",
		"greeting": "hello $(source.cfg.host)",
		"literal":  "untouched",
		"count":    3,
		"nested":   map[string]any{"inner": "$(source.cfg.host)", "kept": true},
		"list":     []any{"$(source.cfg.port)", "plain"},
	}, SurfaceComponent)
	require.NoError(t, err)

	require.Equal(t, CUEType("string"), out["host"])
	require.Equal(t, CUEType("int"), out["port"])
	require.Equal(t, CUEType("[...string]"), out["tags"])

	// Interpolated into text, so the result is a string whatever the parts are.
	require.Equal(t, CUEType("string"), out["greeting"])

	require.Equal(t, "untouched", out["literal"])
	require.Equal(t, 3, out["count"])

	// A struct read keeps its shape, minus what the schema only optionally
	// promises.
	require.Equal(t, map[string]any{"region": CUEType("string")}, out["meta"])

	require.Equal(t, map[string]any{"inner": CUEType("string"), "kept": true}, out["nested"])
	require.Equal(t, []any{CUEType("int"), "plain"}, out["list"])
}

// Nothing to type means the caller's own map back, not a copy of it.
func TestTypedParamsReturnsTheInputWhenThereIsNothingToType(t *testing.T) {
	ctx := typingContext(t)

	require.Nil(t, mustTyped(t, ctx, nil))

	in := map[string]any{"a": "plain", "b": map[string]any{"c": 1}, "d": []any{"e"}}
	require.Equal(t, in, mustTyped(t, ctx, in))
}

func mustTyped(t *testing.T, ctx wfprocess.Context, in map[string]any) map[string]any {
	t.Helper()
	out, err := TypedParams(ctx, in, SurfaceComponent)
	require.NoError(t, err)
	return out
}

// A computed expression has no schema subtree to read, so its type comes from
// what CEL says the result is.
func TestTypedParamsTypesAComputedExpression(t *testing.T) {
	ctx := typingContext(t)

	out, err := TypedParams(ctx, map[string]any{
		"doubled":  "$(source.cfg.port * 2)",
		"nonsense": "$(source.cfg.nosuchfield)",
	}, SurfaceComponent)
	require.NoError(t, err)

	require.Equal(t, CUEType("int"), out["doubled"])
	// Unknowable means unconstrained, never falsely refused.
	require.Equal(t, CUEType("_"), out["nonsense"])
}

// A type must reach CUE bare; quoting it would make it a string and collide
// with every non-string constraint.
func TestParamsAsCUERendersTypesBareAndValuesAsJSON(t *testing.T) {
	got, err := ParamsAsCUE(map[string]any{
		"typed":   CUEType("int & >0"),
		"text":    "hello",
		"number":  2,
		"yes":     true,
		"nothing": nil,
		"list":    []any{CUEType("string"), "plain"},
		"nested":  map[string]any{"deep": CUEType("bool")},
		"odd key": "quoted",
	})
	require.NoError(t, err)

	// Keys are sorted, so the rendering is stable.
	require.Equal(t,
		`{"list": [string, "plain"], "nested": {"deep": bool}, "nothing": null, "number": 2, "odd key": "quoted", "text": "hello", "typed": int & >0, "yes": true}`,
		got)

	// It has to be CUE that compiles, not just text that looks right.
	v := cuecontext.New().CompileString("parameter: " + got)
	require.NoError(t, v.Err())
	require.NoError(t, v.Validate(cue.Concrete(false)))
}

func TestCelTypeExprNamesCUETypes(t *testing.T) {
	for celType, want := range map[string]string{
		"string":       "string",
		"int":          "int",
		"uint":         "int",
		"double":       "float",
		"bool":         "bool",
		"bytes":        "bytes",
		"null_type":    "null",
		"list(string)": "[...string]",
		"list(int)":    "[...int]",
		"map(string)":  "{...}",
		"dyn":          "_",
		"SomeMessage":  "_",
	} {
		require.Equal(t, want, celTypeExpr(celType), "CEL %q", celType)
	}
}

func TestKindExprNamesEveryKindItHandles(t *testing.T) {
	cuectx := cuecontext.New()
	schema := cuectx.CompileString(`{
	s: string
	i: int
	f: float
	n: number
	b: bool
	l: [...string]
	bare: [...]
	nul: null
	any: _
}`)
	require.NoError(t, schema.Err())

	for name, want := range map[string]string{
		"s": "string", "i": "int", "f": "float", "n": "number", "b": "bool",
		"l": "[...string]", "bare": "[..._]", "nul": "null", "any": "_",
	} {
		field := schema.LookupPath(cue.MakePath(cue.Str(name)))
		require.Equal(t, want, kindExpr(field.IncompleteKind(), field), "field %q", name)
	}
}

// A list unifies by position and cannot lose an element without moving the
// rest, so one unknowable element takes the whole list. A stand-in at that
// index would conflict with whatever a trait patches there.
func TestConcreteForValidationDropsAListWithAnUnknowableElement(t *testing.T) {
	cuectx := cuecontext.New()
	v := cuectx.CompileString(`{args: ["keep", string], flags: [true], name: "web"}`)
	require.NoError(t, v.Err())

	out, changed := ConcreteForValidation(v)
	require.True(t, changed)

	require.False(t, out.LookupPath(cue.ParsePath("args")).Exists(),
		"a list with an unknowable element is left out entirely")

	// A list that is knowable throughout is kept as it was.
	flags, err := out.LookupPath(cue.ParsePath("flags")).List()
	require.NoError(t, err)
	require.True(t, flags.Next())
	kept, err := flags.Value().Bool()
	require.NoError(t, err)
	require.True(t, kept)

	name, err := out.LookupPath(cue.ParsePath("name")).String()
	require.NoError(t, err)
	require.Equal(t, "web", name)

	// The trait that patches the dropped list can now supply it.
	patched := out.Unify(cuectx.CompileString(`{args: ["keep", "real"]}`))
	require.NoError(t, patched.Err())

	_, err = out.MarshalJSON()
	require.NoError(t, err, "the base must still marshal")
}

// The resolver is the choke point: under the marker it types from the schema
// and never reaches the code that would fetch.
func TestResolveSourceExpressionsTypesUnderTheMarker(t *testing.T) {
	ctx := typingContext(t)
	ctx.PushData(process.ContextAppAnnotations, map[string]string{
		oam.AnnotationCelExpressions: "true",
	})
	ctx.SetCtx(WithTypeOnly(context.Background()))

	out, err := ResolveSourceExpressions(ctx, map[string]interface{}{
		"port": "$(source.cfg.port)",
	}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"port": CUEType("int")}, out)

	// Anything that is not a properties map has nothing to type.
	same, err := ResolveSourceExpressions(ctx, []any{"$(source.cfg.port)"}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, []any{"$(source.cfg.port)"}, same)
}

// An expression sees what the definition it feeds sees, so the surface decides
// which context fields have a type at all. Typing every surface as a component
// would quietly stop checking the fields only a trait or policy can read.
func TestTypedParamsTypesContextPerSurface(t *testing.T) {
	ctx := typingContext(t)

	trait, err := TypedParams(ctx, map[string]any{"tt": "$(context.traitType)"}, SurfaceTrait)
	require.NoError(t, err)
	require.Equal(t, CUEType("string"), trait["tt"],
		"a trait reads its own type, so it has one")

	// Not readable on a component, so there is nothing to promise.
	comp, err := TypedParams(ctx, map[string]any{"tt": "$(context.traitType)"}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, CUEType("_"), comp["tt"])

	// A field both surfaces carry is typed on both.
	for _, surface := range []string{SurfaceComponent, SurfaceTrait} {
		out, err := TypedParams(ctx, map[string]any{"an": "$(context.appName)"}, surface)
		require.NoError(t, err)
		require.Equal(t, CUEType("string"), out["an"], "surface %q", surface)
	}
}

// `$$(` is how an author writes a literal `$(`, and the render collapses it.
// Validation has to judge the string the render will produce, or a parameter
// constrained to the collapsed form is refused for a value that renders fine.
func TestTypedParamsCollapsesAnEscape(t *testing.T) {
	ctx := typingContext(t)

	out, err := TypedParams(ctx, map[string]any{
		"escaped": "$$(MY_VAR)",
		"mixed":   "cost: $$(100) and $(source.cfg.host)",
	}, SurfaceComponent)
	require.NoError(t, err)

	require.Equal(t, "$(MY_VAR)", out["escaped"],
		"the escape is collapsed, as the render collapses it")
	require.Equal(t, CUEType("string"), out["mixed"],
		"an escape alongside a real expression still yields a string")
}
