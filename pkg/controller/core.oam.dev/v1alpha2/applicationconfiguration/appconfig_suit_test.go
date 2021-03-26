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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("CRD without definition can run in an ApplicationConfiguration", func() {
	ctx := context.Background()
	It("create an application without CRD", func() {

		By("Creating CRD foo.crdtest1.com")
		// Create a crd for appconfig dependency test
		crd = crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foo.crdtest1.com",
				Labels: map[string]string{"crd": "dependency"},
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: "crdtest1.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:     "Foo",
					ListKind: "FooList",
					Plural:   "foo",
					Singular: "foo",
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
										"key": {Type: "string"},
									}}}}}},
				},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Creating namespace trait-no-def-test")
		namespace := "trait-no-def-test"
		var ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("creating component using workload by foo.crdtest1.com without definition")
		tempFoo := &unstructured.Unstructured{}
		tempFoo.SetAPIVersion("crdtest1.com/v1")
		tempFoo.SetKind("Foo")
		tempFoo.SetNamespace(namespace)
		// Define a workload
		wl := tempFoo.DeepCopy()
		// Set Name so we can get easily
		wlname := "test-workload"
		wl.SetName(wlname)
		// Create a component
		componentName := "component"
		comp := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wl,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())

		By("Create application configuration with trait using foo.crdtest1.com without definition")
		tr := tempFoo.DeepCopy()
		// Set Name so we can get easily
		trname := "test-trait"
		tr.SetName(trname)
		appConfigName := "appconfig-trait-no-def"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentName,
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: tr,
						}}},
				}}},
		}
		By("Creating application config")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())

		By("Reconcile")
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())

		By("Checking that workload should be created")
		workloadKey := client.ObjectKey{
			Name:      wlname,
			Namespace: namespace,
		}
		workloadFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, workloadKey, workloadFoo)
			if err != nil {
				// Try 3 (= 1s/300ms) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that trait should be created")
		traitKey := client.ObjectKey{
			Name:      trname,
			Namespace: namespace,
		}
		traitFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, traitKey, traitFoo)
			if err != nil {
				// Try 3 (= 1s/300ms) times
				reconciler.Reconcile(req)
			}
			return err
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Checking the application status has right warning message")
		Expect(func() string {
			err := k8sClient.Get(ctx, appconfigKey, &appConfig)
			if err != nil {
				return ""
			}
			return appConfig.Status.Workloads[0].Traits[0].Message
		}()).Should(Equal(util.DummyTraitMessage))
	})

})
