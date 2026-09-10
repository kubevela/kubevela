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

package common

import (
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genProps runs the real GenOpenAPI entry point over a CUE template and returns
// the `properties` block of the generated parameter schema.
func genProps(t *testing.T, src string) map[string]any {
	t.Helper()
	val := cuecontext.New().CompileString(src)
	require.NoError(t, val.Err())

	b, err := GenOpenAPI(val)
	require.NoError(t, err)

	var doc struct {
		Components struct {
			Schemas struct {
				Parameter struct {
					Properties map[string]any `json:"properties"`
				} `json:"parameter"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	return doc.Components.Schemas.Parameter.Properties
}

func prop(t *testing.T, props map[string]any, name string) map[string]any {
	t.Helper()
	v, ok := props[name]
	require.Truef(t, ok, "property %q missing from schema", name)
	m, ok := v.(map[string]any)
	require.Truef(t, ok, "property %q is not an object", name)
	return m
}

// TestGenOpenAPIConstraintFallback covers the constraints cuelang's OpenAPI
// encoder rejects. Every case here fails outright without the fallback.
func TestGenOpenAPIConstraintFallback(t *testing.T) {
	testCases := map[string]struct {
		src    string
		verify func(t *testing.T, props map[string]any)
	}{
		"string inequality is dropped but the type survives": {
			src: `parameter: { bucketName: string & !="" }`,
			verify: func(t *testing.T, props map[string]any) {
				p := prop(t, props, "bucketName")
				assert.Equal(t, "string", p["type"])
				assert.NotContains(t, p, "minLength")
			},
		},
		"all five relational operators on strings": {
			src: `parameter: {
	a: string & >"a" & <"z"
	b: string & >="m"
	c: string & <="q"
	d: string & !=""
}`,
			verify: func(t *testing.T, props map[string]any) {
				for _, k := range []string{"a", "b", "c", "d"} {
					assert.Equal(t, "string", prop(t, props, k)["type"], "field %s", k)
				}
			},
		},
		"bytes keeps its binary format instead of becoming a plain string": {
			src: `parameter: { blob: bytes & !='', plain: bytes }`,
			verify: func(t *testing.T, props map[string]any) {
				blob := prop(t, props, "blob")
				assert.Equal(t, "binary", blob["format"])
				assert.Equal(t, prop(t, props, "plain")["format"], blob["format"])
			},
		},
		"a parameter named string does not capture the substituted type": {
			src: `parameter: { string: "hello", name: !="" }`,
			verify: func(t *testing.T, props map[string]any) {
				name := prop(t, props, "name")
				assert.Equal(t, "string", name["type"])
				assert.NotContains(t, name, "enum")
				assert.Equal(t, []any{"hello"}, prop(t, props, "string")["enum"])
			},
		},
		"a nested parameter named string does not capture either": {
			src: `parameter: { inner: { string: "shadowed", name: !="" } }`,
			verify: func(t *testing.T, props map[string]any) {
				inner := prop(t, props, "inner")["properties"].(map[string]any)
				assert.NotContains(t, inner["name"].(map[string]any), "enum")
			},
		},
		"a parameter named bytes does not capture the substituted type": {
			src: `parameter: { bytes: "hi", blob: !='' }`,
			verify: func(t *testing.T, props map[string]any) {
				blob := prop(t, props, "blob")
				assert.Equal(t, "binary", blob["format"])
				assert.NotContains(t, blob, "enum")
			},
		},
		"encodable siblings are left alone": {
			src: `parameter: {
	bad:    string & !=""
	region: *"us-west-2" | string
	class?: "STANDARD" | "GLACIER"
	count:  int & >0 & <10
	rx:     =~"^a.*"
}`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, "us-west-2", prop(t, props, "region")["default"])
				assert.Equal(t, []any{"STANDARD", "GLACIER"}, prop(t, props, "class")["enum"])
				assert.Equal(t, "^a.*", prop(t, props, "rx")["pattern"])
				count := prop(t, props, "count")
				assert.EqualValues(t, 0, count["minimum"])
				assert.EqualValues(t, 10, count["maximum"])
			},
		},
		"doc comments survive the rewrite": {
			src: `parameter: {
	// +usage=Name of the bucket
	bucketName: string & !=""
}`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, "+usage=Name of the bucket", prop(t, props, "bucketName")["description"])
			},
		},
		"maps keep their value type": {
			src: `parameter: { bad: string & !="", labels: [string]: string }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, map[string]any{"type": "string"}, prop(t, props, "labels")["additionalProperties"])
			},
		},
		"open structs keep their ellipsis": {
			src: `parameter: { bad: string & !="", extra: {known: string, ...} }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Contains(t, prop(t, props, "extra"), "additionalProperties")
			},
		},
		"a map whose value carries the bad constraint": {
			src: `parameter: { labels: [string]: string & !="" }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, map[string]any{"type": "string"}, prop(t, props, "labels")["additionalProperties"])
			},
		},
		"referenced definitions still expand": {
			src: `#Port: { port: int, name?: string }
parameter: { p: #Port, bad: string & !="" }`,
			verify: func(t *testing.T, props map[string]any) {
				inner := prop(t, props, "p")["properties"].(map[string]any)
				assert.Contains(t, inner, "port")
				assert.Contains(t, inner, "name")
			},
		},
		"maps, open structs and references together": {
			src: `#Port: { port: int, name?: string }
parameter: {
	bad:    string & !=""
	labels: [string]: string
	extra:  {known: string, ...}
	p:      #Port
}`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, map[string]any{"type": "string"}, prop(t, props, "labels")["additionalProperties"])
				assert.Contains(t, prop(t, props, "extra"), "additionalProperties")
				assert.Contains(t, prop(t, props, "p")["properties"].(map[string]any), "port")
				assert.Equal(t, "string", prop(t, props, "bad")["type"])
			},
		},
		"deeply nested fields are reached": {
			src: `parameter: { a: b: c: d: string & !="" }`,
			verify: func(t *testing.T, props map[string]any) {
				a := prop(t, props, "a")["properties"].(map[string]any)
				b := a["b"].(map[string]any)["properties"].(map[string]any)
				c := b["c"].(map[string]any)["properties"].(map[string]any)
				assert.Equal(t, "string", c["d"].(map[string]any)["type"])
			},
		},
		"defaults on a disjunction with a bad branch": {
			src: `parameter: { a?: *"d" | (string & !="") }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, "d", prop(t, props, "a")["default"])
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tc.verify(t, genProps(t, tc.src))
		})
	}
}

// TestGenOpenAPIUnaffected pins the behaviour of templates the encoder already
// handles. These must never enter the fallback, so their schemas must not change.
func TestGenOpenAPIUnaffected(t *testing.T) {
	testCases := map[string]struct {
		src    string
		verify func(t *testing.T, props map[string]any)
	}{
		"plain types": {
			src: `parameter: { image: string, replicas: *1 | int, port?: int }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, "string", prop(t, props, "image")["type"])
				assert.EqualValues(t, 1, prop(t, props, "replicas")["default"])
				assert.Equal(t, "integer", prop(t, props, "port")["type"])
			},
		},
		"regex, negated regex, bounds and enum": {
			src: `parameter: { rx: =~"^a.*", nrx: string & !~"^x", n: int & >0 & <10, e: "A" | "B" }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.Equal(t, "^a.*", prop(t, props, "rx")["pattern"])
				assert.Contains(t, prop(t, props, "nrx"), "not")
				assert.EqualValues(t, 10, prop(t, props, "n")["maximum"])
				assert.Equal(t, []any{"A", "B"}, prop(t, props, "e")["enum"])
			},
		},
		"builtin string validators encode natively": {
			src: `import "strings"
parameter: { s: strings.MinRunes(3) }`,
			verify: func(t *testing.T, props map[string]any) {
				assert.EqualValues(t, 3, prop(t, props, "s")["minLength"])
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tc.verify(t, genProps(t, tc.src))
		})
	}
}

// TestGenOpenAPIStillFails covers templates the fallback must not rescue. A
// template that does not compile is broken for rendering too, and the caller
// must see the original diagnostic rather than a masked one.
func TestGenOpenAPIStillFails(t *testing.T) {
	testCases := map[string]struct {
		src     string
		wantErr string
	}{
		"unused import":         {src: "import \"strings\"\nparameter: { a: string }", wantErr: "imported and not used"},
		"undefined reference":   {src: `parameter: { a: nope }`, wantErr: `reference "nope" not found`},
		"no parameter at all":   {src: `output: { kind: "Deployment" }`, wantErr: ""},
		"unencodable but unfix": {src: `parameter: { a: string, _hidden: !="" }`, wantErr: ""},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			val := cuecontext.New().CompileString(tc.src)
			if val.Err() != nil {
				assert.Contains(t, val.Err().Error(), tc.wantErr)
				return
			}
			_, err := GenOpenAPI(val)
			if tc.wantErr == "" {
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestGenOpenAPIPreservesOriginalError asserts the invariant that a failing
// fallback never replaces the encoder's own diagnostic.
func TestGenOpenAPIPreservesOriginalError(t *testing.T) {
	// list.MinItems is not imported, so the template cannot compile and the
	// fallback has nothing to rebuild.
	val := cuecontext.New().CompileString(`parameter: { a: [...string] & list.MinItems(1) }`)
	_, err := GenOpenAPI(val)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list")
}

func TestBaseTypeOfLiteral(t *testing.T) {
	testCases := map[string]struct {
		value string
		want  string
	}{
		"double quoted is a string": {value: `""`, want: "__string"},
		"non empty string":          {value: `"abc"`, want: "__string"},
		"multiline string":          {value: `"""x"""`, want: "__string"},
		"single quoted is bytes":    {value: `''`, want: "__bytes"},
		"non empty bytes":           {value: `'\x00'`, want: "__bytes"},
		"multiline bytes":           {value: `'''x'''`, want: "__bytes"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := baseTypeOfLiteral(&ast.BasicLit{Kind: token.STRING, Value: tc.value})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRewriteUnencodableReportsFieldPaths(t *testing.T) {
	testCases := map[string]struct {
		src         string
		wantDropped []string
	}{
		"single field": {
			src:         `parameter: { bucketName: string & !="" }`,
			wantDropped: []string{`bucketName (!="")`},
		},
		"two fields": {
			src:         `parameter: { a: string & !="", b: string & >"m" }`,
			wantDropped: []string{`a (!="")`, `b (>"m")`},
		},
		"nested field reports its full path": {
			src:         `parameter: { cfg: inner: name: string & !="" }`,
			wantDropped: []string{`cfg.inner.name (!="")`},
		},
		"nothing to drop": {
			src:         `parameter: { a: string }`,
			wantDropped: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			val := cuecontext.New().CompileString(tc.src)
			require.NoError(t, val.Err())

			_, dropped, err := rewriteUnencodable(val)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDropped, dropped)
		})
	}
}

func TestGenSanitizedRejectsZeroValue(t *testing.T) {
	_, ok := genSanitized(cue.Value{}, RefineParameterValue,
		&openapi.Config{ExpandReferences: true}, assert.AnError)
	assert.False(t, ok, "a zero cue.Value must not be rewritten")
}

func TestAsOpenAPIFile(t *testing.T) {
	t.Run("file passes through", func(t *testing.T) {
		in := &ast.File{}
		got, err := asOpenAPIFile(in)
		require.NoError(t, err)
		assert.Same(t, in, got)
	})

	t.Run("struct literal becomes a file", func(t *testing.T) {
		got, err := asOpenAPIFile(&ast.StructLit{Elts: []ast.Decl{&ast.EmbedDecl{Expr: ast.NewIdent("string")}}})
		require.NoError(t, err)
		assert.Len(t, got.Decls, 1)
	})

	t.Run("bare expression is embedded", func(t *testing.T) {
		got, err := asOpenAPIFile(ast.NewIdent("string"))
		require.NoError(t, err)
		assert.Len(t, got.Decls, 1)
	})

	t.Run("unsupported node is an error", func(t *testing.T) {
		_, err := asOpenAPIFile(&ast.CommentGroup{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected node")
	})
}

// badValue is a template whose parameter block carries an unencodable constraint,
// used to drive the fallback into its error branches.
const badValue = `parameter: { a: string & !="" }`

func TestGenSanitizedRecoversFromPanic(t *testing.T) {
	val := cuecontext.New().CompileString(badValue)
	require.NoError(t, val.Err())

	panicking := func(cue.Value) (cue.Value, error) { panic("boom") }

	out, ok := genSanitized(val, panicking, &openapi.Config{ExpandReferences: true}, assert.AnError)
	assert.False(t, ok, "a panic in the fallback must not be reported as success")
	assert.Nil(t, out)
}

func TestGenSanitizedKeepsOriginalErrorWhenRefineFails(t *testing.T) {
	val := cuecontext.New().CompileString(badValue)
	require.NoError(t, val.Err())

	failing := func(cue.Value) (cue.Value, error) { return cue.Value{}, assert.AnError }

	out, ok := genSanitized(val, failing, &openapi.Config{ExpandReferences: true}, assert.AnError)
	assert.False(t, ok)
	assert.Nil(t, out)
}

func TestRewriteAndGenReportsNothingDropped(t *testing.T) {
	val := cuecontext.New().CompileString(`parameter: { a: string }`)
	require.NoError(t, val.Err())

	_, _, err := rewriteAndGen(val, RefineParameterValue, &openapi.Config{ExpandReferences: true})
	assert.ErrorIs(t, err, errNothingDropped)
}

// TestGenOpenAPIFallbackWithSurroundingDecls drives templates that carry imports
// or hidden fields alongside the offending constraint.
func TestGenOpenAPIFallbackWithSurroundingDecls(t *testing.T) {
	testCases := map[string]string{
		"imported builtin alongside a bad constraint": `import "strings"
parameter: { a: string & !="", b: strings.MinRunes(2) }`,
		"hidden field alongside a bad constraint": `_hidden: "x"
parameter: { a: string & !="" }`,
	}

	for name, src := range testCases {
		t.Run(name, func(t *testing.T) {
			props := genProps(t, src)
			assert.Equal(t, "string", prop(t, props, "a")["type"])
		})
	}
}

func TestGenOpenAPIWithCueXConstraintFallback(t *testing.T) {
	val := cuecontext.New().CompileString(`parameter: { bucketName: string & !="" }`)
	require.NoError(t, val.Err())

	b, err := GenOpenAPIWithCueX(val)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"bucketName"`)
	assert.NotContains(t, string(b), "minLength")
}

// TestGenOpenAPIUnfixableFailureKeepsOriginalError covers a template the encoder
// rejects for a reason the rewrite cannot address. Both tiers must decline and
// the caller must receive the encoder's own diagnostic.
func TestGenOpenAPIUnfixableFailureKeepsOriginalError(t *testing.T) {
	val := cuecontext.New().CompileString(`parameter: { a: [string, int] }`)
	require.NoError(t, val.Err())

	_, err := GenOpenAPI(val)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert incomplete value")
}

// TestGenOpenAPINumericOperatorsUntouched pins that relational operators on
// numbers are encodable and must never be rewritten.
func TestGenOpenAPINumericOperatorsUntouched(t *testing.T) {
	props := genProps(t, `parameter: { a: number & !=1.5, b: int & >=2 }`)
	assert.Contains(t, prop(t, props, "a"), "not")
	assert.EqualValues(t, 2, prop(t, props, "b")["minimum"])
}
