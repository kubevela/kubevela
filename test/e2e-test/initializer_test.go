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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Initializer Normal tests", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("initializer-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		worker := workerWithNoTemplate.DeepCopy()
		worker.Spec.Workload = common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		}
		worker.Spec.Schematic.CUE.Template = workerV2Template
		worker.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, worker)).Should(Succeed())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Initializer{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("Test apply initializer without dependsOn", func() {
		compName := "env1-comp"
		init := &v1beta1.Initializer{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Initializer",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env1",
				Namespace: namespace,
			},
			Spec: v1beta1.InitializerSpec{
				AppTemplate: v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []v1beta1.ApplicationComponent{
							{
								Name: compName,
								Type: "worker",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"image": "busybox",
									"cmd":   []string{"sleep", "10000"},
								}),
							},
						},
					},
				},
			},
		}

		By("Create Initializer")
		Eventually(func() error {
			return k8sClient.Create(ctx, init)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the application is created successfully")
		app := new(v1beta1.Application)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: init.Name, Namespace: namespace}, app)
		}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the initializer successfully initialized the environment")
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: init.Name, Namespace: namespace}, init)
			if err != nil {
				return err
			}
			if init.Status.Phase != v1beta1.InitializerSuccess {
				return fmt.Errorf("environment was not successfully initialized")
			}
			return nil
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})

	It("Test apply initializer depends on other initializer", func() {
		compName := "env2-comp"

		init := &v1beta1.Initializer{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Initializer",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env2",
				Namespace: namespace,
			},
			Spec: v1beta1.InitializerSpec{
				AppTemplate: v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []v1beta1.ApplicationComponent{
							{
								Name: compName,
								Type: "worker",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"image": "busybox",
									"cmd":   []string{"sleep", "10000"},
								}),
							},
						},
					},
				},
			},
		}

		By("Create Initializer env2")
		Eventually(func() error {
			return k8sClient.Create(ctx, init)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		compName2 := "env3-comp"
		init2 := &v1beta1.Initializer{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Initializer",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env3",
				Namespace: namespace,
			},
			Spec: v1beta1.InitializerSpec{
				AppTemplate: v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []v1beta1.ApplicationComponent{
							{
								Name: compName2,
								Type: "worker",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"image": "busybox",
									"cmd":   []string{"sleep", "10000"},
								}),
							},
						},
					},
				},
				DependsOn: []v1beta1.DependsOn{
					{
						Ref: corev1.ObjectReference{
							APIVersion: "core.oam.dev/v1beta1",
							Kind:       "Initializer",
							Name:       "env2",
							Namespace:  namespace,
						},
					},
				},
			},
		}

		By("Create Initializer env3 which depends on env2")
		Eventually(func() error {
			return k8sClient.Create(ctx, init2)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the application is created successfully")
		app2 := new(v1beta1.Application)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: init2.Name, Namespace: namespace}, app2)
		}, 60*time.Second, 2*time.Millisecond).Should(Succeed())

		By("Verify the initializer env3 successfully initialized the environment")
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: init2.Name, Namespace: namespace}, init2)
			if err != nil {
				return err
			}
			if init2.Status.Phase != v1beta1.InitializerSuccess {
				return fmt.Errorf("environment was not successfully initialized")
			}
			return nil
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})

	It("Test apply initializer which will create namespace", func() {
		randomNs := randomNamespaceName("initializer-createns")
		init := &v1beta1.Initializer{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Initializer",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env4",
				Namespace: namespace,
			},
			Spec: v1beta1.InitializerSpec{
				AppTemplate: v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []v1beta1.ApplicationComponent{
							{
								Name: randomNs,
								Type: "raw",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "Namespace",
									"metadata": map[string]interface{}{
										"name": randomNs,
									},
								}),
							},
						},
					},
				},
			},
		}
		By("Create Initializer createNamespaceInit")
		Eventually(func() error {
			return k8sClient.Create(ctx, init)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the application is created successfully")
		app := new(v1beta1.Application)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: init.Name, Namespace: namespace}, app)
		}, 60*time.Second, 2*time.Millisecond).Should(Succeed())

		By("Verify the initializer env3 successfully initialized the environment")
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: init.Name, Namespace: namespace}, init)
			if err != nil {
				return err
			}
			if init.Status.Phase != v1beta1.InitializerSuccess {
				return fmt.Errorf("environment was not successfully initialized")
			}
			return nil
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})
})
