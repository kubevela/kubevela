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

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const parentTemplate = `
parameter: {
	image:    string
	replicas: *1 | int
}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
	spec: replicas: parameter.replicas
}
outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
}
`

func schemaAt(s *openapi3.Schema, path ...string) *openapi3.Schema {
	for _, p := range path {
		if s == nil || s.Properties[p] == nil {
			return nil
		}
		s = s.Properties[p].Value
	}
	return s
}

func generated(t *testing.T, template string) *OutputSchemas {
	t.Helper()
	s, err := GenerateOutputSchemas(context.Background(), template)
	require.NoError(t, err)
	return s
}

func TestExtendOutputSchemas(t *testing.T) {
	parent := generated(t, parentTemplate)
	child := generated(t, `
parameter: tenant: string
$super: properties: image: "nginx"
output: metadata: {
	name: $super.output.metadata.name
	labels: tenant: parameter.tenant
}
outputs: quota: kind: "ResourceQuota"
`)

	merged := parent.Extend(child, true, true)
	assert.True(t, typeIs(schemaAt(merged.Output, "spec", "replicas"), openapi3.TypeInteger), "the parent's fields stand")
	assert.True(t, typeIs(schemaAt(merged.Output, "metadata", "labels", "tenant"), openapi3.TypeString), "the child's are added")
	assert.True(t, typeIs(schemaAt(merged.Output, "metadata", "name"), openapi3.TypeString),
		"a field the child writes from $super keeps the parent's type")
	assert.NotNil(t, schemaAt(merged.Output, "status"))
	assert.ElementsMatch(t, []string{"svc", "quota"}, resourceNames(merged.Outputs), "outputs join by name")

	replaced := parent.Extend(child, false, false)
	assert.Nil(t, schemaAt(replaced.Output, "spec"), "$inherit: {output: false} takes the output over")
	assert.ElementsMatch(t, []string{"quota"}, resourceNames(replaced.Outputs), "$inherit: {outputs: false} takes outputs over")

	assert.Same(t, parent.Output, parent.Extend(nil, true, true).Output, "a child declaring nothing leaves the parent's")
	assert.Nil(t, (*OutputSchemas)(nil).Extend(nil, true, true).Output)
}

func resourceNames(m map[string]*openapi3.Schema) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
