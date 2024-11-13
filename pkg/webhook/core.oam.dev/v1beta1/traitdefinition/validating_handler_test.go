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
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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
var testEnv *envtest.Environment
var validCueTemplate string
var inValidCueTemplate string
var cfg *rest.Config

func TestTraitdefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Traitdefinition Suite")
}

var _ = BeforeSuite(func() {

	validCueTemplate = "{hello: 'world'}"
	inValidCueTemplate = "{hello: world}"

	td = v1beta1.TraitDefinition{}
	td.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)

	var err error
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../..", "charts", "vela-core", "crds")
	}
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{yamlPath},
	}
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	decoder, err = admission.NewDecoder(scheme)
	Expect(err).Should(BeNil())
})

var _ = Describe("Test TraitDefinition validating handler", func() {
	BeforeEach(func() {
		cli, err := client.New(cfg, client.Options{})
		Expect(err).Should(BeNil())
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "traitdefinitions"}
		handler = ValidatingHandler{Client: cli}
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
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason("mock validator error")))
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

		It("Test Version field validation passed", func() {
			td := v1beta1.TraitDefinition{}
			td.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
			td.SetName("Correcttd")
			td.Spec = v1beta1.TraitDefinitionSpec{
				Version: "1.10.1",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			tdRaw, _ := json.Marshal(td)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test Version validation failed", func() {
			wrongtd := v1beta1.TraitDefinition{}
			wrongtd.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
			wrongtd.SetName("Wrongtd")
			wrongtd.Spec = v1beta1.TraitDefinitionSpec{
				Version: "a.b.c",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongtdRaw, _ := json.Marshal(wrongtd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongtdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Reason)).Should(ContainSubstring("Not a valid version"))
		})

		It("Test TraitDefintion has both spec.version and revision name annotation", func() {
			wrongtd := v1beta1.TraitDefinition{}
			wrongtd.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
			wrongtd.SetName("Wrongtd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			wrongtd.SetAnnotations(annotations)
			wrongtd.SetNamespace("default")
			wrongtd.Spec = v1beta1.TraitDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongtdRaw, _ := json.Marshal(wrongtd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongtdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Reason)).Should(ContainSubstring("Only one should be present"))
		})

		It("Test TraitDefintion with spec.version and without revision name annotation", func() {
			td := v1beta1.TraitDefinition{}
			td.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
			td.SetName("td")
			td.Spec = v1beta1.TraitDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			tdRaw, _ := json.Marshal(td)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test TraitDefintion without spec.version and with revision name annotation", func() {
			td := v1beta1.TraitDefinition{}
			td.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
			td.SetName("td")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			td.SetAnnotations(annotations)
			td.SetNamespace("default")
			td.Spec = v1beta1.TraitDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			tdRaw, _ := json.Marshal(td)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: tdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})
})
