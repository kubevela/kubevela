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

package utils

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

const schemaParent = `
output: {
	kind: "Deployment"
	spec: replicas: parameter.replicas
}

parameter: {
	// +usage=Which image would you like to use
	image: string
	// +usage=Number of replicas
	replicas: *1 | int
	// +usage=CPU request
	cpu?: string
}
`

const schemaChild = `
$super: properties: {image: parameter.image}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

parameter: $super.parameter & {
	// +usage=Owning tenant
	tenant: string
}
`

// The published schema is what `vela show` prints and what an application is
// validated against. A child that inherits its parent's parameters must publish
// them, or a definition that renders perfectly well looks like it takes nothing.
func TestInheritedSchemaPublishesTheWholeParameterSet(t *testing.T) {
	raw, err := inheritedSchema(context.Background(), "tenant-webservice", schemaChild,
		[]inherit.Level{{Name: "webservice", Template: schemaParent}}, inherit.ComponentSurface)
	require.NoError(t, err)

	var s openapi3.Schema
	require.NoError(t, json.Unmarshal(raw, &s))

	for _, field := range []string{"tenant", "image", "replicas", "cpu"} {
		require.Contains(t, s.Properties, field, "expected %q in the published schema", field)
	}

	// The parent's documentation travels with its parameters.
	require.Equal(t, "Which image would you like to use", s.Properties["image"].Value.Description)
	require.Equal(t, "Owning tenant", s.Properties["tenant"].Value.Description)

	// A defaulted parameter is published with its default.
	require.EqualValues(t, 1, s.Properties["replicas"].Value.Default)

	// What the parent insists on, the child inherits the insistence on.
	require.Contains(t, s.Required, "image")
	require.Contains(t, s.Required, "tenant")
	require.NotContains(t, s.Required, "cpu", "an optional parent parameter stays optional")

	// Inheriting a parameter set does not change what the generator makes of it:
	// a defaulted parameter is listed as required here exactly as it is for a
	// definition that extends nothing, which TestSchemaOfADefinitionThatExtendsNothing
	// pins. That is existing behaviour of the OpenAPI generation, not something
	// inheritance introduces, so it is asserted rather than corrected here.
	require.Contains(t, s.Required, "replicas")
}

// A chain of one goes through the same path and must behave as it always did.
func TestSchemaOfADefinitionThatExtendsNothing(t *testing.T) {
	raw, err := inheritedSchema(context.Background(), "webservice", schemaParent, nil, inherit.ComponentSurface)
	require.NoError(t, err)

	var s openapi3.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	require.Contains(t, s.Properties, "image")
	require.NotContains(t, s.Properties, "tenant")
	// The baseline for the note in TestInheritedSchemaPublishesTheWholeParameterSet:
	// a defaulted parameter is published as required with no inheritance involved.
	require.Contains(t, s.Required, "replicas")
	require.EqualValues(t, 1, s.Properties["replicas"].Value.Default)
}
