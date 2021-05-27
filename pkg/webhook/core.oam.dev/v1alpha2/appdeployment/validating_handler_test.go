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

package appdeployment

import (
	"context"
	"encoding/json"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("Test AppDeployment validating handler", func() {
	appDeploymentResource := metav1.GroupVersionResource{
		Group:    v1beta1.Group,
		Version:  v1beta1.Version,
		Resource: "appdeployments",
	}

	It("Test wrong resource of admission request", func() {
		resp := handler.Handle(context.Background(), admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource: metav1.GroupVersionResource{
					Group:    v1beta1.Group,
					Version:  v1beta1.Version,
					Resource: "foos",
				},
				Object: runtime.RawExtension{Raw: []byte("")},
			},
		})
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test wrong object of admission request", func() {
		resp := handler.Handle(context.Background(), admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  appDeploymentResource,
				Object:    runtime.RawExtension{Raw: []byte("bad object")},
			},
		})
		Expect(resp.Allowed).Should(BeFalse())
	})

	Context("Test wrong application revision name", func() {
		It("Test empty application revision name", func() {
			ad := v1beta1.AppDeployment{}
			ad.SetGroupVersionKind(v1beta1.AppDeploymentGroupVersionKind)
			ad.Name = "empty"
			ad.Namespace = "namespace"
			ad.Spec.AppRevisions = []v1beta1.AppRevision{{RevisionName: ""}}
			raw, _ := json.Marshal(ad)

			resp := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  appDeploymentResource,
					Object:    runtime.RawExtension{Raw: raw},
				},
			})
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason("spec.apprevisions.[0]: Required value: target application revision name cannot be empty")))
		})

		It("Test non-existent application revision name", func() {
			ad := v1beta1.AppDeployment{}
			ad.SetGroupVersionKind(v1beta1.AppDeploymentGroupVersionKind)
			ad.Name = "nonexistent"
			ad.Namespace = "namespace"
			ad.Spec.AppRevisions = []v1beta1.AppRevision{{RevisionName: "none"}}
			raw, _ := json.Marshal(ad)

			resp := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  appDeploymentResource,
					Object:    runtime.RawExtension{Raw: raw},
				},
			})
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason(`spec.apprevisions.revisionName: Not found: "none"`)))
		})
	})
})
