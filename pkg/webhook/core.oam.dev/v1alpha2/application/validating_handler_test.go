/*
Copyright 2021 The KubeVela Authors.

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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("Test Application Validator", func() {
	BeforeEach(func() {
		Expect(handler.InjectClient(k8sClient)).Should(BeNil())
		Expect(handler.InjectDecoder(decoder)).Should(BeNil())
	})

	It("Test Application Validator [bad request]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator [Allow]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"type":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"type":"scaler","properties":{"replicas":10}}],"type":"worker"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
	})

	It("Test Application Validater [Error]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"type":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"type":"scaler","properties":{"replicas":10}}],"type":"worker1"}]}}`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator Forbid rollout annotation", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample", "annotations": {"app.oam.dev/rollout" : "true"},}
"spec":{"components":[{"type":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"type":"scaler","properties":{"replicas":10}}],"type":"worker"}]}}
`),
				},
				OldObject: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"type":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"type":"scaler","properties":{"replicas":10}}],"type":"worker"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator rollout-template annotation [error]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application",
"metadata":{"name":"application-sample","annotations":{"app.oam.dev/rollout-template":"false"}},
"spec":{"components":[{"type":"worker","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"type":"scaler","properties":{"replicas":10}}]}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator rolloutPlan [error]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-rolling","annotations":null},
"spec":{"components":[{"name":"metrics-provider","type":"worker",
"properties":{"cmd":["./podinfo","stress-cpu=3.0"],
"image":"stefanprodan/podinfo:4.0.6","port":8080}}],
"rolloutPlan":{"rolloutStrategy":"IncreaseFirst","targetSize":3}}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})
})
