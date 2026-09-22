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

	"github.com/stretchr/testify/require"
)

// A parent with something worth forgetting to forward.
const inertParent = `
output: {
	kind: "Deployment"
	spec: replicas: parameter.replicas
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

// The combination that goes wrong: the whole of the parent's schema is
// published, only part of it is forwarded, and the remainder is inert. An
// application setting `replicas: 5` on this gets one replica and no complaint,
// which was reproduced on a cluster before this check existed.
func TestInertInheritedParameterIsReported(t *testing.T) {
	warnings, err := CheckCall(context.Background(), []Level{
		{Name: "tenant-app", Template: `
$super: properties: {image: parameter.image}

output: metadata: labels: tenant: parameter.tenant

parameter: $super.parameter & {
	tenant: string
}
`},
		{Name: "webservice", Template: inertParent},
	}, ComponentSurface, testCompile())

	require.NoError(t, err, "inert parameters are worth saying, not worth refusing over")
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], `"replicas"`)
	require.Contains(t, warnings[0], "will do nothing")
	require.NotContains(t, warnings[0], `"image"`, "image is forwarded")
	require.NotContains(t, warnings[0], `"tenant"`, "tenant is used by the child itself")
}

// Forwarding the lot is the coherent way to inherit the lot.
func TestForwardingEverythingIsNotInert(t *testing.T) {
	warnings, err := CheckCall(context.Background(), []Level{
		{Name: "tenant-app", Template: `
$super: properties: parameter

output: metadata: labels: tenant: parameter.tenant

parameter: $super.parameter & {
	tenant: string
}
`},
		{Name: "webservice", Template: inertParent},
	}, ComponentSurface, testCompile())

	require.NoError(t, err)
	require.Empty(t, warnings)
}

// The abstracting shape: declare what you expose, decide the rest yourself.
func TestAbstractingChildHasNothingInert(t *testing.T) {
	warnings, err := CheckCall(context.Background(), []Level{
		{Name: "tenant-app", Template: `
$super: properties: {
	image:    parameter.image
	replicas: 3
}

output: metadata: labels: tenant: parameter.tenant

parameter: {
	image:  string
	tenant: string
}
`},
		{Name: "webservice", Template: inertParent},
	}, ComponentSurface, testCompile())

	require.NoError(t, err)
	require.Empty(t, warnings)
}

// A parameter read only by a health policy is used, and saying otherwise would
// be a false alarm on a perfectly good definition.
func TestParameterUsedOnlyByHealthPolicyIsNotInert(t *testing.T) {
	warnings, err := CheckCall(context.Background(), []Level{
		{Name: "tenant-app", Template: `
$super: properties: {image: parameter.image}

parameter: {
	image:     string
	minReady:  *1 | int
}
`},
		{Name: "webservice", Template: inertParent},
	}, ComponentSurface, testCompile(),
		`isHealth: context.output.status.readyReplicas >= parameter.minReady`)

	require.NoError(t, err)
	require.Empty(t, warnings, "minReady is read by the health policy")
}

func TestParameterUsageScanning(t *testing.T) {
	t.Run("selector and index both count", func(t *testing.T) {
		use := scanParameterUse(`a: parameter.image` + "\n" + `b: parameter["tenant"]`)
		require.False(t, use.wholesale)
		require.True(t, use.fields["image"])
		require.True(t, use.fields["tenant"])
	})

	t.Run("a declaration is not a use", func(t *testing.T) {
		use := scanParameterUse(`parameter: {image: string}`)
		require.False(t, use.wholesale)
		require.Empty(t, use.fields)
	})

	t.Run("comprehension over the whole struct stands the check down", func(t *testing.T) {
		use := scanParameterUse(`patch: metadata: labels: {for k, v in parameter {(k): v}}`)
		require.True(t, use.wholesale)
	})

	t.Run("passing the whole struct stands the check down", func(t *testing.T) {
		use := scanParameterUse(`$super: parameter`)
		require.True(t, use.wholesale)
	})
}
