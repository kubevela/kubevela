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

package appfile

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func namedDef(name, template string) *v1beta1.ComponentDefinition {
	cd := compDef(name, "", template)
	return cd
}

// An application can use a definition directly and, through another component,
// extend a different revision of the same one. Both go into the revision, and
// keying either by the bare definition name loses one of them: a replay then
// renders a component against a definition it never used.
func TestADirectDefinitionAndAnInheritedRevisionBothSurvive(t *testing.T) {
	// One component is `bar` directly. The definition embedded in a pinned
	// revision carries the plain name too, which is what makes this collide.
	direct := &Component{
		Name: "uses-live",
		FullTemplate: &Template{
			ComponentDefinition: namedDef("bar", "output: live: true"),
		},
	}

	// Another extends `bar@v2`, so that revision is recorded beside it.
	inherited := &Component{
		Name: "extends-pinned",
		FullTemplate: &Template{
			ComponentDefinition: namedDef("child", "$super: properties: {}"),
			AncestorComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
				"bar@v2": namedDef("bar", "output: pinned: true"),
			},
		},
	}

	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	setComponentDefinitions(af, []*Component{direct, inherited})

	require.Contains(t, af.RelatedComponentDefinitions, "bar", "the live definition")
	require.Contains(t, af.RelatedComponentDefinitions, "bar@v2", "and the pinned revision beside it")
	require.Contains(t, af.RelatedComponentDefinitions, "child")

	require.Equal(t, "output: live: true",
		af.RelatedComponentDefinitions["bar"].Spec.Schematic.CUE.Template,
		"the live entry must not be overwritten by the pinned one")
	require.Equal(t, "output: pinned: true",
		af.RelatedComponentDefinitions["bar@v2"].Spec.Schematic.CUE.Template)
}

// The same definition reached both directly and as an unpinned ancestor is one
// entry, not two, and the entry is that definition.
func TestAnUnpinnedAncestorSharesTheDirectEntry(t *testing.T) {
	direct := &Component{
		Name:         "uses-bar",
		FullTemplate: &Template{ComponentDefinition: namedDef("bar", "output: bar: true")},
	}
	inherited := &Component{
		Name: "extends-bar",
		FullTemplate: &Template{
			ComponentDefinition: namedDef("child", "$super: properties: {}"),
			AncestorComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
				"bar": namedDef("bar", "output: bar: true"),
			},
		},
	}

	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	setComponentDefinitions(af, []*Component{direct, inherited})

	require.Len(t, af.RelatedComponentDefinitions, 2)
	require.Equal(t, "output: bar: true",
		af.RelatedComponentDefinitions["bar"].Spec.Schematic.CUE.Template)
}
