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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	Context("Test PostDispatch status for trait, component and application", func() {
		It("Should mark application, component, and PostDispatch traits healthy", func() {
			deploymentTraitName := "test-deployment-trait-" + randomNamespaceName("")
			cmTraitName := "test-cm-trait-" + randomNamespaceName("")

			By("Creating PostDispatch deployment trait definition")
			deploymentTrait := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentTraitName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Stage: v1beta1.PostDispatch,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
outputs: statusPod: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
	}
	spec: {
		replicas: 2
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
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `pod: context.outputs.statusPod
ready: {
	updatedReplicas:    *0 | int
	readyReplicas:      *0 | int
	replicas:           *0 | int
	observedGeneration: *0 | int
} & {
	if pod.status.updatedReplicas != _|_ {
		updatedReplicas: pod.status.updatedReplicas
	}
	if pod.status.readyReplicas != _|_ {
		readyReplicas: pod.status.readyReplicas
	}
	if pod.status.replicas != _|_ {
		replicas: pod.status.replicas
	}
	if pod.status.observedGeneration != _|_ {
		observedGeneration: pod.status.observedGeneration
	}
}
_isHealth: (pod.spec.replicas == ready.readyReplicas) && (pod.spec.replicas == ready.updatedReplicas) && (pod.spec.replicas == ready.replicas) && (ready.observedGeneration == pod.metadata.generation || ready.observedGeneration > pod.metadata.generation)
isHealth: *_isHealth | bool
if pod.metadata.annotations != _|_ {
	if pod.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
		isHealth: true
	}
}
`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTrait)).Should(Succeed())

			By("Creating PostDispatch configmap trait definition")
			cmTrait := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmTraitName,
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
		replicas: "2"
		readyReplicas: "3"
		componentName: context.name
	}
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `cm: context.outputs.statusConfigMap
_isHealth: cm.data.readyReplicas != "2"
isHealth: *_isHealth | bool
`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cmTrait)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, deploymentTrait)
				_ = k8sClient.Delete(ctx, cmTrait)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-postdispatch-status",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "test-deployment",
							Type:       "webservice",
							Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.21","port":80,"cpu":"100m","memory":"128Mi"}`)},
							Traits: []common.ApplicationTrait{
								{Type: "scaler", Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}},
								{Type: deploymentTraitName, Properties: &runtime.RawExtension{Raw: []byte(`{"name":"trait-deployment","image":"nginx:1.21"}`)}},
								{Type: cmTraitName},
							},
						},
					},
				},
			}
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, app) })

			By("Creating application that uses PostDispatch traits")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Waiting for application, component, and traits to become healthy")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "app-with-postdispatch-status"}, checkApp)).Should(Succeed())
				g.Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(checkApp.Status.Services).ShouldNot(BeEmpty())
				for _, svc := range checkApp.Status.Services {
					g.Expect(svc.Healthy).Should(BeTrue())
					for _, traitStatus := range svc.Traits {
						g.Expect(traitStatus.Healthy).Should(BeTrue())
					}
				}
			}, 120*time.Second, 3*time.Second).Should(Succeed())
			By("Ensuring the primary component deployment is healthy")
			Eventually(func(g Gomega) {
				componentDeploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-deployment"}, componentDeploy)).Should(Succeed())
				g.Expect(componentDeploy.Status.ReadyReplicas).Should(Equal(int32(3)))
				g.Expect(componentDeploy.Status.Replicas).Should(Equal(int32(3)))
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			By("Ensuring PostDispatch trait-managed deployment reflects component status")
			Eventually(func(g Gomega) {
				traitDeploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "trait-deployment"}, traitDeploy)).Should(Succeed())
				g.Expect(traitDeploy.Status.ReadyReplicas).Should(Equal(int32(2)))
				g.Expect(traitDeploy.Status.Replicas).Should(Equal(int32(2)))
			}, 90*time.Second, 3*time.Second).Should(Succeed())

			By("Ensuring PostDispatch status ConfigMap reflects healthy state")
			Eventually(func(g Gomega) {
				statusCM := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "test-deployment-status"}, statusCM)).Should(Succeed())
				g.Expect(statusCM.Data["componentName"]).Should(Equal("test-deployment"))
				g.Expect(statusCM.Data["readyReplicas"]).Should(Equal("3"))
				g.Expect(statusCM.Data["replicas"]).Should(Equal("2"))
			}, 90*time.Second, 3*time.Second).Should(Succeed())
		})

		It("Should keep PostDispatch trait pending when component image fails", func() {
			deploymentTraitName := "test-deployment-trait-" + randomNamespaceName("")
			cmTraitName := "test-cm-trait-" + randomNamespaceName("")

			By("Creating PostDispatch deployment trait definition")
			deploymentTrait := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentTraitName,
					Namespace: "vela-system",
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Stage: v1beta1.PostDispatch,
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
outputs: statusPod: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: parameter.name
	}
	spec: {
		replicas: 2
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
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `pod: context.outputs.statusPod
ready: {
	updatedReplicas:    *0 | int
	readyReplicas:      *0 | int
	replicas:           *0 | int
	observedGeneration: *0 | int
} & {
	if pod.status.updatedReplicas != _|_ {
		updatedReplicas: pod.status.updatedReplicas
	}
	if pod.status.readyReplicas != _|_ {
		readyReplicas: pod.status.readyReplicas
	}
	if pod.status.replicas != _|_ {
		replicas: pod.status.replicas
	}
	if pod.status.observedGeneration != _|_ {
		observedGeneration: pod.status.observedGeneration
	}
}
_isHealth: (pod.spec.replicas == ready.readyReplicas) && (pod.spec.replicas == ready.updatedReplicas) && (pod.spec.replicas == ready.replicas) && (ready.observedGeneration == pod.metadata.generation || ready.observedGeneration > pod.metadata.generation)
isHealth: *_isHealth | bool
if pod.metadata.annotations != _|_ {
	if pod.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
		isHealth: true
	}
}
`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTrait)).Should(Succeed())

			By("Creating PostDispatch configmap trait definition")
			cmTrait := &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmTraitName,
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
		replicas: "2"
		readyReplicas: "3"
		componentName: context.name
	}
}
`,
						},
					},
					Status: &common.Status{
						HealthPolicy: `cm: context.outputs.statusConfigMap
_isHealth: cm.data.readyReplicas != "2"
isHealth: *_isHealth | bool
`,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cmTrait)).Should(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, deploymentTrait)
				_ = k8sClient.Delete(ctx, cmTrait)
			})

			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-postdispatch-status",
					Namespace: namespace,
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "test-deployment",
							Type:       "webservice",
							Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.21abc","port":80,"cpu":"100m","memory":"128Mi"}`)},
							Traits: []common.ApplicationTrait{
								{Type: "scaler", Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}},
								{Type: deploymentTraitName, Properties: &runtime.RawExtension{Raw: []byte(`{"name":"trait-deployment","image":"nginx:1.21"}`)}},
								{Type: cmTraitName},
							},
						},
					},
				},
			}
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, app) })

			By("Creating application that uses PostDispatch traits")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Waiting for trait to remain pending while component image fails")
			Eventually(func(g Gomega) {
				checkApp := &v1beta1.Application{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: app.Name}, checkApp)).Should(Succeed())
				g.Expect(checkApp.Status.Services).ShouldNot(BeEmpty())
				svc := checkApp.Status.Services[0]
				g.Expect(svc.Healthy).Should(BeFalse())

				traitFound := false
				for _, traitStatus := range svc.Traits {
					if traitStatus.Type == deploymentTraitName {
						traitFound = true
						g.Expect(traitStatus.Healthy).Should(BeFalse())
						g.Expect(traitStatus.Pending).Should(BeTrue())
						g.Expect(traitStatus.Message).Should(ContainSubstring("Waiting for component to be healthy"))
					}
				}
				g.Expect(traitFound).Should(BeTrue())
			}, 180*time.Second, 5*time.Second).Should(Succeed())
		})
	})

})