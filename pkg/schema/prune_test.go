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
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	cuex.EnableExternalPackageForDefaultCompiler = false
}

func TestPruneToParameter(t *testing.T) {
	cases := map[string]struct {
		src      string
		contains []string
		omits    []string
	}{
		"keeps referenced definitions, lets and imports": {
			src: `
import (
	"strings"
	"vela/base64"
)

#Port: {port: int}
let defaultImage = "nginx"

parameter: {
	// +usage=Which image to run
	image: *defaultImage | string & strings.MinRunes(1)
	ports: [...#Port]
}

output: data: base64.#Encode & {$params: parameter.image}
`,
			contains: []string{`"strings"`, "#Port", "defaultImage", "+usage=Which image to run"},
			omits:    []string{"vela/base64", "output"},
		},
		"follows definitions transitively": {
			src: `
#A: {b: #B}
#B: {c: string}
#Unused: {d: int}
parameter: a: #A
`,
			contains: []string{"#A", "#B"},
			omits:    []string{"#Unused"},
		},
		"labels and selectors are not references": {
			src: `
parameter: {output: string, x: context.name}
output: {name: parameter.output}
`,
			omits: []string{"output: {name"},
		},
		"comprehension variables are not references": {
			src: `
parameter: {
	names: [...string]
	byName: {for i, n in names {(n): i}}
}
i: "unrelated"
n: "unrelated"
`,
			contains: []string{"for i, n in names"},
			omits:    []string{"unrelated"},
		},
		"merges split declarations": {
			src: `
parameter: a: string
parameter: b: int
`,
			contains: []string{"a: string", "b: int"},
		},
		"drops comprehensions that only produce output": {
			src: `
parameter: enabled: bool
if parameter.enabled {
	outputs: x: {}
}
`,
			contains: []string{"enabled: bool"},
			omits:    []string{"outputs"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := PruneToParameter(tc.src)
			require.NoError(t, err)
			for _, s := range tc.contains {
				assert.Contains(t, got, s)
			}
			for _, s := range tc.omits {
				assert.NotContains(t, got, s)
			}
		})
	}
}

func TestPruneToParameterUnprunable(t *testing.T) {
	for name, src := range map[string]string{
		"parameter inside a comprehension": `
x: true
if x {
	parameter: a: string
}
`,
		"top-level embedding": `
#Base: parameter: a: string
#Base
`,
		"dynamic top-level label": `
k: "parameter"
(k): a: string
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PruneToParameter(src)
			assert.ErrorIs(t, err, ErrUnprunable)
		})
	}
}

func TestParseParameterSchemaIgnoresOutput(t *testing.T) {
	ctx := context.Background()
	for name, src := range map[string]string{
		"import the schema compiler lacks": `
import "vela/base64"

parameter: image: string
output: data: base64.#Encode & {$params: parameter.image}
`,
		"conflict in output": `
parameter: image: string
output: replicas: 1
output: replicas: 2
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePropertiesToSchema(ctx, src)
			require.Error(t, err, "the whole-template compile should fail on this")

			sch, path, err := ParseParameterSchema(ctx, src)
			require.NoError(t, err)
			assert.Equal(t, PathPlain, path)
			assert.Contains(t, sch.Properties, "image")
		})
	}
}

func TestParseParameterSchemaFallsBack(t *testing.T) {
	src := `
#Base: parameter: a: string
#Base
`
	sch, path, err := ParseParameterSchema(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, PathFull, path)
	assert.Contains(t, sch.Properties, "a")
}

func TestParseParameterSchemaNeedsCuex(t *testing.T) {
	src := `
import "vela/kube"

parameter: resource: kube.#Read.$params.value
output: {}
`
	sch, path, err := ParseParameterSchema(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, PathCuex, path)
	assert.Contains(t, sch.Properties, "resource")
}
