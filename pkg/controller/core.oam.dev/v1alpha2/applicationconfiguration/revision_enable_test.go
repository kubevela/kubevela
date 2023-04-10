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
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test ApplicationConfiguration Component Revision Enabled trait", func() {
	const (
		namespace = "revision-enable-test"
		appName   = "revision-test-app"
		compName  = "revision-test-comp"
	)
	var (
		ctx          = context.Background()
		wr           v1.Deployment
		component    v1alpha2.Component
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

	BeforeEach(func() {})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("revision enabled should create workload with revisionName and work upgrade with new revision successfully", func() {

		getDeploy := func(image string) *v1.Deployment {
			return &v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": compName,
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": compName,
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "wordpress",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "wordpress",
									ContainerPort: 80,
								},
							},
						},
						}}},
				},
			}
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
					Object: getDeploy("wordpress:4.6.1-apache"),
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
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("component handler will automatically create controller revision")
		Expect(func() bool {
			_, ok := componentHandler.createControllerRevision(cmpV1, cmpV1)
			return ok
		}()).Should(BeTrue())
		var crList v1.ControllerRevisionList
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 1 {
				return fmt.Errorf("want only 1 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Create an ApplicationConfiguration")
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: compName,
					RevisionName:  compName + "-v1",
					Traits: []v1alpha2.ComponentTrait{
						{
							Trait: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
								"apiVersion": "example.com/v1",
								"kind":       "Foo",
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"trait.oam.dev/type": "rollout-revision",
									},
								},
								"spec": map[string]interface{}{
									"key": "test1",
								},
							}}},
						},
					},
				},
			}},
		}
		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Check workload created successfully")
		Eventually(func() error {
			By("Reconcile")
			testutil.ReconcileRetry(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, 3*time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		testutil.ReconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Reconcile again and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))

		By("Check workload will not update when reconcile again but no appconfig changed")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should should still be 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		By("Check trait was created as expected")
		var tr unstructured.Unstructured
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test1"}))

		By("===================================== Start to Update =========================================")
		cmpV2 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV2)).Should(Succeed())
		cmpV2.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v2"),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV2)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV2, cmpV2); return ok }()).Should(BeTrue())
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 2 {
				return fmt.Errorf("there should be exactly 2 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Update appConfig & check successfully")
		appConfig.Spec.Components[0].RevisionName = compName + "-v2"
		appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Foo",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type": "rollout-revision",
				},
			},
			"spec": map[string]interface{}{
				"key": "test2",
			},
		}}}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Check new revision workload created successfully")
		Eventually(func() error {
			By("Reconcile for new revision")
			testutil.ReconcileRetry(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v2"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:v2"))

		By("Check the old workload is still there with no change")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		testutil.ReconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Once more Reconcile and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))
		By("Check trait was updated as expected")
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test2"}))
	})

})

var _ = Describe("Test Component Revision Enabled with custom component revision hook", func() {
	const (
		namespace = "revision-enable-test2"
		compName  = "revision-test-comp2"
	)
	var (
		ctx       = context.Background()
		component v1alpha2.Component
		ns        = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	)

	BeforeEach(func() {})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("custom component change revision lead to revision difference, it should not loop infinitely create", func() {
		srv := httptest.NewServer(RevisionHandler)
		defer srv.Close()
		customComponentHandler := &ComponentHandler{Client: k8sClient, RevisionLimit: 100, CustomRevisionHookURL: srv.URL}
		getDeploy := func(image string) *v1.Deployment {
			return &v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": compName,
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": compName,
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "wordpress",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "wordpress",
									ContainerPort: 80,
								},
							},
						},
						}}},
				},
			}
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
					Object: getDeploy("wordpress:4.6.1-apache"),
				},
			},
		}

		By("Create namespace")
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())

		By("component handler will automatically create controller revision")
		Expect(func() bool {
			_, ok := customComponentHandler.createControllerRevision(component.DeepCopy(), component.DeepCopy())
			return ok
		}()).Should(BeTrue())

		By("it should not create again for the same generation component")
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())
		Expect(func() bool {
			_, ok := customComponentHandler.createControllerRevision(cmpV1, cmpV1)
			return ok
		}()).Should(BeFalse())

		var crList v1.ControllerRevisionList
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 1 {
				return fmt.Errorf("want only 1 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("===================================== Start to Update =========================================")
		cmpV2 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV2)).Should(Succeed())
		cmpV2.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v2"),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV2)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV2, cmpV2); return ok }()).Should(BeTrue())

		cmpV3 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV3)).Should(Succeed())
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV3, cmpV3); return ok }()).Should(BeFalse())

		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 2 {
				return fmt.Errorf("there should be exactly 2 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())
	})
})

var _ = Describe("Component Revision Enabled with apply once only force", func() {
	const (
		namespace = "revision-and-apply-once-force"
		appName   = "revision-apply-once"
		compName  = "revision-apply-once-comp"
	)
	var (
		ctx          = context.Background()
		wr           v1.Deployment
		component    v1alpha2.Component
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

	BeforeEach(func() {})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("revision enabled should create workload with revisionName and work upgrade with new revision successfully", func() {

		getDeploy := func(image string) *v1.Deployment {
			return &v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": compName,
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": compName,
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "wordpress",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "wordpress",
									ContainerPort: 80,
								},
							},
						},
						}}},
				},
			}
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
					Object: getDeploy("wordpress:4.6.1-apache"),
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
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("component handler will automatically create controller revision")
		Expect(func() bool {
			_, ok := componentHandler.createControllerRevision(cmpV1, cmpV1)
			return ok
		}()).Should(BeTrue())
		var crList v1.ControllerRevisionList
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 1 {
				return fmt.Errorf("want only 1 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Create an ApplicationConfiguration")
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					RevisionName: compName + "-v1",
					Traits: []v1alpha2.ComponentTrait{
						{
							Trait: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
								"apiVersion": "example.com/v1",
								"kind":       "Foo",
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"trait.oam.dev/type": "rollout-revision",
									},
								},
								"spec": map[string]interface{}{
									"key": "test1",
								},
							}}},
						},
					},
				},
			}},
		}
		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Reconcile")
		reconciler.applyOnceOnlyMode = "force"
		testutil.ReconcileRetry(reconciler, req)

		By("Check workload created successfully")
		var workloadKey1 = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
		Eventually(func() error {
			testutil.ReconcileRetry(reconciler, req)
			return k8sClient.Get(ctx, workloadKey1, &wr)
		}, 3*time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Delete the workload")
		Expect(k8sClient.Delete(ctx, &wr)).Should(BeNil())

		By("Check reconcile again and no error will happen")
		testutil.ReconcileRetry(reconciler, req)

		By("Check workload will not created after reconcile because apply once force enabled")
		Expect(k8sClient.Get(ctx, workloadKey1, &wr)).Should(SatisfyAll(util.NotFoundMatcher{}))
		Expect(k8sClient.Get(ctx, appConfigKey, &appConfig)).Should(BeNil())
		By("update the trait of ac")
		appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Foo",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type": "rollout-revision",
				},
			},
			"spec": map[string]interface{}{
				"key": "test2",
			},
		}}}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())

		By("Reconcile and Check appconfig condition should not have error")
		testutil.ReconcileRetry(reconciler, req)
		Eventually(func() string {
			By("Reconcile again and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))
		time.Sleep(time.Second)
		By("Check workload will not created even AC changed because apply once force working")
		Expect(k8sClient.Get(ctx, workloadKey1, &wr)).Should(SatisfyAll(util.NotFoundMatcher{}))
		By("Check the trait was updated as expected")
		var tr unstructured.Unstructured
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test2"}))

		By("===================================== Start to Upgrade revision of component =========================================")
		cmpV2 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV2)).Should(Succeed())
		cmpV2.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v2"),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV2)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV2, cmpV2); return ok }()).Should(BeTrue())
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 2 {
				return fmt.Errorf("there should be exactly 2 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Update appConfig & check successfully")
		appConfig.Spec.Components[0].RevisionName = compName + "-v2"
		appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Foo",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type": "rollout-revision",
				},
			},
			"spec": map[string]interface{}{
				"key": "test3",
			},
		}}}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Reconcile for new revision")
		testutil.ReconcileRetry(reconciler, req)

		By("Check new revision workload created successfully")
		Eventually(func() error {
			testutil.ReconcileRetry(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v2"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:v2"))

		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		testutil.ReconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Once more Reconcile and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))
		By("Check trait was updated as expected")
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test3"}))
		reconciler.applyOnceOnlyMode = "off"
	})

})

var _ = Describe("Component Revision Enabled with workloadName set and apply once only force", func() {
	const (
		namespace         = "revision-and-workload-name-specified"
		appName           = "revision-apply-once2"
		compName          = "revision-apply-once-comp2"
		specifiedNameBase = "specified-name-base"
		specifiedNameV1   = "specified-name-v1"
		specifiedNameV2   = "specified-name-v2"
	)
	var (
		ctx          = context.Background()
		wr           v1.Deployment
		component    v1alpha2.Component
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

	BeforeEach(func() {})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("revision enabled should create workload with specified name protect delete with replicas larger than 0", func() {

		getDeploy := func(image, name string) *v1.Deployment {
			return &v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": compName,
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": compName,
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "wordpress",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "wordpress",
									ContainerPort: 80,
								},
							},
						},
						}}},
				},
			}
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
					Object: getDeploy("wordpress:4.6.1-apache", specifiedNameBase),
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
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("component handler will automatically create controller revision")
		Expect(func() bool {
			_, ok := componentHandler.createControllerRevision(cmpV1, cmpV1)
			return ok
		}()).Should(BeTrue())
		var crList v1.ControllerRevisionList
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 1 {
				return fmt.Errorf("want only 1 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Create an ApplicationConfiguration")
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					RevisionName: compName + "-v1",
				},
			}},
		}

		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Reconcile")
		reconciler.applyOnceOnlyMode = "force"
		testutil.ReconcileRetry(reconciler, req)

		By("Check workload created successfully")
		var workloadKey1 = client.ObjectKey{Namespace: namespace, Name: specifiedNameBase}
		Eventually(func() error {
			testutil.ReconcileRetry(reconciler, req)
			return k8sClient.Get(ctx, workloadKey1, &wr)
		}, 3*time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		testutil.ReconcileRetry(reconciler, req)

		Expect(k8sClient.Get(ctx, appConfigKey, &appConfig)).Should(BeNil())

		By("===================================== Start to Upgrade revision of component =========================================")
		cmpV2 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV2)).Should(Succeed())
		cmpV2.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v2", specifiedNameV1),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV2)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV2, cmpV2); return ok }()).Should(BeTrue())
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 2 {
				return fmt.Errorf("there should be exactly 2 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Update appConfig & check successfully")
		appConfig.Spec.Components[0].RevisionName = compName + "-v2"

		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Reconcile for new revision")
		testutil.ReconcileRetry(reconciler, req)

		By("Check new revision workload created successfully")
		Eventually(func() error {
			testutil.ReconcileRetryAndExpectErr(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: specifiedNameV1}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:v2"))

		By("Check appconfig condition should have error")
		Eventually(func() string {
			testutil.ReconcileRetryAndExpectErr(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			By(fmt.Sprintf("Reconcile with condition %v", appConfig.Status.Conditions[0]))
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileError"))

		By("Check the old workload still there")
		Eventually(func() error {
			testutil.ReconcileRetryAndExpectErr(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: specifiedNameBase}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the old workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:4.6.1-apache"))

		wr.Spec.Replicas = pointer.Int32(0)
		Expect(k8sClient.Update(ctx, &wr)).Should(Succeed())

		By("Reconcile Again and appconfig condition should not have error")
		Eventually(func() string {
			By("Once more Reconcile and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: specifiedNameBase}, &wr)).Should(SatisfyAny(util.NotFoundMatcher{}))

		By("===================================== Start to Upgrade revision of component again =========================================")
		cmpV3 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV3)).Should(Succeed())
		cmpV3.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v3", specifiedNameV2),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV3)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(func() bool { _, ok := componentHandler.createControllerRevision(cmpV3, cmpV3); return ok }()).Should(BeTrue())
		By("Update the AC and add the revisionEnabled Trait")
		appConfig.Spec.Components[0].RevisionName = compName + "-v3"
		appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{
			{Trait: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "example.com/v1",
				"kind":       "Foo",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"trait.oam.dev/type": "rollout-revision",
					},
				},
				"spec": map[string]interface{}{
					"key": "test3",
				},
			}}}},
		}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(len(appConfig.Spec.Components[0].Traits)).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen, revisionEnabled will skip delete")
		testutil.ReconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Once more Reconcile and should not have error")
			testutil.ReconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))

		By("Check new revision workload created successfully")
		Eventually(func() error {
			testutil.ReconcileRetry(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: specifiedNameV2}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:v3"))
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		By("Check the old workload still there")
		Eventually(func() error {
			testutil.ReconcileRetry(reconciler, req)
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: specifiedNameV1}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		reconciler.applyOnceOnlyMode = "off"
	})

})
