/*
Copyright 2024 The KubeVela Authors.

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

package controllers_test

import (
	"context"
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

var _ = Describe("Application Required Parameter Validation", func() {
	var (
		ctx       context.Context
		namespace string
		ns        corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = randomNamespaceName("requiredparam-validation-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		By("Creating a namespace for the test")
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, 3*time.Second, 300*time.Microsecond).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Cleaning up resources after the test")
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))).To(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))).To(Succeed())
		Expect(k8sClient.Delete(ctx, &ns)).To(Succeed())
	})

	Context("when required parameter is missing", func() {
		It("should fail to create the application with an error message", func() {
			By("Creating a component definition")
			componentType := "configmap-component"
			component := createComponentDef(namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).To(Succeed())

			By("Creating an application missing a required parameter")
			app := updateAppForReqParam(appWithWorkflow, "app-missing-param", namespace, componentType, "configmap-component")
			err := k8sClient.Create(ctx, app)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("component %q: missing parameters: secondkey.value2.value3.value5", componentType)))
		})
	})

	Context("when required parameter is provided", func() {
		It("should successfully create the application", func() {
			By("Creating a component definition")
			componentType := "configmap-component"
			component := createComponentDef(namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).To(Succeed())

			By("Creating an application with all required parameters")
			app := updateAppForReqParam(appWithWorkflow, "app-with-param", namespace, componentType, "configmap-component")

			// Add the missing required parameter
			app.Spec.Workflow.Steps[0].Inputs = append(app.Spec.Workflow.Steps[0].Inputs, workflowv1alpha1.InputItem{
				ParameterKey: "secondkey.value2.value3.value5",
				From:         "dummy",
			})

			Expect(k8sClient.Create(ctx, app)).To(Succeed())
		})
	})

})

// --- Helper functions ---

func updateAppForReqParam(appTemplate v1beta1.Application, appName, namespace, typeName, componentName string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.Name = appName
	app.Namespace = namespace
	app.Spec.Components[0].Type = typeName
	app.Spec.Components[0].Name = componentName
	return app
}

func createComponentDef(namespace, name string) *v1beta1.ComponentDefinition {
	component := configMapComponentReq.DeepCopy()
	component.Name = name
	component.Namespace = namespace
	return component
}

// --- Static test data ---

var configMapComponentReq = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "configmap-component",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: configMapOutputTemp,
			},
		},
	},
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
	metadata: {
		name: context.name
	}
	data: {
		one: parameter.firstkey
		two: parameter.secondkey.value2.value3.value5
		three: parameter.secondkey.value1
		four: parameter.thirdkey
	}
}
`

var appWithWorkflow = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "Name",
		Namespace: "Namespace",
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name: "comp1Name",
				Type: "type",
				Properties: &runtime.RawExtension{Raw: []byte(`{
					"secondkey": {
						"value2": {
							"value3": {
								"value4": "1"
							}
						}
					}
				}`)},
			},
		},
		Workflow: &v1beta1.Workflow{
			Steps: []workflowv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
						Name:      "apply",
						Type:      "apply-component",
						DependsOn: []string{"read-network-properties"},
						Inputs: workflowv1alpha1.StepInputs{
							{ParameterKey: "firstkey", From: "dummy1"},
							{ParameterKey: "secondkey.value1", From: "dummy2"},
							{ParameterKey: "thirdkey", From: "dummy3"},
						},
						Properties: util.Object2RawExtension(map[string]interface{}{
							"component": "express-server",
						}),
					},
				},
			},
		},
	},
}
