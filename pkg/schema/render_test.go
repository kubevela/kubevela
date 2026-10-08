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

package schema

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	uischema "github.com/oam-dev/kubevela/pkg/utils/schema"
)

func generate(t *testing.T, src string) *ParameterSchemas {
	t.Helper()
	ps, err := GenerateParameterSchemas(context.Background(), src)
	require.NoError(t, err)
	return ps
}

func uiParam(t *testing.T, ps uischema.UISchema, key string) *uischema.UIParameter {
	t.Helper()
	for _, p := range ps {
		if p.JSONKey == key {
			return p
		}
	}
	t.Fatalf("no UI parameter %q", key)
	return nil
}

func TestGenerateConditionalFields(t *testing.T) {
	ps := generate(t, `parameter: {
	type: "something" | "other" | "third"
	if type == "something" { key: string }
	if type != "something" { aDifferentKey: string }
}`)
	assert.Equal(t, "Select", uiParam(t, ps.UI, "type").UIType)
	assert.Equal(t, []uischema.Condition{{JSONKey: "type", Value: "something"}}, uiParam(t, ps.UI, "key").Conditions)
	assert.Equal(t, []uischema.Condition{{JSONKey: "type", Op: "in", Value: []any{"other", "third"}}},
		uiParam(t, ps.UI, "aDifferentKey").Conditions)

	assert.Equal(t, []string{"type"}, ps.OpenAPI.Required)
	require.NotNil(t, ps.OpenAPI.Discriminator)
	assert.Equal(t, "type", ps.OpenAPI.Discriminator.PropertyName)
	require.Len(t, ps.OpenAPI.OneOf, 3)
	assert.Equal(t, []string{"key"}, ps.OpenAPI.OneOf[0].Value.Required)
	assert.Equal(t, []string{"aDifferentKey"}, ps.OpenAPI.OneOf[1].Value.Required)
}

func TestGenerateNestedAndBoolConditions(t *testing.T) {
	ps := generate(t, `parameter: {
	enabled: *false | bool
	if enabled { port: *80 | int }
	storage: {
		kind: "pvc" | "emptyDir"
		if kind == "pvc" { size: string }
	}
}`)
	assert.Equal(t, []uischema.Condition{{JSONKey: "enabled", Value: true}}, uiParam(t, ps.UI, "port").Conditions)
	storage := uiParam(t, ps.UI, "storage")
	assert.Equal(t, []uischema.Condition{{JSONKey: "kind", Value: "pvc"}}, uiParam(t, storage.SubParameters, "size").Conditions)
}

func TestGenerateRecursiveDefinition(t *testing.T) {
	ps := generate(t, "#T: {n?: #T, v?: string}\nparameter: a: #T")
	n := ps.OpenAPI.Properties["a"].Value.Properties["n"].Value
	require.NotNil(t, n.AdditionalProperties.Has)
	assert.True(t, *n.AdditionalProperties.Has)
}

func TestGenerateShapesTheEncoderRejects(t *testing.T) {
	ps := generate(t, `
import "list"

#Port: {
	// +usage=Container port
	port:     int & >=1 & <=65535
	protocol: *"TCP" | "UDP"
}
parameter: {
	replicas: *1 | int & >=1
	level:    *2 | 1 | 3
	ne:       int & !=0
	either:   string | int
	ports:    *[{port: 80}] | [...#Port]
	items:    [...string] & list.MinItems(1)
}`)
	replicas := ps.OpenAPI.Properties["replicas"].Value
	assert.Equal(t, int64(1), replicas.Default)
	assert.Equal(t, 1.0, *replicas.Min)
	assert.NotContains(t, ps.OpenAPI.Required, "replicas", "a defaulted field need not be supplied")

	assert.Equal(t, "Select", uiParam(t, ps.UI, "level").UIType)
	assert.Equal(t, []any{int64(0)}, ps.OpenAPI.Properties["ne"].Value.Not.Value.Enum)
	for _, b := range ps.OpenAPI.Properties["either"].Value.OneOf {
		assert.NotNil(t, b.Value.Type)
	}
	ports := ps.OpenAPI.Properties["ports"].Value
	assert.Equal(t, []any{map[string]any{"port": int64(80)}}, ports.Default)
	assert.Equal(t, "Container port", ports.Items.Value.Properties["port"].Value.Description)
	assert.Equal(t, uint64(1), ps.OpenAPI.Properties["items"].Value.MinItems)
	assert.Equal(t, "Strings", uiParam(t, ps.UI, "items").UIType)
}

func TestGenerateDeclarationOrder(t *testing.T) {
	ps := generate(t, `parameter: {zeta: string, alpha?: int, mid: *true | bool}`)
	var keys []string
	for _, p := range ps.UI {
		keys = append(keys, p.JSONKey)
	}
	assert.Equal(t, []string{"zeta", "alpha", "mid"}, keys)
}

func TestGenerateDefaultedAndMultipleDiscriminators(t *testing.T) {
	ps := generate(t, `parameter: {
	image: string
	type: *"something" | "other" | "third"
	if type == "something" { key: string }
	if type != "something" { aDifferentKey: string }
	metrics: *false | bool
	if metrics { metricsPort: *9090 | int }
	ports: [...int]
}`)
	var keys []string
	for _, p := range ps.UI {
		keys = append(keys, p.JSONKey)
	}
	assert.Equal(t, []string{"image", "type", "key", "aDifferentKey", "metrics", "metricsPort", "ports"}, keys)
	assert.Equal(t, []uischema.Condition{{JSONKey: "type", Value: "something"}}, uiParam(t, ps.UI, "key").Conditions,
		"a field of the default branch is still conditional")
	assert.Equal(t, []uischema.Condition{{JSONKey: "metrics", Value: true}}, uiParam(t, ps.UI, "metricsPort").Conditions)
	assert.Len(t, ps.OpenAPI.AllOf, 2)
}

func conditionsOf(t *testing.T, ps uischema.UISchema, path ...string) []uischema.Condition {
	t.Helper()
	p := uiParam(t, ps, path[0])
	for _, key := range path[1:] {
		p = uiParam(t, p.SubParameters, key)
	}
	return p.Conditions
}

func TestGenerateNestedConditions(t *testing.T) {
	cases := map[string]struct {
		src  string
		path []string
		want []uischema.Condition
	}{
		"inside a list item": {
			src:  `parameter: ports: [...{type: "a" | "b", if type == "a" {x: string}}]`,
			path: []string{"ports", "x"},
			want: []uischema.Condition{{JSONKey: "type", Value: "a"}},
		},
		"on a field of the enclosing object": {
			src:  `parameter: {mode: "simple" | "advanced", storage: {size: string, if mode == "advanced" {iops: int}}}`,
			path: []string{"storage", "iops"},
			want: []uischema.Condition{{JSONKey: "../mode", Value: "advanced"}},
		},
		"on a field of a child object": {
			src:  `parameter: {storage: {kind: "pvc" | "emptyDir"}, if storage.kind == "pvc" {pvcName: string}}`,
			path: []string{"pvcName"},
			want: []uischema.Condition{{JSONKey: "storage.kind", Value: "pvc"}},
		},
		"inside another condition": {
			src:  `parameter: {type: "x" | "y", if type == "x" {mode: "a" | "b", if mode == "a" {deep: string}}}`,
			path: []string{"deep"},
			want: []uischema.Condition{{JSONKey: "type", Value: "x"}, {JSONKey: "mode", Value: "a"}},
		},
		"on a field being set": {
			src:  `parameter: {tls?: {secret: string}, if tls != _|_ {port: *443 | int}}`,
			path: []string{"port"},
			want: []uischema.Condition{{JSONKey: "tls", Op: "!=", Value: nil}},
		},
		"in a definition used as a field": {
			src:  "#S: {kind: \"pvc\" | \"e\", if kind == \"pvc\" {size: string}}\nparameter: primary: #S",
			path: []string{"primary", "size"},
			want: []uischema.Condition{{JSONKey: "kind", Value: "pvc"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, conditionsOf(t, generate(t, tc.src).UI, tc.path...))
		})
	}
}

func TestGenerateSameNameDifferentShapes(t *testing.T) {
	ps := generate(t, `parameter: {type: "a" | "b", if type == "a" {v: string}, if type == "b" {v: int}}`)
	var shapes []string
	for _, p := range ps.UI {
		if p.JSONKey == "v" {
			shapes = append(shapes, p.UIType)
		}
	}
	assert.Equal(t, []string{"Input", "Number"}, shapes)
	assert.Len(t, ps.OpenAPI.Properties["v"].Value.OneOf, 2)
}

func TestGenerateConditionalFieldOrder(t *testing.T) {
	ps := generate(t, `parameter: {kind: "pvc" | "e", if kind == "pvc" {size: string, class?: string, zone?: string}}`)
	var keys []string
	for _, p := range ps.UI {
		keys = append(keys, p.JSONKey)
	}
	assert.Equal(t, []string{"kind", "size", "class", "zone"}, keys)
}

func TestGenerateRejectsInvalidParameter(t *testing.T) {
	_, err := GenerateParameterSchemas(context.Background(), `parameter: example: *"default"`)
	assert.Error(t, err)
}

func TestGenerateShapeDescriptions(t *testing.T) {
	ps := generate(t, `parameter: {
	probe: "http" | *"exec"
	if probe == "http" {
		// +usage=URL path to probe
		target: *"/healthz" | string
	}
	if probe == "exec" {
		// +usage=Command to run
		target: [...string]
	}
}`)
	got := map[string]string{}
	for _, p := range ps.UI {
		if p.JSONKey == "target" {
			got[p.UIType] = p.Description
		}
	}
	assert.Equal(t, map[string]string{"Input": "URL path to probe", "Strings": "Command to run"}, got)
}

func TestGenerateUIHints(t *testing.T) {
	ps := generate(t, `parameter: {
	// +usage=Container image
	// +ui:placeholder=nginx:1.25
	// +ui:colSpan=12
	image: string
	// +ui:type=Password
	// +ui:label=Access token
	token: string
	// +ui:advanced
	debug: *false | bool
	// +ignore
	cliOnly?: string
	// +ui:hidden
	secret?: string
	// +ui:order=0
	name: string
	// +usage=Ports to expose
	// +ui:format=table
	// +ui:rowKey=name
	ports: [...{name: string, port: int}]
	// +ui:unknownHint=ignored
	extra?: string
}`)
	var keys []string
	for _, p := range ps.UI {
		keys = append(keys, p.JSONKey)
	}
	assert.Equal(t, "name", keys[0], "an explicit order goes ahead of declaration order")

	image := uiParam(t, ps.UI, "image")
	assert.Equal(t, "Container image", image.Description, "hint lines are not part of the description")
	assert.Equal(t, &uischema.Style{ColSpan: 12, Placeholder: "nginx:1.25"}, image.Style)

	token := uiParam(t, ps.UI, "token")
	assert.Equal(t, "Password", token.UIType)
	assert.Equal(t, "Access token", token.Label)

	assert.True(t, uiParam(t, ps.UI, "debug").Style.Advanced)
	assert.Nil(t, uiParam(t, ps.UI, "cliOnly").Disable, "+ignore hides a field from the CLI, not the UI")
	assert.True(t, *uiParam(t, ps.UI, "secret").Disable)
	assert.Equal(t, &uischema.Style{Format: "table", RowKey: "name"}, uiParam(t, ps.UI, "ports").Style)
	assert.Nil(t, uiParam(t, ps.UI, "extra").Style)

	assert.Equal(t, UIHints{Format: "table", RowKey: "name"}, ps.OpenAPI.Properties["ports"].Value.Extensions[ExtensionUI])
}

func TestGenerateSuggestions(t *testing.T) {
	ps := generate(t, `parameter: {
	lang: "go" | "java" | string
	// +ui:suggest=eu-west-1, us-east-1
	region: string
}`)
	lang := uiParam(t, ps.UI, "lang")
	assert.Equal(t, "Suggest", lang.UIType)
	assert.Len(t, lang.Validate.Options, 2)
	region := uiParam(t, ps.UI, "region")
	assert.Equal(t, "Suggest", region.UIType)
	assert.Equal(t, "us-east-1", region.Validate.Options[1].Value)
}

func TestGenerateStructMap(t *testing.T) {
	ps := generate(t, `parameter: sidecars: [string]: {image: string, cpu?: string}`)
	p := uiParam(t, ps.UI, "sidecars")
	assert.Equal(t, "StructMap", p.UIType)
	assert.Len(t, p.SubParameters, 2)
}

func TestGenerateConditionsAcrossCollections(t *testing.T) {
	ps := generate(t, `parameter: {
	mode: "simple" | "advanced"
	ports: *[{port: 80}] | [...{port: int, if mode == "advanced" {appProtocol?: string}}]
	deep: {a: {b: {kind: "x" | "y"}}}
	if deep.a.b.kind == "x" {note: string}
}`)
	assert.Equal(t, []uischema.Condition{{JSONKey: "../mode", Value: "advanced"}},
		conditionsOf(t, ps.UI, "ports", "appProtocol"))
	assert.Equal(t, []uischema.Condition{{JSONKey: "deep.a.b.kind", Value: "x"}}, conditionsOf(t, ps.UI, "note"))
}

func TestGenerateParameterSchemasAt(t *testing.T) {
	ps, err := GenerateParameterSchemasAt(context.Background(), `
metadata: name: "demo"
template: parameter: {
	kind: "token" | "basic"
	if kind == "token" {
		// +usage=Access token
		token: string
	}
	if kind == "basic" {
		// +usage=User name
		user: string
	}
}
`, "template")
	require.NoError(t, err)
	assert.Equal(t, "Access token", uiParam(t, ps.UI, "token").Description)
	assert.Equal(t, []uischema.Condition{{JSONKey: "kind", Value: "basic"}}, uiParam(t, ps.UI, "user").Conditions)
}

func TestPlaceByOrder(t *testing.T) {
	two, zero, nine := 2, 0, 9
	fields := []*Field{{Name: "a"}, {Name: "b", UI: UIHints{Order: &two}}, {Name: "c"}, {Name: "d", UI: UIHints{Order: &zero}}, {Name: "e", UI: UIHints{Order: &nine}}}
	var names []string
	for _, f := range placeByOrder(fields) {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"d", "a", "b", "c", "e"}, names)
}

func TestGenerateConditionInsideDefaultedList(t *testing.T) {
	ps := generate(t, `parameter: ports: *[{type: "a", x: "1"}] | [...{type: "a" | "b", if type == "a" {x: string}, if type == "b" {y: int}}]`)
	assert.Equal(t, []uischema.Condition{{JSONKey: "type", Value: "b"}}, conditionsOf(t, ps.UI, "ports", "y"))
}

func TestGeneratePatterns(t *testing.T) {
	ps := generate(t, `
import "strings"

#Name: string & =~"^[a-z]+$"
parameter: {
	plain:  string & =~"^[a-z]+$"
	either: =~"^a" | =~"^b" | "exact.value"
	open:   =~"^a" | string
	viaDef: #Name & strings.MaxRunes(5)
	items: [...string & =~"^[0-9]+$"]
}`)
	pattern := func(key string) string { return ps.OpenAPI.Properties[key].Value.Pattern }
	assert.Equal(t, "^[a-z]+$", pattern("plain"))
	assert.Equal(t, `(?:^a)|(?:^b)|(?:^exact\.value$)`, pattern("either"))
	assert.Equal(t, "", pattern("open"), "a plain string branch matches anything")
	assert.Equal(t, "^[a-z]+$", pattern("viaDef"))
	assert.Equal(t, uint64(5), *ps.OpenAPI.Properties["viaDef"].Value.MaxLength)
	assert.Equal(t, "^[0-9]+$", ps.OpenAPI.Properties["items"].Value.Items.Value.Pattern)
	assert.Equal(t, "^[a-z]+$", uiParam(t, ps.UI, "viaDef").Validate.Pattern)
}

func TestGenerateErrorSectionOptionsFrom(t *testing.T) {
	ps := generate(t, `parameter: {
	// +usage=Service name
	// +ui:error=Lowercase letters and dashes only
	name: string & =~"^[a-z-]+$"
	// +ui:section=Networking
	port: *80 | int
	// +ui:section=Networking
	// +ui:optionsFrom=configs:image-registry
	registry?: string
	// +ui:expression=never
	literal?: string
}`)
	name := uiParam(t, ps.UI, "name")
	assert.Equal(t, "Lowercase letters and dashes only", name.Validate.Message)
	assert.Equal(t, "^[a-z-]+$", name.Validate.Pattern)
	assert.Equal(t, "Networking", uiParam(t, ps.UI, "port").Style.Section)
	registry := uiParam(t, ps.UI, "registry")
	assert.Equal(t, &uischema.Style{Section: "Networking", OptionsFrom: "configs:image-registry"}, registry.Style)
	assert.Equal(t, "never", uiParam(t, ps.UI, "literal").Style.Expression)
}

func TestGenerateSourceSchemas(t *testing.T) {
	ss, err := GenerateSourceSchemas(context.Background(), `
import "vela/kube"

#Endpoint: {
	host: string
	port: int & >0 & <65536
}

parameter: {
	// +usage=Secret the database details are read from
	secret: string
	kind: *"postgres" | "mysql"
	if kind == "mysql" {
		// +usage=Character set of the connection
		charset: *"utf8mb4" | string
	}
}

// Every field is read by $(source.<name>.<field>).
schema: {
	// +usage=Where the database listens
	endpoint: #Endpoint
	replicas?: [...#Endpoint]
	tls: bool
}

secret: kube.#Read & {$params: resource: {apiVersion: "v1", kind: "Secret", metadata: name: parameter.secret}}
output: endpoint: host: secret.$returns.value.data.host
`)
	require.NoError(t, err)

	assert.Equal(t, []string{"secret"}, sortedRequired(ss.Parameter.OpenAPI.Required))
	assert.Equal(t, []uischema.Condition{{JSONKey: "kind", Value: "mysql"}}, uiParam(t, ss.Parameter.UI, "charset").Conditions)

	out := ss.Output
	require.NotNil(t, out)
	assert.Equal(t, []string{"endpoint", "tls"}, sortedRequired(out.Required))
	endpoint := out.Properties["endpoint"].Value
	assert.Equal(t, "Where the database listens", endpoint.Description)
	assert.True(t, endpoint.Properties["port"].Value.Type.Is("integer"))
	assert.True(t, out.Properties["replicas"].Value.Items.Value.Properties["host"].Value.Type.Is("string"))
}

func TestGenerateSourceSchemasWithoutSchema(t *testing.T) {
	ss, err := GenerateSourceSchemas(context.Background(), `parameter: name: string
output: name: parameter.name`)
	require.NoError(t, err)
	assert.Nil(t, ss.Output, "a source without a schema declares no output shape")
	assert.NotNil(t, ss.Parameter.OpenAPI.Properties["name"])
}

func sortedRequired(r []string) []string {
	out := append([]string(nil), r...)
	sort.Strings(out)
	return out
}

func TestGenerateParameterSchemasAtReadsContext(t *testing.T) {
	ps, err := GenerateParameterSchemasAt(context.Background(), `template: parameter: ns: *context.namespace | string`, "template")
	require.NoError(t, err)
	assert.True(t, ps.OpenAPI.Properties["ns"].Value.Type.Is("string"))

	_, err = GenerateParameterSchemasAt(context.Background(), `template: parameter: ns: string`, "tempalte")
	assert.Error(t, err, "a path that does not resolve is an error")
}

func TestGenerateListOfObjectUnion(t *testing.T) {
	ps := generate(t, `parameter: mounts: [...({pvc: string} | {configMap: string})]`)
	p := uiParam(t, ps.UI, "mounts")
	assert.Equal(t, "Structs", p.UIType)
	assert.Len(t, p.SubParameters, 2)
}

func TestGenerateObjectUnion(t *testing.T) {
	ps := generate(t, `parameter: target: {host: string, port: int} | {socket: string, port: int}`)
	target := uiParam(t, ps.UI, "target")
	assert.False(t, uiParam(t, target.SubParameters, "host").Validate.Required, "only one variant has host")
	assert.False(t, uiParam(t, target.SubParameters, "socket").Validate.Required, "only one variant has socket")
	assert.True(t, uiParam(t, target.SubParameters, "port").Validate.Required, "every variant requires port")
	schema := ps.OpenAPI.Properties["target"].Value
	require.Len(t, schema.OneOf, 2)
	assert.Equal(t, []string{"host", "port"}, schema.OneOf[0].Value.Required)
}

func TestGenerateObjectUnionSharedName(t *testing.T) {
	ps := generate(t, `parameter: {
	value: {value: string} | {value: int}
	port: {port?: string} | {port: int}
}`)
	v := ps.OpenAPI.Properties["value"].Value.Properties["value"].Value
	require.Len(t, v.OneOf, 2, "value is a string or an integer")
	assert.True(t, v.OneOf[0].Value.Type.Is("string"))
	assert.True(t, v.OneOf[1].Value.Type.Is("integer"))
	sub := uiParam(t, ps.UI, "value").SubParameters
	require.Len(t, sub, 1, "one form field for value")
	assert.Equal(t, "Input", sub[0].UIType)
	assert.True(t, sub[0].Validate.Required, "every variant requires value")
	assert.False(t, uiParam(t, uiParam(t, ps.UI, "port").SubParameters, "port").Validate.Required)
}

func TestGenerateRequiresOnlyUnderFullConjunction(t *testing.T) {
	ps := generate(t, `parameter: {type: "a" | "b", if type == "a" {mode: "on" | "off", if mode == "on" {y: string}}}`)
	require.Len(t, ps.OpenAPI.OneOf, 2)
	assert.Equal(t, []string{"mode"}, ps.OpenAPI.OneOf[0].Value.Required, "y is required only when mode is on as well")
}
