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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

func cdLevel(name, extends string, wl common.WorkloadTypeDescriptor) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   extends,
			Workload:  wl,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "$super: properties: {}\nparameter: {}"}},
		},
	}
}

func fetcherFor(defs ...*v1beta1.ComponentDefinition) componentFetcher {
	byName := map[string]*v1beta1.ComponentDefinition{}
	for _, d := range defs {
		byName[d.Name] = d
	}
	return func(_ context.Context, name string) (*v1beta1.ComponentDefinition, error) {
		return byName[name].DeepCopy(), nil
	}
}

// Three levels, and the leaf states its own workload. Whatever the middle level
// is silent about is the middle level's business; the leaf said what it is.
func TestALeafsOwnWorkloadSurvivesAThreeLevelChain(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	leaf := cdLevel("leaf", "mid", common.WorkloadTypeDescriptor{
		Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}})
	mid := cdLevel("mid", "root", common.WorkloadTypeDescriptor{})
	root := cdLevel("root", "", common.WorkloadTypeDescriptor{
		Definition: common.WorkloadGVK{APIVersion: "batch/v1", Kind: "CronJob"}})

	tmpl := &Template{Reference: leaf.Spec.Workload}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl, leaf, fetcherFor(mid, root)))

	require.Equal(t, "Deployment", tmpl.Reference.Definition.Kind,
		"the leaf declared Deployment; the root's CronJob is not its workload")
}

// The leaf is silent and so is the middle level, so the root's workload is what
// it renders as, and the leaf's own spec has to say so too: the definition
// recorded in the ApplicationRevision is read back later.
func TestASilentLeafTakesTheRootsWorkloadThroughAMiddleLevel(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	leaf := cdLevel("leaf", "mid", common.WorkloadTypeDescriptor{})
	mid := cdLevel("mid", "root", common.WorkloadTypeDescriptor{})
	root := cdLevel("root", "", common.WorkloadTypeDescriptor{
		Definition: common.WorkloadGVK{APIVersion: "batch/v1", Kind: "CronJob"}})

	tmpl := &Template{Reference: leaf.Spec.Workload}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl, leaf, fetcherFor(mid, root)))

	require.Equal(t, "CronJob", tmpl.Reference.Definition.Kind)
	require.Equal(t, "CronJob", leaf.Spec.Workload.Definition.Kind,
		"the leaf's own spec is what the revision records")
}

// A boolean attribute is true if any level sets it, however far up.
func TestATraitInheritsPodDisruptiveFromItsGrandparent(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	td := func(name, extends string, disruptive bool) *v1beta1.TraitDefinition {
		return &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
			Spec: v1beta1.TraitDefinitionSpec{
				Extends:       extends,
				PodDisruptive: disruptive,
				Schematic:     &common.Schematic{CUE: &common.CUE{Template: "$super: properties: {}\nparameter: {}"}},
			},
		}
	}
	leaf, mid, root := td("leaf", "mid", false), td("mid", "root", false), td("root", "", true)

	byName := map[string]*v1beta1.TraitDefinition{"mid": mid, "root": root}
	fetch := func(_ context.Context, name string) (*v1beta1.TraitDefinition, error) {
		return byName[name].DeepCopy(), nil
	}

	tmpl := &Template{}
	require.NoError(t, resolveTraitChain(context.Background(), tmpl, leaf, fetch))

	require.True(t, leaf.Spec.PodDisruptive,
		"the root is pod-disruptive, so anything extending it is too")
}
