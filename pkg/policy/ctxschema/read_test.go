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

package ctxschema

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func TestReadSetsCtx(t *testing.T) {
	cases := map[string]struct {
		template string
		want     bool
	}{
		"no output":                             {`parameter: {}`, false},
		"labels only":                           {`output: labels: a: "b"`, false},
		"several fields, no ctx":                {`output: {labels: a: "b", annotations: c: "d"}`, false},
		"shorthand":                             {`output: ctx: team: "x"`, true},
		"struct":                                {`output: {labels: a: "b", ctx: {team: "x"}}`, true},
		"quoted label":                          {`output: "ctx": team: "x"`, true},
		"inside if":                             {"x: true\noutput: {if x {ctx: team: \"a\"}}", true},
		"inside for":                            {"xs: [1]\noutput: {for _ in xs {ctx: team: \"a\"}}", true},
		"nested comprehension":                  {"x: true\noutput: {if x {if x {ctx: team: \"a\"}}}", true},
		"output declared twice":                 {"output: labels: a: \"b\"\noutput: ctx: team: \"x\"", true},
		"a reference cannot be read":            {"_o: ctx: team: \"x\"\noutput: _o", true},
		"an embedding cannot be read":           {"_o: ctx: team: \"x\"\noutput: {_o}", true},
		"ctx deeper than output does not count": {`output: labels: ctx: "x"`, false},
		"a hidden field is not output":          {`_output: ctx: team: "x"`, false},
		"output in a top-level if":              {"x: true\nif x {output: ctx: team: \"a\"}", true},
		"unified structs":                       {"output: {labels: a: \"b\"} & {ctx: team: \"x\"}", true},
		"unified structs, no ctx":               {"output: {labels: a: \"b\"} & {annotations: c: \"d\"}", false},
		"parenthesised":                         {"output: ({labels: a: \"b\"})", false},
		"a top-level embedding":                 {"_base: output: ctx: team: \"x\"\n_base", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := Read(tc.template)
			require.NoError(t, err)
			require.Equal(t, tc.want, info.SetsCtx)
		})
	}
}

func TestReadSetsPolicies(t *testing.T) {
	cases := map[string]struct {
		template string
		want     bool
	}{
		"ctx only":              {`output: ctx: a: "b"`, false},
		"policies":              {`output: policies: []`, true},
		"policies in an if":     {"x: true\noutput: {if x {policies: []}}", true},
		"unreadable output":     {"_o: {}\noutput: _o", true},
		"no output":             {`parameter: {}`, false},
		"a top-level embedding": {"_b: output: policies: []\n_b", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := Read(tc.template)
			require.NoError(t, err)
			require.Equal(t, tc.want, info.SetsPolicies)
		})
	}
}

func TestReadConditional(t *testing.T) {
	cases := map[string]struct {
		template string
		want     bool
	}{
		"no enabled":                          {`output: ctx: a: "b"`, false},
		"config without enabled":              {`config: autoRevision: true`, false},
		"config.enabled true":                 {`config: enabled: true`, false},
		"config struct, enabled true":         {`config: {enabled: true, autoRevision: false}`, false},
		"root enabled true":                   {`enabled: true`, false},
		"config.enabled computed":             {`config: enabled: context.namespace == "prod"`, true},
		"config.enabled from a parameter":     {"parameter: on: bool\nconfig: enabled: parameter.on", true},
		"root enabled computed":               {`enabled: context.namespace != "x"`, true},
		"enabled false":                       {`config: enabled: false`, true},
		"enabled with a default":              {`config: enabled: *true | bool`, true},
		"config in an if":                     {"x: true\nif x {config: enabled: false}", true},
		"config by reference":                 {"_c: enabled: true\nconfig: _c", true},
		"enabled under an if inside config":   {"x: true\nconfig: {if x {enabled: false}}", true},
		"an if inside config without enabled": {"x: true\nconfig: {if x {autoRevision: true}}", false},
		"an embedding inside config":          {"_c: enabled: false\nconfig: {_c}", true},
		"a top-level embedding":               {"_base: config: enabled: false\n_base", true},
		"enabled true under an if":            {"x: true\nif x {config: enabled: true}", true},
		"root enabled true under an if":       {"x: true\nif x {enabled: true}", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := Read(tc.template)
			require.NoError(t, err)
			require.Equal(t, tc.want, info.Conditional)
		})
	}
}

func TestReadSchema(t *testing.T) {
	type check struct {
		value string
		ok    bool
	}
	cases := map[string]struct {
		template   string
		noSchema   bool
		compileErr string
		checks     []check
	}{
		"none": {
			template: `output: ctx: team: "x"`,
			noSchema: true,
		},
		"schema without ctx": {
			template: `schema: labels: team: string`,
			noSchema: true,
		},
		"shorthand, open": {
			template: `schema: ctx: {team: string, replicas?: int}`,
			checks: []check{
				{`{team: "a"}`, true},
				{`{team: "a", replicas: 2}`, true},
				{`{team: "a", extra: 1}`, true},
				{`{team: 1}`, false},
				{`{team: "a", replicas: "two"}`, false},
			},
		},
		"struct form, closed": {
			template: `schema: {ctx: close({team: =~"^[a-z]+$"})}`,
			checks: []check{
				{`{team: "abc"}`, true},
				{`{team: "ABC"}`, false},
				{`{team: "abc", extra: 1}`, false},
			},
		},
		"closed, explicitly open": {
			template: `schema: ctx: close({team: string, ...})`,
			checks:   []check{{`{team: "a", extra: 1}`, true}},
		},
		"nested": {
			template: `schema: ctx: org: {team: string, zone?: string}`,
			checks: []check{
				{`{org: team: "a"}`, true},
				{`{org: team: 1}`, false},
			},
		},
		"declared twice is unified": {
			template: "schema: ctx: team: string\nschema: ctx: zone: =~\"^eu-\"",
			checks: []check{
				{`{team: "a", zone: "eu-west"}`, true},
				{`{team: "a", zone: "us-east"}`, false},
			},
		},
		"a standard-library import the schema uses is carried": {
			template: "import \"strings\"\nschema: ctx: team: strings.MinRunes(3)\noutput: ctx: team: \"abc\"",
			checks: []check{
				{`{team: "abc"}`, true},
				{`{team: "ab"}`, false},
			},
		},
		"an aliased import is carried": {
			template: "import s \"strings\"\nschema: ctx: team: s.MinRunes(3)",
			checks:   []check{{`{team: "ab"}`, false}},
		},
		"a provider import the schema does not use is left out": {
			template: "import (\n\t\"vela/kube\"\n\t\"strings\"\n)\n_x: kube.#Get\nschema: ctx: team: strings.MinRunes(1)",
			checks:   []check{{`{team: "a"}`, true}},
		},
		"a field labelled like an unused import does not carry it": {
			template: "import \"strings\"\n_x: strings.ToLower(\"A\")\nschema: ctx: strings: string",
			checks:   []check{{`{strings: "a"}`, true}},
		},
		"a deep conflict does not compile": {
			template:   "schema: ctx: org: unit: team: string & int",
			compileErr: "conflicting",
		},
		"a conflict in a list does not compile": {
			template:   "schema: ctx: teams: [string & int]",
			compileErr: "conflicting",
		},
		"a schema under an if is not read": {
			template: "x: true\nif x {schema: ctx: team: string}",
			noSchema: true,
		},
		"a schema given by reference is not read": {
			template: "_s: ctx: team: string\nschema: _s",
			noSchema: true,
		},
		"a schema field that is a helper, not a ctx schema, is not read": {
			template: "#Shape: {name: string}\nschema: #Shape\noutput: labels: a: \"b\"",
			noSchema: true,
		},
		"a nested standard-library path is carried": {
			template: "import \"encoding/json\"\nschema: ctx: raw: json.Valid",
			checks: []check{
				{`{raw: "{}"}`, true},
				{`{raw: "{"}`, false},
			},
		},
		"a provider import the schema uses does not compile": {
			template:   "import \"vela/kube\"\nschema: ctx: x: kube.#Get",
			compileErr: "vela/kube",
		},
		"a reference to the rest of the template does not compile": {
			template:   "_t: string\nschema: ctx: team: _t",
			compileErr: "_t",
		},
		"a schema that is not a struct does not compile": {
			template:   `schema: ctx: string`,
			compileErr: "struct",
		},
		"a schema that conflicts with itself does not compile": {
			template:   "schema: ctx: team: string\nschema: ctx: team: int",
			compileErr: "conflicting",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := Read(tc.template)
			require.NoError(t, err)
			if tc.noSchema {
				require.Empty(t, info.Schema)
				return
			}
			require.NotEmpty(t, info.Schema)
			cctx := cuecontext.New()
			schema, err := Compile(cctx, info.Schema)
			if tc.compileErr != "" {
				require.ErrorContains(t, err, tc.compileErr)
				return
			}
			require.NoError(t, err)
			for _, c := range tc.checks {
				v := schema.Unify(cctx.CompileString(c.value))
				err := v.Validate(cue.Concrete(true))
				if c.ok {
					require.NoError(t, err, c.value)
				} else {
					require.Error(t, err, c.value)
				}
			}
		})
	}
}

func TestReadDoesNotParse(t *testing.T) {
	_, err := Read(`output: {`)
	require.Error(t, err)
}

func TestReadLabelsWithoutAStaticName(t *testing.T) {
	type want struct{ setsCtx, setsPolicies, conditional, schemaUnread bool }
	all := want{true, true, true, true}
	cases := map[string]struct {
		template string
		want     want
	}{
		"a computed top-level label":          {"_k: \"output\"\n(_k): ctx: team: \"x\"", all},
		"an interpolated top-level label":     {"_k: \"out\"\n\"\\(_k)put\": ctx: team: \"x\"", all},
		"a top-level pattern label":           {"[string]: ctx: team: \"x\"", all},
		"a computed label inside output":      {"_k: \"ctx\"\noutput: (_k): team: \"x\"", want{setsCtx: true, setsPolicies: true}},
		"an interpolated label inside output": {"_k: \"c\"\noutput: \"\\(_k)tx\": team: \"x\"", want{setsCtx: true, setsPolicies: true}},
		"a pattern label inside output":       {"output: [string]: team: \"x\"", want{setsCtx: true, setsPolicies: true}},
		"a computed label inside config":      {"_k: \"enabled\"\nconfig: (_k): false", want{conditional: true}},
		"a pattern label inside config":       {"config: [string]: false", want{conditional: true}},
		"a computed label inside schema":      {"_k: \"ctx\"\nschema: (_k): {team: string}", want{schemaUnread: true}},
		"a pattern label inside schema":       {"schema: [string]: {team: string}", want{schemaUnread: true}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := Read(tc.template)
			require.NoError(t, err)
			got := want{info.SetsCtx, info.SetsPolicies, info.Conditional, info.SchemaUnread}
			require.Equal(t, tc.want, got)
		})
	}
}
