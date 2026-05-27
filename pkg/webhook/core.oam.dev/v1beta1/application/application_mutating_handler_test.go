/*
 /*
Copyright 2022 The KubeVela Authors.

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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test Application Mutator", func() {

	var mutatingHandler *MutatingHandler

	BeforeEach(func() {
		mutatingHandler = &MutatingHandler{
			skipUsers: []string{types.VelaCoreName},
			Decoder:   decoder,
		}
	})

	It("Test Application Mutator [no authentication]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.AuthenticateApplication))).Should(Succeed())
		resp := mutatingHandler.Handle(ctx, admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{Raw: []byte(`{}`)},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).Should(BeNil())
	})

	It("Test Application Mutator [ignore authentication]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.AuthenticateApplication))).Should(Succeed())
		resp := mutatingHandler.Handle(ctx, admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: authv1.UserInfo{Username: types.VelaCoreName},
				Object:   runtime.RawExtension{Raw: []byte(`{}`)},
			}})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).Should(BeNil())
	})

	It("Test Application Mutator [bad request]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.AuthenticateApplication))).Should(Succeed())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Mutator [bad request with service-account]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.AuthenticateApplication))).Should(Succeed())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"app.oam.dev/service-account-name":"default"}}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
		Expect(resp.Result.Message).Should(ContainSubstring("service-account annotation is not permitted when authentication enabled"))
	})

	It("Test Application Mutator [with patch]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.AuthenticateApplication))).Should(Succeed())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example"},"spec":{"workflow":{"steps":[{"properties":{"duration":"3s"},"type":"suspend"}]}}}`)},
				UserInfo: authv1.UserInfo{
					Username: "example-user",
					Groups:   []string{"kubevela:example-group1", "kubevela:example-group2"},
				},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).Should(ContainElement(jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/metadata/annotations",
			Value: map[string]interface{}{
				oam.AnnotationApplicationGroup: "kubevela:example-group1,kubevela:example-group2",
			},
		}))
		Expect(resp.Patches).Should(ContainElement(jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/spec/workflow/steps/0/name",
			Value:     "step-0",
		}))
	})

	It("Test Application Mutator [CREATE stamps traceID annotation]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.AuthenticateApplication))).Should(Succeed())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       "trace-uid-create",
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example"}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).Should(ContainElement(jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/metadata/annotations",
			Value: map[string]interface{}{
				oam.AnnotationTraceID: "trace-uid-create",
			},
		}))
	})

	It("Test Application Mutator [UPDATE stamps traceID when annotation missing]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.AuthenticateApplication))).Should(Succeed())
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       "trace-uid-update",
				Operation: admissionv1.Update,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"keep":"me"}}}`)},
				OldObject: runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"keep":"me"}}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).Should(ContainElement(jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/metadata/annotations/" + jsonPointerEscape(oam.AnnotationTraceID),
			Value:     "trace-uid-update",
		}))
	})

	It("Test Application Mutator [UPDATE preserves existing traceID annotation]", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.AuthenticateApplication))).Should(Succeed())
		existing := `{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"app.oam.dev/traceID":"original-trace"}}}`
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       "fresh-uid-must-not-overwrite",
				Operation: admissionv1.Update,
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(existing)},
				OldObject: runtime.RawExtension{Raw: []byte(existing)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
		for _, p := range resp.Patches {
			Expect(p.Path).ShouldNot(ContainSubstring("traceID"))
		}
	})
})

// jsonPointerEscape escapes a JSON pointer path segment per RFC 6901: "~" -> "~0", "/" -> "~1".
func jsonPointerEscape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
