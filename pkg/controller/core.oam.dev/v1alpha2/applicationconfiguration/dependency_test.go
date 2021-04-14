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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Resource Dependency in an ApplicationConfiguration", func() {
	ctx := context.Background()
	namespace := "appconfig-dependency-test"
	var ns corev1.Namespace
	tempFoo := &unstructured.Unstructured{}
	tempFoo.SetAPIVersion("example.com/v1")
	tempFoo.SetKind("Foo")
	tempFoo.SetNamespace(namespace)
	outName := "data-output"
	out := tempFoo.DeepCopy()
	out.SetName(outName)
	inName := "data-input"
	in := tempFoo.DeepCopy()
	in.SetName(inName)
	componentOutName := "component-out"
	componentInName := "component-in"
	ref := &unstructured.Unstructured{}
	ref.SetAPIVersion("v1")
	ref.SetKind("ConfigMap")
	ref.SetNamespace(namespace)
	refConfigName := "ref-configmap"
	refConfig := ref.DeepCopy()
	refConfig.SetName(refConfigName)

	test := "test"
	testNew := "test-new"
	hashV1 := "hash-v1"
	hashV2 := "hash-v2"
	store := &unstructured.Unstructured{}
	store.SetAPIVersion("v1")
	store.SetKind("ConfigMap")
	store.SetNamespace(namespace)
	store.SetName("test-configmap")
	typeJP := "jsonPatch"

	BeforeEach(func() {
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

		By(" Create two components, one for data output and one for data input")
		compOut := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentOutName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: out,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compOut)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		compIn := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentInName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: in,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compIn)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		ac := &v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.DeleteAllOf(ctx, ac, client.InNamespace(namespace))).Should(Succeed())
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.DeleteAllOf(ctx, cm, client.InNamespace(namespace))).Should(Succeed())
		foo := &unstructured.Unstructured{}
		foo.SetAPIVersion("example.com/v1")
		foo.SetKind("Foo")
		Expect(k8sClient.DeleteAllOf(ctx, foo, client.InNamespace(namespace))).Should(Succeed())
		Eventually(func() bool {
			l := &unstructured.UnstructuredList{}
			l.SetAPIVersion("example.com/v1")
			l.SetKind("Foo")
			if err := k8sClient.List(ctx, l, client.InNamespace(namespace)); err != nil {
				return false
			}
			return len(l.Items) == 0
		}, 3*time.Second, time.Second).Should(BeTrue())
	})

	// common function for verification
	verify := func(appConfigName, reason string) {
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}

		// Verification before satisfying dependency
		By("Checking that resource which accepts data isn't created yet")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		By(fmt.Sprintf("Checking on resource that inputs data Key: %s", inFooKey))
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(&util.NotFoundMatcher{})

		By("Reconcile")
		req := reconcile.Request{NamespacedName: appconfigKey}
		reconcileRetry(reconciler, req)

		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		By(fmt.Sprintf("Checking that resource which provides(output) data is created, Key %s", outFoo))
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Verify the appconfig's dependency is unsatisfied, waiting for the outside controller to satisfy the requirement")
		appconfig := &v1alpha2.ApplicationConfiguration{}
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{{
				Reason: reason,
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       inName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPaths: []string{
						"spec.key",
					}}}}}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Expect(func() v1alpha2.DependencyStatus {
			k8sClient.Get(ctx, appconfigKey, appconfig)
			return appconfig.Status.Dependency
		}()).Should(Equal(depStatus))

		// fill value to fieldPath
		Expect(k8sClient.Get(ctx, outFooKey, outFoo)).Should(Succeed())
		Expect(unstructured.SetNestedField(outFoo.Object, test, "status", "key")).Should(BeNil())
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() string {
			k8sClient.Get(ctx, outFooKey, outFoo)
			data, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return data
		}, 3*time.Second, time.Second).Should(Equal(test))

		By("Reconcile")
		reconcileRetry(reconciler, req)

		// Verification after satisfying dependency
		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(Succeed())

		By("Verify the appconfig's dependency is satisfied")
		appconfig = &v1alpha2.ApplicationConfiguration{}
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			reconciler.Reconcile(req)
			k8sClient.Get(ctx, appconfigKey, appconfig)
			return appconfig.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())
	}

	It("trait depends on another trait", func() {
		label := map[string]string{"trait": "trait"}
		// Define a workload
		wl := tempFoo.DeepCopy()
		wl.SetName("workload")
		// Create a component
		componentName := "component"
		comp := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wl,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		// Create application configuration
		appConfigName := "appconfig-trait-trait"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentName,
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "trait-trait",
							FieldPath: "status.key",
						}}}, {
						Trait: runtime.RawExtension{
							Object: in,
						},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom: v1alpha2.DataInputValueFrom{
								DataOutputName: "trait-trait",
							},
							ToFieldPaths: []string{"spec.key"},
						}},
					}},
				}}},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on another component", func() {
		label := map[string]string{"component": "component"}
		// Create application configuration
		appConfigName := "appconfig-comp-comp"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentOutName,
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "comp-comp",
						FieldPath: "status.key",
					}}}, {
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "comp-comp",
						},
						ToFieldPaths: []string{"spec.key"},
					}}}}},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on trait", func() {
		label := map[string]string{"trait": "component"}
		// Create application configuration
		appConfigName := "appconfig-trait-comp"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "trait-comp",
						},
						ToFieldPaths: []string{"spec.key"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "trait-comp",
							FieldPath: "status.key",
						}}}}}}},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, "status.key not found in object")
	})

	It("trait depends on component", func() {
		label := map[string]string{"component": "trait"}
		// Create application configuration
		appConfigName := "appconfig-comp-trait"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentOutName,
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "comp-trait",
						FieldPath: "status.key",
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: in,
						},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom: v1alpha2.DataInputValueFrom{
								DataOutputName: "comp-trait",
							},
							ToFieldPaths: []string{"spec.key"},
						}}}}}}},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on trait with updated condition", func() {
		label := map[string]string{"trait": "component", "app-hash": hashV1}
		// Create application configuration
		appConfigName := "appconfig-trait-comp-update"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "trait-comp",
						},
						ToFieldPaths: []string{"spec.key"}}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "trait-comp",
							FieldPath: "status.key",
							Conditions: []v1alpha2.ConditionRequirement{{
								Operator:  v1alpha2.ConditionEqual,
								ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
								FieldPath: "status.app-hash",
							}}}}}}}}},
		}
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		By("Create appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &appConfig)
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Checking that resource which accepts data isn't created yet")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(&util.NotFoundMatcher{})

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Checking that resource which provides data is created")
		// Originally the trait has value in `status.key`, but the hash label is old
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		err := unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		// Verification after satisfying dependency
		By("Verify the appconfig's dependency is satisfied")
		newAppConfig := &v1alpha2.ApplicationConfiguration{}

		Eventually(func() []v1alpha2.UnstaifiedDependency {
			var tempApp = &v1alpha2.ApplicationConfiguration{}
			err = k8sClient.Get(ctx, appconfigKey, tempApp)
			tempApp.DeepCopyInto(newAppConfig)
			if err != nil || tempApp.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempApp.Status.Dependency.Unsatisfied
		}(), 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(BeNil())

		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == test
		}, 3*time.Second, time.Second).Should(BeTrue())

		newAppConfig.Labels["app-hash"] = hashV2
		By("Update newAppConfig & check successfully")
		Expect(k8sClient.Update(ctx, newAppConfig)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, appconfigKey, newAppConfig); err != nil {
				logf.Log.Error(err, "failed get AppConfig")
				return false
			}
			return newAppConfig.Labels["app-hash"] == hashV2
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency should be unsatisfied, because requirementCondition valueFrom not match")
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{{
				Reason: "got(hash-v1) expected to be hash-v2",
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       inName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPaths: []string{
						"spec.key",
					}}}},
		}
		Eventually(func() v1alpha2.DependencyStatus {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			k8sClient.Get(ctx, appconfigKey, newAppConfig)
			return newAppConfig.Status.Dependency
		}, 3*time.Second, time.Second).Should(Equal(depStatus))

		By("Update trait resource to meet the requirement")
		Expect(k8sClient.Get(ctx, outFooKey, outFoo)).Should(BeNil()) // Get the latest before update
		Expect(unstructured.SetNestedField(outFoo.Object, testNew, "status", "key")).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, hashV2, "status", "app-hash")).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == testNew
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Verify the appconfig's dependency is satisfied")
		Eventually(
			func() []v1alpha2.UnstaifiedDependency {
				tempAppConfig := &v1alpha2.ApplicationConfiguration{}
				err := k8sClient.Get(ctx, appconfigKey, tempAppConfig)
				if err != nil || tempAppConfig.Status.Dependency.Unsatisfied != nil {
					// Try 3 (= 3s/1s) times
					reconciler.Reconcile(req)
				}
				return tempAppConfig.Status.Dependency.Unsatisfied
			}(), 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which accepts data is updated")
		Expect(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			return outdata
		}()).Should(Equal(testNew))
	})

	It("component depends on trait with complex merge keys", func() {
		compName := "complex-comp-in"
		infoo := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"complex1": []interface{}{
						map[string]interface{}{
							"name":  "key1",
							"value": "value0",
						},
						map[string]interface{}{
							"name":  "key2",
							"value": "value2",
						},
					},
					"complex2": []interface{}{
						map[string]interface{}{
							"configMapRef": map[string]interface{}{
								"name": "mymap",
							},
						},
						map[string]interface{}{
							"secretRef": map[string]interface{}{
								"name": "mysec1",
							},
						},
					},
				},
			},
		}
		infoo.SetAPIVersion("example.com/v1")
		infoo.SetKind("Foo")
		infoo.SetNamespace(namespace)
		infoo.SetName(compName)

		compIn := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      compName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: infoo,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compIn)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		label := map[string]string{"trait": "component"}
		// Create application configuration
		appConfigName := "appconfig-trait-comp-complex"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: compName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "o1",
						},
						ToFieldPaths:      []string{"spec.complex1"},
						StrategyMergeKeys: []string{"name"},
					}, {
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "o2",
						},
						ToFieldPaths:      []string{"spec.complex2"},
						StrategyMergeKeys: []string{"configMapRef.name", "secretRef.name"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "o1",
							FieldPath: "status.complex1",
						}, {
							Name:      "o2",
							FieldPath: "status.complex2",
						}}}}}}},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())

		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &v1alpha2.ApplicationConfiguration{})
		}, 3*time.Second, time.Second).Should(Succeed())
		By("Reconcile")
		req := reconcile.Request{NamespacedName: appconfigKey}
		reconcileRetry(reconciler, req)

		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		By(fmt.Sprintf("Checking that resource which provides(output) data is created, Key %s", outFoo))
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Get reconciled AppConfig the first time")
		appconfig := &v1alpha2.ApplicationConfiguration{}
		logf.Log.Info("Get appconfig", "Key", appconfigKey)
		Expect(k8sClient.Get(ctx, appconfigKey, appconfig)).Should(BeNil())

		By("Update output object")
		// fill value to fieldPath
		Expect(unstructured.SetNestedField(outFoo.Object, []interface{}{map[string]interface{}{
			"name":  "key1",
			"value": "value1",
		}}, "status", "complex1")).Should(BeNil())

		complex2 := []interface{}{map[string]interface{}{
			"configMapRef": map[string]interface{}{
				"name":  "mymap",
				"value": "myvalue",
			}}, map[string]interface{}{
			"secretRef": map[string]interface{}{
				"name": "mysec2",
			}}}
		Expect(unstructured.SetNestedField(outFoo.Object, complex2, "status", "complex2")).Should(BeNil())
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() []interface{} {
			k8sClient.Get(ctx, outFooKey, outFoo)
			data, _, _ := unstructured.NestedSlice(outFoo.Object, "status", "complex2")
			return data
		}, 3*time.Second, time.Second).Should(BeEquivalentTo(complex2))

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Verify the appconfig's dependency is satisfied")
		appconfig = &v1alpha2.ApplicationConfiguration{}
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			reconciler.Reconcile(req)
			k8sClient.Get(ctx, appconfigKey, appconfig)
			return appconfig.Status.Dependency.Unsatisfied
		}, 2*3*time.Second, time.Second).Should(BeNil())
		// Verification after satisfying dependency
		By("Checking that resource which accepts data is created now")
		inFooKey := client.ObjectKey{
			Name:      compName,
			Namespace: namespace,
		}

		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, infoo)).Should(Succeed())
		Expect(infoo.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{

			"complex1": []interface{}{
				map[string]interface{}{
					"name":  "key1",
					"value": "value1",
				},
				map[string]interface{}{
					"name":  "key2",
					"value": "value2",
				},
			},
			"complex2": []interface{}{
				map[string]interface{}{
					"configMapRef": map[string]interface{}{
						"name":  "mymap",
						"value": "myvalue",
					},
				},
				map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": "mysec1",
					},
				},
				map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": "mysec2",
					},
				},
			},
		}))
	})

	// With both data dependency and data passing, to check the compatibility
	It("data passing with datainput in component and dataoutput in trait", func() {
		label := map[string]string{"trait": "component", "app-hash": hashV1}
		// Create application configuration
		appConfigName := "appconfig-trait-comp-update"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "trait-comp",
						},
						ToFieldPaths: []string{"spec.key"},
						InputStore: v1alpha2.StoreReference{
							TypedReference: v1alpha1.TypedReference{
								APIVersion: store.GetAPIVersion(),
								Name:       store.GetName(),
								Kind:       store.GetKind(),
							},
							Operations: []v1alpha2.DataOperation{{
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec",
								Value:       "{}",
							}, {
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec.key",
								ValueFrom: v1alpha2.ValueFrom{
									FieldPath: "data.app-hash",
								},
							}},
						}}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "trait-comp",
							FieldPath: "status.key",
							OutputStore: v1alpha2.StoreReference{
								TypedReference: v1alpha1.TypedReference{
									APIVersion: store.GetAPIVersion(),
									Name:       store.GetName(),
									Kind:       store.GetKind(),
								},
								Operations: []v1alpha2.DataOperation{{
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data",
									Value:       "{}",
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									ValueFrom: v1alpha2.ValueFrom{
										FieldPath: "status.key",
									},
									Conditions: []v1alpha2.ConditionRequirement{{
										Operator:  v1alpha2.ConditionEqual,
										ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
										FieldPath: "status.app-hash",
									}},
								}},
							},
							Conditions: []v1alpha2.ConditionRequirement{{
								Operator:  v1alpha2.ConditionEqual,
								ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
								FieldPath: "status.app-hash",
							}}}}}}}}},
		}
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		By("Create appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &appConfig)
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		// Check if outputStore has been created
		By("Checking outputstore in dataoutput with unready conditions")
		storeKey := client.ObjectKey{
			Name:      store.GetName(),
			Namespace: namespace,
		}
		storeObj := store.DeepCopy()
		logf.Log.Info("Checking on outputstore ", "Key", storeKey)
		Expect(k8sClient.Get(ctx, storeKey, storeObj)).Should(&util.NotFoundMatcher{})

		By("Checking resource with datainputs, which contains not ready contidions")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(&util.NotFoundMatcher{})

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Checking that resource which provides data is created")
		// Originally the trait has value in `status.key`, but the hash label is old
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		// Change the object in traits to statify the conditions
		err := unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())

		// Verification after satisfying dependency
		By("Verify the appconfig's dependency is satisfied")
		newAppConfig := &v1alpha2.ApplicationConfiguration{}
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			var tempApp = &v1alpha2.ApplicationConfiguration{}
			err = k8sClient.Get(ctx, appconfigKey, tempApp)
			tempApp.DeepCopyInto(newAppConfig)
			if err != nil || tempApp.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempApp.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that the store is created now")
		logf.Log.Info("Checking the store", "Key", storeKey)
		Expect(k8sClient.Get(ctx, storeKey, storeObj)).Should(BeNil())
		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(BeNil())

		By("Checking that filepath value of input data resource")
		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != test {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(test))

		err = unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == test
		}, 3*time.Second, time.Second).Should(BeTrue())

		newAppConfig.Labels["app-hash"] = hashV2
		By("Update newAppConfig & check successfully")
		Expect(k8sClient.Update(ctx, newAppConfig)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, appconfigKey, newAppConfig); err != nil {
				logf.Log.Error(err, "failed get AppConfig")
				return false
			}
			return newAppConfig.Labels["app-hash"] == hashV2
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency should be unsatisfied, because requirementCondition valueFrom not match")
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{{
				Reason: "got(hash-v1) expected to be hash-v2",
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       inName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPaths: []string{
						"spec.key",
					},
				}}, {
				Reason: "got(hash-v1) expected to be hash-v2",
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: store.GetAPIVersion(),
						Name:       store.GetName(),
						Kind:       store.GetKind(),
					},
					FieldPaths: []string{
						"data.app-hash",
					}}}},
		}
		Eventually(func() v1alpha2.DependencyStatus {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			k8sClient.Get(ctx, appconfigKey, newAppConfig)
			return newAppConfig.Status.Dependency
		}, 3*time.Second, time.Second).Should(Equal(depStatus))

		By("Update trait resource to meet the requirement")
		Expect(k8sClient.Get(ctx, outFooKey, outFoo)).Should(BeNil()) // Get the latest before update
		Expect(unstructured.SetNestedField(outFoo.Object, testNew, "status", "key")).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, hashV2, "status", "app-hash")).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == testNew
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency is satisfied")
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			tempAppConfig := &v1alpha2.ApplicationConfiguration{}
			err := k8sClient.Get(ctx, appconfigKey, tempAppConfig)
			if err != nil || tempAppConfig.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempAppConfig.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which accepts data is updated")

		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != testNew {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(testNew))
	})
	//  With only the data passing
	It("data passing with only OutputStore and InputStore", func() {
		label := map[string]string{"trait": "component", "app-hash": hashV1}
		// Create application configuration
		appConfigName := "appconfig-trait-comp-update"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						InputStore: v1alpha2.StoreReference{
							TypedReference: v1alpha1.TypedReference{
								APIVersion: store.GetAPIVersion(),
								Name:       store.GetName(),
								Kind:       store.GetKind(),
							},
							Operations: []v1alpha2.DataOperation{{
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec",
								Value:       "{}",
							}, {
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec.key",
								ValueFrom: v1alpha2.ValueFrom{
									FieldPath: "data.app-hash",
								},
								Conditions: []v1alpha2.ConditionRequirement{{
									Operator:  v1alpha2.ConditionNotEqual,
									Value:     "",
									FieldPath: "data.app-hash",
								}},
							}},
						}}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							OutputStore: v1alpha2.StoreReference{
								TypedReference: v1alpha1.TypedReference{
									APIVersion: store.GetAPIVersion(),
									Name:       store.GetName(),
									Kind:       store.GetKind(),
								},
								Operations: []v1alpha2.DataOperation{{
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data",
									Value:       "{}",
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									Value:       `{}`,
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									ToDataPath:  "sub-path",
									ValueFrom: v1alpha2.ValueFrom{
										FieldPath: "status.key",
									},
									Conditions: []v1alpha2.ConditionRequirement{{
										Operator:  v1alpha2.ConditionNotEqual,
										Value:     "",
										FieldPath: "status.app-hash",
									}, {
										Operator:  v1alpha2.ConditionEqual,
										ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
										FieldPath: "status.app-hash",
									}},
								}}}}}}}}}},
		}
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		By("Create appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &appConfig)
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Checking outputstore in dataoutput with unready conditions")
		storeKey := client.ObjectKey{
			Name:      store.GetName(),
			Namespace: namespace,
		}
		storeObj := store.DeepCopy()
		logf.Log.Info("Checking on outputstore ", "Key", storeKey)
		Expect(k8sClient.Get(ctx, storeKey, storeObj)).Should(&util.NotFoundMatcher{})

		By("Checking resource with datainputs, which contains not ready contidions")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(&util.NotFoundMatcher{})

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Checking that resource which provides data is created")
		// Originally the trait has value in `status.key`, but the hash label is old
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())

		err := unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())

		// Verification after satisfying dependency
		By("Verify the appconfig's dependency is satisfied")
		newAppConfig := &v1alpha2.ApplicationConfiguration{}

		Eventually(func() []v1alpha2.UnstaifiedDependency {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			var tempApp = &v1alpha2.ApplicationConfiguration{}
			err = k8sClient.Get(ctx, appconfigKey, tempApp)
			tempApp.DeepCopyInto(newAppConfig)
			if err != nil || tempApp.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempApp.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that the store is created now")
		logf.Log.Info("Checking the store", "Key", storeKey)
		Expect(k8sClient.Get(ctx, storeKey, storeObj)).Should(BeNil())
		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Expect(k8sClient.Get(ctx, inFooKey, inFoo)).Should(BeNil())

		By("Checking that filepath value of input data resource")
		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != test {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(`{"sub-path":"test"}`))

		err = unstructured.SetNestedField(outFoo.Object, test, "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")
		Expect(err).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == test
		}, 3*time.Second, time.Second).Should(BeTrue())

		newAppConfig.Labels["app-hash"] = hashV2
		By("Update newAppConfig & check successfully")
		Expect(k8sClient.Update(ctx, newAppConfig)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, appconfigKey, newAppConfig); err != nil {
				logf.Log.Error(err, "failed get AppConfig")
				return false
			}
			return newAppConfig.Labels["app-hash"] == hashV2
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency should be unsatisfied, because requirementCondition valueFrom not match")
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{{
				Reason: "got(hash-v1) expected to be hash-v2",
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: store.GetAPIVersion(),
						Name:       store.GetName(),
						Kind:       store.GetKind(),
					},
					FieldPaths: []string{
						"data.app-hash(sub-path)",
					}}}},
		}
		Eventually(func() v1alpha2.DependencyStatus {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			k8sClient.Get(ctx, appconfigKey, newAppConfig)
			return newAppConfig.Status.Dependency
		}, 3*time.Second, time.Second).Should(Equal(depStatus))

		By("Update trait resource to meet the requirement")
		Expect(k8sClient.Get(ctx, outFooKey, outFoo)).Should(BeNil()) // Get the latest before update
		Expect(unstructured.SetNestedField(outFoo.Object, testNew, "status", "key")).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, hashV2, "status", "app-hash")).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == testNew
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency is satisfied")
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			tempAppConfig := &v1alpha2.ApplicationConfiguration{}
			err := k8sClient.Get(ctx, appconfigKey, tempAppConfig)
			if err != nil || tempAppConfig.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempAppConfig.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which accepts data is updated")

		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != testNew {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(`{"sub-path":"test-new"}`))
	})
	// Check the conditions in datainputs for data dependency
	It("data passing with conditions set in dataInput", func() {
		label := map[string]string{"trait": "component", "app-hash": hashV1}
		// Create application configuration
		appConfigName := "appconfig-trait-comp-update"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom: v1alpha2.DataInputValueFrom{
							DataOutputName: "trait-comp",
						},
						ToFieldPaths: []string{"spec.key"},
						InputStore: v1alpha2.StoreReference{
							TypedReference: v1alpha1.TypedReference{
								APIVersion: store.GetAPIVersion(),
								Name:       store.GetName(),
								Kind:       store.GetKind(),
							},
							Operations: []v1alpha2.DataOperation{{
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec",
								Value:       "{}",
							}, {
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: "spec.key",
								ValueFrom: v1alpha2.ValueFrom{
									FieldPath: "data.app-hash",
								},
							}},
						},
						// Only set spec.key when it is empty
						Conditions: []v1alpha2.ConditionRequirement{{
							Operator:  v1alpha2.ConditionEqual,
							Value:     "",
							FieldPath: "spec.key",
						}}}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "trait-comp",
							FieldPath: "status.key",
							OutputStore: v1alpha2.StoreReference{
								TypedReference: v1alpha1.TypedReference{
									APIVersion: store.GetAPIVersion(),
									Name:       store.GetName(),
									Kind:       store.GetKind(),
								},
								Operations: []v1alpha2.DataOperation{{
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data",
									Value:       "{}",
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									ValueFrom: v1alpha2.ValueFrom{
										FieldPath: "status.key",
									},
									Conditions: []v1alpha2.ConditionRequirement{{
										Operator:  v1alpha2.ConditionEqual,
										ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
										FieldPath: "status.app-hash",
									}},
								}},
							},
							Conditions: []v1alpha2.ConditionRequirement{{
								Operator:  v1alpha2.ConditionEqual,
								ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
								FieldPath: "status.app-hash",
							}}}}}}}}},
		}
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		By("Create appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &appConfig)
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which provides data is created")
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, test, "status", "key")).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, hashV1, "status", "app-hash")).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == test
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency is satisfied")
		newAppConfig := &v1alpha2.ApplicationConfiguration{}
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			tempAppConfig := &v1alpha2.ApplicationConfiguration{}
			err := k8sClient.Get(ctx, appconfigKey, tempAppConfig)
			tempAppConfig.DeepCopyInto(newAppConfig)
			if err != nil || tempAppConfig.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return tempAppConfig.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that filepath value of input data resource")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != test {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(test))

		newAppConfig.Labels["app-hash"] = hashV2
		By("Update newAppConfig & check successfully")
		Expect(k8sClient.Update(ctx, newAppConfig)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, appconfigKey, newAppConfig); err != nil {
				logf.Log.Error(err, "failed get AppConfig")
				return false
			}
			return newAppConfig.Labels["app-hash"] == hashV2
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Update trait resource to meet the requirement")
		Expect(k8sClient.Get(ctx, outFooKey, outFoo)).Should(BeNil()) // Get the latest before update
		Expect(unstructured.SetNestedField(outFoo.Object, testNew, "status", "key")).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, hashV2, "status", "app-hash")).Should(BeNil())
		By("Update outFoo & check successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == testNew
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency should be unsatisfied, because requirementCondition value is not empty")
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{{
				Reason: "DataInputs Conditions: got(test) expected to be ",
				From: v1alpha2.DependencyFromObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       outName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPath: "status.key",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: v1alpha1.TypedReference{
						APIVersion: tempFoo.GetAPIVersion(),
						Name:       inName,
						Kind:       tempFoo.GetKind(),
					},
					FieldPaths: []string{
						"spec.key",
					}}}},
		}
		Eventually(func() v1alpha2.DependencyStatus {
			By("Reconcile")
			reconcileRetry(reconciler, req)
			k8sClient.Get(ctx, appconfigKey, newAppConfig)
			return newAppConfig.Status.Dependency
		}, 3*time.Second, time.Second).Should(Equal(depStatus))

		By("Checking that resource which accepts data is updated")
		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
			if outdata != test {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(test))
	})
	// check when the fieldPath in jsonPatch with . or / as part of name, for example the label with key: app.io/hash.
	It("data passing with special characters in fieldPath", func() {
		label := map[string]string{"trait": "component", "app.io/hash": hashV1}
		appConfigName := "appconfig-trait-comp-update"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: componentInName,
					DataInputs: []v1alpha2.DataInput{{
						InputStore: v1alpha2.StoreReference{
							TypedReference: v1alpha1.TypedReference{
								APIVersion: store.GetAPIVersion(),
								Name:       store.GetName(),
								Kind:       store.GetKind(),
							},
							Operations: []v1alpha2.DataOperation{{
								Type:        typeJP,
								Operator:    "add",
								ToFieldPath: `metadata.labels.app\.io~1hash`,
								ValueFrom: v1alpha2.ValueFrom{
									FieldPath: "data.app-hash",
								},
							}},
						}}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{
							Object: out,
						},
						DataOutputs: []v1alpha2.DataOutput{{
							OutputStore: v1alpha2.StoreReference{
								TypedReference: v1alpha1.TypedReference{
									APIVersion: store.GetAPIVersion(),
									Name:       store.GetName(),
									Kind:       store.GetKind(),
								},
								Operations: []v1alpha2.DataOperation{{
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data",
									Value:       "{}",
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									Value:       `{}`,
								}, {
									Type:        typeJP,
									Operator:    "add",
									ToFieldPath: "data.app-hash",
									ValueFrom: v1alpha2.ValueFrom{
										FieldPath: "status.key",
									},
								}}}}}}}}}},
		}
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		req := reconcile.Request{NamespacedName: appconfigKey}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		By("Create appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appconfigKey, &appConfig)
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that resource which provides data is created")
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(func() error {
			err := k8sClient.Get(ctx, outFooKey, outFoo)
			if err != nil {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return err
		}, 3*time.Second, time.Second).Should(BeNil())
		Expect(unstructured.SetNestedField(outFoo.Object, test, "status", "key")).Should(BeNil())
		By("Update outFoo successfully")
		Expect(k8sClient.Status().Update(ctx, outFoo)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, outFooKey, outFoo); err != nil {
				return false
			}
			s, _, _ := unstructured.NestedString(outFoo.Object, "status", "key")
			return s == test
		}, 3*time.Second, time.Second).Should(BeTrue())

		By("Verify the appconfig's dependency is satisfied")
		newAppConfig := &v1alpha2.ApplicationConfiguration{}
		Eventually(func() []v1alpha2.UnstaifiedDependency {
			reconcileRetry(reconciler, req)
			tempAppConfig := &v1alpha2.ApplicationConfiguration{}
			err := k8sClient.Get(ctx, appconfigKey, tempAppConfig)
			tempAppConfig.DeepCopyInto(newAppConfig)
			if err != nil || tempAppConfig.Status.Dependency.Unsatisfied != nil {
				// Try 3 (= 3s/1s) times
				reconcileRetry(reconciler, req)
			}
			return tempAppConfig.Status.Dependency.Unsatisfied
		}, 3*time.Second, time.Second).Should(BeNil())

		By("Checking that filepath value of input data resource")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		Eventually(func() string {
			k8sClient.Get(ctx, inFooKey, inFoo)
			outdata, _, _ := unstructured.NestedString(inFoo.Object, "metadata", "labels", "app.io/hash")
			if outdata != test {
				// Try 3 (= 3s/1s) times
				reconciler.Reconcile(req)
			}
			return outdata
		}, 3*time.Second, time.Second).Should(Equal(test))
	})
})
