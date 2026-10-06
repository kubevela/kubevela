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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestSourceDefinitionStoreOpenAPISchema(t *testing.T) {
	ctx := context.Background()
	def := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "vela-system", UID: "def-uid", Labels: map[string]string{"team": "data"}},
		Spec: v1beta1.SourceDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: secret: string
schema: {
	host: string
	port: int
}
output: host: "db"
`}}},
	}
	rev := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "db-v1", Namespace: "vela-system", UID: "rev-uid"},
		Spec:       v1beta1.DefinitionRevisionSpec{SourceDefinition: *def},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(rev).Build()

	capability := NewCapabilitySourceDef(def)
	cmName, err := capability.StoreOpenAPISchema(ctx, k8sClient, "vela-system", "db-v1")
	require.NoError(t, err)
	assert.Equal(t, "source-schema-db", cmName)

	for name, owner := range map[string]string{"source-schema-db": v1beta1.SourceDefinitionKind, "source-schema-db-v1": v1beta1.DefinitionRevisionKind} {
		var cm corev1.ConfigMap
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: name}, &cm), name)
		require.Len(t, cm.OwnerReferences, 1)
		assert.Equal(t, owner, cm.OwnerReferences[0].Kind, name)
		assert.Equal(t, "core.oam.dev/v1beta1", cm.OwnerReferences[0].APIVersion, name)

		var param, out map[string]any
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.OpenapiV3JSONSchema]), &param))
		assert.Contains(t, param["properties"], "secret")
		assert.NotEmpty(t, cm.Data[types.DefaultUISchema])
		require.NoError(t, json.Unmarshal([]byte(cm.Data[types.SourceOutputSchema]), &out))
		assert.Contains(t, out["properties"], "port")
	}
	assert.Equal(t, map[string]string{"team": "data"}, def.Labels, "the definition's own labels are not written to")
}

// A template with no parameter has no form, so no UI schema is stored for it.
func TestATemplateWithNoParameterStoresNoUISchema(t *testing.T) {
	ctx := context.Background()
	_, ui, err := getParameterSchemas(ctx, types.Capability{Name: "bare", CueTemplate: `output: {apiVersion: "v1", kind: "ConfigMap"}`})
	require.NoError(t, err)
	assert.Empty(t, ui)

	def := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "static", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
schema: host: string
output: host: "db"
`}}},
	}
	capability := NewCapabilitySourceDef(def)
	data, err := capability.sourceSchemas(ctx)
	require.NoError(t, err)
	assert.NotContains(t, data, types.DefaultUISchema)
}
