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

package docgen

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func unstructuredDef(kind, name, namespace, extends string) unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"extends": extends,
			"schematic": map[string]interface{}{
				"cue": map[string]interface{}{"template": "$super: properties: {}\nparameter: {}\n"},
			},
		},
	}
	if kind == "TraitDefinition" {
		obj["spec"].(map[string]interface{})["definitionRef"] = map[string]interface{}{"name": "deployments.apps"}
	}
	return unstructured.Unstructured{Object: obj}
}

// Resolving a chain needs to know where to look, and a parent may only live
// beside its child. Carrying `extends` without the namespace leaves the lookup
// searching nowhere in particular, so the parent's parameters go undocumented.
func TestTheNamespaceTravelsWithExtends(t *testing.T) {
	for _, tc := range []struct{ kind, name string }{
		{"ComponentDefinition", "tenant-webservice"},
		{"TraitDefinition", "tenant-gateway"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			cap, err := ParseCapabilityFromUnstructured(nil, unstructuredDef(tc.kind, tc.name, "team-a", "base"))
			require.NoError(t, err)
			require.Equal(t, "base", cap.Extends)
			require.Equal(t, "team-a", cap.Namespace, "without this the chain resolves in no namespace")
		})
	}
}
