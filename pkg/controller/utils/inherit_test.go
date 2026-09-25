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

parameter: {
	image: string
	// +usage=Owning tenant
	tenant: string
}
`

// The published schema is what `vela show` prints and what an application is
// validated against. A definition that extends another publishes the parameters
// it declares, exactly as one that extends nothing does.
func TestAnExtendingDefinitionPublishesItsOwnParameters(t *testing.T) {
	raw, err := inheritedSchema(context.Background(), "tenant-webservice", schemaChild,
		inherit.ComponentSurface)
	require.NoError(t, err)

	var s openapi3.Schema
	require.NoError(t, json.Unmarshal(raw, &s))

	for _, field := range []string{"tenant", "image"} {
		require.Contains(t, s.Properties, field, "expected %q in the published schema", field)
	}

	// The parent's parameters stay the parent's. A definition publishes what it
	// declares, so an application is validated against what this one states it
	// takes rather than against everything the chain could accept.
	require.NotContains(t, s.Properties, "replicas")
	require.NotContains(t, s.Properties, "cpu")

	require.Equal(t, "Owning tenant", s.Properties["tenant"].Value.Description)
	require.Contains(t, s.Required, "image")
	require.Contains(t, s.Required, "tenant")
}

// A chain of one goes through the same path and must behave as it always did.
func TestSchemaOfADefinitionThatExtendsNothing(t *testing.T) {
	raw, err := inheritedSchema(context.Background(), "webservice", schemaParent, inherit.ComponentSurface)
	require.NoError(t, err)

	var s openapi3.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	require.Contains(t, s.Properties, "image")
	require.NotContains(t, s.Properties, "tenant")
	// A defaulted parameter is published as required. That is existing behaviour
	// of the OpenAPI generation, asserted rather than corrected here.
	require.Contains(t, s.Required, "replicas")
	require.EqualValues(t, 1, s.Properties["replicas"].Value.Default)
}
