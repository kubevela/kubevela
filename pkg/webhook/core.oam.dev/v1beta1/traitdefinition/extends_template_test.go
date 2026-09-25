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

package traitdefinition

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Everything the handler knows about `extends` sat inside the branch guarding
// the CUE schematic, so a definition with no schematic was admitted with its
// chain never resolved and the feature gate never consulted. It then failed at
// render, naming a missing `$super` block rather than the empty definition.
func TestExtendsWithNoTemplateIsRefusedOnWrite(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}

	def := &v1beta1.TraitDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: "empty-child", Namespace: "vela-system"},
		Spec:       v1beta1.TraitDefinitionSpec{Extends: "scaler"},
	}
	raw, err := json.Marshal(def)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "test-uid",
		Operation: admissionv1.Create,
		Resource: metav1.GroupVersionResource{
			Group: traitDefGVR.Group, Version: traitDefGVR.Version, Resource: traitDefGVR.Resource,
		},
		Object: runtime.RawExtension{Raw: raw},
	}})

	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "no CUE template to call it from")
}
