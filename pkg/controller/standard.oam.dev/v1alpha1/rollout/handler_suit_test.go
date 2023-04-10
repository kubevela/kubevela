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

package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

var _ = Describe("Test rollout related handler func", func() {
	namespace := "rollout-test-namespace"
	ctx := context.Background()

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Test assemble workload info", func() {
		tarWorkload := &unstructured.Unstructured{}
		tarWorkload.SetAPIVersion("apps/v1")
		tarWorkload.SetKind("Deployment")

		srcWorkload := &unstructured.Unstructured{}
		srcWorkload.SetAPIVersion("apps/v1")
		srcWorkload.SetKind("Deployment")
		compName := "comp-test"
		appRevName := "app-revision-v2"
		h := handler{
			reconciler: &reconciler{
				Client: k8sClient,
			},
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Labels: map[string]string{
						oam.LabelAppRevision: appRevName,
					},
				}},
			targetWorkload: tarWorkload,
			sourceWorkload: srcWorkload,
			targetRevName:  "comp-test-v2",
			sourceRevName:  "comp-test-v1",
			compName:       compName,
		}
		util.AddLabels(h.targetWorkload, map[string]string{oam.LabelAppComponent: compName})
		util.AddLabels(h.sourceWorkload, map[string]string{oam.LabelAppComponent: compName})
		h.setWorkloadBaseInfo()
		Expect(h.targetWorkload.GetName()).Should(BeEquivalentTo(compName))
		Expect(h.sourceWorkload.GetName()).Should(BeEquivalentTo(compName))
		Expect(h.targetWorkload.GetNamespace()).Should(BeEquivalentTo(namespace))
		Expect(h.sourceWorkload.GetNamespace()).Should(BeEquivalentTo(namespace))
		tarLabel := h.targetWorkload.GetLabels()
		Expect(tarLabel[oam.LabelAppRevision]).Should(BeEquivalentTo(appRevName))
		Expect(tarLabel[oam.LabelAppComponentRevision]).Should(BeEquivalentTo("comp-test-v2"))
		srcLabel := h.sourceWorkload.GetLabels()
		Expect(srcLabel[oam.LabelAppRevision]).Should(BeEquivalentTo(appRevName))
		Expect(srcLabel[oam.LabelAppComponentRevision]).Should(BeEquivalentTo("comp-test-v1"))

		Expect(h.assembleWorkload(ctx)).Should(BeNil())
		Expect(h.targetWorkload.GetName()).Should(BeEquivalentTo("comp-test-v2"))
		Expect(h.sourceWorkload.GetName()).Should(BeEquivalentTo("comp-test-v1"))
		pv := fieldpath.Pave(h.targetWorkload.UnstructuredContent())
		Expect(pv.GetBool("spec.paused")).Should(BeEquivalentTo(true))
		replicas, err := pv.GetInteger("spec.replicas")
		Expect(err).Should(BeNil())
		Expect(replicas).Should(BeEquivalentTo(0))
	})

	It("Test prepare workload from revision", func() {
		compName := "metrics-provider"
		ctlV1 := new(appsv1.ControllerRevision)
		ctlV1Json, err := yaml.YAMLToJSON([]byte(compRevisionV1))
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ctlV1Json, ctlV1)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctlV1)).Should(BeNil())

		ctlV2 := new(appsv1.ControllerRevision)
		ctlV2Json, err := yaml.YAMLToJSON([]byte(compRevisionV2))
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ctlV2Json, ctlV2)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctlV2)).Should(BeNil())

		h := handler{
			reconciler: &reconciler{
				Client: k8sClient,
			},
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				}},
			targetRevName: "metrics-provider-v2",
			sourceRevName: "metrics-provider-v1",
			compName:      compName,
		}

		Eventually(func() error {
			wd, err := h.extractWorkload(ctx, namespace, h.targetRevName)
			if err != nil {
				return err
			}
			if wd == nil || wd.GetKind() != "Deployment" {
				return fmt.Errorf("extract error")
			}
			return nil
		}, 15*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(func() error {
			wd, err := h.extractWorkload(ctx, namespace, h.sourceRevName)
			if err != nil {
				return err
			}
			if wd == nil || wd.GetKind() != "Deployment" {
				return fmt.Errorf("extract error")
			}
			return nil
		}, 15*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	Describe("Test Handle rollout modified", func() {
		It("succeed rollout", func() {
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: &v1alpha1.Rollout{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: v1alpha1.RolloutSpec{
						TargetRevisionName: "metrics-provider-v2",
					},
					Status: v1alpha1.CompRolloutStatus{
						RolloutStatus: v1alpha1.RolloutStatus{
							RollingState: v1alpha1.RolloutSucceedState,
						},
						LastUpgradedTargetRevision: "metrics-provider-v1",
					},
				},
			}
			h.handleRolloutModified()
			Expect(h.targetRevName).Should(BeEquivalentTo("metrics-provider-v2"))
			Expect(h.sourceRevName).Should(BeEquivalentTo("metrics-provider-v1"))
		})

		It("middle state rollout", func() {
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: &v1alpha1.Rollout{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: v1alpha1.RolloutSpec{
						TargetRevisionName: "metrics-provider-v3",
					},
					Status: v1alpha1.CompRolloutStatus{
						RolloutStatus: v1alpha1.RolloutStatus{
							RollingState: v1alpha1.RollingInBatchesState,
						},
						LastUpgradedTargetRevision: "metrics-provider-v2",
						LastSourceRevision:         "metrics-provider-v1",
					},
				},
			}
			h.handleRolloutModified()
			Expect(h.targetRevName).Should(BeEquivalentTo("metrics-provider-v2"))
			Expect(h.sourceRevName).Should(BeEquivalentTo("metrics-provider-v1"))
		})

		It("handle finalizer test", func() {
			deletTime := metav1.NewTime(time.Now())
			rollout := &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         namespace,
					Name:              "rollout-test",
					DeletionTimestamp: &deletTime,
				},
				Status: v1alpha1.CompRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RolloutFailedState,
					},
				},
			}
			meta.AddFinalizer(rollout, rolloutFinalizer)
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: rollout,
			}
			done, _, _ := h.handleFinalizer(ctx, rollout)
			Expect(done).Should(BeTrue())
			Expect(len(rollout.Finalizers)).Should(BeEquivalentTo(0))
		})

		It("handle finalizer in progress rollout test", func() {
			deletTime := metav1.NewTime(time.Now())
			rollout := &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         namespace,
					Name:              "rollout-test",
					DeletionTimestamp: &deletTime,
				},
				Status: v1alpha1.CompRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
				},
			}
			meta.AddFinalizer(rollout, rolloutFinalizer)
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: rollout,
			}
			done, _, _ := h.handleFinalizer(ctx, rollout)
			Expect(done).Should(BeFalse())
			Expect(len(rollout.Finalizers)).Should(BeEquivalentTo(1))
			Expect(rollout.Status.RollingState).Should(BeEquivalentTo(v1alpha1.RolloutDeletingState))
		})

		It("Test recordeWorkloadInResourceTracker func", func() {
			ctx := context.Background()
			rtName := "resourcetracker-v1-test-namespace"
			rt := v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: rtName,
				},
			}
			Expect(k8sClient.Create(ctx, &rt)).Should(BeNil())
			rollout := v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(&rt, v1beta1.ResourceTrackerKindVersionKind),
					},
				},
			}
			u := &unstructured.Unstructured{}
			u.SetAPIVersion("apps/v1")
			u.SetNamespace("test-namespace")
			u.SetName("test-workload")
			u.SetUID("test-uid")
			u.SetKind("Deployment")
			h := &handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout:        &rollout,
				targetWorkload: u,
			}
			Expect(h.recordWorkloadInResourceTracker(ctx, u)).Should(BeNil())
			checkRt := v1beta1.ResourceTracker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rtName}, &checkRt)).Should(BeNil())
			Expect(len(checkRt.Status.TrackedResources)).Should(BeEquivalentTo(1))
			Expect(checkRt.Status.TrackedResources[0].Name).Should(BeEquivalentTo("test-workload"))
			Expect(checkRt.Status.TrackedResources[0].UID).Should(BeEquivalentTo("test-uid"))
		})

		It("Test handle succeed func", func() {
			ctx := context.Background()
			namespaceName := "default"
			rtName := "resourcetracker-v1-test-default"
			rt := v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: rtName,
				},
			}
			Expect(k8sClient.Create(ctx, &rt)).Should(BeNil())
			rollout := v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(&rt, v1beta1.ResourceTrackerKindVersionKind),
					},
				},
			}
			deploy := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"workload.oam.dev/type": "worker",
					},
					Name:      "test-workload",
					Namespace: namespaceName,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"workload.oam.dev/type": "worker",
					}},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"workload.oam.dev/type": "worker",
						}},
						Spec: v1.PodSpec{Containers: []v1.Container{{
							Image:   "busybox",
							Name:    "comp-name",
							Command: []string{"sleep", "1000"},
						},
						}}},
				},
			}
			u, err := util.Object2Unstructured(deploy)
			Expect(err).Should(BeNil())
			Expect(k8sClient.Create(ctx, u)).Should(BeNil())
			h := &handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout:        &rollout,
				targetWorkload: u,
			}
			Expect(h.handleFinalizeSucceed(ctx)).Should(BeNil())
			checkDeploy := new(appsv1.Deployment)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, checkDeploy)).Should(BeNil())
			Expect(len(checkDeploy.OwnerReferences)).Should(BeEquivalentTo(1))
			Expect(checkDeploy.OwnerReferences[0].Kind).Should(BeEquivalentTo(v1beta1.ResourceTrackerKind))
			Expect(checkDeploy.OwnerReferences[0].Name).Should(BeEquivalentTo(rtName))
			checkRt := v1beta1.ResourceTracker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rtName}, &checkRt)).Should(BeNil())
			Expect(len(checkRt.Status.TrackedResources)).Should(BeEquivalentTo(1))
			Expect(checkRt.Status.TrackedResources[0].Name).Should(BeEquivalentTo("test-workload"))
			Expect(checkRt.Status.TrackedResources[0].UID).Should(BeEquivalentTo(u.GetUID()))
		})

		It("Test handle failed func", func() {
			ctx := context.Background()
			namespaceName := "default"
			rtName := "resourcetracker-handle-failed-v1-test-default"
			rt := v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: rtName,
				},
			}
			Expect(k8sClient.Create(ctx, &rt)).Should(BeNil())
			rollout := v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(&rt, v1beta1.ResourceTrackerKindVersionKind),
					},
				},
			}
			deploy := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"workload.oam.dev/type": "worker",
					},
					Name:      "target-workload",
					Namespace: namespaceName,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"workload.oam.dev/type": "worker",
					}},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"workload.oam.dev/type": "worker",
						}},
						Spec: v1.PodSpec{Containers: []v1.Container{{
							Image:   "busybox",
							Name:    "comp-name",
							Command: []string{"sleep", "1000"},
						},
						}}},
				},
			}
			u, err := util.Object2Unstructured(deploy)
			Expect(err).Should(BeNil())
			source := u.DeepCopy()
			source.SetName("source-deploy")
			Expect(k8sClient.Create(ctx, u)).Should(BeNil())
			Expect(k8sClient.Create(ctx, source)).Should(BeNil())
			h := &handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout:        &rollout,
				targetWorkload: u,
				sourceWorkload: source,
			}
			Expect(h.handleFinalizeFailed(ctx)).Should(BeNil())
			targetDeploy := new(appsv1.Deployment)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, targetDeploy)).Should(BeNil())
			Expect(len(targetDeploy.OwnerReferences)).Should(BeEquivalentTo(1))
			Expect(targetDeploy.OwnerReferences[0].Kind).Should(BeEquivalentTo(v1beta1.ResourceTrackerKind))
			Expect(targetDeploy.OwnerReferences[0].Name).Should(BeEquivalentTo(rtName))

			sourceDeploy := new(appsv1.Deployment)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: source.GetNamespace(), Name: source.GetName()}, sourceDeploy)).Should(BeNil())
			Expect(len(sourceDeploy.OwnerReferences)).Should(BeEquivalentTo(1))
			Expect(sourceDeploy.OwnerReferences[0].Kind).Should(BeEquivalentTo(v1beta1.ResourceTrackerKind))
			Expect(sourceDeploy.OwnerReferences[0].Name).Should(BeEquivalentTo(rtName))

			checkRt := v1beta1.ResourceTracker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rtName}, &checkRt)).Should(BeNil())
			Expect(len(checkRt.Status.TrackedResources)).Should(BeEquivalentTo(2))
			Expect(checkRt.Status.TrackedResources[0].Name).Should(BeEquivalentTo(u.GetName()))
			Expect(checkRt.Status.TrackedResources[0].UID).Should(BeEquivalentTo(u.GetUID()))

			Expect(checkRt.Status.TrackedResources[1].Name).Should(BeEquivalentTo(source.GetName()))
			Expect(checkRt.Status.TrackedResources[1].UID).Should(BeEquivalentTo(source.GetUID()))
		})

		It("Test passResourceTrackerToWorkload func", func() {
			ctx := context.Background()
			namespaceName := "default"
			rtName := "resourcetracker-v1-test-default-pass-workload"
			rt := v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: rtName,
				},
			}
			Expect(k8sClient.Create(ctx, &rt)).Should(BeNil())
			rollout := v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(&rt, v1beta1.ResourceTrackerKindVersionKind),
					},
				},
			}
			deploy := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"workload.oam.dev/type": "worker",
					},
					Name:      "test-workload-pass-rt-wl",
					Namespace: namespaceName,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"workload.oam.dev/type": "worker",
					}},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"workload.oam.dev/type": "worker",
						}},
						Spec: v1.PodSpec{Containers: []v1.Container{{
							Image:   "busybox",
							Name:    "comp-name",
							Command: []string{"sleep", "1000"},
						},
						}}},
				},
			}
			u, err := util.Object2Unstructured(deploy)
			Expect(err).Should(BeNil())
			Expect(k8sClient.Create(ctx, u)).Should(BeNil())
			h := &handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout:        &rollout,
				targetWorkload: u,
			}
			Expect(h.passResourceTrackerToWorkload(ctx, u)).Should(BeNil())
			checkDeploy := new(appsv1.Deployment)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, checkDeploy)).Should(BeNil())
			Expect(len(checkDeploy.OwnerReferences)).Should(BeEquivalentTo(1))
			Expect(checkDeploy.OwnerReferences[0].Kind).Should(BeEquivalentTo(v1beta1.ResourceTrackerKind))
			Expect(checkDeploy.OwnerReferences[0].Name).Should(BeEquivalentTo(rtName))
			checkRt := v1beta1.ResourceTracker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rtName}, &checkRt)).Should(BeNil())
			Expect(len(checkRt.Status.TrackedResources)).Should(BeEquivalentTo(1))
			Expect(checkRt.Status.TrackedResources[0].Name).Should(BeEquivalentTo(u.GetName()))
			Expect(checkRt.Status.TrackedResources[0].UID).Should(BeEquivalentTo(u.GetUID()))
		})

		It("TestGetWorkloadReplicasNum", func() {
			deployName := "test-workload-get"
			deploy := appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      deployName,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: pointer.Int32(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test-container",
									Image: "test-image",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &deploy)).Should(BeNil())
			u := unstructured.Unstructured{}
			u.SetAPIVersion("apps/v1")
			u.SetKind("Deployment")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deployName, Namespace: namespace}, &u)).Should(BeNil())
			rep, err := getWorkloadReplicasNum(u)
			Expect(err).Should(BeNil())
			Expect(rep).Should(BeEquivalentTo(3))
		})
	})
})

const (
	compRevisionV1 = `
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  labels:
    app.oam.dev/component-revision-hash: ec7fede55af903d5
    controller.oam.dev/component: metrics-provider
  name: metrics-provider-v1
  namespace: rollout-test-namespace
data:
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata: 
    name: metrics-provider
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/component: metrics-provider
          app.oam.dev/name: test-rolling
          workload.oam.dev/type: webservice
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: metrics-provider
        template:
          metadata:
            labels:
              app.oam.dev/component: metrics-provider
          spec:
            containers:
            - command:
              - ./podinfo
              - stress-cpu=1
              image: stefanprodan/podinfo:4.0.6
              name: metrics-provider
              ports:
              - containerPort: 8080
`
	compRevisionV2 = `
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  labels:
    app.oam.dev/component-revision-hash: acdd0c76bd3c8f07
    controller.oam.dev/component: metrics-provider
  name: metrics-provider-v2
  namespace: rollout-test-namespace
data:
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    name: metrics-provider
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/component: metrics-provider
          app.oam.dev/name: test-rolling
          workload.oam.dev/type: webservice
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: metrics-provider
        template:
          metadata:
            labels:
              app.oam.dev/component: metrics-provider
          spec:
            containers:
            - command:
              - ./podinfo
              - stress-cpu=1
              image: stefanprodan/podinfo:5.0.2
              name: metrics-provider
              ports:
              - containerPort: 8080
`
)
