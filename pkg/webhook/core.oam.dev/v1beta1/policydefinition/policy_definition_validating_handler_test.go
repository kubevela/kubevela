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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var handler ValidatingHandler
var req admission.Request
var reqResource metav1.GroupVersionResource
var decoder admission.Decoder
var pd v1beta1.PolicyDefinition
var pdRaw []byte
var scheme = runtime.NewScheme()
var testEnv *envtest.Environment
var validCueTemplate string
var inValidCueTemplate string
var cfg *rest.Config

func TestPolicydefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Policydefinition Suite")
}

var _ = BeforeSuite(func() {

	validCueTemplate = "{hello: 'world'}"
	inValidCueTemplate = "{hello: world}"

	pd = v1beta1.PolicyDefinition{}
	pd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
	decoder = admission.NewDecoder(scheme)
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
})

var _ = Describe("Test PolicyDefinition validating handler", func() {
	BeforeEach(func() {
		cli, err := client.New(cfg, client.Options{})
		Expect(err).Should(BeNil())
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "policydefinitions"}
		handler = ValidatingHandler{
			Decoder: decoder,
			Client:  cli,
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

		It("Test Version field validation passed", func() {
			pd := v1beta1.PolicyDefinition{}
			pd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
			pd.SetName("CorrectPd")
			pd.Spec = v1beta1.PolicyDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			pdRaw, _ := json.Marshal(pd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: pdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test Version field validation failed", func() {
			wrongPd := v1beta1.PolicyDefinition{}
			wrongPd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
			wrongPd.SetName("WrongPd")
			wrongPd.Spec = v1beta1.PolicyDefinitionSpec{
				Version: "1.10..0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongPdRaw, _ := json.Marshal(wrongPd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongPdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Message)).Should(ContainSubstring("Not a valid version"))
		})

		It("Test PolicyDefintion has both spec.version and revision name annotation", func() {
			wrongPd := v1beta1.PolicyDefinition{}

			wrongPd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
			wrongPd.SetName("wrongPd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			wrongPd.SetAnnotations(annotations)
			wrongPd.SetNamespace("default")
			wrongPd.Spec = v1beta1.PolicyDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongPdRaw, _ := json.Marshal(wrongPd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongPdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Message)).Should(ContainSubstring("Only one can be present"))
		})

		It("Test PolicyDefintion with spec.version and without revision name annotation", func() {
			pd := v1beta1.PolicyDefinition{}

			pd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
			pd.SetName("pd")
			pd.Spec = v1beta1.PolicyDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			pdRaw, _ := json.Marshal(pd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: pdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test PolicyDefintion without spec.version and with revision name annotation", func() {
			pd := v1beta1.PolicyDefinition{}

			pd.SetGroupVersionKind(v1beta1.PolicyDefinitionGroupVersionKind)
			pd.SetName("pd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			pd.SetAnnotations(annotations)
			pd.SetNamespace("default")
			pd.Spec = v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			pdRaw, _ := json.Marshal(pd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: pdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})
})
