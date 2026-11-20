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

package policydefinition

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestContextSchemaAtPolicyDefinitionAdmission(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{Decoder: admission.NewDecoder(sc), Client: fake.NewClientBuilder().WithScheme(sc).Build()}
	handle := func(template string) admission.Response {
		def := &v1beta1.PolicyDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "PolicyDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "identity", Namespace: oam.SystemDefinitionNamespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope, Global: true, Priority: 10,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
			},
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "test-uid", Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{Group: policyDefGVR.Group, Version: policyDefGVR.Version, Resource: policyDefGVR.Resource},
			Object:   runtime.RawExtension{Raw: raw},
		}})
	}

	t.Run("a schema leaning on the rest of the template is denied", func(t *testing.T) {
		resp := handle("parameter: {}\n_name: string\nschema: ctx: team: _name\noutput: ctx: team: \"a\"")
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "schema.ctx must stand on its own")
	})
	t.Run("a schema field that is not a ctx schema literal is admitted", func(t *testing.T) {
		resp := handle("parameter: {}\n#Shape: {name: string}\nschema: #Shape\noutput: labels: a: \"b\"")
		require.True(t, resp.Allowed, "%v", resp.Result)
	})
	t.Run("a schema that stands alone is admitted without a warning about it", func(t *testing.T) {
		resp := handle("parameter: {}\nschema: ctx: close({team: string})\noutput: ctx: team: \"a\"")
		require.True(t, resp.Allowed, "%v", resp.Result)
		require.NotContains(t, strings.Join(resp.Warnings, "\n"), "schema: ctx")
	})
	t.Run("ctx without a schema is admitted with a warning", func(t *testing.T) {
		resp := handle("parameter: {}\noutput: ctx: team: \"a\"")
		require.True(t, resp.Allowed, "%v", resp.Result)
		require.Contains(t, strings.Join(resp.Warnings, "\n"), "declare schema: ctx")
	})
}

// The worked example's policy: a global reading its namespace through a
// provider, which the required-parameter check must not take for a broken template.
func TestGlobalPolicyReadingContextIsAdmitted(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{Decoder: admission.NewDecoder(sc), Client: fake.NewClientBuilder().WithScheme(sc).Build()}
	b, err := os.ReadFile("../../../../../docs/examples/policy-context-schema/definitions/tenant-identity.yaml")
	require.NoError(t, err)
	raw, err := yaml.YAMLToJSON(b)
	require.NoError(t, err)
	resp := h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID: "test-uid", Operation: admissionv1.Create,
		Resource: metav1.GroupVersionResource{Group: policyDefGVR.Group, Version: policyDefGVR.Version, Resource: policyDefGVR.Resource},
		Object:   runtime.RawExtension{Raw: raw},
	}})
	require.True(t, resp.Allowed, "%v", resp.Result)
}
