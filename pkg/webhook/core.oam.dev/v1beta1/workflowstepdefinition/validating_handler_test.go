package workflowstepdefinition

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var handler ValidatingHandler
var reqResource metav1.GroupVersionResource
var decoder *admission.Decoder
var td v1beta1.WorkflowStepDefinition
var validCueTemplate string
var inValidCueTemplate string
var cfg *rest.Config
var testScheme = runtime.NewScheme()
var testEnv *envtest.Environment

func TestWorkflowStepDefinition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Traitdefinition Suite")
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

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	decoder, err = admission.NewDecoder(testScheme)
	Expect(err).Should(BeNil())

	td = v1beta1.WorkflowStepDefinition{}
	td.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
})

var _ = Describe("Test workflowstepdefinition validating handler", func() {
	BeforeEach(func() {
		cli, err := client.New(cfg, client.Options{})
		Expect(err).Should(BeNil())
		reqResource = metav1.GroupVersionResource{
			Group:    v1beta1.Group,
			Version:  v1beta1.Version,
			Resource: "workflowstepdefinitions"}
		handler = ValidatingHandler{Client: cli}
		handler.InjectDecoder(decoder)
	})

	Context("Test create/update operation admission request", func() {
		It("Test Version validation passed", func() {
			wsd := v1beta1.WorkflowStepDefinition{}
			wsd.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
			wsd.SetName("Correctwsd")
			wsd.Spec = v1beta1.WorkflowStepDefinitionSpec{
				Reference: common.DefinitionReference{Name: "testname", Version: "1"},
				Version:   "1.10.1",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wsdRaw, _ := json.Marshal(wsd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wsdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test Version validation passed", func() {
			wrongWsd := v1beta1.WorkflowStepDefinition{}
			wrongWsd.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
			wrongWsd.SetName("wrongwsd")
			wrongWsd.Spec = v1beta1.WorkflowStepDefinitionSpec{
				Version: "1.B.1",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongWsdRaw, _ := json.Marshal(wrongWsd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongWsdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Reason)).Should(ContainSubstring("Not a valid version"))
		})

		It("Test workflowstepdefinition has both spec.version and revision name annotation", func() {
			wrongWsd := v1beta1.WorkflowStepDefinition{}
			wrongWsd.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
			wrongWsd.SetName("wrongwsd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			wrongWsd.SetAnnotations(annotations)
			wrongWsd.SetNamespace("default")
			wrongWsd.Spec = v1beta1.WorkflowStepDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wrongWsdRaw, _ := json.Marshal(wrongWsd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wrongWsdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(string(resp.Result.Reason)).Should(ContainSubstring("Only one should be present"))
		})

		It("Test workflowstepdefinition without spec.version and with revision name annotation", func() {
			wsd := v1beta1.WorkflowStepDefinition{}
			wsd.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
			wsd.SetName("wsd")
			annotations := map[string]string{
				"definitionrevision.oam.dev/name": "v1.0.0",
			}
			wsd.SetAnnotations(annotations)
			wsd.SetNamespace("default")
			wsd.Spec = v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wsdRaw, _ := json.Marshal(wsd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wsdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test workflowstepdefinition with spec.version and without revision name annotation", func() {
			wsd := v1beta1.WorkflowStepDefinition{}
			wsd.SetGroupVersionKind(v1beta1.WorkflowStepDefinitionGroupVersionKind)
			wsd.SetName("wsd")
			wsd.Spec = v1beta1.WorkflowStepDefinitionSpec{
				Version: "1.10.0",
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: validCueTemplate,
					},
				},
			}
			wsdRaw, _ := json.Marshal(wsd)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: wsdRaw},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})
})
