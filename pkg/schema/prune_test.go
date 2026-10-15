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

	"cuelang.org/go/cue/cuecontext"
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

// What PruneTo keeps of a template's imports compiles, however they were written.
func TestPruneToKeepsImportsThatCompile(t *testing.T) {
	for name, tc := range map[string]struct {
		imports string
		uses    string
		want    []string
	}{
		"one import in parentheses":  {"import (\n\t\"strconv\"\n)", `strconv.FormatInt(1, 10)`, []string{`"strconv"`}},
		"one import, no parentheses": {`import "strconv"`, `strconv.FormatInt(1, 10)`, []string{`"strconv"`}},
		"two in parentheses, both used": {"import (\n\t\"strconv\"\n\t\"strings\"\n)",
			`strings.ToUpper(strconv.FormatInt(1, 10))`, []string{`"strconv"`, `"strings"`}},
		"two in parentheses, one used": {"import (\n\t\"strconv\"\n\t\"strings\"\n)", `strconv.FormatInt(1, 10)`, []string{`"strconv"`}},
		"an aliased import":            {"import (\n\tconv \"strconv\"\n)", `conv.FormatInt(1, 10)`, []string{`conv "strconv"`}},
		"two declarations": {"import \"strconv\"\nimport \"strings\"", `strings.ToUpper(strconv.FormatInt(1, 10))`,
			[]string{`"strconv"`, `"strings"`}},
	} {
		t.Run(name, func(t *testing.T) {
			src := tc.imports + "\n\nparameter: n: *" + tc.uses + " | string\noutput: kind: \"ConfigMap\"\n"
			pruned, err := PruneTo(src, "parameter")
			require.NoError(t, err)
			v := cuecontext.New().CompileString(pruned)
			require.NoError(t, v.Err(), "pruned:\n%s", pruned)
			for _, w := range tc.want {
				require.Contains(t, pruned, w)
			}
			require.NotContains(t, pruned, "output")
		})
	}
}
