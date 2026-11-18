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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// Every revision of a definition carries the definition's own name, so two
// components pinning different revisions of it would share one entry if the
// entry were keyed by that name.
func TestComponentsPinningDifferentRevisionsEachKeepTheirOwn(t *testing.T) {
	p := &Parser{}
	v1, err := p.convertTemplate2Component("old", "bar@v1", types.TypeComponentDefinition, nil,
		&Template{ComponentDefinition: namedDef("bar", "output: revision: 1")})
	require.NoError(t, err)
	v2, err := p.convertTemplate2Component("new", "bar@v2", types.TypeComponentDefinition, nil,
		&Template{ComponentDefinition: namedDef("bar", "output: revision: 2")})
	require.NoError(t, err)

	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	setComponentDefinitions(af, []*Component{v1, v2})

	require.Contains(t, af.RelatedComponentDefinitions, "bar@v1")
	require.Equal(t, "output: revision: 1", af.RelatedComponentDefinitions["bar@v1"].Spec.Schematic.CUE.Template)
	require.Contains(t, af.RelatedComponentDefinitions, "bar@v2")
	require.Equal(t, "output: revision: 2", af.RelatedComponentDefinitions["bar@v2"].Spec.Schematic.CUE.Template)
	require.Contains(t, af.RelatedComponentDefinitions, "bar", "the bare entry stays for readers that look up by name")
}

func TestTraitsPinningDifferentRevisionsEachKeepTheirOwn(t *testing.T) {
	p := &Parser{}
	v1, err := p.convertTemplate2Trait("scaler@v1", nil, &Template{TraitDefinition: traitDef("scaler", "", "outputs: revision: 1")})
	require.NoError(t, err)
	v2, err := p.convertTemplate2Trait("scaler@v2", nil, &Template{TraitDefinition: traitDef("scaler", "", "outputs: revision: 2")})
	require.NoError(t, err)

	comp := &Component{
		Name:         "web",
		FullTemplate: &Template{ComponentDefinition: namedDef("web", "output: {}")},
		Traits:       []*Trait{v1, v2},
	}
	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	setComponentDefinitions(af, []*Component{comp})

	require.Contains(t, af.RelatedTraitDefinitions, "scaler@v1")
	require.Equal(t, "outputs: revision: 1", af.RelatedTraitDefinitions["scaler@v1"].Spec.Schematic.CUE.Template)
	require.Contains(t, af.RelatedTraitDefinitions, "scaler@v2")
	require.Equal(t, "outputs: revision: 2", af.RelatedTraitDefinitions["scaler@v2"].Spec.Schematic.CUE.Template)
	require.Contains(t, af.RelatedTraitDefinitions, "scaler")
}

// A replay renders a pinned component from the entry recorded for that pin,
// and from the bare entry only when the revision predates pinned entries.
func TestRevisionReplayReadsThePinnedEntry(t *testing.T) {
	def := func(template string) *v1beta1.ComponentDefinition {
		return &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "bar"},
			Spec: v1beta1.ComponentDefinitionSpec{
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
				Workload:  common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "Pod"}},
			},
		}
	}
	rev := func(defs map[string]*v1beta1.ComponentDefinition) *v1beta1.ApplicationRevision {
		return &v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "app-v1"},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{ComponentDefinitions: defs},
			},
		}
	}

	t.Run("the pinned entry wins over the bare one", func(t *testing.T) {
		apprev := rev(map[string]*v1beta1.ComponentDefinition{
			"bar":    def("output: revision: 2"),
			"bar@v1": def("output: revision: 1"),
			"bar@v2": def("output: revision: 2"),
		})
		tmpl, err := LoadTemplateFromRevision("bar@v1", types.TypeComponentDefinition, apprev, fakeRESTMapper{})
		require.NoError(t, err)
		require.Equal(t, "output: revision: 1", tmpl.TemplateStr)
	})

	t.Run("a revision with only the bare entry still replays", func(t *testing.T) {
		apprev := rev(map[string]*v1beta1.ComponentDefinition{"bar": def("output: revision: 1")})
		tmpl, err := LoadTemplateFromRevision("bar@v1", types.TypeComponentDefinition, apprev, fakeRESTMapper{})
		require.NoError(t, err)
		require.Equal(t, "output: revision: 1", tmpl.TemplateStr)
	})
}
