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

package utils

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

func schemaClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func schemaComponent(namespace, name, extends, template string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   extends,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

// The published schema is what an Application is validated against and what
// VelaUX draws a form from, so for an extending definition it has to be the
// chain's parameters, not the child's own. Compiling the child alone would fail
// outright: `$super` is declared nowhere in it.
func TestPublishedSchemaOfAnExtendingComponent(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	child := schemaComponent("vela-system", "tenant-webservice", "webservice", schemaChild)
	cli := schemaClient(t,
		schemaComponent("vela-system", "webservice", "", schemaParent),
		child,
	)

	raw, err := inheritedComponentSchema(context.Background(), cli, child)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &doc))

	props, ok := doc["properties"].(map[string]interface{})
	require.True(t, ok, "schema has properties: %s", raw)

	require.Contains(t, props, "tenant", "the child's own")
	require.Contains(t, props, "image", "and what it passes up")
	require.Contains(t, props, "replicas", "and what the parent defaults, which an app may override")
}

func TestPublishedSchemaOfAnExtendingTrait(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	parent := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "base-labels", Namespace: "vela-system"},
		Spec: v1beta1.TraitDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `
patch: metadata: labels: team: parameter.team

parameter: {team: string}
`}},
		},
	}
	child := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-labels", Namespace: "vela-system"},
		Spec: v1beta1.TraitDefinitionSpec{
			Extends: "base-labels",
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `
$super: properties: {team: parameter.team}

parameter: {
	team:   string
	tenant: string
}
`}},
		},
	}

	raw, err := inheritedTraitSchema(context.Background(), schemaClient(t, parent, child), child)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &doc))
	props, ok := doc["properties"].(map[string]interface{})
	require.True(t, ok, "schema has properties: %s", raw)

	require.Contains(t, props, "tenant")
	require.Contains(t, props, "team")
}

// A parent that is not there is reported, rather than publishing a schema that
// describes only half the definition.
func TestPublishedSchemaNeedsItsParent(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, true)

	child := schemaComponent("vela-system", "orphan", "gone", schemaChild)

	_, err := inheritedComponentSchema(context.Background(), schemaClient(t, child), child)
	require.Error(t, err)
}
