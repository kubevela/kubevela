/*
Copyright 2025 The KubeVela Authors.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("PostDispatch Trait tests", func() {
	ctx := context.Background()
	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("postdispatch-test")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up test namespace")
		ns := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
	})

	Context("Test PostDispatch trait with component status", func() {
		It("Should render PostDispatch trait after component is healthy and has status", func() {
			compDefName := "test-worker-" + randomNamespaceName("")
			traitDefName := "test-status-trait-" + randomNamespaceName("")

			By("Creating ComponentDefinition")
			compDef := &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
	}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: {
			app: parameter.name
		}
		template: {
			metadata: labels: {
				app: parameter.name
			}
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

// Add health policy to wait for status fields
output: status: {
	readyReplicas: *0 | int
	replicas: *0 | int
}

parameter: {
	name: string
	image: string
	replicas: *1 | int
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `isHealth: context.output.status.readyReplicas > 0`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, compDef)).Should(Succeed())

			By("Creating PostDispatch TraitDefinition")
			traitDef := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      traitDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Stage: v1beta1.PostDispatch,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
outputs: statusConfigMap: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: context.name + "-status"
		namespace: context.namespace
	}
	data: {
		// Access the component's output status
		replicas: "\(context.output.status.replicas)"
		readyReplicas: "\(context.output.status.readyReplicas)"
		componentName: context.name
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, traitDef)).Should(Succeed())

			By("Creating Application with PostDispatch trait")
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-postdispatch-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "test-component",
							Type: compDefName,
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"name":"test-worker","image":"nginx:1.14.2","replicas":1}`),
							},
							Traits: []common.ApplicationTrait{
								{
									Type: traitDefName,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Waiting for Application to be running")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-postdispatch-app"}, checkApp)).Should(Succeed())
				g.Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying component Deployment is created and healthy")
			Eventually(func(g Gomega) {
				deploy := &unstructured.Unstructured{}
				deploy.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				})
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-worker"}, deploy)).Should(Succeed())

				status, found, _ := unstructured.NestedMap(deploy.Object, "status")
				g.Expect(found).Should(BeTrue())
				g.Expect(status).ShouldNot(BeNil())

				replicas, _, _ := unstructured.NestedInt64(status, "replicas")
				g.Expect(replicas).Should(Equal(int64(1)))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Verifying PostDispatch trait ConfigMap was created with status data")
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-component-status"}, cm)).Should(Succeed())
				g.Expect(cm.Data).ShouldNot(BeNil())
				g.Expect(cm.Data["componentName"]).Should(Equal("test-component"))
				g.Expect(cm.Data["replicas"]).Should(Equal("1"))
				g.Expect(cm.Data["readyReplicas"]).Should(Equal("1"))
			}, 45*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying PostDispatch trait appears in application status")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-postdispatch-app"}, checkApp)).Should(Succeed())
				g.Expect(checkApp.Status.Services).Should(HaveLen(1))

				svc := checkApp.Status.Services[0]
				g.Expect(svc.Healthy).Should(BeTrue())

				// Find the PostDispatch trait in the status
				var foundTrait bool
				for _, trait := range svc.Traits {
					if trait.Type == traitDefName {
						foundTrait = true
						g.Expect(trait.Healthy).Should(BeTrue())
						break
					}
				}
				g.Expect(foundTrait).Should(BeTrue(), "PostDispatch trait should appear in application status")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Cleaning up test resources")
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, traitDef)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, compDef)).Should(Succeed())
		})

		It("Should show PostDispatch trait as pending before component is healthy", func() {
			compDefName := "test-slow-worker-" + randomNamespaceName("")
			traitDefName := "test-pending-trait-" + randomNamespaceName("")

			By("Creating ComponentDefinition with readiness probe")
			compDef := &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
	}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: {
			app: parameter.name
		}
		template: {
			metadata: labels: {
				app: parameter.name
			}
			spec: containers: [{
				name: parameter.name
				image: parameter.image
				// Add readiness probe that delays health
				readinessProbe: {
					httpGet: {
						path: "/"
						port: 80
					}
					initialDelaySeconds: 10
					periodSeconds: 2
				}
			}]
		}
	}
}

// Add health policy to wait for status fields
output: status: {
	readyReplicas: *0 | int
	replicas: *0 | int
}

parameter: {
	name: string
	image: string
	replicas: *1 | int
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `isHealth: context.output.status.readyReplicas > 0`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, compDef)).Should(Succeed())

			By("Creating PostDispatch TraitDefinition")
			traitDef := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      traitDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Stage: v1beta1.PostDispatch,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
outputs: marker: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: context.name + "-marker"
		namespace: context.namespace
	}
	data: {
		status: "deployed"
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, traitDef)).Should(Succeed())

			By("Creating Application")
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pending-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "slow-component",
							Type: compDefName,
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"name":"slow-worker","image":"nginx:1.14.2","replicas":1}`),
							},
							Traits: []common.ApplicationTrait{
								{
									Type: traitDefName,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Verifying PostDispatch trait shows as pending while component is not healthy")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-pending-app"}, checkApp)).Should(Succeed())

				// Application should be running workflow while component is deploying
				g.Expect(checkApp.Status.Phase).Should(BeElementOf(common.ApplicationRunningWorkflow, common.ApplicationRunning))

				// Look for pending trait in status
				if len(checkApp.Status.Services) > 0 {
					svc := checkApp.Status.Services[0]
					foundPendingTrait := false
					for _, trait := range svc.Traits {
						if trait.Type == traitDefName && trait.Pending {
							// Trait should show as pending and not healthy
							foundPendingTrait = true
							g.Expect(trait.Healthy).Should(BeFalse())
							g.Expect(trait.Message).Should(ContainSubstring("Waiting for component to be healthy"))
							break
						}
					}
					// If we have services in status, we should see the pending trait
					if checkApp.Status.Phase == common.ApplicationRunningWorkflow {
						g.Expect(foundPendingTrait).Should(BeTrue(), "Should have found pending trait while workflow is running")
					}
				}
			}, 20*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Waiting for component to become healthy and PostDispatch trait to be deployed")
			Eventually(func(g Gomega) {
				// Check if the PostDispatch ConfigMap has been created
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "slow-component-marker"}, cm)).Should(Succeed())
				g.Expect(cm.Data).ShouldNot(BeNil())
				g.Expect(cm.Data["status"]).Should(Equal("deployed"))
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			By("Verifying PostDispatch trait is no longer pending")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-pending-app"}, checkApp)).Should(Succeed())

				if len(checkApp.Status.Services) > 0 {
					svc := checkApp.Status.Services[0]
					for _, trait := range svc.Traits {
						if trait.Type == traitDefName {
							// Trait should be healthy, not pending, and not waiting anymore
							g.Expect(trait.Pending).Should(BeFalse())
							g.Expect(trait.Healthy).Should(BeTrue())
							g.Expect(trait.Message).ShouldNot(ContainSubstring("waiting"))
						}
					}
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, traitDef)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, compDef)).Should(Succeed())
		})

		It("Should fail when PostDispatch trait accesses status without health policy", func() {
			compDefName := "test-no-health-" + randomNamespaceName("")
			traitDefName := "test-status-access-trait-" + randomNamespaceName("")

			By("Creating ComponentDefinition WITHOUT health policy")
			compDef := &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
	}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: {
			app: parameter.name
		}
		template: {
			metadata: labels: {
				app: parameter.name
			}
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

parameter: {
	name: string
	image: string
	replicas: *1 | int
}
`,
						},
					},
					// Deliberately NO Status or HealthPolicy - component will be immediately healthy
				},
			}
			Expect(k8sClient.Create(ctx, compDef)).Should(Succeed())

			By("Creating PostDispatch trait that accesses output.status")
			traitDef := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      traitDefName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Stage: v1beta1.PostDispatch,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
outputs: statusConfigMap: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: context.name + "-status"
		namespace: context.namespace
	}
	data: {
		// This will fail because status fields don't exist
		replicas: "\(context.output.status.replicas)"
		readyReplicas: "\(context.output.status.readyReplicas)"
		componentName: context.name
	}
}
`,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, traitDef)).Should(Succeed())

			By("Creating Application with PostDispatch trait")
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-health-app",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "test-component",
							Type: compDefName,
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"name":"test-worker","image":"nginx:1.14.2","replicas":1}`),
							},
							Traits: []common.ApplicationTrait{
								{
									Type: traitDefName,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Verifying Application enters failed state due to rendering error")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-no-health-app"}, checkApp)).Should(Succeed())

				// Application should fail during workflow execution
				if checkApp.Status.Workflow != nil {
					// Workflow should either fail or a step should fail
					workflowFailed := string(checkApp.Status.Workflow.Phase) == "failed"
					stepFailed := false
					for _, step := range checkApp.Status.Workflow.Steps {
						if string(step.Phase) == "failed" {
							stepFailed = true
							// Should have error message about CUE evaluation or status access
							g.Expect(step.Message).Should(Or(
								ContainSubstring("failed to evaluate"),
								ContainSubstring("failed to render"),
								ContainSubstring("PostDispatch"),
								ContainSubstring("status"),
							))
							break
						}
					}
					g.Expect(workflowFailed || stepFailed).Should(BeTrue(), "Expected workflow or step to fail due to status access error")
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, traitDef)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, compDef)).Should(Succeed())
		})
	})
})
