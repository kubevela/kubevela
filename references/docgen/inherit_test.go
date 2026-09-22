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
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
)

const docParent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: replicas: parameter.replicas
}

parameter: {
	// +usage=Which image to run
	image:    string
	replicas: *1 | int
}
`

const docChild = `
$super: properties: {image: parameter.image}

parameter: {
	// +usage=Owning tenant
	tenant: string
	image:  string
}
`

func docClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func docComponent(name, extends, template string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   extends,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

// Documentation for an extending definition is the chain's parameters. Reading
// its own template alone would list the child's and stop, and that is the list a
// user is told they may supply.
func TestInheritedParameterValueReadsTheChain(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	cli := docClient(t,
		docComponent("webservice", "", docParent),
		docComponent("tenant-webservice", "webservice", docChild),
	)

	capability := &types.Capability{
		Name:        "tenant-webservice",
		Namespace:   "vela-system",
		Type:        types.TypeComponentDefinition,
		Extends:     "webservice",
		CueTemplate: docChild,
	}

	val, err := inheritedParameterValue(context.Background(), cli, capability)
	require.NoError(t, err)

	for _, field := range []string{"tenant", "image"} {
		require.True(t, val.LookupPath(cue.ParsePath(field)).Exists(),
			"%s is the child's own surface and should be documented", field)
	}

	// The child declares a closed `parameter` of its own and forwards only
	// `image`, so the parent's `replicas` is not something an Application using
	// it can set. Documenting it would offer a parameter that goes nowhere.
	require.False(t, val.LookupPath(cue.ParsePath("replicas")).Exists(),
		"what the child does not forward is not its surface")
}

func TestAncestorsOfAComponentAndATrait(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	t.Run("component", func(t *testing.T) {
		cli := docClient(t,
			docComponent("webservice", "", docParent),
			docComponent("tenant-webservice", "webservice", docChild),
		)

		got, err := ancestorsOf(context.Background(), cli, &types.Capability{
			Name: "tenant-webservice", Namespace: "vela-system",
			Type: types.TypeComponentDefinition, Extends: "webservice", CueTemplate: docChild,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "webservice", got[0].Name)
	})

	t.Run("trait", func(t *testing.T) {
		base := &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "vela-system"},
			Spec: v1beta1.TraitDefinitionSpec{
				Schematic: &common.Schematic{CUE: &common.CUE{Template: "patch: {}\nparameter: {}"}},
			},
		}
		child := &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "tenant-gateway", Namespace: "vela-system"},
			Spec: v1beta1.TraitDefinitionSpec{
				Extends:   "gateway",
				Schematic: &common.Schematic{CUE: &common.CUE{Template: "$super: properties: {}\nparameter: {}"}},
			},
		}

		got, err := ancestorsOf(context.Background(), docClient(t, base, child), &types.Capability{
			Name: "tenant-gateway", Namespace: "vela-system",
			Type: types.TypeTrait, Extends: "gateway", CueTemplate: child.Spec.Schematic.CUE.Template,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "gateway", got[0].Name)
	})
}

// Provider functions are off while documenting: printing a parameter table must
// not fetch a chart or call out to a cluster.
func TestTheDocCompilerLeavesProviderFunctionsAlone(t *testing.T) {
	compile := docCompiler()

	// Referenced, not called: the point is that the package resolves at all. A
	// compiler without it registered fails with `builtin package "vela/helm"
	// undefined` and documents nothing.
	val, err := compile(context.Background(), `
import "vela/helm"

render: helm.#Render

parameter: {image: string}
`)
	require.NoError(t, err, "a provider package must still resolve")
	require.True(t, val.LookupPath(cue.ParsePath("parameter.image")).Exists())
	require.True(t, val.LookupPath(cue.ParsePath("render")).Exists(),
		"the definition from the package is there, unevaluated")
}

func TestCueSchematicWrapsATemplate(t *testing.T) {
	s := cueSchematic("output: {}")

	require.NotNil(t, s)
	require.NotNil(t, s.CUE)
	require.Equal(t, "output: {}", s.CUE.Template)
}
