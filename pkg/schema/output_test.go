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
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// at walks an OpenAPI schema by property names.
func at(t *testing.T, s *openapi3.Schema, path ...string) *openapi3.Schema {
	t.Helper()
	for _, p := range path {
		require.NotNil(t, s, "nil schema before %q in %v", p, path)
		ref, ok := s.Properties[p]
		require.True(t, ok, "no %q in %v (have %v)", p, path, keys(s.Properties))
		s = ref.Value
	}
	return s
}

// typeOf is a schema's single type, or "" for any.
func typeOf(s *openapi3.Schema) string {
	if s.Type == nil || len(s.Type.Slice()) == 0 {
		return ""
	}
	return s.Type.Slice()[0]
}

func keys(m openapi3.Schemas) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestGenerateOutputSchemas(t *testing.T) {
	template := `
parameter: {
	image: string
	replicas: *1 | int
	port?: int
}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{
			name:  context.name
			image: parameter.image
			if parameter.port != _|_ {
				ports: [{containerPort: parameter.port}]
			}
		}]
	}
}
outputs: route: {
	apiVersion: "v1"
	kind:       "Service"
	spec: clusterIP: "None"
}
`
	got, err := GenerateOutputSchemas(context.Background(), template)
	require.NoError(t, err)
	require.NotNil(t, got.Output)

	assert.Equal(t, "string", typeOf(at(t, got.Output, "kind")))
	assert.Equal(t, "integer", typeOf(at(t, got.Output, "spec", "replicas")), "a field set from a parameter takes its type")
	containers := at(t, got.Output, "spec", "template", "spec", "containers")
	assert.Equal(t, "array", typeOf(containers))
	require.NotNil(t, containers.Items)
	assert.Equal(t, "string", typeOf(at(t, containers.Items.Value, "image")))
	assert.Contains(t, keys(containers.Items.Value.Properties), "ports", "a field written under an if is offered")
	assert.Equal(t, "integer", typeOf(at(t, containers.Items.Value, "ports").Items.Value.Properties["containerPort"].Value),
		"a field under an if takes the type of the parameter it reads")

	status := at(t, got.Output, "status")
	assert.Empty(t, typeOf(status), "status is any: the controller's, not the template's")

	require.Contains(t, got.Outputs, "route")
	assert.Equal(t, "string", typeOf(at(t, got.Outputs["route"], "spec", "clusterIP")))
	assert.Empty(t, typeOf(at(t, got.Outputs["route"], "status")))
}

// Bodies that disagree on a value still agree on its type, and resources named
// by a loop are left out.
func TestGenerateOutputSchemasConditionalBodies(t *testing.T) {
	got, err := GenerateOutputSchemas(context.Background(), `
parameter: {
	ha: bool
	names: [...string]
}
output: {
	kind: "Deployment"
	if parameter.ha {
		spec: replicas: 3
	}
	if !parameter.ha {
		spec: replicas: 1
		metadata: labels: tier: "dev-\(context.name)"
	}
}
outputs: {
	for n in parameter.names {
		"svc-\(n)": {kind: "Service"}
	}
	if parameter.ha {
		pdb: {kind: "PodDisruptionBudget", spec: minAvailable: 1}
	}
}
`)
	require.NoError(t, err)
	assert.Equal(t, "integer", typeOf(at(t, got.Output, "spec", "replicas")))
	assert.Equal(t, "string", typeOf(at(t, got.Output, "metadata", "labels", "tier")))
	require.Len(t, got.Outputs, 1, "a resource named in a loop is left out")
	assert.Equal(t, "integer", typeOf(at(t, got.Outputs["pdb"], "spec", "minAvailable")))
	assert.Empty(t, typeOf(at(t, got.Outputs["pdb"], "status")))
}

func TestGenerateOutputSchemasWithoutOutputs(t *testing.T) {
	got, err := GenerateOutputSchemas(context.Background(), "parameter: {}\n")
	require.NoError(t, err)
	assert.Nil(t, got.Output)
	assert.Empty(t, got.Outputs)
}

// shipped reads a definition's CUE template from the vela-core chart.
func shipped(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../charts/vela-core/templates/defwithtemplate/" + name + ".yaml")
	require.NoError(t, err)
	// The chart wraps each definition in Helm conditionals; the CUE sits in the
	// one template block.
	text := string(raw)
	var doc struct {
		Spec struct {
			Schematic struct {
				CUE struct {
					Template string `json:"template"`
				} `json:"cue"`
			} `json:"schematic"`
		} `json:"spec"`
	}
	start := strings.Index(text, "apiVersion:")
	require.NoError(t, yaml.Unmarshal([]byte(strings.ReplaceAll(text[start:], "{{ include \"systemDefinitionNamespace\" . }}", "vela-system")), &doc))
	require.NotEmpty(t, doc.Spec.Schematic.CUE.Template)
	return doc.Spec.Schematic.CUE.Template
}

// The shipped definitions generate schemas a reader can complete against.
func TestGenerateOutputSchemasShipped(t *testing.T) {
	ws, err := GenerateOutputSchemas(context.Background(), shipped(t, "webservice"))
	require.NoError(t, err)
	assert.Equal(t, "string", typeOf(at(t, ws.Output, "kind")))
	assert.NotNil(t, at(t, ws.Output, "spec", "template", "spec", "containers").Items)

	expose, err := GenerateOutputSchemas(context.Background(), shipped(t, "expose"))
	require.NoError(t, err)
	assert.Nil(t, expose.Output, "a trait has no workload")
	assert.NotEmpty(t, expose.Outputs)

	objects, err := GenerateOutputSchemas(context.Background(), shipped(t, "k8s-objects"))
	require.NoError(t, err)
	assert.NotNil(t, objects.Output)
}

// An index into a parameter takes the type of what it selects: a struct's
// field or a map's value by a string key, a list's item by a number.
func TestGenerateOutputSchemasIndexedParameter(t *testing.T) {
	got, err := GenerateOutputSchemas(context.Background(), `
parameter: {
	on: bool
	labels: [string]: int
	named: {a: bool}
	ports: [...{port: int}]
}
output: {
	if parameter.on {
		spec: {
			byKey:   parameter.labels["app"]
			byField: parameter.named["a"]
			byIndex: parameter.ports[0].port
		}
	}
}
`)
	require.NoError(t, err)
	assert.Equal(t, "integer", typeOf(at(t, got.Output, "spec", "byKey")))
	assert.Equal(t, "boolean", typeOf(at(t, got.Output, "spec", "byField")))
	assert.Equal(t, "integer", typeOf(at(t, got.Output, "spec", "byIndex")))
}
