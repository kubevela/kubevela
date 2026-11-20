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

package cuetest

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

const matchActual = `{
	kind: "Deployment"
	spec: {
		replicas: 3
		paused:   false
		template: spec: containers: [{name: "a", image: "nginx:1.25", env: [{name: "A", value: "1"}, {name: "B", value: "2"}]}]
	}
}`

func TestMatch(t *testing.T) {
	cases := map[string]struct {
		expect   string
		failures []string
	}{
		"subset":               {expect: `{kind: "Deployment", spec: replicas: 3}`},
		"constraint":           {expect: `{spec: replicas: >2}`},
		"regex":                {expect: `{spec: template: spec: containers: [{image: =~"^nginx"}]}`},
		"open list prefix":     {expect: `{spec: template: spec: containers: [{name: "a"}, ...]}`},
		"absent field":         {expect: `{spec: strategy?: _|_}`},
		"exact struct matches": {expect: `{spec: template: spec: {containers: [...]} @exact()}`},
		"close() is not exact": {expect: `{spec: close({replicas: 3})}`},
		"wrong value": {
			expect:   `{spec: replicas: 4}`,
			failures: []string{"spec.replicas: expected 4, got 3"},
		},
		"constraint violated": {
			expect:   `{spec: replicas: >5}`,
			failures: []string{"spec.replicas: expected >5, got 3"},
		},
		"missing field": {
			expect:   `{spec: strategy: "Recreate"}`,
			failures: []string{"spec.strategy: missing, expected \"Recreate\""},
		},
		"present but should be absent": {
			expect:   `{spec: paused?: _|_}`,
			failures: []string{"spec.paused: expected absent, got false"},
		},
		"extra field in exact struct": {
			expect:   `{spec: {replicas: 3, template: _} @exact()}`,
			failures: []string{"spec.paused: unexpected field, got false"},
		},
		"closed list wrong length": {
			expect:   `{spec: template: spec: containers: [{name: "a"}, {name: "b"}]}`,
			failures: []string{"spec.template.spec.containers: expected 2 elements, got 1"},
		},
		"open list too short": {
			expect:   `{spec: template: spec: containers: [{}, {}, ...]}`,
			failures: []string{"spec.template.spec.containers: expected at least 2 elements, got 1"},
		},
		"wrong kind": {
			expect:   `{spec: replicas: {}}`,
			failures: []string{"spec.replicas: expected a struct, got 3"},
		},
		"not: value differs":              {expect: `{spec: replicas: 4 @not()}`},
		"not: field absent":               {expect: `{spec: strategy: "Recreate" @not()}`},
		"not: struct only partly matches": {expect: `{spec: {replicas: 3, paused: true} @not()}`},
		"not: struct matches": {
			expect:   `{spec: {replicas: 3, paused: false} @not()}`,
			failures: []string{"spec: expected not to match {\n\treplicas: 3\n\tpaused:   false\n}"},
		},
		"contains: any position": {expect: `{spec: template: spec: containers: [{env: [{name: "B"}] @contains()}]}`},
		"contains: several, any order": {
			expect: `{spec: template: spec: containers: [{env: [{name: "B"}, {name: "A", value: "1"}] @contains()}]}`,
		},
		"contains: missing element": {
			expect:   `{spec: template: spec: containers: [{env: [{name: "B"}, {name: "C"}] @contains()}]}`,
			failures: []string{"spec.template.spec.containers[0].env: no element matches {\n\tname: \"C\"\n}"},
		},
		"contains: each element needs its own match": {
			expect:   `{spec: template: spec: containers: [{env: [{name: "B"}, {name: "B"}] @contains()}]}`,
			failures: []string{"spec.template.spec.containers[0].env: no element matches {\n\tname: \"B\"\n}"},
		},
		"contains: an overlapping pattern still finds a pairing": {
			expect: `{spec: template: spec: containers: [{env: [{name: =~"^[AB]$"}, {name: "A"}] @contains()}]}`,
		},
		"contains not: none present": {expect: `{spec: template: spec: containers: [{env: [{name: "C"}, {name: "D"}] @contains() @not()}]}`},
		"contains not: one present": {
			expect:   `{spec: template: spec: containers: [{env: [{name: "C"}, {name: "B"}] @contains() @not()}]}`,
			failures: []string{"spec.template.spec.containers[0].env: expected no element to match {\n\tname: \"B\"\n}"},
		},
		"contains on a non-list": {
			expect:   `{spec: replicas: [3] @contains()}`,
			failures: []string{"spec.replicas: expected a list, got 3"},
		},
		"reports every failure": {
			expect: `{kind: "StatefulSet", spec: {replicas: 1, template: spec: containers: [{image: "redis"}]}}`,
			failures: []string{
				`kind: expected "StatefulSet", got "Deployment"`,
				"spec.replicas: expected 1, got 3",
				`spec.template.spec.containers[0].image: expected "redis", got "nginx:1.25"`,
			},
		},
	}
	ctx := cuecontext.New()
	actual := ctx.CompileString(matchActual)
	require.NoError(t, actual.Err())
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			expect := ctx.CompileString(tc.expect)
			require.NoError(t, expect.Err())
			require.Equal(t, tc.failures, Match(expect, actual))
		})
	}
}
