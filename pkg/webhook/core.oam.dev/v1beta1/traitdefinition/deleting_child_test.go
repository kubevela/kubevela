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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

var _ = Describe("a child whose parent is already gone", func() {
	var h *ValidatingHandler

	BeforeEach(func() {
		before := map[string]bool{string(features.EnableDefinitionInheritance): utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)}
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableDefinitionInheritance): true})).To(Succeed())
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(before)).To(Succeed()) })

		sc := runtime.NewScheme()
		Expect(v1beta1.SchemeBuilder.AddToScheme(sc)).To(Succeed())
		h = &ValidatingHandler{Decoder: admission.NewDecoder(sc), Client: fake.NewClientBuilder().WithScheme(sc).Build()}
	})

	update := func(deleting bool) admission.Response {
		child := &v1beta1.TraitDefinition{
			TypeMeta: metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-labels", Namespace: "vela-system",
				Finalizers: []string{"foregroundDeletion"}},
			Spec: v1beta1.TraitDefinitionSpec{
				Extends:   "gone-labels",
				Schematic: &common.Schematic{CUE: &common.CUE{Template: "$super: properties: {}\npatch: {}\nparameter: {}\n"}},
			},
		}
		if deleting {
			now := metav1.Now()
			child.DeletionTimestamp = &now
		}
		raw, err := json.Marshal(child)
		Expect(err).NotTo(HaveOccurred())
		return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group: traitDefGVR.Group, Version: traitDefGVR.Version, Resource: traitDefGVR.Resource,
			},
			Object:    runtime.RawExtension{Raw: raw},
			OldObject: runtime.RawExtension{Raw: raw},
		}})
	}

	It("lets it finish deleting", func() {
		Expect(update(true).Allowed).To(BeTrue())
	})

	It("still refuses it while it is not being deleted", func() {
		resp := update(false)
		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("gone-labels"))
	})
})
