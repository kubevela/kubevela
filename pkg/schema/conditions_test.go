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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	uischema "github.com/oam-dev/kubevela/pkg/utils/schema"
)

func TestGenerateConjunctiveConditions(t *testing.T) {
	cases := map[string]struct {
		src  string
		path []string
		want []uischema.Condition
	}{
		"joined by &&": {
			src:  `parameter: {kind: "a" | "b", enabled: bool, if kind == "a" && enabled {x: string}}`,
			path: []string{"x"},
			want: []uischema.Condition{{JSONKey: "kind", Value: "a"}, {JSONKey: "enabled", Value: true}},
		},
		"with a presence condition": {
			src:  `parameter: {kind: "a" | "b", tls?: {secret: string}, if kind == "a" if tls != _|_ {x: string}}`,
			path: []string{"x"},
			want: []uischema.Condition{{JSONKey: "kind", Value: "a"}, {JSONKey: "tls", Op: "!=", Value: nil}},
		},
		"in nested ifs": {
			src:  `parameter: {kind: "a" | "b", enabled: bool, if kind == "a" {if enabled {x: string}}}`,
			path: []string{"x"},
			want: []uischema.Condition{{JSONKey: "kind", Value: "a"}, {JSONKey: "enabled", Value: true}},
		},
		"over several values": {
			src:  `parameter: {kind: "a" | "b" | "c", enabled: bool, if kind != "c" && enabled {x: string}}`,
			path: []string{"x"},
			want: []uischema.Condition{{JSONKey: "kind", Op: "in", Value: []any{"a", "b"}}, {JSONKey: "enabled", Value: true}},
		},
		"with a field one side brings alone": {
			src:  `parameter: {kind: "a" | "b", enabled: bool, if enabled {y: string}, if kind == "a" && enabled {x: string}}`,
			path: []string{"y"},
			want: []uischema.Condition{{JSONKey: "enabled", Value: true}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ps := generate(t, tc.src)
			assert.Equal(t, tc.want, conditionsOf(t, ps.UI, tc.path...))
			for _, b := range ps.OpenAPI.OneOf {
				assert.NotContains(t, b.Value.Required, "x", "no single value brings x")
			}
		})
	}
}

func TestGenerateDisjunctiveConditionsStayApart(t *testing.T) {
	for name, src := range map[string]string{
		"joined by ||":       `parameter: {kind: "a" | "b", enabled: bool, if kind == "a" || enabled {x: string}}`,
		"a negated &&":       `parameter: {kind: "a" | "b", enabled: bool, if !(kind == "a" && enabled) {x: string}}`,
		"|| inside a nested": `parameter: {kind: "a" | "b", enabled: bool, mode: "p" | "q", if kind == "a" {if enabled || mode == "p" {x: string}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			ps := generate(t, src)
			assert.NotContains(t, ps.OpenAPI.Required, "x", "x exists only for some values")
			for _, p := range ps.UI {
				assert.NotEqual(t, "x", p.JSONKey, "an if that is not a conjunction is not read as one")
			}
		})
	}
}

func TestGenerateManyDiscriminators(t *testing.T) {
	var b strings.Builder
	b.WriteString("parameter: {\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "\tflag%d: *false | bool\n\tif flag%d {extra%d: string}\n", i, i, i)
	}
	b.WriteString("}")
	ps := generate(t, b.String())
	assert.Equal(t, []uischema.Condition{{JSONKey: "flag19", Value: true}}, uiParam(t, ps.UI, "extra19").Conditions)
	assert.Len(t, ps.OpenAPI.AllOf, 20)
}
