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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
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

	It("Test Application Validator [Error]", func() {
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

	It("Test Application Validator workflow step name duplicate [error]", func() {
		By("test duplicated step name in workflow")
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"workflow-duplicate","namespace":"default"},"spec":{"components":[{"name":"comp","type":"worker","properties":{"image":"crccheck/hello-world"}}],"workflow":{"steps":[{"name":"suspend","type":"suspend"},{"name":"suspend","type":"suspend"}]}}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("test duplicated sub step name in workflow")
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"workflow-duplicate","namespace":"default"},"spec":{"components":[{"name":"comp","type":"worker","properties":{"image":"crccheck/hello-world"}}],"workflow":{"steps":[{"name":"group","type":"step-group","subSteps":[{"name":"sub","type":"suspend"},{"name":"sub","type":"suspend"}]}]}}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("test duplicated sub and parent step name in workflow")
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"workflow-duplicate","namespace":"default"},"spec":{"components":[{"name":"comp","type":"worker","properties":{"image":"crccheck/hello-world"}}],"workflow":{"steps":[{"name":"group","type":"step-group","subSteps":[{"name":"group","type":"suspend"},{"name":"sub","type":"suspend"}]}]}}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator workflow step invalid timeout [error]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"workflow-timeout","namespace":"default"},"spec":{"components":[{"name":"comp","type":"worker","properties":{"image":"crccheck/hello-world"}}],"workflow":{"steps":[{"name":"group","type":"suspend","timeout":"test"}]}}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application Validator workflow step invalid timeout [allow]", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"apiVersion":"core.oam.dev/v1beta1","kind":"Application","metadata":{"name":"workflow-timeout","namespace":"default"},"spec":{"components":[{"name":"comp","type":"worker","properties":{"image":"crccheck/hello-world"}}],"workflow":{"steps":[{"name":"group","type":"suspend","timeout":"1s"}]}}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
	})

	It("Test Application Validator external revision name [allow]", func() {
		externalComp1 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp1",
				Labels: map[string]string{
					oam.LabelControllerRevisionComponent: "myworker",
					oam.LabelComponentRevisionHash:       "81796829364afe1",
				},
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component",
"metadata":{"name":"myweb"},
"spec":{"workload":{"apiVersion":"apps/v1",
"kind":"Deployment",
"spec": {"containers":[{"image":"stefanprodan/podinfo:4.0.6"}]}}}}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp1"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
	})

	It("Test Application Validator external revision name specify helm repo in component [allow]", func() {
		externalComp2 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp2",
				Labels: map[string]string{
					oam.LabelControllerRevisionComponent: "myworker",
					oam.LabelComponentRevisionHash:       "9be6f6ab47eadbf9",
				},
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component","metadata":{"name":"myweb"},
"spec":{"workload":{"apiVersion":"apps/v1","kind":"Deployment",
"spec":{"containers":[{"image":"stefanprodan/podinfo:4.0.6"}]}},
"helm":{"release":{"chart":{"spec":{"chart":"podinfo","version":"1.0.0"}}},
"repository":{"url":"test.com","secretRef":{"name":"testSecret"}}}}}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp2)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp2"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeTrue())
	})

	It("Test Application Validator external revision name [error]", func() {

		By("Parse component error")
		externalComp4 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp4",
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component",
"metadata":{"name":"myweb"},
"spec":"invalid-component"}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp4)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp4"}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("Parse helm repository error")
		externalComp5 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp5",
				Labels: map[string]string{
					oam.LabelControllerRevisionComponent: "myworker",
					oam.LabelComponentRevisionHash:       "9be6f6ab47eadbf9",
				},
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component","metadata":{"name":"myweb"},
"spec":{"workload":{"apiVersion":"apps/v1","kind":"Deployment",
"spec":{"containers":[{"image":"stefanprodan/podinfo:4.0.6"}]}},
"helm":{"release":{"chart":{"spec":{"chart":"podinfo","version":"1.0.0"}}},
"repository":"invlid-repostitory"}}}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp5)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp5"}]}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("Parse helm release error")
		externalComp7 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp7",
				Labels: map[string]string{
					oam.LabelControllerRevisionComponent: "myworker",
					oam.LabelComponentRevisionHash:       "9be6f6ab47eadbf9",
				},
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component","metadata":{"name":"myweb"},
"spec":{"workload":{"apiVersion":"apps/v1","kind":"Deployment",
"spec":{"containers":[{"image":"stefanprodan/podinfo:4.0.6"}]}},
"helm":{"release":"invalid-release",
"repository":{"url":"test.com","secretRef":{"name":"testSecret"}}}}}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp7)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp7"}]}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("Parse workload error")
		externalComp8 := appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "external-comp8",
			},
			Data: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"core.oam.dev/v1beta1",
"kind":"Component",
"metadata":{"name":"myweb"},
"spec":{"workload": "invalid-workload"}}
`)},
			Revision: 1,
		}
		Expect(k8sClient.Create(ctx, &externalComp8)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"test-external-revision", "namespace":"default"},
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp8"}]}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())

		By("application metadata invalid")
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application",
"spec":{"components":[{"name":"myworker","type":"worker",
"properties":{"image":"stefanprodan/podinfo:4.0.6"},
"externalRevision":"external-comp"}]}}
`),
				},
			},
		}
		resp = handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test Application with empty policy", func() {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
				Object: runtime.RawExtension{
					Raw: []byte(`
{"kind":"Application","metadata":{"name":"app-with-empty-policy-webhook-test", "namespace":"default"},
"spec":{"components":[],"policies":[{"name":"2345","type":"garbage-collect","properties":null}]}}
`),
				},
			},
		}
		resp := handler.Handle(ctx, req)
		Expect(resp.Allowed).Should(BeFalse())
	})
})
