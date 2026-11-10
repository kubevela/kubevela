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
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/encoding/openapi"
	"github.com/stretchr/testify/require"
)

// parameterSchema runs GenOpenAPI on a template and returns the parameter schema.
func parameterSchema(t *testing.T, template string) map[string]any {
	t.Helper()
	b, err := GenOpenAPI(cuecontext.New().CompileString(template))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	return doc["components"].(map[string]any)["schemas"].(map[string]any)["parameter"].(map[string]any)
}

func prop(s map[string]any, path ...string) map[string]any {
	for _, p := range path {
		s = s["properties"].(map[string]any)[p].(map[string]any)
	}
	return s
}

// Numbers with both a default and a bound failed with "unsupported op for
// number &" (#5767).
func TestGenOpenAPIBoundedDefault(t *testing.T) {
	cases := map[string]struct {
		template string
		field    string
		want     map[string]any
	}{
		"issue 5767": {
			template: `parameter: { terminationGracePeriodSeconds: *30 | >=1 & <=3599 & int }`,
			field:    "terminationGracePeriodSeconds",
			want:     map[string]any{"type": "integer", "default": float64(30), "minimum": float64(1), "maximum": float64(3599)},
		},
		"int with both bounds": {
			template: `parameter: { instances: *1 | int & >=1 & <=5 }`,
			field:    "instances",
			want:     map[string]any{"type": "integer", "default": float64(1), "minimum": float64(1), "maximum": float64(5)},
		},
		"one bound": {
			template: `parameter: { replicas: *2 | int & >=0 }`,
			field:    "replicas",
			want:     map[string]any{"type": "integer", "default": float64(2), "minimum": float64(0)},
		},
		"float bounds": {
			template: `parameter: { ratio: *0.5 | >=0 & <=1 }`,
			field:    "ratio",
			want:     map[string]any{"type": "number", "default": 0.5, "minimum": float64(0), "maximum": float64(1)},
		},
		"negative default": {
			template: `parameter: { offset: *-1 | int & >=-10 }`,
			field:    "offset",
			want:     map[string]any{"type": "integer", "default": float64(-1), "minimum": float64(-10)},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, c.want, prop(parameterSchema(t, c.template), c.field))
		})
	}
}

// Bounded defaults inside pattern fields and struct disjunctions are restored too.
func TestGenOpenAPIBoundedDefaultInPatternsAndDisjunctions(t *testing.T) {
	s := parameterSchema(t, `
parameter: {
	limits: [string]: {
		cpu: *2 | int & >=1 & <=64
	}
	volume: {type: "pvc", size: *10 | int & >=1} | {type: "emptyDir", limit: *1 | int & >=1 & <=8}
}
`)
	cpu := prop(s, "limits")["additionalProperties"].(map[string]any)["properties"].(map[string]any)["cpu"].(map[string]any)
	require.Equal(t, float64(2), cpu["default"])
	require.Equal(t, float64(64), cpu["maximum"])
	volume := prop(s, "volume")
	require.Equal(t, float64(10), prop(volume, "size")["default"])
	require.Equal(t, float64(1), prop(volume, "limit")["default"])
	require.Equal(t, float64(8), prop(volume, "limit")["maximum"])
}

// The rest of a schema is unchanged by the fallback: helpers are inlined,
// descriptions kept, other defaults kept, list items and imports handled.
func TestGenOpenAPIBoundedDefaultKeepsTheRest(t *testing.T) {
	template := `
import "strings"

parameter: {
	// +usage=How the service is probed
	probe?: #Probe
	// +usage=Containers to run
	containers: [...{
		name: string & strings.MinRunes(1)
		// +usage=Port to listen on
		port: *8080 | int & >=1 & <=65535
	}]
	// +usage=Pull policy
	pullPolicy: *"IfNotPresent" | "Always" | "Never"
	replicas: *1 | int & >=1
}
#Probe: {
	// +usage=Port the probe calls
	port: *80 | int & >=1 & <=65535
	path: *"/" | string
}
`
	s := parameterSchema(t, template)
	probe := prop(s, "probe")
	require.Equal(t, "+usage=How the service is probed", probe["description"])
	require.Equal(t, "+usage=Port the probe calls", prop(probe, "port")["description"])
	require.Equal(t, float64(80), prop(probe, "port")["default"])
	require.Equal(t, float64(65535), prop(probe, "port")["maximum"])
	require.Equal(t, "/", prop(probe, "path")["default"])

	item := prop(s, "containers")["items"].(map[string]any)
	require.Equal(t, "+usage=Port to listen on", prop(item, "port")["description"])
	require.Equal(t, float64(8080), prop(item, "port")["default"])
	require.Equal(t, "string", prop(item, "name")["type"], "types next to package calls are kept")

	require.Equal(t, "IfNotPresent", prop(s, "pullPolicy")["default"])
	require.Equal(t, []any{"IfNotPresent", "Always", "Never"}, prop(s, "pullPolicy")["enum"])
	require.Equal(t, float64(1), prop(s, "replicas")["default"])
}

// For templates that encode without error, the fallback must produce the same
// document, so taking it can never change a schema beyond adding what failed.
func TestEncodeWithoutNumberDefaultsMatchesEncoder(t *testing.T) {
	templates := map[string]string{
		"helpers, lists, enums, maps": `
parameter: {
	// +usage=Image to run
	image: string
	imagePullPolicy?: "Always" | "Never" | "IfNotPresent"
	replicas: *1 | int
	cpu?: *"0.5" | string
	// +usage=Health probe
	livenessProbe?: #HealthProbe
	ports?: [...{
		// +usage=Port number
		port: int
		protocol: *"TCP" | "UDP"
		probe?: #HealthProbe
	}]
	labels?: [string]: string
}
#HealthProbe: {
	// +usage=Path to call
	path: *"/" | string
	periodSeconds: *10 | int
	exec?: {command: [...string]}
}
`,
		"struct disjunctions": `
parameter: {
	volume?: {type: "pvc", claimName: string} | {type: "emptyDir", medium: *"" | "memory"}
	mode: *{a: 1} | {b: int}
	either: {a?: int} | {b?: int}
	items: [...({t: "x", a: int} | {t: "y", b: int})]
}
`,
	}
	for name, src := range templates {
		t.Run(name, func(t *testing.T) {
			template := cuecontext.New().CompileString(src)
			require.NoError(t, template.Err())
			params, err := RefineParameterValue(template)
			require.NoError(t, err)
			want, err := openapi.Gen(params, expandRefs)
			require.NoError(t, err)
			got, err := encodeWithoutNumberDefaults(template, params, RefineParameterValue)
			require.NoError(t, err)
			requireSameJSON(t, want, got)
		})
	}
}

func requireSameJSON(t *testing.T, want, got []byte) {
	t.Helper()
	var w, g any
	require.NoError(t, json.Unmarshal(want, &w))
	require.NoError(t, json.Unmarshal(got, &g))
	require.Equal(t, w, g)
}

// Errors other than the bounded-default one are returned as the encoder reported them.
func TestEncodeOpenAPIKeepsOtherErrors(t *testing.T) {
	template := cuecontext.New().CompileString(`parameter: { a: int }`)
	params := cuecontext.New().CompileString(`#parameter: { a: int, a: string }`)
	_, err := encodeOpenAPI(template, params, RefineParameterValue)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), boundedDefaultError))
}

// A bound that refers to another field of the template is printed evaluated,
// so the parameter still rebuilds on its own.
func TestGenOpenAPIBoundedDefaultFromAnotherField(t *testing.T) {
	n := prop(parameterSchema(t, `
_max: 5
parameter: { n: *1 | int & >=1 & <=_max }
`), "n")
	require.Equal(t, float64(1), n["default"])
	require.Equal(t, float64(5), n["maximum"])
}

// Only number defaults with a bound beside them are taken out; the encoder
// handles the rest, and a struct default must keep concrete values.
func TestWithoutNumberDefault(t *testing.T) {
	cases := map[string]bool{
		`*1 | int & >=1`:           true,
		`*-2 | int & >=-5`:         true,
		`*0.5 | number & <=1`:      true,
		`*1 | (int & >=1 & <=5)`:   true,
		`*1 | *2 | int & >=0`:      true,
		`*30 | >=1 & <=3599 & int`: true,
		`*-2 | int`:                false,
		`*0.5 | number`:            false,
		`*1 | *2 | int`:            false,
		`*"a" | string`:            false,
		`*"a" | string & !="b"`:    false,
		`*{a: 1} | {b: int}`:       false,
		`"Always" | "Never"`:       false,
	}
	for src, want := range cases {
		t.Run(src, func(t *testing.T) {
			f := mustParseExpr(t, src)
			_, removed := withoutNumberDefault(f)
			require.Equal(t, want, removed)
		})
	}
}

func mustParseExpr(t *testing.T, src string) *ast.BinaryExpr {
	t.Helper()
	e, err := parser.ParseExpr("test", src)
	require.NoError(t, err)
	b, ok := e.(*ast.BinaryExpr)
	require.True(t, ok, "%s is not a binary expression", src)
	return b
}
