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

package componentdefinition

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var handler ValidatingHandler
var req admission.Request
var reqResource metav1.GroupVersionResource
var decoder admission.Decoder
var cd v1beta1.ComponentDefinition
var cdRaw []byte
var testScheme = runtime.NewScheme()
var testEnv *envtest.Environment
var cfg *rest.Config
var validCueTemplate string
var inValidCueTemplate string

func TestComponentdefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Componentdefinition Suite")
}

var _ = BeforeSuite(func() {

	validCueTemplate = "{hello: 'world'}"
	inValidCueTemplate = "{hello: world}"

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

	err := core.AddToScheme(testScheme)
	Expect(err).Should(BeNil())
	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	decoder = admission.NewDecoder(testScheme)

	cd = v1beta1.ComponentDefinition{}
	cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test ComponentDefinition validating handler", func() {
	BeforeEach(func() {
		cli, err := client.New(cfg, client.Options{})
		Expect(err).Should(BeNil())
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "componentdefinitions"}
		handler = ValidatingHandler{
			Client:  cli,
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
		It("Test componentDefinition without type and definition", func() {
			wrongCd := v1beta1.ComponentDefinition{}
			wrongCd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			wrongCd.SetName("wrongCd")
			wrongCdRaw, _ := json.Marshal(wrongCd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongCdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason(http.StatusText(http.StatusForbidden))))
			Expect(resp.Result.Message).Should(Equal("neither the type nor the definition of the workload field in the ComponentDefinition wrongCd can be empty"))
		})

		It("Test componentDefinition which type and definition point to different workload type", func() {
			wrongCd := v1beta1.ComponentDefinition{}
			wrongCd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			wrongCd.SetName("wrongCd")
			wrongCd.Spec.Workload.Type = "jobs.batch"
			wrongCd.Spec.Workload.Definition = common.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}
			wrongCdRaw, _ := json.Marshal(wrongCd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongCdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason(http.StatusText(http.StatusForbidden))))
			Expect(resp.Result.Message).Should(Equal("the type and the definition of the workload field in ComponentDefinition wrongCd should represent the same workload"))
		})
		It("Test cue template validation passed", func() {
			cd.Spec = v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			cdRaw, _ = json.Marshal(cd)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: cdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
		It("Test cue template validation failed", func() {
			cd.Spec = v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: inValidCueTemplate,
					},
				},
			}
			cdRaw, _ = json.Marshal(cd)

			req = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: cdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Reason).Should(Equal(metav1.StatusReason(http.StatusText(http.StatusForbidden))))
			Expect(resp.Result.Message).Should(ContainSubstring("hello: reference \"world\" not found"))
		})

		It("Test Version field validation passed", func() {
			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("CorrectCd")
			cd.Spec = v1beta1.ComponentDefinitionSpec{
				Version: "1.10.0",
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			cdRaw, _ := json.Marshal(cd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: cdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test Version field validation failed", func() {
			wrongCd := v1beta1.ComponentDefinition{}
			wrongCd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			wrongCd.SetName("wrongCd")
			wrongCd.Spec = v1beta1.ComponentDefinitionSpec{
				Version: "1.10..0",
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			correctCdRaw, _ := json.Marshal(wrongCd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: correctCdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Message)).Should(ContainSubstring("Not a valid version"))
		})

		It("Test ComponentDefintion has both spec.version and revision name annotation", func() {
			wrongCd := v1beta1.ComponentDefinition{}
			wrongCd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			wrongCd.SetName("wrongCd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "1.0.0",
			}
			wrongCd.SetAnnotations(annotations)
			wrongCd.SetNamespace("default")
			wrongCd.Spec = v1beta1.ComponentDefinitionSpec{
				Version: "1.10.0",
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongCdRaw, _ := json.Marshal(wrongCd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongCdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Message)).Should(ContainSubstring("Only one can be present"))
		})

		It("Test ComponentDefintion with spec.version and without revision name annotation", func() {
			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("cd")
			cd.Spec = v1beta1.ComponentDefinitionSpec{
				// Version: "1.10.0",
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			cdRaw, _ := json.Marshal(cd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: cdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test ComponentDefintion with revision name annotation and wihout spec.version", func() {
			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("cd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "1.0.0",
			}
			cd.SetAnnotations(annotations)
			cd.SetNamespace("default")
			cd.Spec = v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Type: "deployments.apps",
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			cdRaw, _ := json.Marshal(cd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: cdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

	})
})
