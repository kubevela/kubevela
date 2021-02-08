package applicationconfiguration

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test apply (workloads/traits) once only", func() {
	const (
		namespace       = "apply-once-only-test"
		appName         = "example-app"
		compName        = "example-comp"
		traitName       = "example-trait"
		image1          = "wordpress:latest"
		image2          = "nginx:latest"
		traitSpecValue1 = "test1"
		traitSpecValue2 = "test2"
	)
	var (
		ctx       = context.Background()
		cw        v1alpha2.ContainerizedWorkload
		component v1alpha2.Component
		fakeTrait *unstructured.Unstructured
		appConfig v1alpha2.ApplicationConfiguration
		cwObjKey  = client.ObjectKey{
			Name:      compName,
			Namespace: namespace,
		}
		traitObjKey = client.ObjectKey{
			Name:      traitName,
			Namespace: namespace,
		}
		appConfigKey = client.ObjectKey{
			Name:      appName,
			Namespace: namespace,
		}
		req = reconcile.Request{NamespacedName: appConfigKey}
		ns  corev1.Namespace
	)

	BeforeEach(func() {
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
						Image: image1,
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
				Name:      compName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &cw,
				},
			},
		}

		fakeTrait = &unstructured.Unstructured{}
		fakeTrait.SetAPIVersion("example.com/v1")
		fakeTrait.SetKind("Foo")
		fakeTrait.SetNamespace(namespace)
		fakeTrait.SetName(traitName)
		unstructured.SetNestedField(fakeTrait.Object, traitSpecValue1, "spec", "key")

		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: compName,
						Traits: []v1alpha2.ComponentTrait{
							{Trait: runtime.RawExtension{Object: fakeTrait}},
						},
					},
				},
			},
		}

		By("Create namespace")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)
	})

	AfterEach(func() {
		logf.Log.Info("Clean up previous resources")
		Expect(k8sClient.DeleteAllOf(ctx, &appConfig, client.InNamespace(namespace))).Should(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &cw, client.InNamespace(namespace))).Should(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &component, client.InNamespace(namespace))).Should(Succeed())
		var deleteTrait unstructured.Unstructured
		deleteTrait.SetAPIVersion("example.com/v1")
		deleteTrait.SetKind("Foo")
		Expect(k8sClient.DeleteAllOf(ctx, &deleteTrait, client.InNamespace(namespace))).Should(Succeed())
		// restore as default value
		reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyOff
	})

	When("ApplyOnceOnly is enabled", func() {
		It("should not revert changes of workload/trait made by others", func() {
			By("Enable ApplyOnceOnly")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyOn

			By("Get workload instance & Check workload spec")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				return k8sClient.Get(ctx, cwObjKey, &cwObj)
			}, 5*time.Second, time.Second).Should(BeNil())
			Expect(cwObj.Spec.Containers[0].Image).Should(Equal(image1))

			By("Get trait instance & Check trait spec")
			fooObj := &unstructured.Unstructured{}
			fooObj.SetAPIVersion("example.com/v1")
			fooObj.SetKind("Foo")
			Eventually(func() error {
				return k8sClient.Get(ctx, traitObjKey, fooObj)
			}, 3*time.Second, time.Second).Should(BeNil())
			fooObjV, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
			Expect(fooObjV).Should(Equal(traitSpecValue1))

			By("Modify workload spec & Apply changed workload")
			cwObj.Spec.Containers[0].Image = image2
			Expect(k8sClient.Patch(ctx, &cwObj, client.Merge)).Should(Succeed())

			By("Modify trait spec & Apply changed trait")
			unstructured.SetNestedField(fooObj.Object, traitSpecValue2, "spec", "key")
			Expect(k8sClient.Patch(ctx, fooObj, client.Merge)).Should(Succeed())

			By("Get updated workload instance & Check workload spec")
			updateCwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Get updated trait instance & Check trait spec")
			updatedFooObj := &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue2))

			By("Reconcile")
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
			time.Sleep(3 * time.Second)

			By("Check workload is not changed by reconciliation")
			updateCwObj = v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Check trait is not changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue2))

			By("Disable ApplyOnceOnly & Reconcile again")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyOff
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())

			By("Check workload is changed by reconciliation")
			updateCwObj = v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image1))

			By("Check trait is changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue1))
		})

		It("should re-create workload/trait if it's delete by others", func() {
			By("Enable ApplyOnceOnly")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyOn

			By("Get workload instance & Check workload spec")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				return k8sClient.Get(ctx, cwObjKey, &cwObj)
			}, 5*time.Second, time.Second).Should(BeNil())

			By("Delete the workload")
			Expect(k8sClient.Delete(ctx, &cwObj)).Should(Succeed())
			Expect(k8sClient.Get(ctx, cwObjKey, &v1alpha2.ContainerizedWorkload{})).Should(util.NotFoundMatcher{})

			By("Reconcile")
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
			time.Sleep(3 * time.Second)

			By("Check workload is created by reconciliation")
			recreatedCwObj := v1alpha2.ContainerizedWorkload{}
			Expect(k8sClient.Get(ctx, cwObjKey, &recreatedCwObj)).Should(Succeed())
		})
	})

	When("ApplyOnceOnlyForce is enabled", func() {
		It("should not revert changes of workload/trait made by others", func() {
			By("Enable ApplyOnceOnlyForce")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyForce

			By("Get workload instance & Check workload spec")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				return k8sClient.Get(ctx, cwObjKey, &cwObj)
			}, 5*time.Second, time.Second).Should(BeNil())
			Expect(cwObj.Spec.Containers[0].Image).Should(Equal(image1))

			By("Get trait instance & Check trait spec")
			fooObj := &unstructured.Unstructured{}
			fooObj.SetAPIVersion("example.com/v1")
			fooObj.SetKind("Foo")
			Eventually(func() error {
				return k8sClient.Get(ctx, traitObjKey, fooObj)
			}, 3*time.Second, time.Second).Should(BeNil())
			fooObjV, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
			Expect(fooObjV).Should(Equal(traitSpecValue1))

			By("Modify workload spec & Apply changed workload")
			cwObj.Spec.Containers[0].Image = image2
			Expect(k8sClient.Patch(ctx, &cwObj, client.Merge)).Should(Succeed())

			By("Modify trait spec & Apply changed trait")
			unstructured.SetNestedField(fooObj.Object, traitSpecValue2, "spec", "key")
			Expect(k8sClient.Patch(ctx, fooObj, client.Merge)).Should(Succeed())

			By("Get updated workload instance & Check workload spec")
			updateCwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Get updated trait instance & Check trait spec")
			updatedFooObj := &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue2))

			By("Reconcile")
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
			time.Sleep(3 * time.Second)

			By("Check workload is not changed by reconciliation")
			updateCwObj = v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Check trait is not changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue2))

			By("Disable ApplyOnceOnly & Reconcile again")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyOff
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())

			By("Check workload is changed by reconciliation")
			updateCwObj = v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, cwObjKey, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image1))

			By("Check trait is changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, traitObjKey, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue1))
		})

		It("should not re-create workload/trait if it's delete by others", func() {
			By("Enable ApplyOnceOnlyForce")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyForce

			By("Get workload instance")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				return k8sClient.Get(ctx, cwObjKey, &cwObj)
			}, 5*time.Second, time.Second).Should(BeNil())

			By("Get trait instance & Check trait spec")
			fooObj := unstructured.Unstructured{}
			fooObj.SetAPIVersion("example.com/v1")
			fooObj.SetKind("Foo")
			Eventually(func() error {
				return k8sClient.Get(ctx, traitObjKey, &fooObj)
			}, 3*time.Second, time.Second).Should(BeNil())

			By("Delete the workload")
			Expect(k8sClient.Delete(ctx, &cwObj)).Should(Succeed())
			Expect(k8sClient.Get(ctx, cwObjKey, &v1alpha2.ContainerizedWorkload{})).Should(util.NotFoundMatcher{})

			By("Delete the trait")
			Expect(k8sClient.Delete(ctx, &fooObj)).Should(Succeed())
			Expect(k8sClient.Get(ctx, traitObjKey, &fooObj)).Should(util.NotFoundMatcher{})

			By("Reconcile")
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
			time.Sleep(3 * time.Second)

			By("Check workload is not re-created by reconciliation")
			recreatedCwObj := v1alpha2.ContainerizedWorkload{}
			Expect(k8sClient.Get(ctx, cwObjKey, &recreatedCwObj)).Should(util.NotFoundMatcher{})

			By("Check trait is not re-created by reconciliation")
			recreatedFooObj := unstructured.Unstructured{}
			recreatedFooObj.SetAPIVersion("example.com/v1")
			recreatedFooObj.SetKind("Foo")
			Expect(k8sClient.Get(ctx, traitObjKey, &recreatedFooObj)).Should(util.NotFoundMatcher{})

			By("Update Appconfig to trigger generation augment")
			unstructured.SetNestedField(fakeTrait.Object, "newvalue", "spec", "key")
			appConfig = v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: namespace,
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: compName,
							Traits: []v1alpha2.ComponentTrait{
								{Trait: runtime.RawExtension{Object: fakeTrait}},
							},
						},
					},
				},
			}
			Expect(k8sClient.Patch(ctx, &appConfig, client.Merge)).Should(Succeed())

			By("Check AppConfig is updated successfully")
			updateAC := v1alpha2.ApplicationConfiguration{}
			Eventually(func() int64 {
				if err := k8sClient.Get(ctx, appConfigKey, &updateAC); err != nil {
					return 0
				}
				return updateAC.GetGeneration()
			}, 3*time.Second, time.Second).Should(Equal(int64(2)))

			By("Reconcile")
			reconcileRetry(reconciler, req)
			time.Sleep(3 * time.Second)

			By("Check workload is re-created by reconciliation")
			recreatedCwObj = v1alpha2.ContainerizedWorkload{}
			Expect(k8sClient.Get(ctx, cwObjKey, &recreatedCwObj)).Should(Succeed())

			By("Check trait is re-created by reconciliation")
			recreatedFooObj = unstructured.Unstructured{}
			recreatedFooObj.SetAPIVersion("example.com/v1")
			recreatedFooObj.SetKind("Foo")
			Expect(k8sClient.Get(ctx, traitObjKey, &recreatedFooObj)).Should(Succeed())
		})
	})
})
