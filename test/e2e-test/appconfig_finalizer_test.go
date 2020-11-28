package controllers_test

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	workloadScopeFinalizer = "scope.finalizer.core.oam.dev"
)

var _ = Describe("Finalizer for HealthScope in ApplicationConfiguration", func() {
	ctx := context.Background()
	namespace := "finalizer-test"
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	var cw v1alpha2.ContainerizedWorkload
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
		cw = v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: "wordpress:4.6.1-apache",
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
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
					Object: &cw,
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
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
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
					Name: "healthscopes.core.oam.dev",
				},
				Spec: v1alpha2.ScopeDefinitionSpec{
					AllowComponentOverlap: true,
					WorkloadRefsPath:      "spec.workloadRefs",
					Reference: v1alpha2.DefinitionReference{
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
					WorkloadReferences: []v1alpha1.TypedReference{},
				},
			}
			By("Creat health scope")
			Expect(k8sClient.Create(ctx, &hs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

			appConfig.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Scopes: []v1alpha2.ComponentScope{
						{
							ScopeReference: v1alpha1.TypedReference{
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
