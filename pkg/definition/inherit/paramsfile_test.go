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

package inherit

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// What a child supplies is compiled inside its parent's file, where `parameter`
// is the parent's own block. A field still holding a reference to the child's
// parameters would be re-read there and pick up whatever the parent calls by
// that name.
func TestAFieldWithNothingBehindItIsNotSupplied(t *testing.T) {
	v := cuecontext.New().CompileString(`
$super: properties: {
	image:   "nginx"
	version: parameter.tag
}

parameter: {tag?: string}
`)
	require.NoError(t, v.Err())

	file := paramsFile(v.LookupPath(propertiesPath()), v.LookupPath(cue.ParsePath(ParameterField)))
	require.Contains(t, file, `image:`)
	require.NotContains(t, file, "parameter.tag",
		"a reference to the child's own parameters must not travel into the parent's file")
}

// A forwarded default is a real answer, and the parent is entitled to it.
func TestAForwardedDefaultIsStillSupplied(t *testing.T) {
	v := cuecontext.New().CompileString(`
$super: properties: {
	replicas: parameter.replicas
	tier:     *"gold" | string
}

parameter: {replicas: *3 | int}
`)
	require.NoError(t, v.Err())

	file := paramsFile(v.LookupPath(propertiesPath()), v.LookupPath(cue.ParsePath(ParameterField)))
	require.Contains(t, file, "replicas")
	require.Contains(t, file, "tier")
}

// End to end: the parent has a default for the field, and a child with nothing
// to put there must not stop it applying, nor capture the parent's own field of
// the same name.
func TestTheParentsDefaultStandsWhenTheChildSuppliesNothing(t *testing.T) {
	parent := `
output: {
	kind:    "Deployment"
	version: parameter.version
}

parameter: {
	version: *"1.0" | string
	tag:     *"PARENT-TAG" | string
}
`
	child := `
$super: properties: {version: parameter.tag}

output: metadata: name: "x"

parameter: {tag?: string}
`
	res, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "parent", Template: parent}},
		`parameter: {}`, contextFile, ComponentSurface, SameCompiler(testCompile()))
	require.NoError(t, err)

	version, err := res.Value.LookupPath(cue.ParsePath("output.version")).String()
	require.NoError(t, err)
	require.Equal(t, "1.0", version,
		"the parent's own default, not the parent's `tag` caught by a stray reference")
}
