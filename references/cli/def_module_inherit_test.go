/*
Copyright 2025 The KubeVela Authors.

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

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
	"github.com/oam-dev/kubevela/pkg/definition/goloader"
)

func component(name, extends string) goloader.LoadResult {
	c := defkit.NewComponent(name).Template(func(tpl *defkit.Template) {
		tpl.Output(defkit.NewResource("v1", "ConfigMap"))
	})
	if extends != "" {
		c.Extends(defkit.ComponentRef(extends))
	}
	return goloader.LoadResult{CUE: c.ToCue(), Definition: goloader.DefinitionInfo{Name: name, Type: "component"}}
}

func trait(name string) goloader.LoadResult {
	t := defkit.NewTrait(name).Template(func(tpl *defkit.Template) {
		tpl.Patch().Set("metadata.labels.a", defkit.Lit("b"))
	})
	return goloader.LoadResult{CUE: t.ToCue(), Definition: goloader.DefinitionInfo{Name: name, Type: "trait"}}
}

func names(defs []moduleDefinition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.def.GetKind()+"/"+d.def.GetName())
	}
	return out
}

func TestPrepareModuleDefinitionsAppliesParentsFirst(t *testing.T) {
	defs, err := prepareModuleDefinitions([]goloader.LoadResult{
		component("grandchild", "child@v2"),
		trait("base"),
		component("child", "base"),
		component("unrelated", ""),
		component("base", ""),
		component("external-child", "webservice"),
	}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ComponentDefinition/base",
		"ComponentDefinition/child",
		"ComponentDefinition/grandchild",
		"TraitDefinition/base",
		"ComponentDefinition/unrelated",
		"ComponentDefinition/external-child",
	}, names(defs))
}

func TestPrepareModuleDefinitionsRefusesACycle(t *testing.T) {
	_, err := prepareModuleDefinitions([]goloader.LoadResult{
		component("a", "b"),
		component("b", "a@v1"),
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ComponentDefinition a extends b, which extends a")
}

func TestPrepareModuleDefinitionsPrefixesParentsInTheModule(t *testing.T) {
	defs, err := prepareModuleDefinitions([]goloader.LoadResult{
		component("base", ""),
		component("child", "base"),
		component("pinned", "base@v3"),
		component("external-child", "webservice"),
	}, "org-")
	require.NoError(t, err)

	extends := map[string]string{}
	for _, d := range defs {
		extends[d.def.GetName()] = definitionExtends(d.def)
	}
	assert.Equal(t, map[string]string{
		"org-base":           "",
		"org-child":          "org-base",
		"org-pinned":         "org-base@v3",
		"org-external-child": "webservice",
	}, extends)
}

func TestPrepareModuleDefinitionsKeepsParseErrors(t *testing.T) {
	defs, err := prepareModuleDefinitions([]goloader.LoadResult{
		{CUE: "not: [cue", Definition: goloader.DefinitionInfo{Name: "broken", Type: "component"}},
		component("fine", ""),
	}, "")
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Error(t, defs[0].err)
	assert.Equal(t, "fine", defs[1].def.GetName())
}
