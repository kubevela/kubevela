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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
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

		labelV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "label-v1", Namespace: namespace}, labelV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		labelV2 := new(v1beta1.TraitDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "label", Namespace: namespace}, labelV2)
			if err != nil {
				return err
			}
			labelV2.Spec.Schematic.CUE.Template = labelV2Template
			return k8sClient.Update(ctx, labelV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		labelV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "label-v2", Namespace: namespace}, labelV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		webserviceV1 := webServiceWithNoTemplate.DeepCopy()
		webserviceV1.Spec.Schematic.CUE.Template = webServiceV1Template
		webserviceV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, webserviceV1)).Should(Succeed())

		webserviceV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "webservice-v1", Namespace: namespace}, webserviceV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		webserviceV2 := new(v1beta1.ComponentDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "webservice", Namespace: namespace}, webserviceV2)
			if err != nil {
				return err
			}
			webserviceV2.Spec.Schematic.CUE.Template = webServiceV2Template
			return k8sClient.Update(ctx, webserviceV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		webserviceV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "webservice-v2", Namespace: namespace}, webserviceV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		jobV1 := jobComponentDef.DeepCopy()
		jobV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, jobV1)).Should(Succeed())

		jobV1Rev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "job-v1.2.1", Namespace: namespace}, jobV1Rev)
		}, 15*time.Second, time.Second).Should(BeNil())
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

		workerV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "worker-v1", Namespace: namespace}, workerV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		workerV2 := new(v1beta1.ComponentDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "worker", Namespace: namespace}, workerV2)
			if err != nil {
				return err
			}
			workerV1.Spec.Workload = common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
			}
			workerV2.Spec.Schematic.CUE.Template = workerV2Template
			return k8sClient.Update(ctx, workerV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		workerV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "worker-v2", Namespace: namespace}, workerV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []common.ApplicationTrait{
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
		Eventually(func() error {
			return k8sClient.Create(ctx, app.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		webServiceDeploy := &appsv1.Deployment{}
		deployName := comp1Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, webServiceDeploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		workerDeploy := &appsv1.Deployment{}
		deployName = comp2Name
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
				Components: []common.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []common.ApplicationTrait{
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

		By("Wait for dispatching v2 resources successfully")
		Eventually(func() error {
			RequestReconcileNow(ctx, &app)
			rt := &v1beta1.ResourceTracker{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%s-v2-%s", appName, namespace)}, rt); err != nil {
				return err
			}
			if len(rt.Spec.ManagedResources) != 0 {
				return nil
			}
			return errors.New("v2 resources have not been dispatched")
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		webServiceV1Deploy := &appsv1.Deployment{}
		deployName = comp1Name
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, webServiceV1Deploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify the workload(job) is created successfully")
		workerJob := &batchv1.Job{}
		jobName := comp2Name
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
				Components: []common.ApplicationComponent{
					{
						Name: comp1Name,
						Type: "webservice@v10",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
							"cmd":   []string{"sleep", "1000"},
						}),
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(HaveOccurred())
	})

	It("Test deploy application which specify the name of component", func() {
		compName := "job"
		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-defrevision-app-with-job",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: "job@v1.2.1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "busybox",
							"cmd":   []string{"sleep", "1000"},
						}),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

		By("Verify the workload(job) is created successfully")
		busyBoxJob := &batchv1.Job{}
		jobName := compName
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: namespace}, busyBoxJob)
		}, 30*time.Second, 3*time.Second).Should(Succeed())
	})

	PIt("Test deploy application which containing helm module", func() {
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
				Release: *util.Object2RawExtension(map[string]interface{}{
					"chart": map[string]interface{}{
						"spec": map[string]interface{}{
							"chart":   "podinfo",
							"version": "5.1.4",
						},
					},
				}),
				Repository: *util.Object2RawExtension(map[string]interface{}{
					"url": "https://stefanprodan.github.io/podinfo",
				}),
			},
		}
		Expect(k8sClient.Create(ctx, helmworkerV1)).Should(Succeed())

		helmworkerV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "helm-worker-v1", Namespace: namespace}, helmworkerV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		helmworkerV2 := new(v1beta1.ComponentDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "helm-worker", Namespace: namespace}, helmworkerV2)
			if err != nil {
				return err
			}
			helmworkerV2.Spec.Workload.Definition = common.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			}
			helmworkerV2.Spec.Workload.Type = "deployments.apps"
			helmworkerV2.Spec.Schematic = &common.Schematic{
				HELM: &common.Helm{
					Release: *util.Object2RawExtension(map[string]interface{}{
						"chart": map[string]interface{}{
							"spec": map[string]interface{}{
								"chart":   "podinfo",
								"version": "5.2.0",
							},
						},
					}),
					Repository: *util.Object2RawExtension(map[string]interface{}{
						"url": "https://stefanprodan.github.io/podinfo",
					}),
				},
			}
			return k8sClient.Update(ctx, helmworkerV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		helmworkerV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "helm-worker-v2", Namespace: namespace}, helmworkerV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: "helm-worker",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]interface{}{
								"tag": "5.1.2",
							},
						}),
						Traits: []common.ApplicationTrait{
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
		Eventually(func() error {
			return k8sClient.Create(ctx, app.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

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
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify patch trait is applied")
			templateLabels := deploy.GetLabels()
			return templateLabels["hello"] == "world"
		}, 200*time.Second, 10*time.Second).Should(BeTrue())

		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
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
						Required:    pointer.Bool(true),
						Description: pointer.String("test description"),
					},
				},
			},
		}
		kubeworkerV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, kubeworkerV1)).Should(Succeed())

		kubeworkerV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "kube-worker-v1", Namespace: namespace}, kubeworkerV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		kubeworkerV2 := new(v1beta1.ComponentDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "kube-worker", Namespace: namespace}, kubeworkerV2)
			if err != nil {
				return err
			}
			kubeworkerV2.Spec.Workload.Definition = common.WorkloadGVK{
				APIVersion: "batch/v1",
				Kind:       "Job",
			}
			kubeworkerV2.Spec.Workload.Type = "jobs.batch"
			kubeworkerV2.Spec.Schematic = &common.Schematic{
				KUBE: &common.Kube{
					Template: generateTemplate(KUBEWorkerV2Template),
					Parameters: []common.KubeParameter{
						{
							Name:        "image",
							ValueType:   common.StringType,
							FieldPaths:  []string{"spec.template.spec.containers[0].image"},
							Required:    pointer.Bool(true),
							Description: pointer.String("test description"),
						},
					},
				},
			}
			return k8sClient.Update(ctx, kubeworkerV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		kubeworkerV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "kube-worker-v2", Namespace: namespace}, kubeworkerV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: "kube-worker",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "busybox",
						}),
						Traits: []common.ApplicationTrait{
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
		Eventually(func() error {
			return k8sClient.Create(ctx, app.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(job) is created successfully")
		job := &batchv1.Job{}
		jobName := compName
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
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: "kube-worker@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []common.ApplicationTrait{
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

		By("Verify the workload(deployment) is created successfully")
		deploy := &appsv1.Deployment{}
		deployName := compName
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
				Components: []common.ApplicationComponent{
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
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(HaveOccurred())
	})

	// refer to https://github.com/oam-dev/kubevela/discussions/1810#discussioncomment-914295
	It("Test k8s resources created by application whether with correct label", func() {
		var (
			appName  = "test-resources-labels"
			compName = "web"
		)

		exposeV1 := exposeWithNoTemplate.DeepCopy()
		exposeV1.Spec.Schematic.CUE.Template = exposeV1Template
		exposeV1.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, exposeV1)).Should(Succeed())

		exposeV1DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "expose-v1", Namespace: namespace}, exposeV1DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		exposeV2 := new(v1beta1.TraitDefinition)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "expose", Namespace: namespace}, exposeV2)
			if err != nil {
				return err
			}
			exposeV2.Spec.Schematic.CUE.Template = exposeV2Template
			return k8sClient.Update(ctx, exposeV2)
		}, 15*time.Second, time.Second).Should(BeNil())

		exposeV2DefRev := new(v1beta1.DefinitionRevision)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "expose-v2", Namespace: namespace}, exposeV2DefRev)
		}, 15*time.Second, time.Second).Should(BeNil())

		app := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: "webservice@v1",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "crccheck/hello-world",
							"port":  8000,
						}),
						Traits: []common.ApplicationTrait{
							{
								Type: "expose@v1",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"port": []int{8000},
								}),
							},
						},
					},
				},
			},
		}

		By("Create application")
		Eventually(func() error {
			return k8sClient.Create(ctx, app.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		webServiceDeploy := &appsv1.Deployment{}
		deployName := compName
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, webServiceDeploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify the workload label generated by KubeVela")
		workloadLabel := webServiceDeploy.GetLabels()[oam.WorkloadTypeLabel]
		Expect(workloadLabel).Should(Equal("webservice-v1"))

		By("Verify the traPIt(service) is created successfully")
		exposeSVC := &corev1.Service{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: compName, Namespace: namespace}, exposeSVC)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify the trait label generated by KubeVela")
		traitLabel := exposeSVC.GetLabels()[oam.TraitTypeLabel]
		Expect(traitLabel).Should(Equal("expose-v1"))
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

var jobComponentDef = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "job",
		Annotations: map[string]string{
			oam.AnnotationDefinitionRevisionName: "1.2.1",
		},
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Workload: common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
		},
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: workerV1Template,
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

var exposeWithNoTemplate = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "expose",
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

var exposeV1Template = `
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata:
		name: context.name
	spec: {
		selector:
			"app.oam.dev/component": context.name
		ports: [
			for p in parameter.port {
				port:       p
				targetPort: p
			},
		]
	}
}
parameter: {
	// +usage=Specify the exposion ports
	port: [...int]
}
`

var exposeV2Template = `
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata:
		name: context.name
	spec: {
		selector: {
			"app.oam.dev/component": context.name
		}
		ports: [
			for k, v in parameter.http {
				port:       v
				targetPort: v
			},
		]
	}
}

outputs: ingress: {
	apiVersion: "networking.k8s.io/v1beta1"
	kind:       "Ingress"
	metadata:
		name: context.name
	spec: {
		rules: [{
			host: parameter.domain
			http: {
				paths: [
					for k, v in parameter.http {
						path: k
						backend: {
							serviceName: context.name
							servicePort: v
						}
					},
				]
			}
		}]
	}
}

parameter: {
	domain: string
	http: [string]: int
}
`
