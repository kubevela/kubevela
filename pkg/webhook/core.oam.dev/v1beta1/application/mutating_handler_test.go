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

	It("Test Application Mutator [traceID annotation injection on create]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				UID:       "test-trace-id-1234",
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example"}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())

		found := false
		for _, p := range resp.Patches {
			if p.Operation == "add" && p.Path == "/metadata/annotations" {
				m, ok := p.Value.(map[string]interface{})
				if ok && m[oam.AnnotationTraceID] == "test-trace-id-1234" {
					found = true
				}
			} else if p.Operation == "add" && p.Path == "/metadata/annotations/app.oam.dev~1traceID" {
				if p.Value == "test-trace-id-1234" {
					found = true
				}
			}
		}
		Expect(found).Should(BeTrue(), "traceID annotation should be injected")
	})

	It("Test Application Mutator [traceID annotation unchanged on update]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				UID:       "new-req-uid-5678",
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				OldObject: runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"` + oam.AnnotationTraceID + `":"existing-trace-id"}}}`)},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example","annotations":{"` + oam.AnnotationTraceID + `":"existing-trace-id"}}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
		for _, patch := range resp.Patches {
			if patch.Path == "/metadata/annotations" || patch.Path == "/metadata/annotations/"+oam.AnnotationTraceID {
				Fail("Should not modify existing traceID annotation")
			}
		}
	})

	It("Test Application Mutator [traceID annotation injection on update when missing]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				UID:       "new-req-uid-9012",
				Resource:  metav1.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "applications"},
				OldObject: runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example"}}`)},
				Object:    runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"example"}}`)},
			},
		}
		resp := mutatingHandler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())

		found := false
		for _, p := range resp.Patches {
			if p.Operation == "add" && p.Path == "/metadata/annotations" {
				m, ok := p.Value.(map[string]interface{})
				if ok && m[oam.AnnotationTraceID] == "new-req-uid-9012" {
					found = true
				}
			} else if p.Operation == "add" && p.Path == "/metadata/annotations/app.oam.dev~1traceID" {
				if p.Value == "new-req-uid-9012" {
					found = true
				}
			}
		}
		Expect(found).Should(BeTrue(), "traceID annotation should be injected")
	})
})
