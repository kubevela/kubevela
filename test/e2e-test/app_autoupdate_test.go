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
	"strings"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Application AutoUpdate", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var reconcileSleepTime = 300 * time.Second
	var sleepTime = 5 * time.Second

	BeforeEach(func() {
		By("Create namespace for app-autoupdate-e2e-test")
		namespace = randomNamespaceName("app-autoupdate-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

	Context("Enabled", func() {
		It("When specified exact component version available. App should use exact specified version.", func() {
			By("Create configmap-component with 1.0.0 version")
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 1.2.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1.2.0")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

			By("Create configmap-component with 1.4.0 version")
			updatedComponentVersion = "1.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

		})

		It("When specified component version is unavailable. App should use latest version in specified range.", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1.4")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.4")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

		})

		It("When new component version release after app creation, app should use new version during reconciliation", func() {
			By("Create configmap-component with 2.2.0 version")
			componentVersion := "2.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 2.3.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "2.3.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@2")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "2")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(updatedComponentVersion))

			By("Create configmap-component with 2.4.0 version")
			updatedComponentVersion = "2.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(updatedComponentVersion))

		})

		It("When specified version is available for one component and unavailable for other, app should use autoupdate the latter", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1.4 version")
			app := updateAppComponent(appWithTwoComponentTemplate, "app1", namespace, componentType, "first-component", "1.4")

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(1))

		})

		It("When specified exact trait version available, app should use exact version", func() {
			By("Create scaler-trait with 1.0.0 version and 1 replica")
			traitVersion := "1.0.0"
			traitType := "scaler-trait"
			trait := createTrait(traitVersion, namespace, traitType, "1")
			trait.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create scaler-trait with 1.2.0 version and 2 replicas")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(sleepTime)

			app := updateAppTrait(traitApp, "app1", namespace, traitType, updatedTraitVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(sleepTime)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))

			By("Create scaler-trait with 1.4.0 version")
			updatedTraitVersion = "1.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("3")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)
			pods = new(corev1.PodList)
			opts = []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(sleepTime)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))
		})

		It("When specified trait version is unavailable, app should use latest version specified in range", func() {
			By("Create scaler-trait with 1.4.5 version and 4 replica")
			traitVersion := "1.4.5"
			traitType := "scaler-trait"

			trait := createTrait(traitVersion, namespace, traitType, "4")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using scaler-trait@v1.4")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.4")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(sleepTime)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(4))
		})

		It("When new trait version is created after app creation, app should use new version during reconciliation", func() {
			By("Create scaler-trait with 1.4.5 version and 4 replica")
			traitVersion := "1.4.5"
			traitType := "scaler-trait"

			trait := createTrait(traitVersion, namespace, traitType, "4")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using scaler-trait@v1.4")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.4")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(sleepTime)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(4))

			By("Create scaler-trait with 1.4.8 version and 2 replicas")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.4.8"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)
			pods = new(corev1.PodList)
			opts = []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(sleepTime)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))

		})

		It("When Autoupdate and Publish version annotation are specified in application, app creation should fail", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1.4.5")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.4.5")
			app.ObjectMeta.Annotations[oam.AnnotationPublishVersion] = "alpha"
			err := k8sClient.Create(ctx, app)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("Application has both autoUpdate and publishVersion annotations. Only one can be present"))

		})

		It("When specified exact workflow version available", func() {
			By("Create configmap-workflow with 1.0.0 version")
			workflowVersion := "1.0.0"
			workflowType := "configmap-workflow"
			workflow := createWorkflow(workflowVersion, namespace, workflowType, workflowVersion)
			workflow.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workflow)).Should(Succeed())

			By("Create configmap-workflow with 1.2.0 version")
			updatedWorkflow := new(v1beta1.WorkflowStepDefinition)
			updatedWorkflowVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: workflowType, Namespace: namespace}, updatedWorkflow)
				if err != nil {
					return err
				}
				updatedWorkflow.Spec.Version = updatedWorkflowVersion
				updatedWorkflow.Spec.Schematic.CUE.Template = createWorkflowOutput(updatedWorkflowVersion, namespace)
				return k8sClient.Update(ctx, updatedWorkflow)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-workflow@v1.2.0 which will create configmap")
			app := updatedAppWorkflow(workflowApp, "app1", namespace, workflowType, updatedWorkflowVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedWorkflowVersion))

			By("Create configmap-workflow with 1.5.0 version")
			updatedWorkflowVersion = "1.5.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: workflowType, Namespace: namespace}, updatedWorkflow)
				if err != nil {
					return err
				}
				updatedWorkflow.Spec.Version = updatedWorkflowVersion
				updatedWorkflow.Spec.Schematic.CUE.Template = createWorkflowOutput(updatedWorkflowVersion, namespace)
				return k8sClient.Update(ctx, updatedWorkflow)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo("1.2.0"))

		})

		It("When specified workflow version is unavailable", func() {
			By("Create configmap-workflow with 1.5.0 version")
			workflowVersion := "1.5.0"
			workflowType := "configmap-workflow"
			workflow := createWorkflow(workflowVersion, namespace, workflowType, workflowVersion)
			workflow.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workflow)).Should(Succeed())

			By("Create application using configmap-workflow@v1.5 which will create configmap")
			app := updatedAppWorkflow(workflowApp, "app1", namespace, workflowType, "1.5")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(workflowVersion))
		})

		It("When specified workflow version is unavailable, and after app creation new version of workflow is available", func() {
			By("Create configmap-workflow with 1.5.0 version")
			workflowVersion := "1.5.0"
			workflowType := "configmap-workflow"
			workflow := createWorkflow(workflowVersion, namespace, workflowType, workflowVersion)
			workflow.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workflow)).Should(Succeed())

			By("Create configmap-workflow with 2.5.1 version")
			updatedWorkflow := new(v1beta1.WorkflowStepDefinition)
			updatedWorkflowVersion := "2.5.1"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: workflowType, Namespace: namespace}, updatedWorkflow)
				if err != nil {
					return err
				}
				updatedWorkflow.Spec.Version = updatedWorkflowVersion
				updatedWorkflow.Spec.Schematic.CUE.Template = createWorkflowOutput(updatedWorkflowVersion, namespace)
				return k8sClient.Update(ctx, updatedWorkflow)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-workflow@v2 which will create configmap")
			app := updatedAppWorkflow(workflowApp, "app1", namespace, workflowType, "2")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedWorkflowVersion))

			By("Create configmap-workflow with 2.6.0 version")
			updatedWorkflowVersion = "2.6.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: workflowType, Namespace: namespace}, updatedWorkflow)
				if err != nil {
					return err
				}
				updatedWorkflow.Spec.Version = updatedWorkflowVersion
				updatedWorkflow.Spec.Schematic.CUE.Template = createWorkflowOutput(updatedWorkflowVersion, namespace)
				return k8sClient.Update(ctx, updatedWorkflow)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedWorkflowVersion))

		})

		It("When specified exact policy version available", func() {
			By("Create configmap-policy with 1.0.0 version")
			policyVersion := "1.0.0"
			policyType := "configmap-policy"
			policy := createPolicy(policyVersion, policyType)
			policy.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())

			By("Create configmap-policy with 1.2.0 version")
			updatedPolicy := new(v1beta1.PolicyDefinition)
			updatedPolicyVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: policyType, Namespace: namespace}, updatedPolicy)
				if err != nil {
					return err
				}
				updatedPolicy.Spec.Version = updatedPolicyVersion
				updatedPolicy.Spec.Schematic.CUE.Template = createPolicyOutput(updatedPolicyVersion)
				return k8sClient.Update(ctx, updatedPolicy)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-policy@v1.2.0 which will create configmap")
			app := updatedAppPolicy(policyApp, "app1", namespace, policyType, updatedPolicyVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "policy-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedPolicyVersion))

			By("Create configmap-policy with 1.5.0 version")
			updatedPolicyVersion = "1.5.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: policyType, Namespace: namespace}, updatedPolicy)
				if err != nil {
					return err
				}
				updatedPolicy.Spec.Version = updatedPolicyVersion
				updatedPolicy.Spec.Schematic.CUE.Template = createPolicyOutput(updatedPolicyVersion)
				return k8sClient.Update(ctx, updatedPolicy)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "policy-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo("1.2.0"))

		})

		It("When specified exact policy version is unavailable", func() {
			By("Create configmap-policy with 1.5.0 version")
			policyVersion := "1.5.0"
			policyType := "configmap-policy"
			policy := createPolicy(policyVersion, policyType)
			policy.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())

			By("Create application using configmap-policy@v1.5 which will create configmap")
			app := updatedAppPolicy(policyApp, "app1", namespace, policyType, "1.5")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "policy-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(policyVersion))
		})

		It("When specified policy version is unavailable, and after app creation new version of policy is available", func() {
			By("Create configmap-policy with 1.5.0 version")
			policyVersion := "1.5.0"
			policyType := "configmap-policy"
			policy := createPolicy(policyVersion, policyType)
			policy.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, policy)).Should(Succeed())

			By("Create configmap-policy with 2.5.1 version")
			updatedPolicy := new(v1beta1.PolicyDefinition)
			updatedPolicyVersion := "2.5.1"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: policyType, Namespace: namespace}, updatedPolicy)
				if err != nil {
					return err
				}
				updatedPolicy.Spec.Version = updatedPolicyVersion
				updatedPolicy.Spec.Schematic.CUE.Template = createPolicyOutput(updatedPolicyVersion)
				return k8sClient.Update(ctx, updatedPolicy)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-policy@v2 which will create configmap")
			app := updatedAppPolicy(policyApp, "app1", namespace, policyType, "2")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "policy-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedPolicyVersion))

			By("Create configmap-policy with 2.6.0 version")
			updatedPolicyVersion = "2.6.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: policyType, Namespace: namespace}, updatedPolicy)
				if err != nil {
					return err
				}
				updatedPolicy.Spec.Version = updatedPolicyVersion
				updatedPolicy.Spec.Schematic.CUE.Template = createPolicyOutput(updatedPolicyVersion)
				return k8sClient.Update(ctx, updatedPolicy)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(reconcileSleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "policy-configmap", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["key"]).To(BeEquivalentTo(updatedPolicyVersion))
		})
	})

	Context("Disabled", func() {
		It("When specified component version is available, app should use specified version", func() {
			By("Create configmap-component with 1.0.0 version")
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 1.2.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1.0.0 version")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))
		})

		It("When specified component version is unavailable, app creation should fail", func() {
			By("Create configmap-component with 1.2.0 version")
			componentVersion := "1.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			time.Sleep(sleepTime)

			By("Create application using configmap-component@1 version")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			// TODO
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			time.Sleep(reconcileSleepTime)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)).ShouldNot(BeNil())

			configmaps := new(corev1.ConfigMapList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			Expect(k8sClient.List(ctx, configmaps, opts...)).To(BeNil())
			Expect(len(configmaps.Items)).To(BeEquivalentTo(0))
		})

		It("When specified trait version is available, app should specified trait version", func() {
			By("Create scaler-trait with 1.0.0 version and 1 replica")
			traitVersion := "1.0.0"
			traitType := "scaler-trait"
			trait := createTrait(traitVersion, namespace, traitType, "1")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create scaler-trait with 1.2.0 version and 2 replica")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(sleepTime)

			By("Create application using scaler-trait@1.0.0 version")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, traitVersion)
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Wait for application to be created")
			time.Sleep(reconcileSleepTime)
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(1))

		})

		It("When specified trait version is unavailable, app should fail", func() {
			By("Create scaler-trait with 1.0.0 version and 1 replica")
			traitVersion := "1.0.0"
			traitType := "scaler-trait"
			trait := createTrait(traitVersion, namespace, traitType, "2")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create application using scaler-trait@1.2 version")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.2")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Wait for application to be created")
			time.Sleep(reconcileSleepTime)
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(0))
		})

		It("When specified exact workflow version available, app shouldn't update the workflow version", func() {
			By("Create configmap-workflow with 1.2.0 version")
			workflowVersion := "1.2.0"
			workflowType := "configmap-workflow"
			workflow := createWorkflow(workflowVersion, namespace, workflowType, workflowVersion)
			workflow.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workflow)).Should(Succeed())

			By("Create configmap-workflow with 1.2.1 version")
			updatedWorkflow := new(v1beta1.WorkflowStepDefinition)
			updatedWorkflowVersion := "1.2.1"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: workflowType, Namespace: namespace}, updatedWorkflow)
				if err != nil {
					return err
				}
				updatedWorkflow.Spec.Version = updatedWorkflowVersion
				updatedWorkflow.Spec.Schematic.CUE.Template = createWorkflowOutput(updatedWorkflowVersion, namespace)
				return k8sClient.Update(ctx, updatedWorkflow)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-workflow@v1.2 which will create configmap")
			app := updatedAppWorkflow(workflowApp, "app1", namespace, workflowType, "1.2")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).ShouldNot(Succeed())

		})

		It("When specified workflow version is not available, app should fail", func() {
			By("Create configmap-workflow with 1.2.0 version")
			workflowVersion := "1.2.0"
			workflowType := "configmap-workflow"
			workflow := createWorkflow(workflowVersion, namespace, workflowType, workflowVersion)
			workflow.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workflow)).Should(Succeed())

			By("Create application using configmap-workflow@v2.4 which will create configmap")
			app := updatedAppWorkflow(workflowApp, "app1", namespace, workflowType, "2.4")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Wait for application to be created")
			time.Sleep(reconcileSleepTime)

			cm := new(corev1.ConfigMap)
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "workflow-configmap", Namespace: namespace}, cm)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal(`configmaps "workflow-configmap" not found`))
		})

	})

})

// TODO Add test cases for policydefinition and worflowstepdefinition

func updateAppComponent(appTemplate v1beta1.Application, appName, namespace, typeName, componentName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)

	app.Spec.Components[0].Name = componentName
	return app
}

func updateAppTrait(traitApp v1beta1.Application, appName, namespace, typeName, traitVersion string) *v1beta1.Application {
	app := traitApp.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Traits[0].Type = fmt.Sprintf("%s@v%s", typeName, traitVersion)

	return app
}

func createComponent(componentVersion, namespace, name string) *v1beta1.ComponentDefinition {
	component := configMapComponent.DeepCopy()
	component.ObjectMeta.Name = name
	component.Spec.Version = componentVersion
	component.Spec.Schematic.CUE.Template = createOutputConfigMap(componentVersion)
	component.SetNamespace(namespace)
	return component
}

func createTrait(traitVersion, namespace, name, replicas string) *v1beta1.TraitDefinition {
	trait := scalerTrait.DeepCopy()
	trait.ObjectMeta.Name = name
	trait.Spec.Version = traitVersion
	trait.Spec.Schematic.CUE.Template = createScalerTraitOutput(replicas)
	trait.SetNamespace(namespace)
	return trait
}

func createScalerTraitOutput(replicas string) string {
	return strings.Replace(scalerTraitOutputTemplate, "1", replicas, 1)
}

func createOutputConfigMap(toVersion string) string {
	return strings.Replace(configMapOutputTemplate, "1.0.0", toVersion, 1)
}

var configMapComponent = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "configmap-component",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}
var configMapOutputTemplate = `output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "comptest"
		data: {
			expectedVersion:    "1.0.0"
		}
	}`

var appTemplate = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "Name",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name: "comp1Name",
				Type: "type",
			},
		},
	},
}

var appWithTwoComponentTemplate = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "Name",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name: "first-component",
				Type: "configmap-component",
			},
			{
				Name: "second-component",
				Type: "webservice@v1",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"image": "nginx",
				}),
			},
		},
	},
}

var scalerTraitOutputTemplate = `patch: spec: replicas: 1`

var scalerTrait = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:        "scaler-trait",
		Annotations: map[string]string{},
	},
	Spec: v1beta1.TraitDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

var traitApp = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "app-with-trait",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name:       "webservice-component",
				Type:       "webservice",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
				Traits: []common.ApplicationTrait{
					{
						Type: "scaler-trait",
					},
				},
			},
		},
	},
}

func createWorkflow(workflowVersion, namespace, name, index string) *v1beta1.WorkflowStepDefinition {
	workflow := workflowDefinition.DeepCopy()
	workflow.ObjectMeta.Name = name
	workflow.Spec.Version = workflowVersion
	workflow.Spec.Schematic.CUE.Template = createWorkflowOutput(index, namespace)
	workflow.SetNamespace(namespace)
	return workflow
}

var workflowTemplate = `import (
          "vela/kube"
        )

        apply: kube.#Apply & {
          $params: {
            value: {
              apiVersion: "v1"
                kind:       "ConfigMap"
                metadata: {
                  name:  "workflow-configmap"
				  namespace: "name_space"
                }
                data: key: "1.0.0"
            }
          }
        }`

func createWorkflowOutput(version, namespace string) string {
	return strings.Replace(strings.Replace(workflowTemplate, "1.0.0", version, 1), "name_space", namespace, 1)
}

var workflowDefinition = &v1beta1.WorkflowStepDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "WorkflowDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:        "configmap-workflow",
		Annotations: map[string]string{},
	},
	Spec: v1beta1.WorkflowStepDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

var workflowApp = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "app-with-workflow",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{},
		Workflow: &v1beta1.Workflow{
			Steps: []workflowv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
						Type: "configmap-workflow",
					},
				},
			},
		},
	},
}

func updatedAppWorkflow(appTemplate v1beta1.Application, appName, namespace, typeName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Workflow.Steps[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)
	return app
}

func createPolicyOutput(version string) string {
	policyTemplate := `output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: "policy-configmap"
	data: {
		key: "1.0.0"
	}
}`
	return strings.Replace(policyTemplate, "1.0.0", version, 1)
}

func createPolicy(policyVersion, name string) *v1beta1.PolicyDefinition {
	policy := policyDefinition.DeepCopy()
	policy.ObjectMeta.Name = name
	policy.Spec.Version = policyVersion
	policy.Spec.Schematic.CUE.Template = createPolicyOutput(policyVersion)
	return policy
}

func updatedAppPolicy(appTemplate v1beta1.Application, appName, namespace, typeName, policyVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Policies[0].Type = fmt.Sprintf("%s@v%s", typeName, policyVersion)
	return app
}

var policyDefinition = &v1beta1.PolicyDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "PolicyDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "configmap-policy",
	},
	Spec: v1beta1.PolicyDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

var policyApp = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "app-with-policy",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{},
		Policies: []v1beta1.AppPolicy{
			{
				Name:       "test-policy",
				Type:       "configmap-policy",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
			},
		},
	},
}
