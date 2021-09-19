/*
Copyright 2021 The KubeVela Authors.

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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	workloadScopeFinalizer = "scope.finalizer.core.oam.dev"
)

var _ = PDescribe("Finalizer for HealthScope in ApplicationConfiguration", func() {
	ctx := context.Background()
	namespace := "finalizer-test"
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	var component v1alpha2.Component
	var appConfig v1alpha2.ApplicationConfiguration
	componentName := "example-component"
	appConfigName := "example-appconfig"
	healthScopeName := "example-health-scope"

	BeforeEach(func() {
		logf.Log.Info("Start to run a test, clean up previous resources")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		component = v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &appsv1.Deployment{
						Spec: appsv1.DeploymentSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx",
								},
							},
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "nginx:v3",
											Name:  "nginx",
										},
									},
								},
								ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
							},
						},
					},
				},
			},
		}
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
			},
		}
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		logf.Log.Info("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespace,
		}
		res := &corev1.Namespace{}
		Eventually(
			// gomega has a bug that can't take nil as the actual input, so has to make it a func
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// create Component definition
		By("Create Component definition")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())

	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	When("AppConfig has no scopes", func() {
		It("should not register finalizer", func() {
			appConfig.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
				},
			}
			By("Check component should already existed")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &v1alpha2.Component{})
			}, time.Second*30, time.Microsecond*500).Should(BeNil())

			By("Apply AppConfig")
			Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())

			By("Check appConfig reconciliation finished")
			Eventually(func() bool {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.ObjectMeta.Generation >= 1
			}, time.Second*30, time.Microsecond*500).Should(BeTrue())

			By("Check no finalizer registered")
			Eventually(func() []string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.ObjectMeta.Finalizers
			}, time.Second*30, time.Microsecond*500).ShouldNot(ContainElement(workloadScopeFinalizer))

		})
	})

	When("AppConfig has scopes", func() {
		It("should handle finalizer before being deleted", func() {
			// create health scope definition
			sd := v1alpha2.ScopeDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "healthscopes.core.oam.dev",
					Namespace: "vela-system",
				},
				Spec: v1alpha2.ScopeDefinitionSpec{
					AllowComponentOverlap: true,
					WorkloadRefsPath:      "spec.workloadRefs",
					Reference: common.DefinitionReference{
						Name: "healthscope.core.oam.dev",
					},
				},
			}
			By("Creat health scope definition")
			Expect(k8sClient.Create(ctx, &sd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

			// create health scope.
			hs := v1alpha2.HealthScope{
				ObjectMeta: metav1.ObjectMeta{
					Name:      healthScopeName,
					Namespace: namespace,
				},
				Spec: v1alpha2.HealthScopeSpec{
					WorkloadReferences: []corev1.ObjectReference{},
				},
			}
			By("Creat health scope")
			Expect(k8sClient.Create(ctx, &hs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

			appConfig.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Scopes: []v1alpha2.ComponentScope{
						{
							ScopeReference: corev1.ObjectReference{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "HealthScope",
								Name:       healthScopeName,
							},
						},
					},
				},
			}
			By("Apply AppConfig")
			Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())

			By("Check register finalizer")
			Eventually(func() []string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.ObjectMeta.Finalizers
			}, time.Second*30, time.Microsecond*500).Should(ContainElement(workloadScopeFinalizer))

			By("Check HealthScope WorkloadRefs")
			Eventually(func() int {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthScopeName}, &hs)
				return len(hs.Spec.WorkloadReferences)
			}, time.Second*30, time.Millisecond*500).Should(Equal(1))

			By("Delete AppConfig")
			Expect(k8sClient.Delete(ctx, &appConfig)).Should(Succeed())

			By("Check workload ref has been removed from HealthScope's WorkloadRefs")
			Eventually(func() int {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthScopeName}, &hs)
				return len(hs.Spec.WorkloadReferences)
			}, time.Second*30, time.Millisecond*500).Should(Equal(0))

			By("Check AppConfig has been deleted successfully")
			deletedAppConfig := &v1alpha2.ApplicationConfiguration{}
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, deletedAppConfig)
				},
				time.Second*30, time.Microsecond*500).Should(&util.NotFoundMatcher{})
		})

	})

})
