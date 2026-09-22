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
	"github.com/stretchr/testify/require"
)

// A child that never reads `$super` must not be handed the parent's results.
func TestUnreadSuperFieldsAreNotFilled(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

output: metadata: labels: tenant: parameter.tenant

parameter: {tenant: string}
`
	res, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface,
		SameCompiler(testCompile()))
	require.NoError(t, err)

	require.False(t, res.Value.LookupPath(superPath("output")).Exists(), "$super.output was filled but never read")
	require.False(t, res.Value.LookupPath(superPath("parameter")).Exists(), "$super.parameter was filled but never read")
	require.True(t, res.Value.LookupPath(cue.ParsePath("output.kind")).Exists(), "the merge still happened")
}

// One that does read them gets them.
func TestReadSuperFieldsAreFilled(t *testing.T) {
	child := `
$inherit: {output: false}

$super: properties: {image: parameter.image}

output: $super.output

parameter: $super.parameter & {tenant: string}
`
	res, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface,
		SameCompiler(testCompile()))
	require.NoError(t, err)

	require.True(t, res.Value.LookupPath(superPath("output")).Exists())
	require.True(t, res.Value.LookupPath(superPath("parameter")).Exists())
}

// `outputs` is gated exactly as `output` and `parameter` are, and was the one
// component surface field the gating tests did not cover.
func TestUnreadSuperOutputsAreNotFilled(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

outputs: mine: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
}

parameter: {tenant: string}
`
	res, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface,
		SameCompiler(testCompile()))
	require.NoError(t, err)

	require.False(t, res.Value.LookupPath(superPath("outputs")).Exists(),
		"$super.outputs was filled but never read")
	require.True(t, res.Value.LookupPath(cue.ParsePath("outputs.mine.kind")).Exists(),
		"the child's own is still there")
}

func TestReadSuperOutputsAreFilled(t *testing.T) {
	child := `
$inherit: {outputs: false}

$super: properties: {image: parameter.image}

outputs: $super.outputs

parameter: {tenant: string}
`
	res, err := Render(context.Background(),
		[]Level{{Name: "child", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface,
		SameCompiler(testCompile()))
	require.NoError(t, err)

	require.True(t, res.Value.LookupPath(superPath("outputs")).Exists(),
		"a template that reads $super.outputs is handed them")
}
