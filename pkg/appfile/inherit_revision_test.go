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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// The reproducibility guarantee, at the point where it is actually decided.
//
// A render that went through a chain can only be replayed if every level it went
// through was recorded beside the child. These maps are what the
// ApplicationRevision is built from, so what lands here is what an application
// can be rebuilt from months later, after the parent has moved on.
func TestAncestorsReachTheRelatedDefinitions(t *testing.T) {
	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	comp := &Component{
		Name: "billing-api",
		FullTemplate: &Template{
			ComponentDefinition: compDef("tenant-webservice", "webservice@v3", "$super: properties: {}"),
			AncestorComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
				"webservice@v3": compDef("webservice", "", "output: base: true"),
			},
		},
		Traits: []*Trait{{
			Name: "tenant-labels",
			FullTemplate: &Template{
				TraitDefinition: &v1beta1.TraitDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "tenant-labels"},
				},
				AncestorTraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"labels": {ObjectMeta: metav1.ObjectMeta{Name: "labels"}},
				},
			},
		}},
	}

	setComponentDefinitions(af, []*Component{comp})

	require.Contains(t, af.RelatedComponentDefinitions, "tenant-webservice", "the child itself")
	require.Contains(t, af.RelatedComponentDefinitions, "webservice@v3", "and what it extends")
	require.Equal(t, "output: base: true",
		af.RelatedComponentDefinitions["webservice@v3"].Spec.Schematic.CUE.Template,
		"recorded as it was at render time, not as a name to look up later")

	require.Contains(t, af.RelatedTraitDefinitions, "tenant-labels")
	require.Contains(t, af.RelatedTraitDefinitions, "labels")
}

// A component that extends nothing puts nothing extra in the map, so a cluster
// full of ordinary definitions carries no cost from this.
func TestNoAncestorsNoExtraEntries(t *testing.T) {
	af := &Appfile{
		RelatedComponentDefinitions: map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:     map[string]*v1beta1.TraitDefinition{},
	}
	setComponentDefinitions(af, []*Component{{
		Name:         "billing-api",
		FullTemplate: &Template{ComponentDefinition: compDef("webservice", "", "output: {}")},
	}})

	require.Len(t, af.RelatedComponentDefinitions, 1)
	require.Empty(t, af.RelatedTraitDefinitions,
		"nothing extra means nothing in either map, not just the one being read")
}
