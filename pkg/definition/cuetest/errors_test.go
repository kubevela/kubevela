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

func TestStructuredErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", `d: {
	type: "component"
	attributes: status: healthPolicy: "isHealth: context.output.status.ready"
}
template: {
	output: {apiVersion: "v1", kind: "X", spec: replicas: parameter.replicas}
	if parameter.fail {
		errs: ["asked to fail"]
	}
	if parameter.crash {
		output: spec: paused: true & false
	}
	parameter: {replicas: *1 | int, fail: *false | bool, crash: *false | bool}
}`)
	write(t, dir, "a_test.cue", `import "vela/test"

"rejected by the parameter schema": test.#ComponentRender & {
	definition: "d"
	parameter: replicas: "x"
	expect: error: {parameter: [=~"replicas"] @contains(), template?: _|_, user?: _|_}
}
"raised by the definition": test.#ComponentRender & {
	definition: "d"
	parameter: fail: true
	expect: error: {user: ["asked to fail"], parameter?: _|_}
}
"broken template": test.#ComponentRender & {
	definition: "d"
	parameter: crash: true
	expect: error: template: [=~"conflicting values"] @contains()
}
"shorthand matches the message": test.#ComponentRender & {
	definition: "d"
	parameter: replicas: "x"
	expect: error: =~"Parameter errors:"
}
"wrong category": test.#ComponentRender & {
	definition: "d"
	parameter: replicas: "x"
	expect: error: template: [_, ...]
}
"status evaluation": test.#ComponentStatus & {
	definition: "d"
	expect: error: {status: [=~"isHealth"] @contains(), template?: _|_}
}
"render error in a status test": test.#ComponentStatus & {
	definition: "d"
	parameter: replicas: "x"
	expect: error: {parameter: [_, ...], status?: _|_}
}
`)
	suites, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	got := map[string][]string{}
	for _, c := range suites[0].Cases {
		got[c.Name] = c.Run()
	}
	require.Equal(t, map[string][]string{
		"rejected by the parameter schema": nil,
		"raised by the definition":         nil,
		"broken template":                  nil,
		"shorthand matches the message":    nil,
		"wrong category":                   {"error.template: missing, expected [_, ...]"},
		"status evaluation":                nil,
		"render error in a status test":    nil,
	}, got)
}

func TestErrorCategoryTypo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {}, parameter: {}}")
	write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test.#ComponentRender & {definition: \"d\", expect: error: paramter: [_]}")
	suites, err := Load(dir)
	require.NoError(t, err)
	require.ErrorContains(t, suites[0].Err, "expect.error.paramter: not an error category")
}

// The categories an unknown one is told to use are every category there is.
func TestUnknownErrorCategoryListsThemAll(t *testing.T) {
	err := checkErrorCategories(cuecontext.New().CompileString(`{schemas: ["x"]}`))
	require.EqualError(t, err, "expect.error.schemas: not an error category; use message, parameter, schema, status, template or user")
}
