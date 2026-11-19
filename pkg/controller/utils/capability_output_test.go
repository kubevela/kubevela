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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/features"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func schemaConfigMaps(t *testing.T, k8sClient client.Client, names ...string) []corev1.ConfigMap {
	t.Helper()
	var cms []corev1.ConfigMap
	for _, name := range names {
		var cm corev1.ConfigMap
		require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "vela-system", Name: name}, &cm), name)
		cms = append(cms, cm)
	}
	return cms
}

func TestComponentDefinitionStoresOutputSchemas(t *testing.T) {
	ctx := context.Background()
	def := &v1beta1.ComponentDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.ComponentDefinitionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "vela-system", UID: "def-uid"},
		Spec: v1beta1.ComponentDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {
	image: string
	port:  *80 | int
}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{image: parameter.image}]
}
outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
	spec: ports: [{port: parameter.port}]
}
`}}},
	}
	rev := &v1beta1.DefinitionRevision{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.DefinitionRevisionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "web-v1", Namespace: "vela-system", UID: "rev-uid"},
		Spec:       v1beta1.DefinitionRevisionSpec{ComponentDefinition: *def},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(rev).Build()

	capability := NewCapabilityComponentDef(def)
	_, err := capability.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "web", "web-v1")
	require.NoError(t, err)

	for _, cm := range schemaConfigMaps(t, k8sClient, "component-schema-web", "component-schema-web-v1") {
		var output map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputSchema]), &output), cm.Name)
		assert.Contains(t, output["properties"], "spec", cm.Name)
		assert.Contains(t, output["properties"], "status", cm.Name)

		var outputs map[string]map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputsSchema]), &outputs), cm.Name)
		require.Contains(t, outputs, "svc", cm.Name)
		assert.Contains(t, outputs["svc"]["properties"], "spec", cm.Name)
	}
}

func TestTraitDefinitionStoresOutputsSchema(t *testing.T) {
	ctx := context.Background()
	def := &v1beta1.TraitDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.TraitDefinitionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "expose", Namespace: "vela-system", UID: "def-uid"},
		Spec: v1beta1.TraitDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: port: int
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	spec: ports: [{port: parameter.port}]
}
`}}},
	}
	rev := &v1beta1.DefinitionRevision{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.DefinitionRevisionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "expose-v1", Namespace: "vela-system", UID: "rev-uid"},
		Spec:       v1beta1.DefinitionRevisionSpec{TraitDefinition: *def},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(rev).Build()

	capability := NewCapabilityTraitDef(def)
	_, err := capability.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "expose", "expose-v1")
	require.NoError(t, err)

	for _, cm := range schemaConfigMaps(t, k8sClient, "trait-schema-expose", "trait-schema-expose-v1") {
		assert.NotContains(t, cm.Data, types.OutputSchema, cm.Name)
		var outputs map[string]map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputsSchema]), &outputs), cm.Name)
		require.Contains(t, outputs, "service", cm.Name)
		assert.Contains(t, outputs["service"]["properties"], "status", cm.Name)
	}
}

func TestOutputSchemaDataLeavesOutWhatTheTemplateLacks(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, outputSchemaData(ctx, []inherit.Level{{Name: "x", Template: `parameter: a: string`}}))
	assert.Empty(t, outputSchemaData(ctx, []inherit.Level{{Name: "x", Template: `output: {`}}), "a template that does not parse stores no output keys")
}

func TestExtendingDefinitionsStoreInheritedOutputSchemas(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableDefinitionInheritance, true)
	ctx := context.Background()
	component := func(name, extends, template string) *v1beta1.ComponentDefinition {
		return &v1beta1.ComponentDefinition{
			TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.ComponentDefinitionKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system", UID: k8stypes.UID(name)},
			Spec: v1beta1.ComponentDefinitionSpec{Extends: extends,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}},
		}
	}
	trait := func(name, extends, template string) *v1beta1.TraitDefinition {
		return &v1beta1.TraitDefinition{
			TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.TraitDefinitionKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system", UID: k8stypes.UID(name)},
			Spec: v1beta1.TraitDefinitionSpec{Extends: extends,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}},
		}
	}
	base := component("base-web", "", `
parameter: image: string
output: {
	kind: "Deployment"
	spec: replicas: 1
}
outputs: svc: kind: "Service"
`)
	middle := component("team-web", "base-web", `
parameter: image: string
$super: properties: image: parameter.image
output: metadata: labels: team: "a"
`)
	leaf := component("tenant-web", "team-web", `
parameter: {
	image:  string
	tenant: string
}
$super: properties: image: parameter.image
output: metadata: labels: tenant: parameter.tenant
outputs: quota: kind: "ResourceQuota"
`)
	expose := trait("base-expose", "", `
parameter: port: int
outputs: service: spec: port: parameter.port
`)
	ownExpose := trait("own-expose", "base-expose", `
parameter: port: int
$super: properties: port: parameter.port
$inherit: outputs: false
outputs: ingress: kind: "Ingress"
`)
	rev := func(name string, spec v1beta1.DefinitionRevisionSpec) *v1beta1.DefinitionRevision {
		return &v1beta1.DefinitionRevision{
			TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.DefinitionRevisionKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system", UID: k8stypes.UID(name)},
			Spec:       spec,
		}
	}
	k8sClient := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(base, middle, leaf, expose, ownExpose,
		rev("tenant-web-v1", v1beta1.DefinitionRevisionSpec{ComponentDefinition: *leaf}),
		rev("own-expose-v1", v1beta1.DefinitionRevisionSpec{TraitDefinition: *ownExpose})).Build()

	leafCap := NewCapabilityComponentDef(leaf)
	_, err := leafCap.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "tenant-web", "tenant-web-v1")
	require.NoError(t, err)
	for _, cm := range schemaConfigMaps(t, k8sClient, "component-schema-tenant-web", "component-schema-tenant-web-v1") {
		var output map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputSchema]), &output), cm.Name)
		assert.Contains(t, output["properties"], "spec", "%s: the root's fields are inherited", cm.Name)
		labels := output["properties"].(map[string]any)["metadata"].(map[string]any)["properties"].(map[string]any)["labels"].(map[string]any)
		assert.Contains(t, labels["properties"], "team", "%s: and the middle level's", cm.Name)
		assert.Contains(t, labels["properties"], "tenant", "%s: under the leaf's own", cm.Name)

		var outputs map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputsSchema]), &outputs), cm.Name)
		assert.Contains(t, outputs, "svc", cm.Name)
		assert.Contains(t, outputs, "quota", cm.Name)
	}

	traitCap := NewCapabilityTraitDef(ownExpose)
	_, err = traitCap.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "own-expose", "own-expose-v1")
	require.NoError(t, err)
	for _, cm := range schemaConfigMaps(t, k8sClient, "trait-schema-own-expose", "trait-schema-own-expose-v1") {
		var outputs map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OutputsSchema]), &outputs), cm.Name)
		assert.Contains(t, outputs, "ingress", cm.Name)
		assert.NotContains(t, outputs, "service", "%s: $inherit turns the parent's outputs off", cm.Name)
	}
}

func TestExtendingDefinitionWithMissingParentStoresNoOutputSchemas(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableDefinitionInheritance, true)
	ctx := context.Background()
	orphan := &v1beta1.ComponentDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.ComponentDefinitionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "vela-system", UID: "orphan"},
		Spec: v1beta1.ComponentDefinitionSpec{Extends: "gone", Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: image: string
$super: properties: image: parameter.image
output: metadata: labels: a: "b"
`}}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(orphan, &v1beta1.DefinitionRevision{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: v1beta1.DefinitionRevisionKind},
		ObjectMeta: metav1.ObjectMeta{Name: "orphan-v1", Namespace: "vela-system", UID: "orphan-v1"},
		Spec:       v1beta1.DefinitionRevisionSpec{ComponentDefinition: *orphan},
	}).Build()

	capability := NewCapabilityComponentDef(orphan)
	_, err := capability.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "orphan", "orphan-v1")
	require.NoError(t, err, "the parameter schema still publishes")
	cm := schemaConfigMaps(t, k8sClient, "component-schema-orphan")[0]
	assert.NotEmpty(t, cm.Data[types.OpenapiV3JSONSchema])
	assert.NotContains(t, cm.Data, types.OutputSchema, "a partial output would describe something the chain cannot render")
}
