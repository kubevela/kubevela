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

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestRenderApplicationPolicyAppliesItsOutput(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", Labels: map[string]string{"team": "payments"}},
		Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "web", Type: "webservice"}}},
	}
	def := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tagger"},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: owner: string
output: {
	labels: "platform.io/owner": parameter.owner
	components: [for c in context.appComponents {c & {properties: owner: parameter.owner}}]
}
`}}},
	}
	out, result, err := RenderApplicationPolicy(context.Background(), nil, ApplicationPolicyRender{
		App:        app,
		Policy:     v1beta1.AppPolicy{Name: "tag", Type: "tagger", Properties: &runtime.RawExtension{Raw: []byte(`{"owner":"payments"}`)}},
		Definition: def,
	})
	require.NoError(t, err)
	require.True(t, result.Enabled)
	require.Equal(t, map[string]string{"team": "payments", "platform.io/owner": "payments"}, out.Labels, "labels merge")
	require.JSONEq(t, `{"owner":"payments"}`, string(out.Spec.Components[0].Properties.Raw), "components are replaced")
	require.Nil(t, app.Spec.Components[0].Properties, "the Application given is left as it was")
}

func TestRenderApplicationPolicyRecordsItsProvenance(t *testing.T) {
	def := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "noop"},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {}
output: {}
`}}},
	}
	version := &v1beta1.PolicyVersionMetadata{DefinitionRevisionName: "noop-v3", Revision: 3, RevisionHash: "abc123"}
	_, result, err := RenderApplicationPolicy(context.Background(), nil, ApplicationPolicyRender{
		App:        &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a"}},
		Policy:     v1beta1.AppPolicy{Name: "n", Type: "noop"},
		Definition: def,
		Version:    version,
	})
	require.NoError(t, err)
	require.Equal(t, "noop-v3", result.DefinitionRevisionName)
	require.Equal(t, int64(3), result.Revision)
	require.Equal(t, "abc123", result.RevisionHash)
	require.Same(t, def, result.PolicyDefinitionUsed)
}

func TestRenderApplicationPolicyRequiresADefinition(t *testing.T) {
	_, _, err := RenderApplicationPolicy(context.Background(), nil, ApplicationPolicyRender{
		App:    &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a"}},
		Policy: v1beta1.AppPolicy{Name: "n", Type: "noop"},
	})
	require.EqualError(t, err, "policy definition is required")
}

func TestRenderApplicationPolicyRequiresAnApplication(t *testing.T) {
	_, _, err := RenderApplicationPolicy(context.Background(), nil, ApplicationPolicyRender{
		Policy:     v1beta1.AppPolicy{Name: "n", Type: "noop"},
		Definition: &v1beta1.PolicyDefinition{},
	})
	require.EqualError(t, err, "application is required")
}
