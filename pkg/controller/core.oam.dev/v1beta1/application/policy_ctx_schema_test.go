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

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func TestCheckCtxSchema(t *testing.T) {
	cases := map[string]struct {
		template string
		// wantErr is the whole message; empty means the output passes.
		wantErr string
	}{
		"no schema": {
			template: `output: ctx: team: 1`,
		},
		"matching": {
			template: "schema: ctx: team: string\noutput: ctx: team: \"a\"",
		},
		"a value out of bound": {
			template: "schema: ctx: team: =~\"^[a-z]+$\"\noutput: ctx: team: \"A\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.team: invalid value "A" (out of bound =~"^[a-z]+$")`,
		},
		"a required key not set": {
			template: "schema: ctx: {team: string, costCentre: string}\noutput: ctx: team: \"a\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.costCentre: required by schema.ctx (string), not set`,
		},
		"a key marked required (!) not set": {
			template: "schema: ctx: {team!: string, zone: string}\noutput: ctx: zone: \"z\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.team: required by schema.ctx (string), not set`,
		},
		"a key marked required (!) set": {
			template: "schema: ctx: {team!: string}\noutput: ctx: team: \"a\"",
		},
		"no ctx at all": {
			template: "schema: ctx: team: string\noutput: labels: a: \"b\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.team: required by schema.ctx (string), not set`,
		},
		"an undeclared key under a closed schema": {
			template: "schema: ctx: close({team: string})\noutput: ctx: {team: \"a\", region: \"eu\"}",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.region: not declared by schema.ctx, which is closed`,
		},
		"every error is reported, in path order": {
			template: "schema: ctx: close({team: string, zone: string, unit: int})\noutput: ctx: {team: 1, unit: \"x\", region: \"eu\"}",
			wantErr: `output.ctx does not match schema.ctx: ` +
				`output.ctx.region: not declared by schema.ctx, which is closed; ` +
				`output.ctx.team: conflicting values string and 1 (mismatched types string and int); ` +
				`output.ctx.unit: conflicting values int and "x" (mismatched types int and string); ` +
				`output.ctx.zone: required by schema.ctx (string), not set`,
		},
		"every error in a nested struct is reported": {
			template: "schema: ctx: org: close({a: string, b: int})\noutput: ctx: org: {a: 1, c: 2}",
			wantErr: `output.ctx does not match schema.ctx: ` +
				`output.ctx.org.a: conflicting values string and 1 (mismatched types string and int); ` +
				`output.ctx.org.b: required by schema.ctx (int), not set; ` +
				`output.ctx.org.c: not declared by schema.ctx, which is closed`,
		},
		"a nested struct the output leaves out reports its required keys": {
			template: "schema: ctx: {team: string, org: {unit: string, zone?: string}}\noutput: ctx: team: \"a\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.org.unit: required by schema.ctx (string), not set`,
		},
		"an open struct inside a closed one accepts extra keys": {
			template: "schema: ctx: close({org: {unit: string}})\noutput: ctx: org: {unit: \"u\", extra: 1}",
		},
		"a closed struct inside an open one refuses them": {
			template: "schema: ctx: {org: close({unit: string})}\noutput: ctx: {other: 1, org: {unit: \"u\", extra: 1}}",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.org.extra: not declared by schema.ctx, which is closed`,
		},
		"an optional struct the output sets is checked": {
			template: "schema: ctx: close({mesh?: close({domain: string})})\noutput: ctx: mesh: {domain: 1, port: 2}",
			wantErr: `output.ctx does not match schema.ctx: ` +
				`output.ctx.mesh.domain: conflicting values string and 1 (mismatched types string and int); ` +
				`output.ctx.mesh.port: not declared by schema.ctx, which is closed`,
		},
		"a struct where a scalar is declared": {
			template: "schema: ctx: team: string\noutput: ctx: team: {name: \"a\"}",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.team: conflicting values string and {name:"a"} (mismatched types string and struct)`,
		},
		"a scalar where a struct is declared": {
			template: "schema: ctx: org: {unit: string}\noutput: ctx: org: \"a\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.org: conflicting values "a" and {unit:string} (mismatched types string and struct)`,
		},
		"a list element out of bound": {
			template: "schema: ctx: zones: [...=~\"^eu-\"]\noutput: ctx: zones: [\"eu-west\", \"us-east\"]",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.zones.1: invalid value "us-east" (out of bound =~"^eu-")`,
		},
		"a pattern constraint allows matching keys under a closed schema": {
			template: "schema: ctx: close({tags: close({[=~\"^t-\"]: string})})\noutput: ctx: tags: {\"t-a\": \"x\", other: \"y\"}",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.tags.other: not declared by schema.ctx, which is closed`,
		},
		"a closed struct inside a list": {
			template: "schema: ctx: zones: [...close({name: string})]\noutput: ctx: zones: [{name: \"a\"}, {name: \"b\", weight: 1}]",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.zones.1.weight: not declared by schema.ctx, which is closed`,
		},
		"a nested path": {
			template: "schema: ctx: org: unit: =~\"^u-\"\noutput: ctx: org: unit: \"x\"",
			wantErr:  `output.ctx does not match schema.ctx: output.ctx.org.unit: invalid value "x" (out of bound =~"^u-")`,
		},
		"an optional key may be absent": {
			template: "schema: ctx: {team: string, zone?: string}\noutput: ctx: team: \"a\"",
		},
		"a default fills an unset key": {
			template: "schema: ctx: team: *\"x\" | string\noutput: labels: a: \"b\"",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tc.template)
			require.NoError(t, v.Err())
			err := checkCtxSchema(v)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestCtxWithSchemaDefaults(t *testing.T) {
	cases := map[string]struct {
		template string
		// decoded is output.ctx as the controller already decoded it.
		decoded map[string]interface{}
		want    map[string]interface{}
	}{
		"no schema is the decoded output, untouched": {
			template: `output: ctx: team: 1`,
			decoded:  map[string]interface{}{"team": "as decoded"},
			want:     map[string]interface{}{"team": "as decoded"},
		},
		"no schema and no ctx is nothing": {
			template: `output: labels: a: "b"`,
		},
		"a default fills an unset key": {
			template: "schema: ctx: {team: *\"x\" | string, zone: string}\noutput: ctx: zone: \"z\"",
			want:     map[string]interface{}{"team": "x", "zone": "z"},
		},
		"a default fills when the output sets no ctx": {
			template: "schema: ctx: team: *\"x\" | string\noutput: labels: a: \"b\"",
			want:     map[string]interface{}{"team": "x"},
		},
		"the output overrides a default": {
			template: "schema: ctx: team: *\"x\" | string\noutput: ctx: team: \"a\"",
			want:     map[string]interface{}{"team": "a"},
		},
		"a default in a nested struct fills": {
			template: "schema: ctx: org: {unit: *\"u\" | string, zone: string}\noutput: ctx: org: zone: \"z\"",
			want:     map[string]interface{}{"org": map[string]interface{}{"unit": "u", "zone": "z"}},
		},
		"an optional key with a default stays unset": {
			template: "schema: ctx: {team: string, zone?: *\"z\" | string}\noutput: ctx: team: \"a\"",
			want:     map[string]interface{}{"team": "a"},
		},
		"undeclared keys under an open schema are kept": {
			template: "schema: ctx: team: *\"x\" | string\noutput: ctx: region: \"eu\"",
			want:     map[string]interface{}{"team": "x", "region": "eu"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tc.template)
			require.NoError(t, v.Err())
			got, err := ctxWithSchemaDefaults(v, tc.decoded)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
