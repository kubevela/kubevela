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

package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// storeModule is the in-memory shape of the store module published from the
// module registry: module-level and v1 auxiliary objects of both classes, a
// v1beta1 line with definitions and no auxiliary, and a v2 line with both.
// explicit controls whether the runtime objects carry placement: runtime and
// the hub objects carry placement: hub. With explicit false, every auxiliary
// object is unmarked and therefore hub.
// The package format requires every line to have a definition, so the
// "no defs tier" case is storeModuleWithoutDefs, not a publishable line.
func storeModule(explicit bool) *module.Module {
	hub, runtime := "", ""
	if explicit {
		hub = oam.PlacementHub
		runtime = oam.PlacementRuntime
	}
	return &module.Module{
		Name:    "store",
		Version: "1.0.0",
		Auxiliary: []map[string]interface{}{
			placed("store-defaults", runtime),
			placed("store-admission", hub),
		},
		Lines: map[string]module.Line{
			"v1": {
				APIVersion: "v1",
				Enabled:    true,
				Auxiliary: []map[string]interface{}{
					placed("store-v1-defaults", runtime),
					placed("store-v1-controller", hub),
				},
				Definitions: []map[string]interface{}{definition("bucket")},
			},
			"v1beta1": {
				APIVersion:  "v1beta1",
				Enabled:     true,
				Definitions: []map[string]interface{}{definition("preview")},
			},
			"v2": {
				APIVersion:  "v2",
				Enabled:     true,
				Auxiliary:   []map[string]interface{}{placed("store-v2-migration", runtime)},
				Definitions: []map[string]interface{}{definition("bucket")},
			},
		},
	}
}

func placed(name, class string) map[string]interface{} {
	obj := auxObject("ConfigMap", name)
	if class == "" {
		return obj
	}
	obj["metadata"].(map[string]interface{})["annotations"] = map[string]interface{}{
		oam.AnnotationPlacement: class,
	}
	return obj
}

func componentNames(t *testing.T, app map[string]interface{}) []string {
	t.Helper()
	comps := components(t, app)
	names := make([]string, len(comps))
	for i, c := range comps {
		names[i] = c["name"].(string)
	}
	return names
}

func dependsOn(t *testing.T, app map[string]interface{}, name string) string {
	t.Helper()
	for _, c := range components(t, app) {
		if c["name"] != name {
			continue
		}
		raw, ok := c["dependsOn"]
		if !ok {
			return ""
		}
		deps := raw.([]interface{})
		require.Len(t, deps, 1)
		return deps[0].(string)
	}
	t.Fatalf("no component %s", name)
	return ""
}

func TestRenderApplication_UnmarkedStoreStaysFlat(t *testing.T) {
	app, err := RenderApplication(storeModule(false), "", nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"store-aux",
		"store-v1-aux",
		"store-v1-defs",
		"store-v1beta1-defs",
		"store-v2-aux",
		"store-v2-defs",
	}, componentNames(t, app))

	spec := app["spec"].(map[string]interface{})
	require.Len(t, spec["policies"].([]interface{}), 1)
	_, hasWorkflow := spec["workflow"]
	require.False(t, hasWorkflow)
}

func TestRenderApplication_HubMarkerPartitionsWithoutARequest(t *testing.T) {
	app, err := RenderApplication(storeModule(true), "", nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"store-aux",
		"store-aux-hub",
		"store-v1-aux",
		"store-v1-aux-hub",
		"store-v1-defs",
		"store-v1beta1-defs",
		"store-v2-aux",
		"store-v2-defs",
	}, componentNames(t, app))

	require.Equal(t, "", dependsOn(t, app, "store-aux"))
	require.Equal(t, "", dependsOn(t, app, "store-aux-hub"))
	require.Equal(t, "store-aux", dependsOn(t, app, "store-v1-aux"))
	require.Equal(t, "store-aux-hub", dependsOn(t, app, "store-v1-aux-hub"))
	require.Equal(t, "store-v1-aux-hub", dependsOn(t, app, "store-v1-defs"))
	require.Equal(t, "store-aux-hub", dependsOn(t, app, "store-v1beta1-defs"))
	require.Equal(t, "store-aux", dependsOn(t, app, "store-v2-aux"))

	spec := app["spec"].(map[string]interface{})
	policies := spec["policies"].([]interface{})
	require.Equal(t, []string{"module-state-keep", "module-hub", "module-hub-override", "module-runtime-override"}, policyNames(policies))
	steps := spec["workflow"].(map[string]interface{})["steps"].([]interface{})
	require.Equal(t, []string{"deploy-module-hub", "deploy-module-runtime"}, stepNames(steps))
	require.Equal(t, []interface{}{"module-hub", "module-runtime-override"}, steps[1].(map[string]interface{})["properties"].(map[string]interface{})["policies"])
}

func TestRenderApplication_PlacementRequestSetsRuntimeClusters(t *testing.T) {
	ask := func(clusters ...string) map[string]interface{} {
		app, err := RenderApplication(storeModule(true), "", &v1alpha1.Placement{Clusters: clusters})
		require.NoError(t, err)
		return app
	}
	first := ask("spoke-a", "spoke-b")
	second := ask("spoke-c")

	require.Equal(t, componentNames(t, first), componentNames(t, second))
	require.Equal(t, []interface{}{"spoke-a", "spoke-b"}, policyProp(t, first, "module-runtime", "clusters"))
	require.Equal(t, []interface{}{"spoke-c"}, policyProp(t, second, "module-runtime", "clusters"))

	steps := first["spec"].(map[string]interface{})["workflow"].(map[string]interface{})["steps"].([]interface{})
	require.Equal(t, []interface{}{"module-runtime", "module-runtime-override"}, steps[1].(map[string]interface{})["properties"].(map[string]interface{})["policies"])
}

func TestRenderApplication_PlacementRequestLeavesUnmarkedObjectsOnHub(t *testing.T) {
	app, err := RenderApplication(storeModule(false), "", &v1alpha1.Placement{Clusters: []string{"spoke-a"}})
	require.NoError(t, err)
	require.Equal(t, []string{
		"store-aux-hub",
		"store-v1-aux-hub",
		"store-v1-defs",
		"store-v1beta1-defs",
		"store-v2-aux-hub",
		"store-v2-defs",
	}, componentNames(t, app))
	require.Empty(t, policyProp(t, app, "module-runtime-override", "selector"))
	require.Equal(t, []interface{}{
		"store-aux-hub",
		"store-v1-aux-hub",
		"store-v1-defs",
		"store-v1beta1-defs",
		"store-v2-aux-hub",
		"store-v2-defs",
	}, policyProp(t, app, "module-hub-override", "selector"))
	require.Equal(t, []interface{}{"spoke-a"}, policyProp(t, app, "module-runtime", "clusters"))
}

func TestRenderApplication_LineWithoutDefinitionsHasNoDefsTier(t *testing.T) {
	mod := &module.Module{
		Name:    "store",
		Version: "1.0.0",
		Lines: map[string]module.Line{
			"v1": {
				APIVersion: "v1",
				Enabled:    true,
				Auxiliary:  []map[string]interface{}{placed("store-v1-controller", oam.PlacementHub)},
			},
		},
	}
	app, err := RenderApplication(mod, "", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"store-v1-aux-hub"}, componentNames(t, app))
}

func TestRenderApplication_InvalidPlacementClassFails(t *testing.T) {
	mod := storeModule(false)
	mod.Auxiliary[0] = placed("store-defaults", "spoke")
	_, err := RenderApplication(mod, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "store-defaults")
}

func TestRenderApplication_LineDefaultRuntimeYieldsToHubLabel(t *testing.T) {
	mod := &module.Module{
		Name:      "store",
		Version:   "1.0.0",
		Placement: oam.PlacementRuntime,
		Auxiliary: []map[string]interface{}{auxObject("ConfigMap", "store-defaults")},
		Lines: map[string]module.Line{
			"v1": {
				APIVersion: "v1",
				Enabled:    true,
				Placement:  oam.PlacementRuntime,
				Auxiliary: []map[string]interface{}{
					auxObject("ConfigMap", "store-v1-defaults"),
					placed("store-v1-controller", oam.PlacementHub),
				},
				Definitions: []map[string]interface{}{definition("bucket")},
			},
		},
	}
	app, err := RenderApplication(mod, "", &v1alpha1.Placement{Clusters: []string{"spoke"}})
	require.NoError(t, err)
	require.Equal(t, []string{
		"store-aux",
		"store-v1-aux",
		"store-v1-aux-hub",
		"store-v1-defs",
	}, componentNames(t, app))
	require.Equal(t, []interface{}{"store-aux", "store-v1-aux"}, policyProp(t, app, "module-runtime-override", "selector"))
	require.Equal(t, []interface{}{"store-v1-aux-hub", "store-v1-defs"}, policyProp(t, app, "module-hub-override", "selector"))
	require.Equal(t, []interface{}{"spoke"}, policyProp(t, app, "module-runtime", "clusters"))
}

func TestRenderApplication_LineDefaultBeatsModuleDefault(t *testing.T) {
	mod := &module.Module{
		Name:      "store",
		Version:   "1.0.0",
		Placement: oam.PlacementRuntime,
		Auxiliary: []map[string]interface{}{auxObject("ConfigMap", "store-defaults")},
		Lines: map[string]module.Line{
			"v1": {
				APIVersion: "v1",
				Enabled:    true,
				Placement:  oam.PlacementHub,
				Auxiliary: []map[string]interface{}{
					auxObject("ConfigMap", "store-v1-defaults"),
					placed("store-v1-extra", oam.PlacementRuntime),
				},
				Definitions: []map[string]interface{}{definition("bucket")},
			},
		},
	}
	app, err := RenderApplication(mod, "", &v1alpha1.Placement{Clusters: []string{"spoke"}})
	require.NoError(t, err)
	require.Equal(t, []interface{}{"store-aux", "store-v1-aux"}, policyProp(t, app, "module-runtime-override", "selector"))
	require.Equal(t, []interface{}{"store-v1-aux-hub", "store-v1-defs"}, policyProp(t, app, "module-hub-override", "selector"))
}

func policyNames(policies []interface{}) []string {
	out := make([]string, len(policies))
	for i, p := range policies {
		out[i] = p.(map[string]interface{})["name"].(string)
	}
	return out
}

func stepNames(steps []interface{}) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.(map[string]interface{})["name"].(string)
	}
	return out
}

func policyProp(t *testing.T, app map[string]interface{}, policyName, key string) interface{} {
	t.Helper()
	for _, p := range app["spec"].(map[string]interface{})["policies"].([]interface{}) {
		pol := p.(map[string]interface{})
		if pol["name"] != policyName {
			continue
		}
		return pol["properties"].(map[string]interface{})[key]
	}
	t.Fatalf("no policy %s", policyName)
	return nil
}
