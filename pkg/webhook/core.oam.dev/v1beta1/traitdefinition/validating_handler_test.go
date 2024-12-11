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

package traitdefinition

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var handler ValidatingHandler
var req admission.Request
var reqResource metav1.GroupVersionResource
var decoder *admission.Decoder
var td v1beta1.TraitDefinition
var tdRaw []byte
var scheme = runtime.NewScheme()
var validCueTemplate string
var inValidCueTemplate string

func TestTraitdefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Traitdefinition Suite")
}

var _ = BeforeSuite(func() {

	validCueTemplate = "{hello: 'world'}"
	inValidCueTemplate = "{hello: world}"

	td = v1beta1.TraitDefinition{}
	td.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)

	decoder = admission.NewDecoder(scheme)
})

var _ = Describe("Test TraitDefinition validating handler", func() {
	BeforeEach(func() {
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "traitdefinitions"}
		handler = ValidatingHandler{
			Decoder: decoder,
		}
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
		var mockValidator TraitDefValidatorFn
		It("Test validation passed", func() {
			// mock a validator that always validates successfully
			mockValidator = func(_ context.Context, _ v1beta1.TraitDefinition) error {
				return nil
			}
			handler.Validators = []TraitDefValidator{
				TraitDefValidatorFn(mockValidator),
			}
			tdRaw, _ = json.Marshal(td)
			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
		It("Test validation failed", func() {
			// mock a validator that always failed
			mockValidator = func(_ context.Context, _ v1beta1.TraitDefinition) error {
				return errors.New("mock validator error")
			}
			handler.Validators = []TraitDefValidator{
				TraitDefValidatorFn(mockValidator),
			}
			tdRaw, _ = json.Marshal(td)
			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason(http.StatusText(http.StatusForbidden))))
			Expect(resp.Result.Message).Should(Equal("mock validator error"))
		})
		It("Test cue template validation passed", func() {
			td.Spec = v1beta1.TraitDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			tdRaw, _ = json.Marshal(td)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
		It("Test cue template validation failed", func() {
			td.Spec = v1beta1.TraitDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: inValidCueTemplate,
					},
				},
			}
			tdRaw, _ = json.Marshal(td)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
		})
	})
})
