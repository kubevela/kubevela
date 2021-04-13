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

package applicationconfiguration

import (
	"context"
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test apply changes to trait", func() {
	const (
		namespace        = "update-apply-trait-test"
		appName          = "example-app"
		compName         = "example-comp"
		fakeTraitCRDName = "bars.example.com"
		fakeTraitGroup   = "example.com"
		fakeTraitKind    = "Bar"
	)
	var (
		ctx          = context.Background()
		cw           v1alpha2.ContainerizedWorkload
		component    v1alpha2.Component
		traitDef     v1alpha2.TraitDefinition
		appConfig    v1alpha2.ApplicationConfiguration
		fakeTratiCRD crdv1.CustomResourceDefinition
		appConfigKey = client.ObjectKey{
			Name:      appName,
			Namespace: namespace,
		}
		req = reconcile.Request{NamespacedName: appConfigKey}
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

		By("Create namespace")
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create a CRD used as fake trait")
		fakeTratiCRD = crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fakeTraitCRDName,
				Labels: map[string]string{"crd": namespace},
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: fakeTraitGroup,
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:     fakeTraitKind,
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
		Expect(k8sClient.Create(context.Background(), &fakeTratiCRD)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("Creat TraitDefinition")
		traitDef = v1alpha2.TraitDefinition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "core.oam.dev/v1alpha2",
				APIVersion: "TraitDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bars.example.com",
				Namespace: "vela-system",
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				Reference: common.DefinitionReference{
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

		By("Creat appConfig & check trait is created")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() int64 {
			reconcileRetry(reconciler, req)
			if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
				return 0
			}
			if appConfig.Status.Workloads == nil {
				reconcileRetry(reconciler, req)
				return 0
			}
			var traitObj unstructured.Unstructured
			traitName := appConfig.Status.Workloads[0].Traits[0].Reference.Name
			traitObj.SetAPIVersion("example.com/v1")
			traitObj.SetKind("Bar")
			if err := k8sClient.Get(ctx,
				client.ObjectKey{Namespace: namespace, Name: traitName}, &traitObj); err != nil {
				return 0
			}
			return traitObj.GetGeneration()
		}, 3*time.Second, 500*time.Millisecond).Should(Equal(int64(1)))
	})

	AfterEach(func() {
		Expect(k8sClient.DeleteAllOf(ctx, &appConfig, client.InNamespace(namespace))).Should(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &cw, client.InNamespace(namespace))).Should(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &component, client.InNamespace(namespace))).Should(Succeed())
		var deleteTrait unstructured.Unstructured
		deleteTrait.SetAPIVersion("example.com/v1")
		deleteTrait.SetKind("Bar")
		Expect(k8sClient.DeleteAllOf(ctx, &deleteTrait, client.InNamespace(namespace))).Should(Succeed())
	})

	When("update an ApplicationConfiguration with traits changed", func() {
		It("should apply all changes(add/reset/remove fields) to the trait", func() {
			By("Modify the ApplicationConfiguration")
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
			// remove metadata.labels
			// change a field (valueChanged: bar ==> foo)
			// added a field
			// removed a field
			appConfigUpdated := v1alpha2.ApplicationConfiguration{}
			Expect(yaml.Unmarshal([]byte(appConfigYAMLUpdated), &appConfigUpdated)).Should(BeNil())
			appConfigUpdated.SetNamespace(namespace)

			By("Apply appConfig & check successfully")
			Expect(k8sClient.Patch(ctx, &appConfigUpdated, client.Merge)).Should(Succeed())
			Eventually(func() int64 {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return 0
				}
				return appConfig.GetGeneration()
			}, time.Second, 300*time.Millisecond).Should(Equal(int64(2)))

			By("Reconcile & check updated trait")
			var traitObj unstructured.Unstructured
			Eventually(func() int64 {
				reconcileRetry(reconciler, req)
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return 0
				}
				if appConfig.Status.Workloads == nil {
					reconcileRetry(reconciler, req)
					return 0
				}
				traitName := appConfig.Status.Workloads[0].Traits[0].Reference.Name
				traitObj.SetAPIVersion("example.com/v1")
				traitObj.SetKind("Bar")
				if err := k8sClient.Get(ctx,
					client.ObjectKey{Namespace: namespace, Name: traitName}, &traitObj); err != nil {
					return 0
				}

				// TODO(roywang) 2021/04/13 remove below 'By' if this case no longer breaks.
				v, _, _ := unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "valueChanged")
				By(fmt.Sprintf(`trait field: want "foo", got %q`, v))

				return traitObj.GetGeneration()
			}, 60*time.Second, time.Second).Should(Equal(int64(2)))

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
			By("Check removed field")
			_, found, _ = unstructured.NestedString(traitObj.UnstructuredContent(), "spec", "removed")
			Expect(found).Should(Equal(false))
		})
	})

	// others means anything except AppConfig controller
	// e.g. trait controllers
	When("trait instance is changed by others", func() {
		// if others make changes on the fields managed by AppConfig controller
		// these changes will reverted but not retained.
		It("should retain changes by others, even after AppConfig spec is changed", func() {
			By("Get the trait newly created")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return ""
				}
				if appConfig.Status.Workloads == nil {
					reconcileRetry(reconciler, req)
					return ""
				}
				return appConfig.Status.Workloads[0].Traits[0].Reference.Name
			}, 5*time.Second, time.Second).ShouldNot(BeEmpty())
			traitName := appConfig.Status.Workloads[0].Traits[0].Reference.Name
			var traitObj unstructured.Unstructured
			traitObj.SetAPIVersion("example.com/v1")
			traitObj.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &traitObj)).Should(Succeed())

			By("Others change the trait")
			// add a field
			unstructured.SetNestedField(traitObj.Object, "bar", "spec", "added")
			Expect(k8sClient.Patch(ctx, &traitObj, client.Merge)).Should(Succeed())

			By("Check the change works")
			var changedTrait unstructured.Unstructured
			changedTrait.SetAPIVersion("example.com/v1")
			changedTrait.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &changedTrait)).Should(Succeed())
			v, _, _ := unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "added")
			Expect(v).Should(Equal("bar"))

			By("Reconcile")
			reconcileRetry(reconciler, req)

			By("Check others' change is retained")
			changedTrait = unstructured.Unstructured{}
			changedTrait.SetAPIVersion("example.com/v1")
			changedTrait.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &changedTrait)).Should(Succeed())
			v, _, _ = unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "added")
			Expect(v).Should(Equal("bar"))

			By("Modify AppConfig without touching the field changed by others")
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
              valueChanged: foo`
			appConfigUpdated := v1alpha2.ApplicationConfiguration{}
			Expect(yaml.Unmarshal([]byte(appConfigYAMLUpdated), &appConfigUpdated)).Should(BeNil())
			appConfigUpdated.SetNamespace(namespace)

			By("Apply appConfig & check successfully")
			Expect(k8sClient.Patch(ctx, &appConfigUpdated, client.Merge)).Should(Succeed())
			Eventually(func() int64 {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return 0
				}
				return appConfig.GetGeneration()
			}, time.Second, 300*time.Millisecond).Should(Equal(int64(2)))

			Eventually(func() int64 {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				changedTrait = unstructured.Unstructured{}
				changedTrait.SetAPIVersion("example.com/v1")
				changedTrait.SetKind("Bar")
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &changedTrait); err != nil {
					return 0
				}
				return changedTrait.GetGeneration()
			}, 5*time.Second, time.Second).Should(Equal(int64(3)))

			By("Check AppConfig's change works")
			// changed a field
			v, _, _ = unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "valueChanged")
			Expect(v).Should(Equal("foo"))
			// removed a field
			_, found, _ := unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "removed")
			Expect(found).Should(Equal(false))

			By("Check others' change is still retained")
			v, _, _ = unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "added")
			Expect(v).Should(Equal("bar"))
		})

		It("should override others' changes on fields rendered from AppConfig", func() {
			By("Get the trait newly created")
			Eventually(func() string {
				if err := k8sClient.Get(ctx, appConfigKey, &appConfig); err != nil {
					return ""
				}
				if appConfig.Status.Workloads == nil {
					reconcileRetry(reconciler, req)
					return ""
				}
				return appConfig.Status.Workloads[0].Traits[0].Reference.Name
			}, 5*time.Second, time.Second).ShouldNot(BeEmpty())
			traitName := appConfig.Status.Workloads[0].Traits[0].Reference.Name
			var traitObj unstructured.Unstructured
			traitObj.SetAPIVersion("example.com/v1")
			traitObj.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &traitObj)).Should(Succeed())

			By("Others change the field which should be rendered from AppConfig")
			// unchanged: bar ==> foo
			unstructured.SetNestedField(traitObj.Object, "foo", "spec", "unchanged")
			Expect(k8sClient.Patch(ctx, &traitObj, client.Merge)).Should(Succeed())

			By("Check the change works")
			var changedTrait unstructured.Unstructured
			changedTrait.SetAPIVersion("example.com/v1")
			changedTrait.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &changedTrait)).Should(Succeed())
			v, _, _ := unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "unchanged")
			Expect(v).Should(Equal("foo"))

			By("Reconcile")
			reconcileRetry(reconciler, req)

			By("Check others' change is overrided(reset)")
			changedTrait = unstructured.Unstructured{}
			changedTrait.SetAPIVersion("example.com/v1")
			changedTrait.SetKind("Bar")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitName}, &changedTrait)).Should(Succeed())
			v, _, _ = unstructured.NestedString(changedTrait.UnstructuredContent(), "spec", "unchanged")
			// unchanged: foo ==> bar
			Expect(v).Should(Equal("bar"))
		})
	})
})
