package application

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("Test Application Validator", func() {
	ctx := context.Background()
	handler := &ValidatingHandler{}

	BeforeEach(func() {
		Expect(handler.InjectClient(k8sClient)).Should(BeNil())
		Expect(handler.InjectDecoder(decoder)).Should(BeNil())
	})

	It("Test Application Validator [bad request]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator [Allow]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"name":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"kind":"scaler","properties":{"replicas":10}}],"kind":"worker"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
	})

	It("Test Application Validater [Error]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"name":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"kind":"scaler","properties":{"replicas":10}}],"kind":"worker1"}]}}`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator Forbid rollout annotation", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Update,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample", "annotations": {"app.oam.dev/rollout" : "true"},}
"spec":{"components":[{"name":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"kind":"scaler","properties":{"replicas":10}}],"kind":"worker"}]}}
`),
				},
				OldObject: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Application",
"metadata":{"name":"application-sample"},
"spec":{"components":[{"name":"myweb","properties":{"cmd":["sleep","1000"],"image":"busybox"},
"traits":[{"kind":"scaler","properties":{"replicas":10}}],"kind":"worker"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})
})
