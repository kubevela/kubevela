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

package cuetest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const statusComponentTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: replicas: parameter.replicas
}
parameter: replicas: *1 | int
`

var statusSubject = Subject{
	Kind:         KindComponent,
	Name:         "web",
	Template:     statusComponentTemplate,
	HealthPolicy: "isHealth: context.output.status.readyReplicas == context.output.spec.replicas && context.output.metadata.generation == 1",
	CustomStatus: "message: \"\\(context.output.metadata.namespace)/\\(context.output.metadata.name)\"",
}

func TestStatus(t *testing.T) {
	r := require.New(t)
	res, err := Status(statusSubject, Input{Context: Context{Name: "api", Namespace: "prod"}, Parameter: map[string]any{"replicas": 2}},
		Observed{Output: map[string]any{"status": map[string]any{"readyReplicas": 2}}})
	r.NoError(err)
	r.True(res.Healthy, "the API server's generation is filled in")
	r.Equal("prod/api", res.Message, "an unnamed output is found by the component name")
}

func TestStatusObservedErrors(t *testing.T) {
	_, err := Status(statusSubject, Input{}, Observed{Outputs: map[string]map[string]any{"nope": {}}})
	require.ErrorContains(t, err, "observed.outputs.nope: the definition renders no such output")
}

func TestMerge(t *testing.T) {
	base := map[string]any{
		"spec":     map[string]any{"replicas": 1, "template": map[string]any{"a": 1}},
		"metadata": map[string]any{"name": "x"},
		"list":     []any{1, 2},
	}
	got := merge(base, map[string]any{
		"spec":   map[string]any{"replicas": 3},
		"list":   []any{9},
		"status": map[string]any{"ready": true},
	})
	require.Equal(t, map[string]any{
		"spec":     map[string]any{"replicas": 3, "template": map[string]any{"a": 1}},
		"metadata": map[string]any{"name": "x"},
		"list":     []any{9},
		"status":   map[string]any{"ready": true},
	}, got, "maps merge, anything else is replaced")
	require.Equal(t, 1, base["spec"].(map[string]any)["replicas"], "base is not modified")
}

func TestStatusUnseedableObject(t *testing.T) {
	s := Subject{Kind: KindComponent, Name: "web", Template: `output: {kind: "X"}` + "\nparameter: {}"}
	require.NotPanics(t, func() {
		_, err := Status(s, Input{}, Observed{})
		require.ErrorContains(t, err, "Object 'apiVersion' is missing")
	})
}
