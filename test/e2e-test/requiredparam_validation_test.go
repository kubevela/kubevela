package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Application required-parameter validation", Ordered, func() {
	var (
		ctx       context.Context
		nsName    string
		namespace corev1.Namespace
	)

	BeforeAll(func() {
		ctx = context.Background()
		nsName = randomNamespaceName("requiredparam-validation-test")
		namespace = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}

		By("creating the test namespace")
		Eventually(func() error {
			return k8sClient.Create(ctx, &namespace)
		}, 3*time.Second, 300*time.Millisecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Apply the component definition")
		Expect(k8sClient.Create(ctx, newConfigMapComponent(nsName))).To(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up resources after each test")
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(nsName))).To(Succeed())
	})

	AfterAll(func() {
		By("Cleaning up resources after all the test")
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(nsName))).To(Succeed())
		Expect(k8sClient.Delete(ctx, &namespace)).To(Succeed())
	})

	// -------------------------------------------------------------------------
	// Scenario 1: missing parameter → expect failure
	// -------------------------------------------------------------------------
	It("fails when the required parameter is missing", func() {
		app := appWithWorkflow.DeepCopy()
		app.Name = "app-missing-param"
		app.Namespace = nsName

		err := k8sClient.Create(ctx, app)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf(`component %q: missing parameters: secondkey.value2.value3.value5`, "configmap-component")))
	})

	// -------------------------------------------------------------------------
	// Scenario 2: param provided via workflow → expect success
	// -------------------------------------------------------------------------
	It("succeeds when the parameter is provided in the workflow", func() {
		app := appWithWorkflow.DeepCopy()
		app.Name = "app-with-param-wf"
		app.Namespace = nsName

		// inject missing parameter
		app.Spec.Workflow.Steps[0].Inputs = append(app.Spec.Workflow.Steps[0].Inputs,
			workflowv1alpha1.InputItem{
				ParameterKey: "secondkey.value2.value3.value5",
				From:         "dummy",
			})

		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	})

	// -------------------------------------------------------------------------
	// Scenario 3: param provided via policy → expect success
	// -------------------------------------------------------------------------
	It("succeeds when the parameter is provided in a policy", func() {
		app := appWithPolicy.DeepCopy()
		app.Name = "app-with-param-policy"
		app.Namespace = nsName

		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	})
})

/* -------------------------------------------------------------------------- */
/* Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func newConfigMapComponent(namespace string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComponentDefinition",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap-component",
			Namespace: namespace, // set it here
		},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: configMapOutputTemp},
			},
		},
	}
}

var configMapOutputTemp = `
parameter: {
	firstkey: string & !="" & !~".*-$"
	secondkey: {
		value1: string
		value2: {
			value3: {
				value4: *"default-value-2" | string
				value5: string
			}
		}
	}
	thirdkey?: string
}
output: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: { name: context.name }
	data: {
		one: parameter.firstkey
		two: parameter.secondkey.value2.value3.value5
		three: parameter.secondkey.value1
		four: parameter.thirdkey
	}
}
`

var appWithWorkflow = v1beta1.Application{
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{{
			Name: "configmap-component",
			Type: "configmap-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{
				"secondkey": { "value2": { "value3": { "value4": "1" } } }
			}`)},
		}},
		Workflow: &v1beta1.Workflow{
			Steps: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "apply",
					Type: "apply-component",
					Inputs: workflowv1alpha1.StepInputs{
						{ParameterKey: "firstkey", From: "dummy1"},
						{ParameterKey: "secondkey.value1", From: "dummy2"},
						{ParameterKey: "thirdkey", From: "dummy3"},
					},
					Properties: util.Object2RawExtension(map[string]any{"component": "express-server"}),
				},
			}},
		},
	},
}

var appWithPolicy = v1beta1.Application{
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{{
			Name: "app-policy",
			Type: "configmap-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{
				"secondkey": { "value2": { "value3": { "value4": "1" } } }
			}`)},
		}},
		Policies: []v1beta1.AppPolicy{{
			Name:       "override-configmap-data",
			Type:       "override",
			Properties: &runtime.RawExtension{Raw: mustJSON(policyProperties)},
		}},
	},
}

var policyProperties = map[string]any{
	"components": []any{map[string]any{
		"name": "express-server",
		"properties": map[string]any{
			"firstkey": "nginx:1.20",
			"secondkey": map[string]any{
				"value1": "abc",
				"value2": map[string]any{
					"value3": map[string]any{
						"value5": "1",
					},
				},
			},
			"thirdkey": "123",
		},
	}},
}

func mustJSON(v any) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return out
}
