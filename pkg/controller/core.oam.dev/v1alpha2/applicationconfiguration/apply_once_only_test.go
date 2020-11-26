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

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
		ctx          = context.Background()
		cw           v1alpha2.ContainerizedWorkload
		component    v1alpha2.Component
		fakeTrait    *unstructured.Unstructured
		appConfig    v1alpha2.ApplicationConfiguration
		appConfigKey = client.ObjectKey{
			Name:      appName,
			Namespace: namespace,
		}
		req = reconcile.Request{NamespacedName: appConfigKey}
		ns  = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
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

		logf.Log.Info("Start to run a test, clean up previous resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		logf.Log.Info("make sure all the resources are removed")
		Eventually(
			// gomega has a bug that can't take nil as the actual input, so has to make it a func
			func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, &corev1.Namespace{})
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})

		By("Create namespace")
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

		By("Enable ApplyOnceOnly")
		reconciler.applyOnceOnly = true

		By("Reconcile")
		Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
	})

	AfterEach(func() {
		// restore as default value
		reconciler.applyOnceOnly = false
	})

	When("Change workload/trait instance bypass ApplicationConfiguration", func() {
		It("should keep workload instanced not changed by reconciliation", func() {
			By("Get workload instance & Check workload spec")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: compName, Namespace: namespace}, &cwObj)
			}, 3*time.Second, time.Second).Should(BeNil())
			Expect(cwObj.Spec.Containers[0].Image).Should(Equal(image1))

			By("Get trait instance & Check trait spec")
			fooObj := &unstructured.Unstructured{}
			fooObj.SetAPIVersion("example.com/v1")
			fooObj.SetKind("Foo")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, fooObj)
			}, 3*time.Second, time.Second).Should(BeNil())
			fooObjV, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
			Expect(fooObjV).Should(Equal(traitSpecValue1))

			By("Modify workload spec & Apply changed workload")
			cwObj.Spec.Containers[0].Image = image2
			Expect(k8sClient.Apply(ctx, &cwObj)).Should(Succeed())

			By("Modify trait spec & Apply changed trait")
			unstructured.SetNestedField(fooObj.Object, traitSpecValue2, "spec", "key")
			Expect(k8sClient.Apply(ctx, fooObj)).Should(Succeed())

			By("Get updated workload instance & Check workload spec")
			updateCwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: compName, Namespace: namespace}, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Get updated trait instance & Check trait spec")
			updatedFooObj := &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, updatedFooObj); err != nil {
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
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: compName, Namespace: namespace}, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image2))

			By("Check trait is not changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue2))

			By("Disable ApplyOnceOnly & Reconcile again")
			reconciler.applyOnceOnly = false
			Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())

			By("Check workload is changed by reconciliation")
			updateCwObj = v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: compName, Namespace: namespace}, &updateCwObj); err != nil {
					return ""
				}
				return updateCwObj.Spec.Containers[0].Image
			}, 3*time.Second, time.Second).Should(Equal(image1))

			By("Check trait is changed by reconciliation")
			updatedFooObj = &unstructured.Unstructured{}
			updatedFooObj.SetAPIVersion("example.com/v1")
			updatedFooObj.SetKind("Foo")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, updatedFooObj); err != nil {
					return ""
				}
				v, _, _ := unstructured.NestedString(updatedFooObj.Object, "spec", "key")
				return v
			}, 3*time.Second, time.Second).Should(Equal(traitSpecValue1))
		})
	})

})
