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
	})
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
	out, err := TypedParams(ctx, in)
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
	})
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

// A list unifies by position, so an element that is not knowable cannot be left
// out the way a struct field is.
func TestConcreteForValidationZeroesAListElement(t *testing.T) {
	cuectx := cuecontext.New()
	v := cuectx.CompileString(`{args: ["keep", string], flags: [bool], counts: [int]}`)
	require.NoError(t, v.Err())

	out, changed := ConcreteForValidation(v)
	require.True(t, changed)

	args, err := out.LookupPath(cue.ParsePath("args")).List()
	require.NoError(t, err)
	var got []string
	for args.Next() {
		s, err := args.Value().String()
		require.NoError(t, err)
		got = append(got, s)
	}
	require.Equal(t, []string{"keep", ""}, got, "the length is kept, the value stands in")

	_, err = out.MarshalJSON()
	require.NoError(t, err)
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
