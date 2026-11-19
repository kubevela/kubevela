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

package componentdefinition

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

var _ = Describe("a base that marks +final inside a list item", func() {
	It("is refused when it is written, though it extends nothing", func() {
		before := map[string]bool{string(features.EnableDefinitionInheritance): utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)}
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableDefinitionInheritance): true})).To(Succeed())
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(before)).To(Succeed()) })

		sc := runtime.NewScheme()
		Expect(v1beta1.SchemeBuilder.AddToScheme(sc)).To(Succeed())
		h := &ValidatingHandler{Decoder: admission.NewDecoder(sc), Client: fake.NewClientBuilder().WithScheme(sc).Build()}

		base := &v1beta1.ComponentDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "list-final-web", Namespace: "vela-system"},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}},
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{
		name: "main"
		// +final
		securityContext: runAsNonRoot: true
	}]
}
parameter: {}
`}},
			},
		}
		raw, err := json.Marshal(base)
		Expect(err).NotTo(HaveOccurred())
		resp := h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group: componentDefGVR.Group, Version: componentDefGVR.Version, Resource: componentDefGVR.Resource,
			},
			Object: runtime.RawExtension{Raw: raw},
		}})
		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring(
			"component list-final-web marks output.spec.template.spec.containers[0].securityContext +final, but +final cannot be enforced on a field inside a list item"))
	})
})
