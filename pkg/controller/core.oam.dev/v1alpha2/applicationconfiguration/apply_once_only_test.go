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

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/types"

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
		It("tests the situation where workload is not applied at the first because of unsatisfied dependency",
			func() {
				componentHandler := &ComponentHandler{Client: k8sClient, RevisionLimit: 100, Logger: logging.NewLogrLogger(ctrl.Log.WithName("component-handler"))}

				By("Enable ApplyOnceOnlyForce")
				reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyForce

				tempFoo := &unstructured.Unstructured{}
				tempFoo.SetAPIVersion("example.com/v1")
				tempFoo.SetKind("Foo")
				tempFoo.SetNamespace(namespace)

				inName := "data-input"
				inputWorkload := &unstructured.Unstructured{}
				inputWorkload.SetAPIVersion("example.com/v1")
				inputWorkload.SetKind("Foo")
				inputWorkload.SetNamespace(namespace)
				inputWorkload.SetName(inName)

				compInName := "comp-in"
				compIn := v1alpha2.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      compInName,
						Namespace: namespace,
					},
					Spec: v1alpha2.ComponentSpec{
						Workload: runtime.RawExtension{
							Object: inputWorkload,
						},
					},
				}

				outName := "data-output"
				outputTrait := tempFoo.DeepCopy()
				outputTrait.SetName(outName)

				acWithDepName := "ac-dep"
				acWithDep := v1alpha2.ApplicationConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      acWithDepName,
						Namespace: namespace,
					},
					Spec: v1alpha2.ApplicationConfigurationSpec{
						Components: []v1alpha2.ApplicationConfigurationComponent{
							{
								ComponentName: compInName,
								DataInputs: []v1alpha2.DataInput{
									{
										ValueFrom: v1alpha2.DataInputValueFrom{
											DataOutputName: "trait-output",
										},
										ToFieldPaths: []string{"spec.key"},
									},
								},
								Traits: []v1alpha2.ComponentTrait{{
									Trait: runtime.RawExtension{Object: outputTrait},
									DataOutputs: []v1alpha2.DataOutput{{
										Name:      "trait-output",
										FieldPath: "status.key",
									}},
								},
								},
							},
						},
					},
				}

				By("Create Component")
				Expect(k8sClient.Create(ctx, &compIn)).Should(Succeed())
				cmp := &v1alpha2.Component{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName}, cmp)).Should(Succeed())

				cmpV1 := cmp.DeepCopy()
				By("component handler will automatically create controller revision")
				Expect(func() bool {
					_, ok := componentHandler.createControllerRevision(cmpV1, cmpV1)
					return ok
				}()).Should(BeTrue())

				By("Creat appConfig & check successfully")
				Expect(k8sClient.Create(ctx, &acWithDep)).Should(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep)
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Reconcile & check successfully")

				reqDep := reconcile.Request{
					NamespacedName: client.ObjectKey{Namespace: namespace, Name: acWithDepName},
				}
				Eventually(func() bool {
					reconcileRetry(reconciler, reqDep)
					acWithDep = v1alpha2.ApplicationConfiguration{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep); err != nil {
						return false
					}
					return len(acWithDep.Status.Workloads) == 1
				}, time.Second, 300*time.Millisecond).Should(BeTrue())

				// because dependency is not satisfied so the workload should not be created
				By("Check the workload is NOT created")
				workloadIn := tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: inName}, workloadIn)).Should(&util.NotFoundMatcher{})

				// modify the trait to make it satisfy comp's dependency
				outputTrait = tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: outName}, outputTrait)).Should(Succeed())
				err := unstructured.SetNestedField(outputTrait.Object, "test", "status", "key")
				Expect(err).Should(BeNil())
				Expect(k8sClient.Status().Update(ctx, outputTrait)).Should(Succeed())
				Eventually(func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: outName}, outputTrait)
					data, _, _ := unstructured.NestedString(outputTrait.Object, "status", "key")
					return data
				}, 3*time.Second, time.Second).Should(Equal("test"))

				By("Reconcile & check ac is satisfied")
				Eventually(func() []v1alpha2.UnstaifiedDependency {
					reconcileRetry(reconciler, reqDep)
					acWithDep = v1alpha2.ApplicationConfiguration{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep); err != nil {
						return []v1alpha2.UnstaifiedDependency{{Reason: err.Error()}}
					}
					return acWithDep.Status.Dependency.Unsatisfied
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Reconcile & check workload is created")
				Eventually(func() error {
					reconcileRetry(reconciler, reqDep)
					// the workload is created now because its dependency is satisfied
					workloadIn := tempFoo.DeepCopy()
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: inName}, workloadIn)
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Delete the workload")
				recreatedWL := tempFoo.DeepCopy()
				recreatedWL.SetName(inName)
				Expect(k8sClient.Delete(ctx, recreatedWL)).Should(Succeed())
				outputTrait = tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: inName}, outputTrait)).Should(util.NotFoundMatcher{})

				By("Reconcile")
				Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
				time.Sleep(3 * time.Second)

				By("Check workload is not re-created by reconciliation")
				inputWorkload = tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: inName}, inputWorkload)).Should(util.NotFoundMatcher{})
			})

		It("tests the situation where workload is not applied at the first because of unsatisfied dependency and revision specified",
			func() {
				componentHandler := &ComponentHandler{Client: k8sClient, RevisionLimit: 100, Logger: logging.NewLogrLogger(ctrl.Log.WithName("component-handler"))}

				By("Enable ApplyOnceOnlyForce")
				reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyForce

				tempFoo := &unstructured.Unstructured{}
				tempFoo.SetAPIVersion("example.com/v1")
				tempFoo.SetKind("Foo")
				tempFoo.SetNamespace(namespace)

				inputWorkload := &unstructured.Unstructured{}
				inputWorkload.SetAPIVersion("example.com/v1")
				inputWorkload.SetKind("Foo")
				inputWorkload.SetNamespace(namespace)

				compInName := "comp-in-revision"
				compIn := v1alpha2.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      compInName,
						Namespace: namespace,
					},
					Spec: v1alpha2.ComponentSpec{
						Workload: runtime.RawExtension{
							Object: inputWorkload,
						},
					},
				}

				outName := "data-output"
				outputTrait := tempFoo.DeepCopy()
				outputTrait.SetName(outName)

				acWithDepName := "ac-dep"
				acWithDep := v1alpha2.ApplicationConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      acWithDepName,
						Namespace: namespace,
					},
					Spec: v1alpha2.ApplicationConfigurationSpec{
						Components: []v1alpha2.ApplicationConfigurationComponent{
							{
								RevisionName: compInName + "-v1",
								DataInputs: []v1alpha2.DataInput{
									{
										ValueFrom: v1alpha2.DataInputValueFrom{
											DataOutputName: "trait-output",
										},
										ToFieldPaths: []string{"spec.key"},
									},
								},
								Traits: []v1alpha2.ComponentTrait{{
									Trait: runtime.RawExtension{Object: outputTrait},
									DataOutputs: []v1alpha2.DataOutput{{
										Name:      "trait-output",
										FieldPath: "status.key",
									}},
								},
								},
							},
						},
					},
				}

				By("Create Component")
				Expect(k8sClient.Create(ctx, &compIn)).Should(Succeed())
				cmp := &v1alpha2.Component{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName}, cmp)).Should(Succeed())

				cmpV1 := cmp.DeepCopy()
				By("component handler will automatically create controller revision")
				Expect(func() bool {
					_, ok := componentHandler.createControllerRevision(cmpV1, cmpV1)
					return ok
				}()).Should(BeTrue())

				By("Creat appConfig & check successfully")
				Expect(k8sClient.Create(ctx, &acWithDep)).Should(Succeed())
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep)
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Reconcile & check successfully")

				reqDep := reconcile.Request{
					NamespacedName: client.ObjectKey{Namespace: namespace, Name: acWithDepName},
				}
				Eventually(func() bool {
					reconcileRetry(reconciler, reqDep)
					acWithDep = v1alpha2.ApplicationConfiguration{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep); err != nil {
						return false
					}
					return len(acWithDep.Status.Workloads) == 1
				}, time.Second, 300*time.Millisecond).Should(BeTrue())

				// because dependency is not satisfied so the workload should not be created
				By("Check the workload is NOT created")
				workloadIn := tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName + "-v1"}, workloadIn)).Should(&util.NotFoundMatcher{})

				// modify the trait to make it satisfy comp's dependency
				outputTrait = tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: outName}, outputTrait)).Should(Succeed())
				err := unstructured.SetNestedField(outputTrait.Object, "test", "status", "key")
				Expect(err).Should(BeNil())
				Expect(k8sClient.Status().Update(ctx, outputTrait)).Should(Succeed())
				Eventually(func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: outName}, outputTrait)
					data, _, _ := unstructured.NestedString(outputTrait.Object, "status", "key")
					return data
				}, 3*time.Second, time.Second).Should(Equal("test"))

				By("Reconcile & check ac is satisfied")
				Eventually(func() []v1alpha2.UnstaifiedDependency {
					reconcileRetry(reconciler, reqDep)
					acWithDep = v1alpha2.ApplicationConfiguration{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: acWithDepName}, &acWithDep); err != nil {
						return []v1alpha2.UnstaifiedDependency{{Reason: err.Error()}}
					}
					return acWithDep.Status.Dependency.Unsatisfied
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Reconcile & check workload is created")
				Eventually(func() error {
					reconcileRetry(reconciler, reqDep)
					// the workload should be created now because its dependency is satisfied
					workloadIn := tempFoo.DeepCopy()
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName}, workloadIn)
				}, time.Second, 300*time.Millisecond).Should(BeNil())

				By("Delete the workload")
				recreatedWL := tempFoo.DeepCopy()
				recreatedWL.SetName(compInName)
				Expect(k8sClient.Delete(ctx, recreatedWL)).Should(Succeed())
				inputWorkload2 := tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName}, inputWorkload2)).Should(util.NotFoundMatcher{})

				By("Reconcile")
				Expect(func() error { _, err := reconciler.Reconcile(req); return err }()).Should(BeNil())
				time.Sleep(3 * time.Second)

				By("Check workload is not re-created by reconciliation")
				inputWorkload = tempFoo.DeepCopy()
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compInName}, inputWorkload)).Should(util.NotFoundMatcher{})
			})

		It("should normally create workload/trait resources at fist time", func() {
			By("Enable ApplyOnceOnlyForce")
			reconciler.applyOnceOnlyMode = core.ApplyOnceOnlyForce
			component2 := v1alpha2.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "core.oam.dev/v1alpha2",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mycomp2",
					Namespace: namespace,
				},
				Spec: v1alpha2.ComponentSpec{
					Workload: runtime.RawExtension{
						Object: &cw,
					},
				},
			}
			newFakeTrait := fakeTrait.DeepCopy()
			newFakeTrait.SetName("mytrait2")
			appConfig2 := v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myac2",
					Namespace: namespace,
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: "mycomp2",
							Traits: []v1alpha2.ComponentTrait{
								{Trait: runtime.RawExtension{Object: newFakeTrait}},
							},
						},
					},
				},
			}

			By("Create Component")
			Expect(k8sClient.Create(ctx, &component2)).Should(Succeed())
			time.Sleep(time.Second)

			By("Creat appConfig & check successfully")
			Expect(k8sClient.Create(ctx, &appConfig2)).Should(Succeed())
			time.Sleep(time.Second)

			By("Reconcile")
			Expect(func() error {
				_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Name: "myac2", Namespace: namespace}})
				return err
			}()).Should(BeNil())
			time.Sleep(2 * time.Second)

			By("Get workload instance & Check workload spec")
			cwObj := v1alpha2.ContainerizedWorkload{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "mycomp2"}, &cwObj)
			}, 5*time.Second, time.Second).Should(BeNil())
			Expect(cwObj.Spec.Containers[0].Image).Should(Equal(image1))

			By("Get trait instance & Check trait spec")
			fooObj := &unstructured.Unstructured{}
			fooObj.SetAPIVersion("example.com/v1")
			fooObj.SetKind("Foo")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "mytrait2"}, fooObj)
			}, 3*time.Second, time.Second).Should(BeNil())
			fooObjV, _, _ := unstructured.NestedString(fooObj.Object, "spec", "key")
			Expect(fooObjV).Should(Equal(traitSpecValue1))

		})

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

			By("Update AppConfig to trigger generation updated")
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
			time.Sleep(1 * time.Second)

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
			time.Sleep(2 * time.Second)

			By("Check workload was not created by reconciliation")
			Eventually(func() error {
				By("Reconcile")
				reconcileRetry(reconciler, req)
				recreatedCwObj = v1alpha2.ContainerizedWorkload{}
				return k8sClient.Get(ctx, cwObjKey, &recreatedCwObj)
			}, 5*time.Second, time.Second).Should(SatisfyAll(util.NotFoundMatcher{}))

			By("Check trait is re-created by reconciliation")
			recreatedFooObj = unstructured.Unstructured{}
			recreatedFooObj.SetAPIVersion("example.com/v1")
			recreatedFooObj.SetKind("Foo")
			Expect(k8sClient.Get(ctx, traitObjKey, &recreatedFooObj)).Should(Succeed())
		})
	})
})
