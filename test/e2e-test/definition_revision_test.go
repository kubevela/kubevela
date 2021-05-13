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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application of the specified definition version", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("defrev-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		labelV1 := labelWithNoTemplate.DeepCopy()
		labelV1.Spec.Schematic.CUE.Template = labelV1Template
		labelV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, labelV1)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "label", Namespace: namespace}, labelV1)
			if err != nil {
				return err
			}
			labelV1.Spec.Schematic.CUE.Template = labelV2Template
			return k8sClient.Update(ctx, labelV1)
		}, 15*time.Second, time.Second).Should(BeNil())
		labelDefRevList := new(v1beta1.DefinitionRevisionList)
		labelDefRevListOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelTraitDefinitionName: "label",
			},
		}
		Eventually(func() error {
			err := k8sClient.List(ctx, labelDefRevList, labelDefRevListOpts...)
			if err != nil {
				return err
			}
			if len(labelDefRevList.Items) != 2 {
				return fmt.Errorf("error defRevison number wants %d, actually %d", 2, len(labelDefRevList.Items))
			}
			return nil
		}, 40*time.Second, time.Second).Should(BeNil())

	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("Test deploy application which containing cue rendering module", func() {
		var (
			appName   = "test-website-app"
			comp1Name = "front"
			comp2Name = "backend"
		)

		workerV1 := workerWithNoTemplate.DeepCopy()
		workerV1.Spec.Workload = common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
		}
		workerV1.Spec.Schematic.CUE.Template = workerV1Template
		workerV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, workerV1)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "worker", Namespace: namespace}, workerV1)
			if err != nil {
				return err
			}
			workerV1.Spec.Workload = common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
			}
			workerV1.Spec.Schematic.CUE.Template = workerV2Template
			return k8sClient.Update(ctx, workerV1)
		}, 15*time.Second, time.Second).Should(BeNil())
		workerDefRevList := new(v1beta1.DefinitionRevisionList)
		workerDefRevListOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelComponentDefinitionName: "worker",
			},
		}
		Eventually(func() error {
			err := k8sClient.List(ctx, workerDefRevList, workerDefRevListOpts...)
			if err != nil {
				return err
			}
			if len(workerDefRevList.Items) != 2 {
				return fmt.Errorf("error defRevison number wants %d, actually %d", 2, len(workerDefRevList.Items))
			}
			return nil
		}, 40*time.Second, time.Second).Should(BeNil())

		webserviceV1 := webServiceWithNoTemplate.DeepCopy()
		webserviceV1.Spec.Schematic.CUE.Template = webServiceV1Template
		webserviceV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, webserviceV1)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "webservice", Namespace: namespace}, webserviceV1)
			if err != nil {
				return err
			}
			webserviceV1.Spec.Schematic.CUE.Template = webServiceV2Template
			return k8sClient.Update(ctx, webserviceV1)
		}, 15*time.Second, time.Second).Should(BeNil())

		webserviceDefRevList := new(v1beta1.DefinitionRevisionList)
		webserviceDefRevListOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelComponentDefinitionName: "webservice",
			},
		}
		Eventually(func() error {
			err := k8sClient.List(ctx, webserviceDefRevList, webserviceDefRevListOpts...)
			if err != nil {
				return err
			}
			if len(webserviceDefRevList.Items) != 2 {
				return fmt.Errorf("error defRevison number wants %d, actually %d", 2, len(webserviceDefRevList.Items))
			}
			return nil
		}, 40*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: "label",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"labels": map[string]string{
										"hello": "world",
									},
								}),
							},
						},
					},
					{
						Name: comp2Name,
						Type: "worker",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "busybox",
							"cmd":   []string{"sleep", "1000"},
						}),
					},
				},
			},
		}

		By("Create application")
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

		ac := &v1alpha2.ApplicationContext{}
		acName := appName
		By("Verify the ApplicationContext is created & reconciled successfully")
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return len(ac.Status.Workloads) > 0
		}, 60*time.Second, time.Second).Should(BeTrue())

		By("Verify the workload(deployment) is created successfully")
		Expect(len(ac.Status.Workloads)).Should(Equal(len(app.Spec.Components)))
		webServiceDeploy := &appsv1.Deployment{}
		deployName := ac.Status.Workloads[0].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, webServiceDeploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		workerDeploy := &appsv1.Deployment{}
		deployName = ac.Status.Workloads[1].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, workerDeploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify trait is applied to the workload")
		webserviceLabels := webServiceDeploy.GetLabels()
		Expect(webserviceLabels["hello"]).Should(Equal("world"))

		By("Update Application and Specify the Definition version in Application")
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: "label@v1",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"labels": map[string]string{
										"hello": "kubevela",
									},
								}),
							},
						},
					},
					{
						Name: comp2Name,
						Type: "worker@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "busybox",
							"cmd":   []string{"sleep", "1000"},
						}),
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		By("Verify the ApplicationContext is update successfully")
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return ac.Generation == 2
		}, 10*time.Second, time.Second).Should(BeTrue())

		By("Verify the workload(deployment) is created successfully")
		Expect(len(ac.Status.Workloads)).Should(Equal(len(app.Spec.Components)))
		webServiceV1Deploy := &appsv1.Deployment{}
		deployName = ac.Status.Workloads[0].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, webServiceV1Deploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify the workload(job) is created successfully")
		workerJob := &batchv1.Job{}
		jobName := ac.Status.Workloads[1].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, workerJob)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify trait is applied to the workload")
		webserviceV1Labels := webServiceV1Deploy.GetLabels()
		Expect(webserviceV1Labels["hello"]).Should(Equal("kubevela"))

		By("Check Application is rendered by the specified version of the Definition")
		Expect(webServiceV1Deploy.Labels["componentdefinition.oam.dev/version"]).Should(Equal("v1"))
		Expect(webServiceV1Deploy.Labels["traitdefinition.oam.dev/version"]).Should(Equal("v1"))

		By("Application specifies the wrong version of the Definition, it will raise an error")
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice@v10",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		apprev := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v3", appName)}, apprev)).Should(HaveOccurred())
	})

	It("Test deploy application which containing helm module", func() {
		var (
			appName  = "test-helm"
			compName = "worker"
		)

		helmworkerV1 := HELMWorker.DeepCopy()
		helmworkerV1.SetNamespace(namespace)
		helmworkerV1.Spec.Workload.Definition = common.WorkloadGVK{
			APIVersion: "batch/v1beta1",
			Kind:       "CronJob",
		}
		helmworkerV1.Spec.Schematic = &common.Schematic{
			HELM: &common.Helm{
				Release: util.Object2RawExtension(map[string]interface{}{
					"chart": map[string]interface{}{
						"spec": map[string]interface{}{
							"chart":   "podinfo",
							"version": "5.1.4",
						},
					},
				}),
				Repository: util.Object2RawExtension(map[string]interface{}{
					"url": "https://stefanprodan.github.io/podinfo",
				}),
			},
		}
		Expect(k8sClient.Create(ctx, helmworkerV1)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "helm-worker", Namespace: namespace}, helmworkerV1)
			if err != nil {
				return err
			}
			helmworkerV1.Spec.Workload.Definition = common.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}
			helmworkerV1.Spec.Schematic = &common.Schematic{
				HELM: &common.Helm{
					Release: util.Object2RawExtension(map[string]interface{}{
						"chart": map[string]interface{}{
							"spec": map[string]interface{}{
								"chart":   "podinfo",
								"version": "5.2.0",
							},
						},
					}),
					Repository: util.Object2RawExtension(map[string]interface{}{
						"url": "https://stefanprodan.github.io/podinfo",
					}),
				},
			}
			return k8sClient.Update(ctx, helmworkerV1)
		}, 15*time.Second, time.Second).Should(BeNil())

		helmworkerDefRevList := new(v1beta1.DefinitionRevisionList)
		helmworkerDefRevListOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelComponentDefinitionName: "helm-worker",
			},
		}
		Eventually(func() error {
			err := k8sClient.List(ctx, helmworkerDefRevList, helmworkerDefRevListOpts...)
			if err != nil {
				return err
			}
			if len(helmworkerDefRevList.Items) != 2 {
				return fmt.Errorf("error defRevison number wants %d, actually %d", 2, len(helmworkerDefRevList.Items))
			}
			return nil
		}, 40*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: compName,
						Type: "helm-worker",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]interface{}{
								"tag": "5.1.2",
							},
						}),
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: "label",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"labels": map[string]string{
										"hello": "world",
									},
								}),
							},
						},
					},
				},
			},
		}

		By("Create application")
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

		ac := &v1alpha2.ApplicationContext{}
		acName := appName
		By("Verify the ApplicationContext is created successfully")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac)
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Verify the workload(deployment) is created successfully by Helm")
		deploy := &appsv1.Deployment{}
		deployName := fmt.Sprintf("%s-%s-podinfo", appName, compName)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
			if err != nil {
				return false
			}
			DeployLabels := deploy.GetLabels()
			return DeployLabels["helm.sh/chart"] == "podinfo-5.2.0"
		}, 120*time.Second, 5*time.Second).Should(BeTrue())

		By("Verify trait is applied to the workload")
		Eventually(func() bool {
			requestReconcileNow(ctx, ac)
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify patch trait is applied")
			templateLabels := deploy.GetLabels()
			return templateLabels["hello"] != "world"
		}, 120*time.Second, 10*time.Second).Should(BeTrue())

		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: compName,
						Type: "helm-worker@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]interface{}{
								"tag": "5.1.2",
							},
						}),
					},
				},
			},
		}

		By("Create application")
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		By("Verify the ApplicationContext is updated")
		Eventually(func() bool {
			ac = &v1alpha2.ApplicationContext{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return ac.GetGeneration() == 2
		}, 15*time.Second, 3*time.Second).Should(BeTrue())

		By("Verify the workload(deployment) is update successfully by Helm")
		deploy = &appsv1.Deployment{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
			if err != nil {
				return false
			}
			DeployLabels := deploy.GetLabels()
			return DeployLabels["helm.sh/chart"] != "podinfo-5.1.4"
		}, 120*time.Second, 5*time.Second).Should(BeTrue())
	})

	It("Test deploy application which containing kube module", func() {
		var (
			appName  = "test-kube-app"
			compName = "worker"
		)

		kubeworkerV1 := KUBEWorker.DeepCopy()
		kubeworkerV1.Spec.Workload.Definition = common.WorkloadGVK{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		}
		kubeworkerV1.Spec.Schematic = &common.Schematic{
			KUBE: &common.Kube{
				Template: generateTemplate(KUBEWorkerV1Template),
				Parameters: []common.KubeParameter{
					{
						Name:        "image",
						ValueType:   common.StringType,
						FieldPaths:  []string{"spec.template.spec.containers[0].image"},
						Required:    pointer.BoolPtr(true),
						Description: pointer.StringPtr("test description"),
					},
				},
			},
		}
		kubeworkerV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, kubeworkerV1)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "kube-worker", Namespace: namespace}, kubeworkerV1)
			if err != nil {
				return err
			}
			kubeworkerV1.Spec.Workload.Definition = common.WorkloadGVK{
				APIVersion: "batch/v1",
				Kind:       "Job",
			}
			kubeworkerV1.Spec.Schematic = &common.Schematic{
				KUBE: &common.Kube{
					Template: generateTemplate(KUBEWorkerV2Template),
					Parameters: []common.KubeParameter{
						{
							Name:        "image",
							ValueType:   common.StringType,
							FieldPaths:  []string{"spec.template.spec.containers[0].image"},
							Required:    pointer.BoolPtr(true),
							Description: pointer.StringPtr("test description"),
						},
					},
				},
			}
			return k8sClient.Update(ctx, kubeworkerV1)
		}, 15*time.Second, time.Second).Should(BeNil())

		kubeworkerDefRevList := new(v1beta1.DefinitionRevisionList)
		kubeworkerDefRevListOpts := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				oam.LabelComponentDefinitionName: "kube-worker",
			},
		}
		Eventually(func() error {
			err := k8sClient.List(ctx, kubeworkerDefRevList, kubeworkerDefRevListOpts...)
			if err != nil {
				return err
			}
			if len(kubeworkerDefRevList.Items) != 2 {
				return fmt.Errorf("error defRevison number wants %d, actually %d", 2, len(kubeworkerDefRevList.Items))
			}
			return nil
		}, 40*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: compName,
						Type: "kube-worker",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "busybox",
						}),
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: "label",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"labels": map[string]string{
										"hello": "world",
									},
								}),
							},
						},
					},
				},
			},
		}

		By("Create application")
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

		ac := &v1alpha2.ApplicationContext{}
		acName := appName
		By("Verify the ApplicationContext is created & reconciled successfully")
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return len(ac.Status.Workloads) > 0
		}, 60*time.Second, time.Second).Should(BeTrue())

		By("Verify the workload(job) is created successfully")
		job := &batchv1.Job{}
		jobName := ac.Status.Workloads[0].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, job)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify trait is applied to the workload")
		Labels := job.GetLabels()
		Expect(Labels["hello"]).Should(Equal("world"))

		By("Update Application and Specify the Definition version in Application")
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: compName,
						Type: "kube-worker@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: "label",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"labels": map[string]string{
										"hello": "kubevela",
									},
								}),
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		By("Verify the ApplicationContext is update successfully")
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return ac.Generation == 2
		}, 10*time.Second, time.Second).Should(BeTrue())

		By("Verify the workload(deployment) is created successfully")
		Expect(len(ac.Status.Workloads)).Should(Equal(len(app.Spec.Components)))
		deploy := &appsv1.Deployment{}
		deployName := ac.Status.Workloads[0].Reference.Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify trait is applied to the workload")
		webserviceV1Labels := deploy.GetLabels()
		Expect(webserviceV1Labels["hello"]).Should(Equal("kubevela"))

		By("Application specifies the wrong version of the Definition, it will raise an error")
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Name: compName,
						Type: "kube-worker@a1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		apprev := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v3", appName)}, apprev)).Should(HaveOccurred())
	})

})

var webServiceWithNoTemplate = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "webservice",
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
				Template: "",
			},
		},
	},
}

var workerWithNoTemplate = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "worker",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

var KUBEWorker = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "kube-worker",
	},
}

var HELMWorker = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "helm-worker",
	},
}

var labelWithNoTemplate = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "label",
	},
	Spec: v1beta1.TraitDefinitionSpec{
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

func generateTemplate(template string) runtime.RawExtension {
	b, _ := yaml.YAMLToJSON([]byte(template))
	return runtime.RawExtension{Raw: b}
}

var webServiceV1Template = `output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: labels: {
		"componentdefinition.oam.dev/version": "v1"
	}
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": context.name
		}
		template: {
			metadata: labels: {
				"app.oam.dev/component": context.name
			}
			spec: {
				containers: [{
					name:  context.name
					image: parameter.image
					if parameter["cmd"] != _|_ {
						command: parameter.cmd
					}
					if parameter["env"] != _|_ {
						env: parameter.env
					}
					if context["config"] != _|_ {
						env: context.config
					}
					ports: [{
						containerPort: parameter.port
					}]
					if parameter["cpu"] != _|_ {
						resources: {
							limits:
								cpu: parameter.cpu
							requests:
								cpu: parameter.cpu
						}
					}
				}]
		}
		}
	}
}
parameter: {
	image: string
	cmd?: [...string]
	port: *80 | int
	env?: [...{
		name:   string
		value?: string
		valueFrom?: {
			secretKeyRef: {
				name: string
				key:  string
			}
		}
	}]
	cpu?: string
}
`

var webServiceV2Template = `output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    spec: {
        selector: matchLabels: {
            "app.oam.dev/component": context.name
            if parameter.addRevisionLabel {
                "app.oam.dev/appRevision": context.appRevision
            }
        }
        template: {
            metadata: labels: {
                "app.oam.dev/component": context.name
                if parameter.addRevisionLabel {
                    "app.oam.dev/appRevision": context.appRevision
               	 }
            }
            spec: {
                containers: [{
                    name:  context.name
                    image: parameter.image
                    if parameter["cmd"] != _|_ {
                        command: parameter.cmd
                    }
                    if parameter["env"] != _|_ {
                        env: parameter.env
                    }
                    if context["config"] != _|_ {
                        env: context.config
                    }
                    ports: [{
                        containerPort: parameter.port
                    }]
                    if parameter["cpu"] != _|_ {
                        resources: {
                            limits:
                                cpu: parameter.cpu
                            requests:
                                cpu: parameter.cpu
                        }
                    }
                }]
        }
        }
    }
}
parameter: {
    image: string
    cmd?: [...string]
    port: *80 | int
    env?: [...{
        name: string
        value?: string
        valueFrom?: {
            secretKeyRef: {
                name: string
                key: string
            }
        }
    }]
    cpu?: string
    addRevisionLabel: *false | bool
}
`

var workerV1Template = `output: {
	apiVersion: "batch/v1"
	kind:       "Job"
	spec: {
		parallelism: parameter.count
		completions: parameter.count
		template: spec: {
			restartPolicy	: parameter.restart
				containers: [{
				name:  context.name
				image: parameter.image
				if parameter["cmd"] != _|_ {
					command: parameter.cmd
				}
			}]
		}
	}
}
parameter: {
	count: *1 | int
	image: string
	restart: *"Never" | string
	cmd?: [...string]
}
`

var workerV2Template = `output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    spec: {
        selector: matchLabels: {
            "app.oam.dev/component": context.name
        }
        template: {
            metadata: labels: {
                "app.oam.dev/component": context.name
            }
            spec: {
                containers: [{
                    name:  context.name
                    image: parameter.image

                    if parameter["cmd"] != _|_ {
                        command: parameter.cmd
                    }
                }]
            }
        }
    }
}
parameter: {
    image: string
    cmd?: [...string]
}
`

var labelV1Template = `patch: {
	metadata: labels: {
		for k, v in parameter.labels {
			"\(k)": v
		}
		"traitdefinition.oam.dev/version": "v1"
	}
}
parameter: {
	labels: [string]: string
}
`

var labelV2Template = `patch: {
	metadata: labels: {
		for k, v in parameter.labels {
			"\(k)": v
		}	
	}
}
parameter: {
	labels: [string]: string
}
`

var KUBEWorkerV1Template = `apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
      ports:
      - containerPort: 80
`

var KUBEWorkerV2Template = `apiVersion: "batch/v1"
kind: "Job"
spec:
  parallelism: 1
  completions: 1
  template:
    spec:
      restartPolicy: "Never"
      containers:
      - name: "job"
        image: "busybox"
        command:
        - "sleep"
        - "1000"
`
