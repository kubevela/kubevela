/*
Copyright 2021. The KubeVela Authors.

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

package policydefinition

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var handler ValidatingHandler
var req admission.Request
var reqResource metav1.GroupVersionResource
var decoder *admission.Decoder
var pd v1beta1.PolicyDefinition
var pdRaw []byte
var scheme = runtime.NewScheme()
var validCueTemplate string
var inValidCueTemplate string

func TestPolicydefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Policydefinition Suite")
}

var _ = BeforeSuite(func() {

	validCueTemplate = "{hello: 'world'}"
	inValidCueTemplate = "{hello: world}"

	pd = v1beta1.PolicyDefinition{}
	pd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)

	var err error
	decoder, err = admission.NewDecoder(scheme)
	Expect(err).Should(BeNil())
})

var _ = Describe("Test PolicyDefinition validating handler", func() {
	BeforeEach(func() {
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "policydefinitions"}
		handler = ValidatingHandler{}
		handler.InjectDecoder(decoder)
	})

	It("Test wrong resource of admission request", func() {
		wrongReqResource := metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "foos"}
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  wrongReqResource,
				Object:    runtime.RawExtension{Raw: []byte("")},
			},
		}
		resp := handler.Handle(context.TODO(), req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	It("Test bad admission request", func() {
		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Resource:  reqResource,
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := handler.Handle(context.TODO(), req)
		Expect(resp.Allowed).Should(BeFalse())
	})

	Context("Test create/update operation admission request", func() {
		It("Test cue template validation passed", func() {
			pd.Spec = v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			pdRaw, _ = json.Marshal(pd)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: pdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
		It("Test cue template validation failed", func() {
			pd.Spec = v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: inValidCueTemplate,
					},
				},
			}
			pdRaw, _ = json.Marshal(pd)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: pdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
		})
	})
})
