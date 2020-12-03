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

var _ = Describe("Test Application Validater", func() {
	ctx := context.Background()
	handler := &ValidatingHandler{}

	BeforeEach(func() {
		Expect(handler.InjectClient(k8sClient)).Should(BeNil())
		Expect(handler.InjectDecoder(decoder)).Should(BeNil())
	})

	It("Test Application Validater [bad request]", func() {
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

	It("Test Application Validater [Allow]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`{
  "apiVersion": "core.oam.dev/v1alpha2",
  "kind": "Application",
  "metadata": {
    "name": "application-sample"
  },
  "spec": {
    "services": {
      "myweb": {
        "type": "worker",
        "image": "busybox",
        "cmd": [
          "sleep",
          "1000"
        ],
        "scaler": {
          "replicas": 10
        }
      }
    }
  }
}`),
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
					Raw: []byte(`{
  "apiVersion": "core.oam.dev/v1alpha2",
  "kind": "Application",
  "metadata": {
    "name": "application-sample"
  },
  "spec": {
    "services": {
      "myweb": {
        "type": "worker1",
        "image": "busybox",
        "cmd": [
          "sleep",
          "1000"
        ],
        "scaler": {
          "replicas": 10
        }
      }
    }
  }
}`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})
})
