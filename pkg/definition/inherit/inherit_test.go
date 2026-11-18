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

// testCompile stands in for the render path's compiler.
//
// It builds a fresh cue.Context per call, exactly as cuex's compiler does. That
// is the harsher of the two possible harnesses and the honest one: values from
// different contexts cannot be unified or filled into one another, and CUE
// panics with "values are not from the same runtime" rather than returning an
// error. Sharing a context here would let a chain pass in tests and panic in a
// controller.
func testCompile() CompileFunc {
	return func(_ context.Context, src string) (cue.Value, error) {
		v := cuecontext.New().CompileString(src)
		return v, v.Err()
	}
}

const parentTemplate = `
import "strings"

output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{
			name:  context.name
			image: strings.ToLower(parameter.image)
		}]
	}
}

outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name: context.name
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

const contextFile = `
context: {
	name:      "billing-api"
	namespace: "acme"
}
`

func render(t *testing.T, childTemplate, paramFile string) *Result {
	t.Helper()
	res, err := Render(
		context.Background(),
		[]Level{{Name: "tenant-webservice", Template: childTemplate}, {Name: "webservice", Template: parentTemplate}},
		paramFile, contextFile, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)
	return res
}

func TestChainOfOneRendersAsBefore(t *testing.T) {
	res, err := Render(
		context.Background(),
		[]Level{{Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", replicas: 3}`, contextFile, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)

	replicas, err := res.Value.LookupPath(cue.ParsePath("output.spec.replicas")).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 3, replicas)
}

func TestChildMergesOntoParentSurfaces(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

outputs: quota: {
	apiVersion: "v1"
	kind:       "ResourceQuota"
	metadata: name: context.name
}

parameter: {
	tenant: string
}
`
	res := render(t, child, `parameter: {image: "NGINX", tenant: "acme"}`)

	// The parent's output survives.
	kind, err := res.Value.LookupPath(cue.ParsePath("output.kind")).String()
	require.NoError(t, err)
	require.Equal(t, "Deployment", kind)

	image, err := res.Value.LookupPath(cue.ParsePath("output.spec.template.spec.containers[0].image")).String()
	require.NoError(t, err)
	require.Equal(t, "nginx", image, "the parent's own template logic still runs")

	// The child's overlay lands on it.
	tenant, err := res.Value.LookupPath(cue.ParsePath(`output.metadata.labels."tenant.oam.dev/name"`)).String()
	require.NoError(t, err)
	require.Equal(t, "acme", tenant)

	// outputs gained a key without losing the parent's.
	require.True(t, res.Value.LookupPath(cue.ParsePath("outputs.svc")).Exists(), "parent's outputs kept")
	require.True(t, res.Value.LookupPath(cue.ParsePath("outputs.quota")).Exists(), "child's outputs added")
}

// The parent sets spec.replicas from `replicas: *1 | int`. Its output crosses
// as a value, so the field is still a disjunction when the child merges onto it
// and the child would win. A child adds to its parent and never changes it, so
// the value has to go up through `$super: properties` instead.
func TestChildChangingAValueFromADefaultedParameterIsRefused(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

output: spec: replicas: 5

parameter: {
	tenant: string
}
`
	_, err := Render(context.Background(),
		[]Level{{Name: "tenant-webservice", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()))
	require.ErrorContains(t, err, "sets output.spec.replicas, which webservice already sets")
}

// A child cannot take its parent's output over.
func TestInheritOutputFalseIsRefused(t *testing.T) {
	child := `
$inherit: {output: false}

$super: properties: {image: parameter.image}

output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
}

parameter: {
	tenant: string
}
`
	_, err := Render(context.Background(),
		[]Level{{Name: "tenant-webservice", Template: child}, {Name: "webservice", Template: parentTemplate}},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()))
	require.ErrorContains(t, err, "`$inherit: {output: false}` is not allowed")
}

// What the parent rendered stays readable on `$super`, so a child can build an
// output of its own from a piece of it.
func TestParentOutputIsReadableForANewOutput(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

outputs: image: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name + "-image"
	data: image: $super.output.spec.template.spec.containers[0].image
}

parameter: {
	tenant: string
}
`
	res := render(t, child, `parameter: {image: "NGINX", tenant: "acme"}`)

	image, err := res.Value.LookupPath(cue.ParsePath("outputs.image.data.image")).String()
	require.NoError(t, err)
	require.Equal(t, "nginx", image)
	require.True(t, res.Value.LookupPath(cue.ParsePath("output.kind")).Exists(), "the parent's output was merged")
	require.True(t, res.Value.LookupPath(cue.ParsePath("outputs.svc")).Exists(), "and its outputs kept")
}

func TestParentAuthoredErrorsReachTheChild(t *testing.T) {
	parent := parentTemplate + `
if parameter.image == "banned" {
	errs: ["image 'banned' is not allowed by webservice"]
}
`
	res, err := Render(
		context.Background(),
		[]Level{
			{Name: "tenant-webservice", Template: "$super: properties: {image: parameter.image}\nparameter: {tenant: string}"},
			{Name: "webservice", Template: parent},
		},
		`parameter: {image: "banned", tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"image 'banned' is not allowed by webservice"}, res.Errs)
}

func TestMissingSuperBlockIsNamed(t *testing.T) {
	_, err := Render(
		context.Background(),
		[]Level{
			{Name: "tenant-webservice", Template: "parameter: {tenant: string}"},
			{Name: "webservice", Template: parentTemplate},
		},
		`parameter: {tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()),
	)
	require.ErrorContains(t, err, "declares no `$super` block")
	require.ErrorContains(t, err, "webservice")
}

func TestThreeLevelChain(t *testing.T) {
	middle := `
$super: properties: {image: parameter.image, replicas: parameter.replicas}

output: metadata: labels: tier: parameter.tier

parameter: {
	image:    string
	replicas: *1 | int
	tier:     *"standard" | string
}
`
	child := `
$super: properties: {image: parameter.image, tier: "gold"}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

parameter: {
	image:  string
	tenant: string
}
`
	res, err := Render(
		context.Background(),
		[]Level{
			{Name: "tenant-webservice", Template: child},
			{Name: "tiered-webservice", Template: middle},
			{Name: "webservice", Template: parentTemplate},
		},
		`parameter: {image: "NGINX", tenant: "acme"}`, contextFile, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)

	labels := res.Value.LookupPath(cue.ParsePath("output.metadata.labels"))
	tier, err := labels.LookupPath(cue.ParsePath("tier")).String()
	require.NoError(t, err)
	require.Equal(t, "gold", tier, "the middle level's label, with the child's value")

	tenant, err := labels.LookupPath(cue.ParsePath(`"tenant.oam.dev/name"`)).String()
	require.NoError(t, err)
	require.Equal(t, "acme", tenant)

	kind, err := res.Value.LookupPath(cue.ParsePath("output.kind")).String()
	require.NoError(t, err)
	require.Equal(t, "Deployment", kind, "the root's output reached the bottom of the chain")
}
