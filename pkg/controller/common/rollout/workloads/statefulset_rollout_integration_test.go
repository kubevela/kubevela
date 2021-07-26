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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
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
		ns                   corev1.Namespace
		namespace            string
		sourceName           string
		targetName           string
		sourceNamespacedName client.ObjectKey
		targetNamespacedName client.ObjectKey
		s                    StatefulSetRolloutController
		sourceStatefulSet    appsv1.StatefulSet
		targetStatefulSet    appsv1.StatefulSet
	)

	BeforeEach(func() {
		By("setup before each test")
		namespace = "rollout-ns"
		sourceName = "source-sts"
		targetName = "target-sts"
		sourceNamespacedName = client.ObjectKey{Name: sourceName, Namespace: namespace}
		targetNamespacedName = client.ObjectKey{Name: targetName, Namespace: namespace}

		appRollout := v1beta1.AppRollout{TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.AppRolloutKind}, ObjectMeta: metav1.ObjectMeta{Name: "test-rollout"}}

		s = StatefulSetRolloutController{
			statefulSetController: statefulSetController{
				workloadController: workloadController{
					client: k8sClient,
					rolloutSpec: &v1alpha1.RolloutPlan{
						RolloutBatches: []v1alpha1.RolloutBatch{
							{
								Replicas: intstr.FromInt(2),
							},
							{
								Replicas: intstr.FromInt(3),
							},
							{
								Replicas: intstr.FromString("50%"),
							},
						},
					},
					rolloutStatus:    &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
					parentController: &appRollout,
					recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
						WithAnnotations("controller", "AppRollout"),
				},
				targetNamespacedName: targetNamespacedName,
			},
			sourceNamespacedName: sourceNamespacedName,
		}

		targetStatefulSet = appsv1.StatefulSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "StatefulSet"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: targetName},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "staging"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: targetName,
						Image: "stefanprodan/podinfo:5.0.3"}}},
				},
			},
		}

		sourceStatefulSet = appsv1.StatefulSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "StatefulSet"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: sourceName},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(10),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "staging"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: sourceName,
						Image: "stefanprodan/podinfo:4.0.6"}}},
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
		By("clean up after each test")
		k8sClient.Delete(ctx, &sourceStatefulSet)
		k8sClient.Delete(ctx, &targetStatefulSet)
	})

	Context("TestNewStatefulSetRolloutController", func() {
		It("init a StatefulSet Rollout Controller", func() {
			recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout")
			parentController := &v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: sourceName}}
			rolloutSpec := &v1alpha1.RolloutPlan{
				RolloutBatches: []v1alpha1.RolloutBatch{{
					Replicas: intstr.FromInt(1),
				},
				},
			}
			rolloutStatus := &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState}
			workloadNamespacedName := client.ObjectKey{Name: sourceName, Namespace: namespace}
			got := NewStatefulSetRolloutController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus,
				workloadNamespacedName, workloadNamespacedName)
			s := &StatefulSetRolloutController{
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
				sourceNamespacedName: workloadNamespacedName,
			}
			Expect(got).Should(Equal(s))
		})
	})

	Context("TestVerifySpec", func() {
		It("failed to fetch statefulSets", func() {
			By("Create only the source statefulSet")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("verify target size value", func() {
			By("Create the statefulSets")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			s.rolloutSpec.TargetSize = pointer.Int32Ptr(8)
			consistent, err := s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("less than source size"))
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(0))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("verify rollout spec hash", func() {
			By("Create the statefulSets")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(1)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			targetHash, _ := utils.ComputeSpecHash(targetStatefulSet.Spec)
			s.rolloutStatus.LastAppliedPodTemplateIdentifier = targetHash
			By("Verify should fail because the the target hash didn't change")
			consistent, err := s.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("there is no difference between the source and target"))
			Expect(consistent).Should(BeFalse())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("verify rolloutBatch replica value", func() {
			By("Create the source statefulSet")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("modify rollout batches")
			s.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(2),
				},
				{
					Replicas: intstr.FromInt(13),
				},
			}
			consistent, err := s.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(consistent).Should(BeFalse())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())

			By("set the correct rollout target size")
			s.rolloutSpec.TargetSize = pointer.Int32Ptr(15)
			consistent, err = s.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err.Error()).ShouldNot(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(15))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the statefulSet should fail when StatefulSets are not stable", func() {
			By("create the source statefulSet with many pods")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(50)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should fail since the source statefulSet is not stable")
			consistent, err := s.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("is still being reconciled, need to be stable"))
			Expect(consistent).Should(BeFalse())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(50))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the statefulSet need to be stable", func() {
			By("Create statefulSets")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceStatefulSet.Status.Replicas = 10
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			var targetReplicas int32 = 15
			targetStatefulSet.Spec.Replicas = &targetReplicas
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Status.Replicas = targetReplicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())

			By("the statefulSets are stable")
			consistent, err := s.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).ShouldNot(BeEmpty())
		})

		It("statefulSet should not have controller", func() {
			By("Create statefulSets")
			sourceStatefulSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.BoolPtr(true),
			}})
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceStatefulSet.Status.Replicas = 10
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			var targetReplicas int32 = 15
			targetStatefulSet.Spec.Replicas = &targetReplicas
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Status.Replicas = targetReplicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())

			By("verify should fail because statefulSet still has a controller")
			consistent, err := s.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("has a controller owner"))
			Expect(consistent).Should(BeFalse())
			Expect(s.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the spec is valid", func() {
			By("Create a statefulSet")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			var targetReplicas int32 = 15
			targetStatefulSet.Spec.Replicas = &targetReplicas
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Status.Replicas = targetReplicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())
			// Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should succeed")
			consistent, err := s.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
		})
	})

	Context("TestInitialize", func() {
		It("failed to fetch StatefulSet", func() {
			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("failed to claim StatefulSet as the owner reference is ill-formated", func() {
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(Succeed())

			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("successfully initialized StatefulSet", func() {
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(Succeed())
			s.parentController.SetUID("abcdedg")
			s.rolloutStatus.RolloutTargetSize = 12

			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is claimed")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(len(sourceStatefulSet.GetOwnerReferences())).Should(Equal(1))
			Expect(sourceStatefulSet.GetOwnerReferences()[0].Kind).Should(Equal(v1beta1.AppRolloutKindVersionKind.Kind))
			Expect(sourceStatefulSet.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(s.parentController.GetUID()))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(10))

			By("Verify the target StatefulSet is claimed and init to zero")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(len(targetStatefulSet.GetOwnerReferences())).Should(Equal(1))
			Expect(targetStatefulSet.GetOwnerReferences()[0].Kind).Should(Equal(v1beta1.AppRolloutKindVersionKind.Kind))
			Expect(targetStatefulSet.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(s.parentController.GetUID()))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(2))
		})

		It("successfully initialized StatefulSet on resume/revert case", func() {
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(7)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(Succeed())
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(5)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(Succeed())
			s.parentController.SetUID("abcdedg")
			s.rolloutStatus.RolloutTargetSize = 10

			initialized, err := s.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is claimed")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(len(sourceStatefulSet.GetOwnerReferences())).Should(Equal(1))
			Expect(sourceStatefulSet.GetOwnerReferences()[0].Kind).Should(Equal(v1beta1.AppRolloutKindVersionKind.Kind))
			Expect(sourceStatefulSet.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(s.parentController.GetUID()))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(7))

			By("Verify the target StatefulSet is claimed with the right amount of replicas")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(len(targetStatefulSet.GetOwnerReferences())).Should(Equal(1))
			Expect(targetStatefulSet.GetOwnerReferences()[0].Kind).Should(Equal(v1beta1.AppRolloutKindVersionKind.Kind))
			Expect(targetStatefulSet.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(s.parentController.GetUID()))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(
				s.rolloutStatus.RolloutTargetSize - *sourceStatefulSet.Spec.Replicas))
		})
	})

	Context("TestRolloutOneBatchPods", func() {
		It("failed to fetch StatefulSets", func() {
			initialized, err := s.RolloutOneBatchPods(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("rollout increase first, first batch", func() {
			By("Create the source StatefulSet")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(10)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(0)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("rollout the first half")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 0
			s.rolloutStatus.RolloutTargetSize = *sourceStatefulSet.Spec.Replicas
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(10))

			By("Verify the target StatefulSet is scaled up first")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(2))

			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("rollout the second half after fake target status update")
			// Replicas has to be more than ReadyReplicas
			targetStatefulSet.Status.Replicas = *targetStatefulSet.Spec.Replicas
			targetStatefulSet.Status.ReadyReplicas = *targetStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is scaled down")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(8))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(2))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(2))
		})

		It("rollout decrease first, first batch", func() {
			By("Create the source StatefulSet")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(10)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(0)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("rollout the first half to decrease first")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 0
			s.rolloutStatus.RolloutTargetSize = *sourceStatefulSet.Spec.Replicas
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is scaled down first")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(8))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(0))

			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("rollout the second half after fake target status update")
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(8))

			By("Verify the target StatefulSet is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(2))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(2))
		})

		It("rollout increase first, last batch", func() {
			By("Create the source StatefulSet")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(4)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(6)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("rollout the first half, omit strategy")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 3
			s.rolloutStatus.RolloutTargetSize = *sourceStatefulSet.Spec.Replicas + *targetStatefulSet.Spec.Replicas
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(4))

			By("Verify the target StatefulSet is scaled up first")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(10))

			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("try to rollout the second half, fail because target status didn't meet")
			targetStatefulSet.Status.Replicas = *targetStatefulSet.Spec.Replicas
			targetStatefulSet.Status.ReadyReplicas = *targetStatefulSet.Spec.Replicas - 1
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("rollout the second half after fake target status update")
			targetStatefulSet.Status.Replicas = *targetStatefulSet.Spec.Replicas
			targetStatefulSet.Status.ReadyReplicas = *targetStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(err).Should(BeNil())
			Expect(rolloutDone).Should(BeTrue())

			By("Verify the source StatefulSet is scaled down")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(0))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(10))
		})

		It("rollout decrease first, last batch", func() {
			By("Create the source StatefulSet")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(4)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			// set status as default is 0
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(6)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("rollout the first half to decrease first")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 3
			s.rolloutStatus.RolloutTargetSize = *sourceStatefulSet.Spec.Replicas + *targetStatefulSet.Spec.Replicas
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is scaled down first")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(0))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))

			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("try to rollout the second half, fail because target status didn't meet")
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas + 1
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("rollout the second half after fake source status update")
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			s.sourceStatefulSet = &appsv1.StatefulSet{}
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(0))

			By("Verify the target StatefulSet is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(10))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(10))
		})

		It("rollout increase first, revert case", func() {
			By("Create the StatefulSets in the middle of rolling out")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(6)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(14)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Status.Replicas = *targetStatefulSet.Spec.Replicas
			targetStatefulSet.Status.ReadyReplicas = *targetStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())

			By("rollout the first batch to start the revert")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 0
			s.rolloutStatus.RolloutTargetSize = 20
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(4))

			By("rollout the second batch")
			s.rolloutStatus.CurrentBatch = 1
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))

			By("rollout the third batch")
			s.rolloutStatus.CurrentBatch = 2
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(8))

			By("rollout the fourth batch")
			s.rolloutStatus.CurrentBatch = 3
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))

			By("Verify the target StatefulSet is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(20))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(8))
		})

		It("rollout decrease first, revert case", func() {
			By("Create the StatefulSets in the middle of rolling out")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(14)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceStatefulSet.Status.Replicas = *sourceStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())

			By("Create the target StatefulSet")
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(6)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Status.Replicas = *targetStatefulSet.Spec.Replicas
			targetStatefulSet.Status.ReadyReplicas = *targetStatefulSet.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())

			By("rollout the first batch to start the revert")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 0
			s.rolloutStatus.RolloutTargetSize = 20
			rolloutDone, err := s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(14))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(4))

			By("rollout the second batch")
			s.rolloutStatus.CurrentBatch = 1
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(14))

			By("Verify the target StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))

			By("rollout the third batch")
			s.rolloutStatus.CurrentBatch = 2
			rolloutDone, err = s.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("Verify the source StatefulSet is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceStatefulSet))
			Expect(*sourceStatefulSet.Spec.Replicas).Should(BeEquivalentTo(12))

			By("Verify the target StatefulSet is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetStatefulSet))
			Expect(*targetStatefulSet.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(s.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))
		})
	})

	Context("TestCheckOneBatchPods", func() {
		It("failed to fetch StatefulSets", func() {
			initialized, err := s.CheckOneBatchPods(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("check batches with rollout increase first", func() {
			By("Create the source StatefulSet")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create the target StatefulSet")
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("check first batch")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 0
			s.rolloutStatus.RolloutTargetSize = 20

			By("source more than goal")
			sourceStatefulSet.Status.Replicas = 17
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err := s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("source meet goal")
			sourceStatefulSet.Status.Replicas = 16
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err = s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("source less than goal")
			sourceStatefulSet.Status.Replicas = 15
			Expect(k8sClient.Status().Update(ctx, &sourceStatefulSet)).Should(Succeed())
			rolloutDone, err = s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
		})

		It("check batches with rollout decrease first", func() {
			By("Create the source StatefulSet")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create the target StatefulSet")
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("check first batch")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutTargetSize = 20

			By("target more than goal")
			targetStatefulSet.Status.Replicas = 9
			targetStatefulSet.Status.ReadyReplicas = 7
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())
			rolloutDone, err := s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("target meet goal")
			targetStatefulSet.Status.Replicas = 7
			targetStatefulSet.Status.ReadyReplicas = 6
			Expect(k8sClient.Status().Update(ctx, &targetStatefulSet)).Should(Succeed())
			rolloutDone, err = s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("target less than goal")
			unavil := intstr.FromString("10%")
			s.rolloutSpec.RolloutBatches[1].MaxUnavailable = &unavil
			rolloutDone, err = s.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalizeOneBatch", func() {
		It("failed to fetch StatefulSets", func() {
			finalized, err := s.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("test rollout batch configured correctly", func() {
			By("Create the StatefulSets")
			sourceStatefulSet.Spec.Replicas = pointer.Int32Ptr(8)
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.Spec.Replicas = pointer.Int32Ptr(5)
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Fail if the targets don't add up")
			s.rolloutSpec = rolloutRelaxSpec
			s.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			s.rolloutStatus.CurrentBatch = 1
			s.rolloutStatus.RolloutTargetSize = 10
			finalized, err := s.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("StatefulSet targets don't match total rollout"))

			By("Success if they do")
			// sum of target and source
			s.rolloutStatus.RolloutTargetSize = 13
			finalized, err = s.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalize", func() {
		It("failed to fetch StatefulSets", func() {
			finalized := s.Finalize(ctx, true)
			Expect(finalized).Should(BeFalse())
		})

		It("release success without ownership", func() {
			By("Create the StatefulSets")
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("no op success if we are not the owner")
			finalized := s.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})

		It("release success as the owner", func() {
			By("Create the StatefulSets")
			sourceStatefulSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.AppRolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.BoolPtr(true),
			}})
			Expect(k8sClient.Create(ctx, &sourceStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetStatefulSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.BoolPtr(true),
			}})
			Expect(k8sClient.Create(ctx, &targetStatefulSet)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("success if we are the owner")
			finalized := s.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})
	})
})
