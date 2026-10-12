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
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module"
	modulerender "github.com/oam-dev/kubevela/pkg/module/service"
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
			Expect(resp.Result.Message).Should(ContainSubstring("neither the type nor the definition of the workload field in the ComponentDefinition wrongCd can be empty"))
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
			Expect(resp.Result.Message).Should(ContainSubstring("the type and the definition of the workload field in ComponentDefinition wrongCd should represent the same workload"))
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

		It("Test ComponentDefinition with non-existent CRD in outputs should be rejected", func() {
			// Enable the ValidateResourcesExist feature gate for this test
			originalState := utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateResourcesExist)
			defer utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
				string(features.ValidateResourcesExist): originalState,
			})
			utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
				string(features.ValidateResourcesExist): true,
			})

			templateWithInvalidCRD := `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	invalidResource: {
		apiVersion: "custom.io/v1alpha1"
		kind: "NonExistentResource"
		metadata: name: parameter.name + "-custom"
		spec: {
			foo: "bar"
		}
	}
}`

			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("test-invalid-outputs")
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
						Template: templateWithInvalidCRD,
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
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("resource type not found on cluster"))
		})

		It("Test ComponentDefinition with valid outputs should pass", func() {
			templateWithValidOutputs := `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{
				port: 80
				targetPort: 8080
			}]
		}
	}
	configmap: {
		apiVersion: "v1"
		kind: "ConfigMap"
		metadata: name: parameter.name + "-config"
		data: {
			config: "test-config"
		}
	}
}`

			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("test-valid-outputs")
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
						Template: templateWithValidOutputs,
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

		It("Test ComponentDefinition with mixed valid and non-k8s outputs should pass", func() {
			templateWithMixedOutputs := `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{port: 80}]
		}
	}
	customData: {
		field1: "value1"
		field2: "value2"
		nested: {
			data: "some-data"
		}
	}
}`

			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("test-mixed-outputs")
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
						Template: templateWithMixedOutputs,
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

		It("Test ComponentDefinition with empty outputs should pass", func() {
			templateWithEmptyOutputs := `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {}`

			cd := v1beta1.ComponentDefinition{}
			cd.SetGroupVersionKind(v1beta1.ComponentDefinitionGroupVersionKind)
			cd.SetName("test-empty-outputs")
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
						Template: templateWithEmptyOutputs,
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

// An extending definition with no template has nothing to call its parent from,
// so it inherits no workload and describes nothing. Admitting it defers the
// failure to render time, where the message is about a missing `$super` block
// rather than about the definition being empty.
func TestExtendingWithNoTemplateIsRefused(t *testing.T) {
	cd := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-child", Namespace: "vela-system"},
		Spec:       v1beta1.ComponentDefinitionSpec{Extends: "webservice"},
	}

	err := ValidateWorkload(nil, cd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no CUE template to call it from")
}

// The refusal above is reached only when the workload is empty too. A
// ComponentDefinition that states a workload, extends another and has no
// template still calls nothing, so the handler has to judge it on the way in.
func TestExtendsWithAWorkloadButNoTemplateIsRefusedOnWrite(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}

	def := &v1beta1.ComponentDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: "workload-child", Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:  "webservice",
			Workload: common.WorkloadTypeDescriptor{Type: "deployments.apps"},
		},
	}
	raw, err := json.Marshal(def)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "test-uid",
		Operation: admissionv1.Create,
		Resource: metav1.GroupVersionResource{
			Group: componentDefGVR.Group, Version: componentDefGVR.Version, Resource: componentDefGVR.Resource,
		},
		Object: runtime.RawExtension{Raw: raw},
	}})

	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "no CUE template to call it from")
}

// With a template it is fine: the workload comes from the parent.
func TestExtendingWithATemplateInheritsTheWorkload(t *testing.T) {
	cd := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   "webservice",
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "$super: properties: {}"}},
		},
	}

	require.NoError(t, ValidateWorkload(nil, cd))
}

// The identity fields only make sense as a pair: a module without an apiVersion
// (or the reverse) cannot name a versioned definition, so the handler rejects it.
func TestHandleValidatesIdentityFields(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{
		Decoder: admission.NewDecoder(sc),
		Client:  fake.NewClientBuilder().WithScheme(sc).Build(),
	}

	build := func(module, apiVersion string) admission.Request {
		def := &v1beta1.ComponentDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "identity-comp", Namespace: "vela-system"},
			Spec:       v1beta1.ComponentDefinitionSpec{Workload: common.WorkloadTypeDescriptor{Type: "deployments.apps"}, Module: module, APIVersion: apiVersion},
		}
		raw, err := json.Marshal(def)
		require.NoError(t, err)
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group: componentDefGVR.Group, Version: componentDefGVR.Version, Resource: componentDefGVR.Resource,
			},
			Object: runtime.RawExtension{Raw: raw},
		}}
	}

	t.Run("module and apiVersion together are admitted", func(t *testing.T) {
		resp := h.Handle(context.Background(), build("s3", "v1"))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("neither field is admitted", func(t *testing.T) {
		resp := h.Handle(context.Background(), build("", ""))
		require.True(t, resp.Allowed, "%v", resp.Result)
	})

	t.Run("module without apiVersion is denied", func(t *testing.T) {
		resp := h.Handle(context.Background(), build("s3", ""))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "spec.apiVersion is empty")
	})

	t.Run("apiVersion without module is denied", func(t *testing.T) {
		resp := h.Handle(context.Background(), build("", "v1"))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "spec.module is empty")
	})

	t.Run("an update is validated too", func(t *testing.T) {
		req := build("s3", "")
		req.Operation = admissionv1.Update
		resp := h.Handle(context.Background(), req)
		require.False(t, resp.Allowed)
	})
}

// The TestHandleValidatesIdentityFields tests above build their
// ValidatingHandler over a fake.NewClientBuilder() client, which never runs
// CRD schema/structural validation -- they prove the webhook's own pairing
// check, nothing about whether the apiserver actually persists the fields.
// These specs go through testEnv's real envtest apiserver instead, loaded
// with the real shipped CRDs (charts/vela-core/crds), so they are the only
// place that proves GWCP-106678's actual claim: declaring module/apiVersion
// as schema properties is what stops the apiserver from pruning them.
var _ = Describe("Identity fields survive a real apiserver apply", func() {
	var identityCli client.Client

	BeforeEach(func() {
		var err error
		identityCli, err = client.New(cfg, client.Options{Scheme: testScheme})
		Expect(err).Should(BeNil())

		// This envtest control plane starts with no namespaces beyond "default";
		// every other Describe block here creates its definitions without a
		// namespace, so nothing has needed vela-system until now.
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
		err = identityCli.Create(context.Background(), ns)
		Expect(err == nil || apierrors.IsAlreadyExists(err)).Should(BeTrue(), "%v", err)
	})

	It("persists spec.module and spec.apiVersion on a hand-authored definition", func() {
		def := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "identity-persist", Namespace: "vela-system"},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:   common.WorkloadTypeDescriptor{Type: "deployments.apps"},
				Module:     "s3",
				APIVersion: "v1",
			},
		}
		Expect(identityCli.Create(context.Background(), def)).Should(Succeed())
		defer func() { _ = identityCli.Delete(context.Background(), def) }()

		got := &v1beta1.ComponentDefinition{}
		Expect(identityCli.Get(context.Background(), client.ObjectKeyFromObject(def), got)).Should(Succeed())
		Expect(got.Spec.Module).Should(Equal("s3"), "the apiserver pruned spec.module")
		Expect(got.Spec.APIVersion).Should(Equal("v1"), "the apiserver pruned spec.apiVersion")
	})

	It("accepts a legacy definition with neither identity field set, unchanged", func() {
		def := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "identity-legacy", Namespace: "vela-system"},
			Spec:       v1beta1.ComponentDefinitionSpec{Workload: common.WorkloadTypeDescriptor{Type: "deployments.apps"}},
		}
		Expect(identityCli.Create(context.Background(), def)).Should(Succeed())
		defer func() { _ = identityCli.Delete(context.Background(), def) }()

		got := &v1beta1.ComponentDefinition{}
		Expect(identityCli.Get(context.Background(), client.ObjectKeyFromObject(def), got)).Should(Succeed())
		Expect(got.Spec.Module).Should(BeEmpty())
		Expect(got.Spec.APIVersion).Should(BeEmpty())
	})

	It("rejects an apiVersion that does not match the CRD schema pattern", func() {
		def := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "identity-bad-pattern", Namespace: "vela-system"},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:   common.WorkloadTypeDescriptor{Type: "deployments.apps"},
				Module:     "s3",
				APIVersion: "1.0",
			},
		}
		err := identityCli.Create(context.Background(), def)
		Expect(err).Should(HaveOccurred(), "the CRD schema pattern should reject a non-matching apiVersion")
		Expect(err.Error()).Should(ContainSubstring("apiVersion"))
	})

	It("persists the identity the real module render path stamps, through RenderApplication/stampIdentity", func() {
		mod := &module.Module{
			Name:    "identity-render",
			Version: "1.0.0",
			Lines: map[string]module.Line{
				"v1": {
					APIVersion: "v1",
					Enabled:    true,
					Definitions: []map[string]interface{}{{
						"apiVersion": "core.oam.dev/v1beta1",
						"kind":       "ComponentDefinition",
						"metadata":   map[string]interface{}{"name": "bucket"},
						"spec":       map[string]interface{}{},
					}},
				},
			},
		}

		app, err := modulerender.RenderApplication(mod, "vela-system")
		Expect(err).ShouldNot(HaveOccurred())

		spec, ok := app["spec"].(map[string]interface{})
		Expect(ok).Should(BeTrue(), "owned Application has no spec")
		comps, ok := spec["components"].([]interface{})
		Expect(ok).Should(BeTrue(), "owned Application has no spec.components")
		Expect(comps).Should(HaveLen(1), "one line, no module/line auxiliary: only the defs tier renders")
		defsComp, ok := comps[0].(map[string]interface{})
		Expect(ok).Should(BeTrue())
		props, ok := defsComp["properties"].(map[string]interface{})
		Expect(ok).Should(BeTrue())
		objects, ok := props["objects"].([]interface{})
		Expect(ok).Should(BeTrue())
		Expect(objects).Should(HaveLen(1))
		stampedDef, ok := objects[0].(map[string]interface{})
		Expect(ok).Should(BeTrue())

		raw, err := json.Marshal(stampedDef)
		Expect(err).ShouldNot(HaveOccurred())
		rendered := &v1beta1.ComponentDefinition{}
		Expect(json.Unmarshal(raw, rendered)).Should(Succeed())

		// stampIdentity's own output, before this ever reaches the apiserver.
		Expect(rendered.Spec.Module).Should(Equal("identity-render"))
		Expect(rendered.Spec.APIVersion).Should(Equal("v1"))

		Expect(identityCli.Create(context.Background(), rendered)).Should(Succeed())
		defer func() { _ = identityCli.Delete(context.Background(), rendered) }()

		got := &v1beta1.ComponentDefinition{}
		Expect(identityCli.Get(context.Background(), client.ObjectKeyFromObject(rendered), got)).Should(Succeed())
		Expect(got.Spec.Module).Should(Equal("identity-render"), "the apiserver pruned the render path's spec.module")
		Expect(got.Spec.APIVersion).Should(Equal("v1"), "the apiserver pruned the render path's spec.apiVersion")
	})
})
