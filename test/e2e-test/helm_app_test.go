package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test application containing helm module", func() {
	ctx := context.Background()
	var (
		namespace = "helm-test-ns"
		appName   = "test-app"
		compName  = "test-comp"
		cdName    = "webapp-chart"
		tdName    = "virtualgroup"
	)
	var app v1alpha2.Application

	var ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

	BeforeEach(func() {
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespace,
		}
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, &corev1.Namespace{})
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cd := v1alpha2.ComponentDefinition{}
		cd.SetName(cdName)
		cd.SetNamespace(namespace)
		cd.Spec.Workload.Definition = v1alpha2.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}
		cd.Spec.Schematic = &v1alpha2.Schematic{
			HELM: &v1alpha2.Helm{
				Release: util.Object2RawExtension(map[string]interface{}{
					"chart": map[string]interface{}{
						"spec": map[string]interface{}{
							"chart":   "podinfo",
							"version": "5.1.4",
						},
					},
				}),
				Repository: util.Object2RawExtension(map[string]interface{}{
					"url": "http://oam.dev/catalog/",
				}),
			},
		}

		Expect(k8sClient.Create(ctx, &cd)).Should(Succeed())

		By("Install a patch trait used to test CUE module")
		td := v1alpha2.TraitDefinition{}
		td.SetName(tdName)
		td.SetNamespace(namespace)
		td.Spec.AppliesToWorkloads = []string{"deployments.apps"}
		td.Spec.Schematic = &v1alpha2.Schematic{
			CUE: &v1alpha2.CUE{
				Template: `patch: {
      	spec: template: {
      		metadata: labels: {
      			if parameter.type == "namespace" {
      				"app.namespace.virtual.group": parameter.group
      			}
      			if parameter.type == "cluster" {
      				"app.cluster.virtual.group": parameter.group
      			}
      		}
      	}
      }
      parameter: {
      	group: *"default" | string
      	type:  *"namespace" | string
      }`,
			},
		}
		Expect(k8sClient.Create(ctx, &td)).Should(Succeed())

		By("Add 'deployments.apps' to scaler's appliesToWorkloads")
		scalerTd := v1alpha2.TraitDefinition{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler", Namespace: "vela-system"}, &scalerTd)).Should(Succeed())
		scalerTd.Spec.AppliesToWorkloads = []string{"deployments.apps", "webservice", "worker"}
		scalerTd.SetResourceVersion("")
		Expect(k8sClient.Patch(ctx, &scalerTd, client.Merge)).Should(Succeed())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1alpha2.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1alpha2.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1alpha2.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1alpha2.TraitDefinition{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		time.Sleep(15 * time.Second)

		By("Remove 'deployments.apps' from scaler's appliesToWorkloads")
		scalerTd := v1alpha2.TraitDefinition{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler", Namespace: "vela-system"}, &scalerTd)).Should(Succeed())
		scalerTd.Spec.AppliesToWorkloads = []string{"webservice", "worker"}
		scalerTd.SetResourceVersion("")
		Expect(k8sClient.Patch(ctx, &scalerTd, client.Merge)).Should(Succeed())
	})

	It("Test deploy an application containing helm module", func() {
		app = v1alpha2.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationSpec{
				Components: []v1alpha2.ApplicationComponent{
					{
						Name:         compName,
						WorkloadType: cdName,
						Settings: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]interface{}{
								"tag": "5.1.2",
							},
						}),
						Traits: []v1alpha2.ApplicationTrait{
							{
								Name: "scaler",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"replicas": 2,
								}),
							},
							{
								Name: tdName,
								Properties: util.Object2RawExtension(map[string]interface{}{
									"group": "my-group",
									"type":  "cluster",
								}),
							},
						},
					},
				},
			},
		}
		By("Create application")
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())

		ac := &v1alpha2.ApplicationConfiguration{}
		acName := fmt.Sprintf("%s-v1", appName)
		By("Verify the AppConfig is created successfully")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac)
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Verify the workload(deployment) is created successfully by Helm")
		deploy := &appsv1.Deployment{}
		deployName := fmt.Sprintf("%s-%s-podinfo", appName, compName)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
		}, 60*time.Second, 5*time.Second).Should(Succeed())

		By("Veriify two traits are applied to the workload")
		Eventually(func() bool {
			if err := reconcileAppConfigNow(ctx, ac); err != nil {
				return false
			}
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify patch trait is applied")
			templateLabels := deploy.Spec.Template.Labels
			if templateLabels["app.cluster.virtual.group"] != "my-group" {
				return false
			}
			By("Verify scaler trait is applied")
			if *deploy.Spec.Replicas != 2 {
				return false
			}
			By("Verify application's settings override chart default values")
			// the default value of 'image.tag' is 5.1.4 in the chart, but settings reset it to 5.1.2
			return strings.HasSuffix(deploy.Spec.Template.Spec.Containers[0].Image, "5.1.2")
			// it takes pretty long time to fetch chart and install the Helm release
		}, 120*time.Second, 10*time.Second).Should(BeTrue())

		By("Update the application")
		app = v1alpha2.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationSpec{
				Components: []v1alpha2.ApplicationComponent{
					{
						Name:         compName,
						WorkloadType: cdName,
						Settings: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]interface{}{
								"tag": "5.1.3", // change 5.1.4 => 5.1.3
							},
						}),
						Traits: []v1alpha2.ApplicationTrait{
							{
								Name: "scaler",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"replicas": 3, // change 2 => 3
								}),
							},
							{
								Name: tdName,
								Properties: util.Object2RawExtension(map[string]interface{}{
									"group": "my-group-0", // change my-group => my-group-0
									"type":  "cluster",
								}),
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		By("Verify the appconfig is updated")
		deploy = &appsv1.Deployment{}
		Eventually(func() bool {
			ac = &v1alpha2.ApplicationConfiguration{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: acName, Namespace: namespace}, ac); err != nil {
				return false
			}
			return ac.GetGeneration() == 2
		}, 15*time.Second, 3*time.Second).Should(BeTrue())

		By("Veriify the changes are applied to the workload")
		Eventually(func() bool {
			if err := reconcileAppConfigNow(ctx, ac); err != nil {
				return false
			}
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify new patch trait is applied")
			templateLabels := deploy.Spec.Template.Labels
			if templateLabels["app.cluster.virtual.group"] != "my-group-0" {
				return false
			}
			By("Verify new scaler trait is applied")
			if *deploy.Spec.Replicas != 3 {
				return false
			}
			By("Verify new application's settings override chart default values")
			return strings.HasSuffix(deploy.Spec.Template.Spec.Containers[0].Image, "5.1.3")
		}, 120*time.Second, 10*time.Second).Should(BeTrue())
	})
})
