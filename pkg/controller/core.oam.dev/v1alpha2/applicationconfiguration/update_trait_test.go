package applicationconfiguration

import (
	"context"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Test Updating-apply trait in an ApplicationConfiguration", func() {
	const (
		namespace        = "update-apply-trait-test"
		appName          = "example-app"
		compName         = "example-comp"
		fakeTraitCRDName = "bars.example.com"
		fakeTraitGroup   = "example.com"
		fakeTraiitKind   = "Bar"
	)
	var (
		ctx          = context.Background()
		cw           v1alpha2.ContainerizedWorkload
		component    v1alpha2.Component
		traitDef     v1alpha2.TraitDefinition
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
				Name:      compName,
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
				Name:      appName,
				Namespace: namespace,
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

		By("Create a CRD used as fake trait")
		crd = crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fakeTraitCRDName,
				Labels: map[string]string{"crd": namespace},
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: fakeTraitGroup,
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:     fakeTraiitKind,
					ListKind: "BarList",
					Plural:   "bars",
					Singular: "bar",
				},
				Versions: []crdv1.CustomResourceDefinitionVersion{{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]crdv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]crdv1.JSONSchemaProps{
										"unchanged":    {Type: "string"},
										"removed":      {Type: "string"},
										"valueChanged": {Type: "string"},
										"added":        {Type: "string"},
									}}}}}},
				},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("Creat TraitDefinition")
		traitDef = v1alpha2.TraitDefinition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "core.oam.dev/v1alpha2",
				APIVersion: "TraitDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "bars.example.com",
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: fakeTraitCRDName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &traitDef)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create an ApplicationConfiguration")
		appConfigYAML := ` 
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-app
spec:
  components:
    - componentName: example-comp
      traits:
        - trait:
            apiVersion: example.com/v1
            kind: Bar
            metadata:
              labels:
                test.label: test
            spec:
              unchanged: bar
              removed: bar
              valueChanged: bar`
		Expect(yaml.Unmarshal([]byte(appConfigYAML), &appConfig)).Should(BeNil())
		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)
	})

	AfterEach(func() {
		logf.Log.Info("delete all cluster-scoped resources")
		By("Delete CRD used as fake trait")
		Expect(k8sClient.Delete(context.Background(), &crd)).Should(BeNil())
		By("Delete fake TraitDefinition ")
		Expect(k8sClient.Delete(context.Background(), &traitDef)).Should(BeNil())
	})

	When("update an ApplicationConfiguration with Traits changed", func() {
		It("should updating-apply the changed trait", func() {
			By("Update the ApplicationConfiguration")
			appConfigYAMLUpdated := `
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-app
spec:
  components:
    - componentName: example-comp
      traits:
        - trait:
            apiVersion: example.com/v1
            kind: Bar
            spec:
              unchanged: bar
              valueChanged: foo
              added: bar`
			appConfigUpdated := v1alpha2.ApplicationConfiguration{}
			Expect(yaml.Unmarshal([]byte(appConfigYAMLUpdated), &appConfigUpdated)).Should(BeNil())
			appConfigUpdated.SetNamespace(namespace)

			By("Apply appConfig & check successfully")
			Expect(k8sClient.Apply(ctx, &appConfigUpdated)).Should(Succeed())
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return false
				}
				return appConfig.GetGeneration() == 2
			}, time.Second, 300*time.Millisecond).Should(BeTrue())

			By("Reconcile")
			reconcileRetry(reconciler, req)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return ""
				}
				if appConfig.Status.Workloads == nil {
					reconcileRetry(reconciler, req)
					return ""
				}
				return appConfig.Status.Workloads[0].Traits[0].Reference.Name
			}, 3*time.Second, time.Second).ShouldNot(BeEmpty())

			By("Get updated trait object")
			traitName := appConfig.Status.Workloads[0].Traits[0].Reference.Name
			var traitObj unstructured.Unstructured
			traitObj.SetAPIVersion("example.com/v1")
			traitObj.SetKind("Bar")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx,
					client.ObjectKey{Namespace: namespace, Name: traitName}, &traitObj); err != nil {
					return false
				}
				return traitObj.GetGeneration() == 2
			}, 3*time.Second, time.Second).Should(BeTrue())

			By("Check labels are removed")
			_, found, _ := unstructured.NestedString(traitObj.UnstructuredContent(), "metadata", "labels", "test.label")
			Expect(found).Should(Equal(false))
			By("Check unchanged field")
			v, _, _ := unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "unchanged")
			Expect(v).Should(Equal("bar"))
			By("Check changed field")
			v, _, _ = unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "valueChanged")
			Expect(v).Should(Equal("foo"))
			By("Check added field")
			v, _, _ = unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "added")
			Expect(v).Should(Equal("bar"))
			// if use patching trait, this field will not be removed
			By("Check removed field")
			_, found, _ = unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "removed")
			Expect(found).Should(Equal(false))
		})
	})

})
