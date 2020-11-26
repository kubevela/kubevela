package component_test

import (
	"context"
	"fmt"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/crossplane/oam-kubernetes-runtime/pkg/webhook/v1alpha2/component"
)

var _ = Describe("Component Admission controller Test", func() {
	var component v1alpha2.Component
	var componentName, namespace string
	var label map[string]string
	BeforeEach(func() {
		namespace = "component-test"
		label = map[string]string{"workload": "test-component"}
		// Create a component definition
		componentName = "example-deployment-workload"
		component = v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Parameters: []v1alpha2.ComponentParameter{
					{
						Name:       "image",
						Required:   utilpointer.BoolPtr(true),
						FieldPaths: []string{"spec.template.spec.containers[0].image"},
					},
				},
			},
		}
	})

	Context("Test Mutation Webhook", func() {
		var handler admission.Handler = &MutatingHandler{}
		var workloadDef v1alpha2.WorkloadDefinition
		var workloadTypeName string
		var baseWorkload unstructured.Unstructured

		BeforeEach(func() {
			decoderInjector := handler.(admission.DecoderInjector)
			decoderInjector.InjectDecoder(decoder)
			// define workloadDefinition
			workloadDef = v1alpha2.WorkloadDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:   workloadTypeName,
					Labels: label,
				},
				Spec: v1alpha2.WorkloadDefinitionSpec{
					Reference: v1alpha2.DefinitionReference{
						Name: "foos.example.com",
					},
				},
			}
			// the base workload
			baseWorkload = unstructured.Unstructured{}
			baseWorkload.SetAPIVersion("example.com/v1")
			baseWorkload.SetKind("Foo")
			baseWorkload.SetName("workloadName")
			Expect(len(crd.Spec.Versions)).Should(Equal(1))
			Expect(component.Spec).NotTo(BeNil())
		})

		It("Test bad admission request format", func() {
			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: []byte("bad request")},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
		})

		It("Test noop mutate admission handle", func() {
			component.Spec.Workload = runtime.RawExtension{Raw: util.JSONMarshal(baseWorkload)}

			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: util.JSONMarshal(component)},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test mutate function", func() {
			// the workload that uses type to refer to the workloadDefinition
			workloadWithType := unstructured.Unstructured{}
			typeContent := make(map[string]interface{})
			typeContent[TypeField] = workloadTypeName
			workloadWithType.SetUnstructuredContent(typeContent)
			workloadWithType.SetName("workloadName")
			// set up the bad type
			workloadWithBadType := workloadWithType.DeepCopy()
			workloadWithBadType.Object[TypeField] = workloadDef
			// set up the result
			mutatedWorkload := baseWorkload.DeepCopy()
			mutatedWorkload.SetNamespace(component.GetNamespace())
			mutatedWorkload.SetLabels(util.MergeMapOverrideWithDst(label, map[string]string{oam.WorkloadTypeLabel: workloadTypeName}))
			tests := map[string]struct {
				client   client.Client
				workload interface{}
				errMsg   string
				wanted   []byte
			}{
				"bad workload case": {
					workload: "bad workload",
					errMsg:   "cannot unmarshal string",
				},
				"bad workload type case": {
					workload: workloadWithBadType,
					errMsg:   "workload content has an unknown type",
				},
				"no op case": {
					workload: baseWorkload,
					wanted:   util.JSONMarshal(baseWorkload),
				},
				"update gvk get failed case": {
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
							switch obj.(type) {
							case *v1alpha2.WorkloadDefinition:
								return fmt.Errorf("does not exist")
							}
							return nil
						},
					},
					workload: workloadWithType.DeepCopyObject(),
					errMsg:   "does not exist",
				},
				"update gvk and label case": {
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
							switch o := obj.(type) {
							case *v1alpha2.WorkloadDefinition:
								Expect(key.Name).Should(BeEquivalentTo(typeContent[TypeField]))
								*o = workloadDef
							case *crdv1.CustomResourceDefinition:
								Expect(key.Name).Should(Equal(workloadDef.Spec.Reference.Name))
								*o = crd
							}
							return nil
						},
					},
					workload: workloadWithType.DeepCopyObject(),
					wanted:   util.JSONMarshal(mutatedWorkload),
				},
			}
			for testCase, test := range tests {
				By(fmt.Sprintf("start test : %s", testCase))
				component.Spec.Workload = runtime.RawExtension{Raw: util.JSONMarshal(test.workload)}
				injc := handler.(inject.Client)
				injc.InjectClient(test.client)
				mutatingHandler := handler.(*MutatingHandler)
				dm := mock.NewMockDiscoveryMapper()
				dm.MockKindsFor = mock.NewMockKindsFor("Foo", "v1")
				mutatingHandler.Mapper = dm
				err := mutatingHandler.Mutate(&component)
				if len(test.errMsg) == 0 {
					Expect(err).Should(BeNil())
					Expect(component.Spec.Workload.Raw).Should(BeEquivalentTo(test.wanted))
				} else {
					Expect(err.Error()).Should(ContainSubstring(test.errMsg))
				}
			}
		})
	})

	It("Test validating handler", func() {
		var handler admission.Handler = &ValidatingHandler{}
		decoderInjector := handler.(admission.DecoderInjector)
		decoderInjector.InjectDecoder(decoder)
		By("Creating valid workload")
		validWorkload := unstructured.Unstructured{}
		validWorkload.SetAPIVersion("validAPI")
		validWorkload.SetKind("validKind")
		By("Creating invalid workload with type")
		workloadWithType := validWorkload.DeepCopy()
		typeContent := make(map[string]interface{})
		typeContent[TypeField] = "should not be here"
		workloadWithType.SetUnstructuredContent(typeContent)
		By("Creating invalid workload without kind")
		noKindWorkload := validWorkload.DeepCopy()
		noKindWorkload.SetKind("")
		tests := map[string]struct {
			workload  interface{}
			operation admissionv1beta1.Operation
			pass      bool
			reason    string
		}{
			"valid create case": {
				workload:  validWorkload.DeepCopyObject(),
				operation: admissionv1beta1.Create,
				pass:      true,
				reason:    "",
			},
			"valid update case": {
				workload:  validWorkload.DeepCopyObject(),
				operation: admissionv1beta1.Update,
				pass:      true,
				reason:    "",
			},
			"malformat component": {
				workload:  "bad format",
				operation: admissionv1beta1.Create,
				pass:      false,
				reason:    "the workload is malformat",
			},
			"workload still has type": {
				workload:  workloadWithType.DeepCopyObject(),
				operation: admissionv1beta1.Create,
				pass:      false,
				reason:    "the workload contains type info",
			},
			"no kind workload component": {
				workload:  noKindWorkload.DeepCopyObject(),
				operation: admissionv1beta1.Update,
				pass:      false,
				reason:    "the workload data missing GVK",
			},
		}
		for testCase, test := range tests {
			By(fmt.Sprintf("start test : %s", testCase))
			component.Spec.Workload = runtime.RawExtension{Raw: util.JSONMarshal(test.workload)}
			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: test.operation,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: util.JSONMarshal(component)},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(Equal(test.pass))
			if !test.pass {
				Expect(string(resp.Result.Reason)).Should(ContainSubstring(test.reason))
			}
		}
		By("Test bad admission request format")
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  reqResource,
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := handler.Handle(context.TODO(), req)
		Expect(resp.Allowed).Should(BeFalse())
	})

})
