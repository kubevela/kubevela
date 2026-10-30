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

package placement

import (
	"testing"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

func obj(name, class string) map[string]interface{} {
	o := map[string]interface{}{
		"metadata": map[string]interface{}{"name": name},
	}
	if class != "" {
		o["metadata"].(map[string]interface{})["annotations"] = map[string]interface{}{
			Annotation: class,
		}
	}
	return o
}

func TestClass(t *testing.T) {
	got, err := Class(obj("plain", ""))
	require.NoError(t, err)
	require.Equal(t, Hub, got)

	marked, err := Marked(obj("plain", ""))
	require.NoError(t, err)
	require.False(t, marked)
	marked, err = Marked(obj("marked", Hub))
	require.NoError(t, err)
	require.True(t, marked)

	got, err = Class(obj("marked", Hub))
	require.NoError(t, err)
	require.Equal(t, Hub, got)

	got, err = Class(obj("explicit", Runtime), Hub)
	require.NoError(t, err)
	require.Equal(t, Runtime, got, "an object's own annotation wins over an inherited default")

	got, err = Class(obj("inherited", ""), Hub)
	require.NoError(t, err)
	require.Equal(t, Hub, got)

	got, err = Class(obj("line-wins", ""), Runtime, Hub)
	require.NoError(t, err)
	require.Equal(t, Runtime, got, "the first inherited default wins")

	_, err = Class(obj("bad", "spoke"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad")
	require.Contains(t, err.Error(), "spoke")
}

func TestSplitAux(t *testing.T) {
	groups, err := SplitAux("store-aux", []map[string]interface{}{
		obj("defaults", ""),
		obj("admission", Hub),
		obj("also-runtime", Runtime),
	})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "store-aux", groups[0].Name)
	require.Equal(t, Runtime, groups[0].Class)
	require.Equal(t, []string{"also-runtime"}, names(groups[0].Objects))
	require.Equal(t, "store-aux-hub", groups[1].Name)
	require.Equal(t, Hub, groups[1].Class)
	require.Equal(t, []string{"defaults", "admission"}, names(groups[1].Objects))

	onlyHub, err := SplitAux("store-v1-aux", []map[string]interface{}{obj("controller", Hub)})
	require.NoError(t, err)
	require.Len(t, onlyHub, 1)
	require.Equal(t, "store-v1-aux-hub", onlyHub[0].Name)

	none, err := SplitAux("store-aux", nil)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestAssembleBindsRuntimeToHubUntilAsked(t *testing.T) {
	comps := []Component{
		{Name: "store-aux", Class: Runtime},
		{Name: "store-aux-hub", Class: Hub},
		{Name: "store-v1-defs", Class: Hub},
	}
	names := Names{
		HubPolicy: "module-hub", RuntimePolicy: "module-runtime",
		HubOverride: "module-hub-override", RuntimeOverride: "module-runtime-override",
		HubStep: "deploy-module-hub", RuntimeStep: "deploy-module-runtime",
	}

	policies, steps := Assemble(comps, nil, names)
	require.Len(t, policies, 3, "no runtime topology policy until a placement request exists")
	require.Equal(t, "module-hub", policies[0]["name"])
	require.Equal(t, []interface{}{pkgmulticluster.Local}, policies[0]["properties"].(map[string]interface{})["clusters"])
	require.Equal(t, []interface{}{"store-aux-hub", "store-v1-defs"}, selector(policies[1]))
	require.Equal(t, []interface{}{"store-aux"}, selector(policies[2]))

	require.Len(t, steps, 2)
	require.Equal(t, "deploy", steps[0]["type"])
	require.Equal(t, step.DeployWorkflowStep, steps[0]["type"])
	require.Equal(t, []interface{}{"module-hub", "module-hub-override"}, steps[0]["properties"].(map[string]interface{})["policies"])
	require.Equal(t, []interface{}{"module-hub", "module-runtime-override"}, steps[1]["properties"].(map[string]interface{})["policies"])
}

func TestAssembleCopiesThePlacementRequest(t *testing.T) {
	names := Names{
		HubPolicy: "module-hub", RuntimePolicy: "module-runtime",
		HubOverride: "module-hub-override", RuntimeOverride: "module-runtime-override",
		HubStep: "deploy-module-hub", RuntimeStep: "deploy-module-runtime",
	}
	comps := []Component{{Name: "store-aux", Class: Runtime}}

	first, steps := Assemble(comps, &v1alpha1.Placement{Clusters: []string{"spoke-a", "spoke-b"}}, names)
	second, _ := Assemble(comps, &v1alpha1.Placement{
		Clusters:             []string{"spoke-c"},
		ClusterLabelSelector: map[string]string{"tier": "runtime"},
		AllowEmpty:           true,
	}, names)

	require.Equal(t, []interface{}{"module-runtime", "module-runtime-override"}, steps[1]["properties"].(map[string]interface{})["policies"])
	require.Equal(t, []interface{}{"spoke-a", "spoke-b"}, first[2]["properties"].(map[string]interface{})["clusters"])
	props := second[2]["properties"].(map[string]interface{})
	require.Equal(t, []interface{}{"spoke-c"}, props["clusters"])
	require.Equal(t, map[string]interface{}{"tier": "runtime"}, props["clusterLabelSelector"])
	require.Equal(t, true, props["allowEmpty"])
	_, hasNamespace := props["namespace"]
	require.False(t, hasNamespace)
}

func names(objs []map[string]interface{}) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o["metadata"].(map[string]interface{})["name"].(string)
	}
	return out
}

func selector(policy map[string]interface{}) []interface{} {
	return policy["properties"].(map[string]interface{})["selector"].([]interface{})
}
