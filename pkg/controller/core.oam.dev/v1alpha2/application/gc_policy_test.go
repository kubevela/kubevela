/*
 Copyright 2021. The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	v1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

var _ = Describe("Test Application with GC options", func() {
	ctx := context.Background()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vela-test-app-with-gc-options",
		},
	}

	worker := &v1beta1.ComponentDefinition{}
	workerCdDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))

	ingressTrait := &v1beta1.TraitDefinition{}
	ingressTdDefJson, _ := yaml.YAMLToJSON([]byte(ingressTraitDefYaml))

	configMap := &v1beta1.TraitDefinition{}
	configMapTdDefJson, _ := yaml.YAMLToJSON([]byte(configMapTraitDefYaml))

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, ns.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(workerCdDefJson, worker)).Should(BeNil())
		Expect(k8sClient.Create(ctx, worker.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(ingressTdDefJson, ingressTrait)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ingressTrait.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(configMapTdDefJson, configMap)).Should(BeNil())
		Expect(k8sClient.Create(ctx, configMap.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	Context("Test Application enable gc option keepLegacyResource", func() {
		baseApp := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "baseApp",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "worker",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []common.ApplicationTrait{{
							Type:       "ingress-without-healthcheck",
							Properties: &runtime.RawExtension{Raw: []byte(`{"domain":"test.com","http":{"/": 80}}`)},
						}},
					},
				},
				Policies: []v1beta1.AppPolicy{{
					Name:       "keep-legacy-resource",
					Type:       "garbage-collect",
					Properties: &runtime.RawExtension{Raw: []byte(`{"keepLegacyResource": true}`)},
				}},
			},
		}

		It("Each update will create a new workload and trait object", func() {
			resourcekeeper.MarkWithProbability = 1.0
			app := baseApp.DeepCopy()
			app.SetNamespace(ns.Name)
			app.SetName("app-with-worker-ingress")

			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			appV1 := new(v1beta1.Application)
			Eventually(func() error {
				_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				if err != nil {
					return err
				}
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), appV1); err != nil {
					return err
				}
				if appV1.Status.Phase != common.ApplicationRunning {
					return errors.New("app is not in running status")
				}
				return nil
			}, 3*time.Second, 300*time.Second).Should(BeNil())

			By("update app with new component name")
			for i := 2; i <= 6; i++ {
				Eventually(func() error {
					oldApp := new(v1beta1.Application)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), oldApp)).Should(BeNil())
					updateApp := oldApp.DeepCopy()
					updateApp.Spec.Components[0].Name = fmt.Sprintf("%s-v%d", "worker", i)
					return k8sClient.Update(ctx, updateApp)
				}, time.Second*3, time.Microsecond*300).Should(BeNil())

				testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				newApp := new(v1beta1.Application)
				Eventually(func() error {
					_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
					if err != nil {
						return err
					}
					if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), newApp); err != nil {
						return err
					}
					if newApp.Status.Phase != common.ApplicationRunning {
						return errors.New("app is not in running status")
					}
					return nil
				}, 3*time.Second, 300*time.Second).Should(BeNil())
				Expect(newApp.Status.LatestRevision.Revision).Should(Equal(int64(i)))

				rtObjKey := client.ObjectKey{Name: fmt.Sprintf("%s-v%d-%s", app.Name, i, ns.Name)}
				rt := new(v1beta1.ResourceTracker)
				Expect(k8sClient.Get(ctx, rtObjKey, rt)).Should(BeNil())
				Expect(len(rt.Spec.ManagedResources)).Should(Equal(3))

				for _, obj := range rt.Spec.ManagedResources {
					un := new(unstructured.Unstructured)
					un.SetGroupVersionKind(obj.GroupVersionKind())
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, un)).Should(BeNil())
				}
			}

			By("check the resourceTrackers number")
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}

			rtList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(7))

			By("delete one resourceTracker to test the gc of legacy resources")
			testRT := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-v%d-%s", app.Name, 1, ns.Name),
				},
			}
			Expect(k8sClient.Delete(ctx, testRT)).Should(BeNil())
			for _, obj := range testRT.Spec.ManagedResources {
				un := &unstructured.Unstructured{}
				un.SetGroupVersionKind(obj.GroupVersionKind())
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Name}, un); kerrors.IsNotFound(err) {
						return nil
					}
					return errors.Errorf("failed to gc resource %v:%s", un.GroupVersionKind(), un.GetName())
				}, 3*time.Second, 300*time.Millisecond).Should(BeNil())
			}

			By("check the latest resources created by application")
			latestApp := new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, latestApp)).Should(BeNil())
			appliedResource := latestApp.Status.AppliedResources
			for _, obj := range appliedResource {
				un := &unstructured.Unstructured{}
				un.SetGroupVersionKind(obj.GroupVersionKind())
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, un)).Should(BeNil())
			}

			By("delete part legacy resource")
			for i := 2; i <= 4; i++ {
				deploy := new(v1.Deployment)
				deploy.SetName(fmt.Sprintf("worker-v%d", i))
				deploy.SetNamespace(ns.Name)
				Expect(k8sClient.Delete(ctx, deploy))

				ingress := new(networkingv1.Ingress)
				ingress.SetName(fmt.Sprintf("worker-v%d", i))
				ingress.SetNamespace(ns.Name)
				Expect(k8sClient.Delete(ctx, ingress))

				svc := new(corev1.Service)
				svc.SetName(fmt.Sprintf("worker-v%d", i))
				svc.SetNamespace(ns.Name)
				Expect(k8sClient.Delete(ctx, svc))
			}

			By("update app with new component name")

			Eventually(func() error {
				oldApp := new(v1beta1.Application)
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), oldApp)).Should(BeNil())
				updateApp := oldApp.DeepCopy()
				updateApp.Spec.Components[0].Name = fmt.Sprintf("%s-v%d", "worker", 12)
				return k8sClient.Update(ctx, updateApp)
			}, time.Second*3, time.Microsecond*300).Should(BeNil())

			testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			newApp := new(v1beta1.Application)
			Eventually(func() error {
				_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				if err != nil {
					return err
				}
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), newApp); err != nil {
					return err
				}
				if newApp.Status.Phase != common.ApplicationRunning {
					return errors.New("app is not in running status")
				}
				return nil
			}, 3*time.Second, 300*time.Microsecond).Should(BeNil())
			Expect(newApp.Status.LatestRevision.Revision).Should(Equal(int64(7)))

			By("check the resourceTrackers number")
			newRTList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(ctx, newRTList, listOpts...))
			Expect(len(newRTList.Items)).Should(Equal(4))

			By("delete all resources")
			Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(0))
		})

		It("Each update will create a new workload and update a trait object", func() {
			app := baseApp.DeepCopy()
			app.SetNamespace(ns.Name)
			app.SetName("app-with-work-ingress-configmap")

			app.Spec.Components[0].Name = "job"
			app.Spec.Components[0].Traits = append(app.Spec.Components[0].Traits, common.ApplicationTrait{
				Type:       "configmap",
				Properties: &runtime.RawExtension{Raw: []byte(`{"volumes": [{"name": "test-cm", "mountPath":"/tmp/test"}]}`)},
			})

			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			appV1 := new(v1beta1.Application)
			Eventually(func() error {
				_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				if err != nil {
					return err
				}
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), appV1); err != nil {
					return err
				}
				if appV1.Status.Phase != common.ApplicationRunning {
					return errors.New("app is not in running status")
				}
				return nil
			}, 3*time.Second, 300*time.Second).Should(BeNil())

			By("update app's component name and the properties of configmap")
			for i := 2; i <= 6; i++ {
				Eventually(func() error {
					oldApp := new(v1beta1.Application)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), oldApp)).Should(BeNil())
					updateApp := oldApp.DeepCopy()
					updateApp.Spec.Components[0].Name = fmt.Sprintf("%s-v%d", "job", i)
					updateApp.Spec.Components[0].Traits = append(app.Spec.Components[0].Traits, common.ApplicationTrait{
						Type:       "configmap",
						Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"volumes": [{"name": "test-cm","mountPath": "/tmp/test","data": {"test": "%d"}}]}`, i))},
					})
					return k8sClient.Update(ctx, updateApp)
				}, time.Second*3, time.Microsecond*300).Should(BeNil())

				newApp := new(v1beta1.Application)
				Eventually(func() error {
					_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
					if err != nil {
						return err
					}
					if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), newApp); err != nil {
						return err
					}
					if newApp.Status.Phase != common.ApplicationRunning {
						return errors.New("app is not in running status")
					}
					return nil
				}, 5*time.Second, 300*time.Second).Should(BeNil())
				Expect(newApp.Status.LatestRevision.Revision).Should(Equal(int64(i)))

				rtObjKey := client.ObjectKey{Name: fmt.Sprintf("%s-v%d-%s", app.Name, i, ns.Name)}
				rt := new(v1beta1.ResourceTracker)
				Expect(k8sClient.Get(ctx, rtObjKey, rt)).Should(BeNil())
				Expect(len(rt.Spec.ManagedResources)).Should(Equal(4))

				for _, obj := range rt.Spec.ManagedResources {
					if obj.Kind == reflect.TypeOf(corev1.ConfigMap{}).Name() {
						Expect(obj.Name).Should(Equal("test-cm"))
						cm := new(corev1.ConfigMap)
						Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, cm)).Should(BeNil())
						Expect(cm.Data["test"]).Should(Equal(strconv.Itoa(i)))
						continue
					}
					un := new(unstructured.Unstructured)
					un.SetGroupVersionKind(obj.GroupVersionKind())
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, un)).Should(BeNil())
				}
			}

			By("check the resourceTrackers number")
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}

			rtList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(7))

			By("delete one resourceTracker to test the gc of legacy resources")
			testRT := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-v%d-%s", app.Name, 1, ns.Name),
				},
			}

			Expect(k8sClient.Delete(ctx, testRT)).Should(BeNil())
			for _, obj := range testRT.Spec.ManagedResources {
				if obj.Kind == reflect.TypeOf(corev1.ConfigMap{}).Name() {
					cm := new(corev1.ConfigMap)
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, cm)).Should(BeNil())
					continue
				}
				un := &unstructured.Unstructured{}
				un.SetGroupVersionKind(obj.GroupVersionKind())
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Name}, un); kerrors.IsNotFound(err) {
						return nil
					}
					return errors.Errorf("failed to gc resource %v:%s", un.GroupVersionKind(), un.GetName())
				}, 3*time.Second, 300*time.Millisecond).Should(BeNil())
			}

			By("check the latest resources created by application")
			latestApp := new(v1beta1.Application)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, latestApp)).Should(BeNil())
			appliedResource := latestApp.Status.AppliedResources
			for _, obj := range appliedResource {
				un := &unstructured.Unstructured{}
				un.SetGroupVersionKind(obj.GroupVersionKind())
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, un)).Should(BeNil())
			}

			By("delete all resources")
			Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(0))
		})

		It("Each update will only update workload", func() {
			resourcekeeper.MarkWithProbability = 1.0
			app := baseApp.DeepCopy()
			app.Spec.Components[0].Traits = nil
			app.Spec.Components[0].Name = "only-work"
			app.SetNamespace(ns.Name)
			app.SetName("app-with-worker")

			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			appV1 := new(v1beta1.Application)
			Eventually(func() error {
				_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				if err != nil {
					return err
				}
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), appV1); err != nil {
					return err
				}
				if appV1.Status.Phase != common.ApplicationRunning {
					return errors.New("app is not in running status")
				}
				return nil
			}, 3*time.Second, 300*time.Second).Should(BeNil())

			By("update component with new properties")
			for i := 2; i <= 11; i++ {
				Eventually(func() error {
					oldApp := new(v1beta1.Application)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), oldApp)).Should(BeNil())
					updateApp := oldApp.DeepCopy()
					updateApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"cmd":["sleep","%d"],"image":"busybox"}`, i))}
					return k8sClient.Update(ctx, updateApp)
				}, time.Second*3, time.Microsecond*300).Should(BeNil())

				testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				newApp := new(v1beta1.Application)
				Eventually(func() error {
					_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
					if err != nil {
						return err
					}
					if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), newApp); err != nil {
						return err
					}
					if newApp.Status.Phase != common.ApplicationRunning {
						return errors.New("app is not in running status")
					}
					return nil
				}, 3*time.Second, 300*time.Second).Should(BeNil())
				Expect(newApp.Status.LatestRevision.Revision).Should(Equal(int64(i)))

				rtObjKey := client.ObjectKey{Name: fmt.Sprintf("%s-v%d-%s", app.Name, i, ns.Name)}
				rt := new(v1beta1.ResourceTracker)
				Expect(k8sClient.Get(ctx, rtObjKey, rt)).Should(BeNil())
				Expect(len(rt.Spec.ManagedResources)).Should(Equal(1))

				for _, obj := range rt.Spec.ManagedResources {
					un := new(unstructured.Unstructured)
					un.SetGroupVersionKind(obj.GroupVersionKind())
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, un)).Should(BeNil())
				}
			}

			By("check the resourceTrackers number")
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}

			rtList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(2))

			By("delete all resources")
			Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
			testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, rtList, listOpts...))
			Expect(len(rtList.Items)).Should(Equal(0))
		})
	})

	Context("Test Application enable gc option sequential", func() {
		baseApp := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sequential-gc",
				Namespace: "default",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "worker1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						DependsOn: []string{
							"worker2",
						},
					},
					{
						Name:       "worker2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "worker3-output",
								ParameterKey: "test",
							},
						},
					},
					{
						Name:       "worker3",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{
								Name:      "worker3-output",
								ValueFrom: "output.metadata.name",
							},
						},
					},
				},
				Policies: []v1beta1.AppPolicy{{
					Name:       "reverse-dependency",
					Type:       "garbage-collect",
					Properties: &runtime.RawExtension{Raw: []byte(`{"order": "dependency"}`)},
				}},
			},
		}

		It("Test GC with sequential", func() {
			resourcekeeper.MarkWithProbability = 1.0
			app := baseApp.DeepCopy()

			Expect(k8sClient.Create(ctx, app)).Should(BeNil())
			appV1 := new(v1beta1.Application)
			Eventually(func() error {
				_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
				if err != nil {
					return err
				}
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), appV1); err != nil {
					return err
				}
				if appV1.Status.Phase != common.ApplicationRunning {
					return errors.New("app is not in running status")
				}
				return nil
			}, 3*time.Second, 300*time.Second).Should(BeNil())

			By("check the resourceTrackers number")
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}

			rtList := &v1beta1.ResourceTrackerList{}
			Expect(k8sClient.List(ctx, rtList, listOpts...)).Should(BeNil())
			Expect(len(rtList.Items)).Should(Equal(2))
			workerList := &v1.DeploymentList{}
			Expect(k8sClient.List(ctx, workerList, listOpts...)).Should(BeNil())
			Expect(len(workerList.Items)).Should(Equal(3))

			By("delete application")
			Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
			By("worker1 will be deleted")
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, workerList, listOpts...)).Should(BeNil())
			for _, worker := range workerList.Items {
				Expect(worker.Name).ShouldNot(Equal("worker1"))
			}
			Expect(len(workerList.Items)).Should(Equal(2))
			By("worker2 will be deleted")
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, workerList, listOpts...)).Should(BeNil())
			Expect(len(workerList.Items)).Should(Equal(1))
			for _, worker := range workerList.Items {
				Expect(worker.Name).ShouldNot(Equal("worker2"))
			}
			By("worker3 will be deleted")
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, workerList, listOpts...)).Should(BeNil())
			Expect(len(workerList.Items)).Should(Equal(0))
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
			Expect(k8sClient.List(ctx, rtList, listOpts...)).Should(BeNil())
			Expect(len(rtList.Items)).Should(Equal(0))
		})
	})
})

const (
	ingressTraitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress-without-healthcheck
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	domain: string
        	http: [string]: int
        }
        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata:
        		name: context.name
        	spec: {
        		selector:
        			app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }
        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
                                pathType: "Prefix"
        						backend: {
        							service: {
                                        name: context.name
                                        port: {
                                            number: v
                                        }
                                    }
        						}
        					},
        				]
        			}
        		}]
        	}
        }
`
	configMapTraitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Create/Attach configmaps on K8s pod for your workload which follows the pod spec in path 'spec.template'.
  name: configmap
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: spec: template: spec: {
        	containers: [{
        		// +patchKey=name
        		volumeMounts: [
        			for v in parameter.volumes {
        				{
        					name:      "volume-\(v.name)"
        					mountPath: v.mountPath
        					readOnly:  v.readOnly
        				}
        			},
        		]
        	}, ...]
        	// +patchKey=name
        	volumes: [
        		for v in parameter.volumes {
        			{
        				name: "volume-\(v.name)"
        				configMap: name: v.name
        			}
        		},
        	]
        }
        outputs: {
        	for v in parameter.volumes {
        		if v.data != _|_ {
        			"\(v.name)": {
        				apiVersion: "v1"
        				kind:       "ConfigMap"
        				metadata: name: v.name
        				data: v.data
        			}
        		}
        	}
        }
        parameter: {
        	// +usage=Specify mounted configmap names and their mount paths in the container
        	volumes: [...{
        		name:      string
        		mountPath: string
        		readOnly:  *false | bool
        		data?: [string]: string
        	}]
        }
`
)
