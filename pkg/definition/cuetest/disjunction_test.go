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

func TestMatchDisjunction(t *testing.T) {
	cases := map[string]struct {
		expect   string
		failures []string
	}{
		"struct: first branch":            {expect: `{spec: {replicas: 3} | {paused: true}}`},
		"struct: second branch":           {expect: `{spec: {replicas: 4} | {paused: false}}`},
		"struct: a non-default branch":    {expect: `{spec: *{replicas: 4} | {paused: false}}`},
		"struct: through a reference":     {expect: `{_s: {replicas: 4} | {paused: false}, spec: _s}`},
		"struct: unified with a struct":   {expect: `{spec: ({replicas: 4} | {replicas: 3}) & {paused: false}}`},
		"list: second branch":             {expect: `{spec: template: spec: containers: [{name: "b"}] | [{name: "a"}]}`},
		"scalar: a non-default branch":    {expect: `{spec: replicas: *4 | 3}`},
		"attributes hold within a branch": {expect: `{spec: {replicas: 4} | {paused: true @not()}}`},
		"struct: no branch matches": {
			expect:   `{spec: {replicas: 4} | {paused: true}}`,
			failures: []string{"spec: expected {\n\treplicas: 4\n} | {\n\tpaused: true\n}, got " + actualSpec},
		},
		"struct: a default alone does not match": {
			expect:   `{spec: *{replicas: 4} | {paused: true}}`,
			failures: []string{"spec: expected *{\n\treplicas: 4\n} | {\n\tpaused: true\n}, got " + actualSpec},
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

const actualSpec = `{
	replicas: 3
	paused:   false
	template: {
		spec: {
			containers: [{
				name:  "a"
				image: "nginx:1.25"
				env: [{
					name:  "A"
					value: "1"
				}, {
					name:  "B"
					value: "2"
				}]
			}]
		}
	}
}`
