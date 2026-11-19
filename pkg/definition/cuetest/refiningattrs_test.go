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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// A refining attribute is read from the expectation's own field, so one on
// a field the expectation only refers to fails the load.
func TestRefiningAttributeOnAReference(t *testing.T) {
	def, err := os.ReadFile("testdata/defs/web.cue")
	require.NoError(t, err)
	cases := map[string]struct{ src, want string }{
		"a hidden field in the case": {
			src: `"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	_spec: {replicas: 1, template: _} @exact()
	expect: output: spec: _spec
}`,
			want: `case "c": expect.output.spec refers to c._spec, whose @exact() applies only where it is written: put @exact() on expect.output.spec`,
		},
		"a hidden field at the top of the file": {
			src: `_containers: [{image: "nginx"}] @contains()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: template: spec: containers: _containers
}`,
			want: `refers to _containers, whose @contains() applies only where it is written`,
		},
		"a reference unified with more": {
			src: `_absent: {paused: true} @not()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: _absent & {replicas: 1}
}`,
			want: `refers to _absent, whose @not() applies only where it is written`,
		},
		"a reference among alternatives": {
			src: `_one: {replicas: 1, template: _} @exact()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: _one | {replicas: 2}
}`,
			want: `refers to _one, whose @exact() applies only where it is written`,
		},
		"on the expectation's field too": {
			src: `_spec: {replicas: 1, template: _} @exact()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: _spec @exact()
}`,
		},
		"outside the expectation": {
			src: `_param: {image: "nginx"} @exact()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: _param
	expect: output: spec: replicas: 1
}`,
		},
		"on a field within what is referred to": {
			src: `_output: spec: {replicas: 1, template: _} @exact()
"c": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: _output
}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "web.cue", string(def))
			write(t, dir, "web_test.cue", "import \"vela/test\"\n\n"+tc.src+"\n")
			suites, err := Load(dir)
			require.NoError(t, err)
			if tc.want == "" {
				require.NoError(t, suites[0].Err)
				for _, c := range suites[0].Cases {
					require.Empty(t, c.Run(), c.Name)
				}
				return
			}
			require.ErrorContains(t, suites[0].Err, tc.want)
		})
	}
}
