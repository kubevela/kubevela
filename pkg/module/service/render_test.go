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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// definition builds a minimal ComponentDefinition map as the parser produces it.
func definition(name string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "ComponentDefinition",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{},
	}
}

// auxObject builds a minimal auxiliary object of the given kind. The apiVersion
// is a placeholder: the renderer reads neither it nor the kind, and passes every
// object through untouched. Tests name a kind only where it makes the fixture
// easier to read, or where one asserts that several kinds share a tier.
func auxObject(kind, name string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
	}
}

// fixtureModule is an s3 module with a module-wide auxiliary XRD and one
// enabled line carrying an auxiliary Composition and one definition.
func fixtureModule() *module.Module {
	return &module.Module{
		Name:      "s3",
		Version:   "1.0.0",
		Auxiliary: []map[string]interface{}{auxObject("CompositeResourceDefinition", "xbuckets.aws.platform.io")},
		Lines: map[string]module.Line{
			"v1": {
				APIVersion:  "v1",
				Enabled:     true,
				Auxiliary:   []map[string]interface{}{auxObject("Composition", "xbuckets-v1")},
				Definitions: []map[string]interface{}{definition("bucket")},
			},
		},
	}
}

// components returns the owned Application's spec.components as typed maps.
func components(t *testing.T, app map[string]interface{}) []map[string]interface{} {
	t.Helper()
	spec, ok := app["spec"].(map[string]interface{})
	require.True(t, ok, "app has no spec")
	raw, ok := spec["components"].([]interface{})
	require.True(t, ok, "app has no spec.components")
	out := make([]map[string]interface{}, 0, len(raw))
	for _, c := range raw {
		m, ok := c.(map[string]interface{})
		require.True(t, ok)
		out = append(out, m)
	}
	return out
}

func TestRenderApplication_OwnedApplicationShape(t *testing.T) {
	app, err := RenderApplication(fixtureModule(), "")
	require.NoError(t, err)

	require.Equal(t, "core.oam.dev/v1beta1", app["apiVersion"])
	require.Equal(t, "Application", app["kind"])
	meta, ok := app["metadata"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "module-s3", meta["name"])

	comps := components(t, app)
	require.Len(t, comps, 3)
	require.Equal(t, "s3-aux", comps[0]["name"])
	require.Equal(t, "s3-v1-aux", comps[1]["name"])
	require.Equal(t, "s3-v1-defs", comps[2]["name"])
}

// TestRenderApplication_StampsResolvedVersionAnnotation asserts the owned
// Application carries the concrete module package version (from the parsed
// module's own _module.cue) as an annotation, so a cluster reader sees the
// installed version without the CLI -- including when a "latest" request
// resolved to a specific tag, since mod.Version is always the tag that was
// actually fetched, never the raw request.
func TestRenderApplication_StampsResolvedVersionAnnotation(t *testing.T) {
	app, err := RenderApplication(fixtureModule(), "")
	require.NoError(t, err)

	meta := app["metadata"].(map[string]interface{})
	annos, ok := meta["annotations"].(map[string]interface{})
	require.True(t, ok, "owned app has no metadata.annotations")
	require.Equal(t, "1.0.0", annos[types.AnnoDefinitionModuleVersion])
}

func TestRenderApplication_DefaultsToVelaSystemWithLabel(t *testing.T) {
	app, err := RenderApplication(&module.Module{Name: "s3"}, "")
	require.NoError(t, err)

	meta := app["metadata"].(map[string]interface{})
	require.Equal(t, types.DefaultKubeVelaNS, meta["namespace"])
	require.Equal(t, "module-s3", meta["name"])

	labels := meta["labels"].(map[string]interface{})
	require.Equal(t, "s3", labels[types.LabelDefinitionModule])
}

func TestRenderApplication_HonorsChosenNamespace(t *testing.T) {
	app, err := RenderApplication(&module.Module{Name: "s3"}, "team-a")
	require.NoError(t, err)

	meta := app["metadata"].(map[string]interface{})
	require.Equal(t, "team-a", meta["namespace"])
}

// twoLineModule ships v1 and v2, both with an auxiliary Composition and a
// definition.
func twoLineModule(v1Enabled, v2Enabled bool) *module.Module {
	line := func(v string, enabled bool) module.Line {
		return module.Line{
			APIVersion:  v,
			Enabled:     enabled,
			Auxiliary:   []map[string]interface{}{auxObject("Composition", "xbuckets-"+v)},
			Definitions: []map[string]interface{}{definition("bucket")},
		}
	}
	return &module.Module{
		Name:      "s3",
		Version:   "1.0.0",
		Auxiliary: []map[string]interface{}{auxObject("CompositeResourceDefinition", "xbuckets.aws.platform.io")},
		Lines: map[string]module.Line{
			"v1": line("v1", v1Enabled),
			"v2": line("v2", v2Enabled),
		},
	}
}

func names(comps []map[string]interface{}) []string {
	out := make([]string, 0, len(comps))
	for _, c := range comps {
		out = append(out, c["name"].(string))
	}
	return out
}

func TestRenderApplication_BothEnabledLinesInstall(t *testing.T) {
	app, err := RenderApplication(twoLineModule(true, true), "")
	require.NoError(t, err)

	require.Equal(t, []string{
		"s3-aux",
		"s3-v1-aux", "s3-v1-defs",
		"s3-v2-aux", "s3-v2-defs",
	}, names(components(t, app)))
}

func TestRenderApplication_DisabledLineIsSkipped(t *testing.T) {
	app, err := RenderApplication(twoLineModule(false, true), "")
	require.NoError(t, err)

	got := names(components(t, app))
	require.Equal(t, []string{"s3-aux", "s3-v2-aux", "s3-v2-defs"}, got)
	require.NotContains(t, got, "s3-v1-defs")
}

func TestRenderApplication_LinesAreSiblingsNotAChain(t *testing.T) {
	app, err := RenderApplication(twoLineModule(true, true), "")
	require.NoError(t, err)

	byName := map[string]map[string]interface{}{}
	for _, c := range components(t, app) {
		byName[c["name"].(string)] = c
	}

	// Both lines hang off the module-level auxiliary tier; v2 must not wait on v1.
	require.Equal(t, []interface{}{"s3-aux"}, byName["s3-v1-aux"]["dependsOn"])
	require.Equal(t, []interface{}{"s3-aux"}, byName["s3-v2-aux"]["dependsOn"])
	require.Equal(t, []interface{}{"s3-v1-aux"}, byName["s3-v1-defs"]["dependsOn"])
	require.Equal(t, []interface{}{"s3-v2-aux"}, byName["s3-v2-defs"]["dependsOn"])
	// The first tier depends on nothing.
	require.NotContains(t, byName["s3-aux"], "dependsOn")
}

func TestRenderApplication_NoModuleAuxiliaryOmitsThatTier(t *testing.T) {
	mod := fixtureModule()
	mod.Auxiliary = nil

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)

	comps := components(t, app)
	require.Equal(t, []string{"s3-v1-aux", "s3-v1-defs"}, names(comps))
	// With no module-level tier above it, the line's auxiliary tier
	// depends on nothing.
	require.NotContains(t, comps[0], "dependsOn")
}

func TestRenderApplication_NoLineAuxiliaryOmitsThatTier(t *testing.T) {
	mod := fixtureModule()
	line := mod.Lines["v1"]
	line.Auxiliary = nil
	mod.Lines["v1"] = line

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)

	comps := components(t, app)
	require.Equal(t, []string{"s3-aux", "s3-v1-defs"}, names(comps))
	// The definitions tier falls back to the module-level auxiliary tier.
	require.Equal(t, []interface{}{"s3-aux"}, comps[1]["dependsOn"])
}

func TestRenderApplication_ModuleWithoutAuxiliaryIsDefinitionsOnly(t *testing.T) {
	mod := fixtureModule()
	mod.Auxiliary = nil
	line := mod.Lines["v1"]
	line.Auxiliary = nil
	mod.Lines["v1"] = line

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)

	comps := components(t, app)
	require.Equal(t, []string{"s3-v1-defs"}, names(comps))
	require.NotContains(t, comps[0], "dependsOn")
}

func TestRenderApplication_AllLinesDisabledYieldsOnlyTheModuleAuxiliaryTier(t *testing.T) {
	app, err := RenderApplication(twoLineModule(false, false), "")
	require.NoError(t, err)

	require.Equal(t, []string{"s3-aux"}, names(components(t, app)))
}

// TestRenderApplication_TierNamesComeFromTheMapKey documents current behaviour,
// not desired behaviour. The parser keys Lines by the apiVersion declared inside
// v<N>/_version.cue and never checks it against the directory name, so a v1/
// directory whose _version.cue says "v9beta1" lands under that key. The render
// iterates the map, so the mismatch surfaces as tiers named for the declared
// version rather than the directory. Recorded so the behaviour is visible if
// someone hits it; fixing the parser is tracked separately.
func TestRenderApplication_TierNamesComeFromTheMapKey(t *testing.T) {
	mod := fixtureModule()
	line := mod.Lines["v1"]
	delete(mod.Lines, "v1")
	mod.Lines["v9beta1"] = line // declared apiVersion disagreed with the directory

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)

	require.Equal(t, []string{"s3-aux", "s3-v9beta1-aux", "s3-v9beta1-defs"}, names(components(t, app)))
}

// TestRenderApplication_TiersWaitOnNothing asserts no tier carries a readiness
// gate. Every tier is a plain k8s-objects component, healthy once its objects
// are applied, so the tier beneath it proceeds immediately rather than waiting
// for anything to report ready.
func TestRenderApplication_TiersWaitOnNothing(t *testing.T) {
	app, err := RenderApplication(fixtureModule(), "")
	require.NoError(t, err)

	for _, c := range components(t, app) {
		require.Equal(t, "k8s-objects", c["type"])
		props := c["properties"].(map[string]interface{})
		require.NotContains(t, props, "readyConditionType", "tier %s carries a readiness gate", c["name"])
	}
}

// TestRenderApplication_OneTierPerScopeRegardlessOfKind covers a scope shipping
// several kinds in the same auxiliary/ folder. They all land in a single tier in
// source order: no kind is singled out, which is what lets a module carry KRO
// ResourceGraphDefinitions, plain CRDs, or anything else without the renderer
// knowing about them.
func TestRenderApplication_OneTierPerScopeRegardlessOfKind(t *testing.T) {
	mod := fixtureModule()
	mod.Auxiliary = []map[string]interface{}{
		auxObject("CompositeResourceDefinition", "xbuckets.aws.platform.io"),
		auxObject("ConfigMap", "s3-defaults"),
	}

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)

	comps := components(t, app)
	require.Equal(t, []string{"s3-aux", "s3-v1-aux", "s3-v1-defs"}, names(comps))

	byName := map[string]map[string]interface{}{}
	for _, c := range comps {
		byName[c["name"].(string)] = c
	}
	objects := byName["s3-aux"]["properties"].(map[string]interface{})["objects"].([]interface{})
	require.Len(t, objects, 2, "both objects share one tier")
	require.Equal(t, "CompositeResourceDefinition", objects[0].(map[string]interface{})["kind"])
	require.Equal(t, "ConfigMap", objects[1].(map[string]interface{})["kind"], "source order is preserved")

	require.Equal(t, []interface{}{"s3-aux"}, byName["s3-v1-aux"]["dependsOn"])
}

func TestRenderApplication_StampsDefinitionIdentity(t *testing.T) {
	app, err := RenderApplication(fixtureModule(), "")
	require.NoError(t, err)

	var defs []interface{}
	for _, c := range components(t, app) {
		if c["name"] == "s3-v1-defs" {
			defs = c["properties"].(map[string]interface{})["objects"].([]interface{})
		}
	}
	require.Len(t, defs, 1)
	def := defs[0].(map[string]interface{})

	meta := def["metadata"].(map[string]interface{})
	require.Equal(t, "s3-v1-bucket", meta["name"])

	labels := meta["labels"].(map[string]interface{})
	require.Equal(t, "s3", labels[types.LabelDefinitionModule])
	require.Equal(t, "v1", labels[types.LabelDefinitionModuleAPIVersion])
	require.Equal(t, "bucket", labels[types.LabelDefinitionName])
	require.Equal(t, "s3", labels[oam.LabelAddonName])

	annos := meta["annotations"].(map[string]interface{})
	require.Equal(t, "s3-v1-bucket", annos[types.AnnoDefinitionModuleFullName])

	spec := def["spec"].(map[string]interface{})
	require.Equal(t, "s3", spec["module"])
	require.Equal(t, "v1", spec["apiVersion"])
}

func TestRenderApplication_DoesNotMutateTheFetchedModule(t *testing.T) {
	mod := fixtureModule()
	_, err := RenderApplication(mod, "")
	require.NoError(t, err)

	// The parsed Module is shared and may be cached; the render must copy.
	original := mod.Lines["v1"].Definitions[0]["metadata"].(map[string]interface{})
	require.Equal(t, "bucket", original["name"])
	require.NotContains(t, original, "labels")
}

func TestRenderApplication_TruncatesLongNamesWithAStableHash(t *testing.T) {
	long := strings.Repeat("a", 300)
	mod := fixtureModule()
	line := mod.Lines["v1"]
	line.Definitions = []map[string]interface{}{definition(long)}
	mod.Lines["v1"] = line

	app, err := RenderApplication(mod, "")
	require.NoError(t, err)
	app2, err := RenderApplication(mod, "")
	require.NoError(t, err)

	nameOf := func(a map[string]interface{}) string {
		for _, c := range components(t, a) {
			if c["name"] == "s3-v1-defs" {
				objs := c["properties"].(map[string]interface{})["objects"].([]interface{})
				return objs[0].(map[string]interface{})["metadata"].(map[string]interface{})["name"].(string)
			}
		}
		return ""
	}

	got := nameOf(app)
	require.Len(t, got, 253, "an over-long name is truncated to the Kubernetes limit")
	require.Equal(t, got, nameOf(app2), "the hash suffix must be stable across renders")

	// The full name survives on the annotation even when the object name cannot hold it.
	for _, c := range components(t, app) {
		if c["name"] == "s3-v1-defs" {
			objs := c["properties"].(map[string]interface{})["objects"].([]interface{})
			meta := objs[0].(map[string]interface{})["metadata"].(map[string]interface{})
			annos := meta["annotations"].(map[string]interface{})
			require.Equal(t, "s3-v1-"+long, annos[types.AnnoDefinitionModuleFullName])

			// The name label value stays within the Kubernetes 63-char label limit,
			// so the definitions tier can apply even for an over-long definition name.
			nameLabel := meta["labels"].(map[string]interface{})[types.LabelDefinitionName].(string)
			require.LessOrEqual(t, len(nameLabel), 63)
		}
	}
}
