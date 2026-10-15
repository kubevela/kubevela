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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatches(t *testing.T) {
	for name, tc := range map[string]struct {
		template        string
		output, outputs bool
	}{
		"a trait that only adds a resource": {`
outputs: svc: {apiVersion: "v1", kind: "Service"}
parameter: port: int
`, false, false},
		"a patch": {`
patch: spec: replicas: parameter.replicas
parameter: replicas: int
`, true, false},
		"a patch under an if": {`
if parameter.enabled {
	patch: spec: paused: true
}
parameter: enabled: bool
`, true, false},
		"a patch in a comprehension": {`
for k, v in parameter.labels {
	patch: metadata: labels: (k): v
}
parameter: labels: [string]: string
`, true, false},
		"patchOutputs": {`
patchOutputs: svc: spec: type: "NodePort"
`, false, true},
		"both": {`
patch: metadata: labels: a: "b"
patchOutputs: svc: metadata: labels: a: "b"
`, true, true},
		"a field named patch inside another block": {`
outputs: cfg: data: patch: "x"
`, false, false},
		"a quoted label": {`
"patch": spec: replicas: 1
`, true, false},
		"a patch in an embedded struct": {`
{patch: spec: replicas: 1}
`, true, false},
		"a patch under an if inside a for": {`
for k, v in parameter.labels {
	if v != "" {
		patch: metadata: labels: (k): v
	}
}
parameter: labels: [string]: string
`, true, false},
		"a computed label may be either": {`
(parameter.field): spec: replicas: 1
parameter: field: string
`, true, true},
		"an interpolated label may be either": {`
"\(parameter.field)": spec: replicas: 1
parameter: field: string
`, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			output, outputs := Patches(tc.template)
			require.Equal(t, tc.output, output, "patch")
			require.Equal(t, tc.outputs, outputs, "patchOutputs")
		})
	}

	t.Run("a template that does not parse may patch anything", func(t *testing.T) {
		output, outputs := Patches(`patch: {`)
		require.True(t, output)
		require.True(t, outputs)
	})
}
