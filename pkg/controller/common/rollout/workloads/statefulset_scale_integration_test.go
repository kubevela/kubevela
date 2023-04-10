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

package workloads

import (
	"github.com/crossplane/crossplane-runtime/pkg/event"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("StatefulSet controller", func() {
	var (
		s              StatefulSetScaleController
		ns             corev1.Namespace
		name           string
		namespace      string
		statefulSet    appsv1.StatefulSet
		namespacedName client.ObjectKey
	)

	BeforeEach(func() {
		namespace = "rollout-ns"
		name = "rollout1"
		appRollout := v1alpha1.Rollout{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: v1alpha1.RolloutKind}, ObjectMeta: metav1.ObjectMeta{Name: name}}
		namespacedName = client.ObjectKey{Name: name, Namespace: namespace}

		s = StatefulSetScaleController{
			statefulSetController: statefulSetController{
				workloadController: workloadController{
					client: k8sClient,
					rolloutSpec: &v1alpha1.RolloutPlan{
						TargetSize: pointer.Int32(10),
						RolloutBatches: []v1alpha1.RolloutBatch{
							{
								Replicas: intstr.FromInt(1),
							},
							{
								Replicas: intstr.FromString("20%"),
							},
							{
								Replicas: intstr.FromString("80%"),
							},
						},
					},
					rolloutStatus:    &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
					parentController: &appRollout,
					recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
						WithAnnotations("controller", "AppRollout"),
				},
				targetNamespacedName: namespacedName,
			},
		}

		statefulSet = appsv1.StatefulSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "StatefulSet"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "staging"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nginx"}}},
				},
			},
		}

		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		By("Create a namespace")
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("clean up")
		k8sClient.Delete(ctx, &statefulSet)
	})

	Context("TestNewStatefulSetScaleController", func() {
		It("init a StatefulSet Scale Controller", func() {
			recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout")
			parentController := &v1alpha1.Rollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			rolloutSpec := &v1alpha1.RolloutPlan{
				RolloutBatches: []v1alpha1.RolloutBatch{{
					Replicas: intstr.FromInt(1),
				}},
			}
			rolloutStatus := &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState}
			workloadNamespacedName := client.ObjectKey{Name: name, Namespace: namespace}
			got := NewStatefulSetScaleController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus, workloadNamespacedName)
			controller := &StatefulSetScaleController{
				statefulSetController: statefulSetController{
					workloadController: workloadController{
						client:           k8sClient,
						recorder:         recorder,
						parentController: parentController,
						rolloutSpec:      rolloutSpec,
						rolloutStatus:    rolloutStatus,
					},
					targetNamespacedName: workloadNamespacedName,
				},
			}
			Expect(got).Should(Equal(controller))
		})
	})

	Context("TestVerifySpec", func() {
		It("rollout need a target size", func() {
			s.rolloutSpec.TargetSize = nil
			ligit, err := s.VerifySpec(ctx)
			Expect(ligit).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("without a target"))
		})

		It("could not fetch StatefulSet workload", func() {
			ligit, err := s.VerifySpec(ctx)
			Expect(ligit).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("rollout batch doesn't fit scale target", func() {
			By("Create a StatefulSet")
			statefulSet.Spec.Replicas = pointer.Int32(15)
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("Verify should fail as the scale batches don't match")
			s.rolloutSpec.RolloutBatches[2].Replicas = intstr.FromInt(10)
			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).ShouldNot(BeNil())
		})

		It("the StatefulSet is in the middle of scaling", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("verify should fail because replica does not match")
			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("the StatefulSet is in the middle of updating", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 1
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("verify should fail because replica are not upgraded")
			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("spec is valid", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 1
			statefulSet.Status.UpdatedReplicas = 1
			statefulSet.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("verify should pass and record the size")
			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.RolloutOriginalSize).Should(BeEquivalentTo(1))
		})
	})

	Context("TestInitialize", func() {
		It("could not fetch StatefulSet workload", func() {
			consistent, err := s.Initialize(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("failed to patch the owner of StatefulSet", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("initialize will fail because StatefulSet has wrong owner reference")
			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("workload StatefulSet is controlled by appRollout already", func() {
			By("Create a StatefulSet")
			statefulSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       v1alpha1.RolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.Bool(true),
			}})
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("initialize succeed without patching")
			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})

		It("successfully initialized StatefulSet", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("initialize succeeds")
			s.parentController.SetUID("1231586900")
			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestRolloutOneBatchPods", func() {
		It("could not fetch StatefulSet workload", func() {
			consistent, err := s.RolloutOneBatchPods(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("successfully rollout, current batch number is not equal to the expected one", func() {
			By("Create a StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("rollout the second batch of current StatefulSet")
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 0
			s.rolloutStatus.RolloutTargetSize = 10
			done, err := s.RolloutOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(3))
			Expect(k8sClient.Get(ctx, s.targetNamespacedName, &statefulSet)).Should(Succeed())
			Expect(*statefulSet.Spec.Replicas).Should(BeEquivalentTo(3))
		})
	})

	Context("TestCheckOneBatchPods", func() {
		It("could not fetch StatefulSet workload", func() {
			consistent, err := s.CheckOneBatchPods(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("current ready Pods are less than expected during scale-up", func() {
			By("Create the StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 3
			statefulSet.Status.ReadyReplicas = 3
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("checking should fail as not enough pod ready")
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 2
			s.rolloutStatus.RolloutTargetSize = 10
			done, err := s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))

			perc := intstr.FromString("20%")
			s.rolloutSpec.RolloutBatches[1] = v1alpha1.RolloutBatch{
				Replicas:       perc,
				MaxUnavailable: &perc,
			}
			By("checking one batch should succeed with unavailble allowed")
			done, err = s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))
		})

		It("current ready Pods are more than expected during scale-down", func() {
			By("Create the StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 10
			statefulSet.Status.ReadyReplicas = 10
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("checking should fail as not enough pod ready")
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 12
			s.rolloutStatus.RolloutTargetSize = 5
			done, err := s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))

			perc := intstr.FromString("20%")
			s.rolloutSpec.RolloutBatches[1] = v1alpha1.RolloutBatch{
				Replicas:       perc,
				MaxUnavailable: &perc,
			}
			By("checking one batch should still fail even with unavailble allowed")
			done, err = s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))
		})

		It("there are more pods increased during scale-up", func() {
			By("Create the StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 9
			statefulSet.Status.ReadyReplicas = 9
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("checking should pass when ready Pods are more than expected during scale-up")
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 2
			s.rolloutStatus.RolloutTargetSize = 10
			done, err := s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))
		})

		It("there are more pods decreased during scale-down", func() {
			By("Create the StatefulSet")
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())
			By("Update the StatefulSet status")
			statefulSet.Status.Replicas = 6
			statefulSet.Status.ReadyReplicas = 6
			Expect(k8sClient.Status().Update(ctx, &statefulSet)).Should(Succeed())

			By("checking should pass when ready Pods are less than expected during scale-down")
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 12
			s.rolloutStatus.RolloutTargetSize = 5
			done, err := s.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(s.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(statefulSet.Status.ReadyReplicas))
		})
	})

	Context("TestFinalizeOneBatch", func() {
		BeforeEach(func() {
			s.rolloutSpec.RolloutBatches[0] = v1alpha1.RolloutBatch{
				Replicas: intstr.FromInt(2),
			}
		})

		It("test illegal batch partition", func() {
			By("finalizing one batch")
			s.rolloutSpec.BatchPartition = pointer.Int32(2)
			s.rolloutStatus.CurrentBatch = 3
			done, err := s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("the current batch value in the status is greater than the batch partition"))
		})

		It("test finalize during scale-up", func() {
			By("finalizing one batch when there're too few upgraded Pods")
			s.rolloutStatus.UpgradedReplicas = 6
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 5
			s.rolloutStatus.RolloutTargetSize = 12
			done, err := s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("upgraded replica in the status is less than the lower bound"))

			By("finalizing one batch with enough upgraded Pods (lower bound)")
			s.rolloutStatus.UpgradedReplicas = 7
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("finalizing one batch with enough upgraded Pods (upper bound)")
			s.rolloutStatus.UpgradedReplicas = 9
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("finalizing one batch when there're too many upgraded Pods")
			s.rolloutStatus.UpgradedReplicas = 10
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("upgraded replica in the status is greater than the upper bound"))
		})

		It("test finalize during scale-down", func() {
			By("finalizing one batch when there're too many upgraded Pods")
			s.rolloutStatus.UpgradedReplicas = 13
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutOriginalSize = 14
			s.rolloutStatus.RolloutTargetSize = 2
			done, err := s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("upgraded replica in the status is greater than the upper bound"))

			By("finalizing one batch with enough upgraded Pods (upper bound)")
			s.rolloutStatus.UpgradedReplicas = 12
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("finalizing one batch with enough upgraded Pods (lower bound)")
			s.rolloutStatus.UpgradedReplicas = 9
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("finalizing one batch when there're too few upgraded Pods")
			s.rolloutStatus.UpgradedReplicas = 8
			done, err = s.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("upgraded replica in the status is less than the lower bound"))
		})
	})

	Context("TestFinalize", func() {
		It("failed to fetch StatefulSet workload", func() {
			By("finalizing")
			finalized := s.Finalize(ctx, true)
			Expect(finalized).Should(BeFalse())
		})

		It("Already finalize StatefulSet", func() {
			By("Create a StatefulSet")
			statefulSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       "notRollout",
				Name:       "def",
				UID:        "123456",
			}})
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("finalizing without patch")
			finalized := s.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})

		It("successfully to finalize StatefulSet", func() {
			By("Create a StatefulSet")
			statefulSet.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: v1alpha1.SchemeGroupVersion.String(),
					Kind:       v1alpha1.RolloutKind,
					Name:       "def",
					UID:        "123456",
					Controller: pointer.Bool(true),
				},
				{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "StatefulSet",
					Name:       "def",
					UID:        "654321",
				},
			})
			Expect(k8sClient.Create(ctx, &statefulSet)).Should(Succeed())

			By("finalizing with patch")
			finalized := s.Finalize(ctx, false)
			Expect(finalized).Should(BeTrue())
			Expect(k8sClient.Get(ctx, s.targetNamespacedName, &statefulSet)).Should(Succeed())
			Expect(len(statefulSet.GetOwnerReferences())).Should(BeEquivalentTo(1))
			Expect(statefulSet.GetOwnerReferences()[0].Kind).Should(Equal("StatefulSet"))
		})
	})
})
