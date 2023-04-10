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
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("cloneset controller", func() {
	var (
		c              CloneSetRolloutController
		ns             corev1.Namespace
		name           string
		namespace      string
		cloneSet       kruise.CloneSet
		namespacedName client.ObjectKey
	)

	BeforeEach(func() {
		namespace = "rollout-ns"
		name = "rollout1"
		appRollout := v1alpha1.Rollout{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: v1alpha1.RolloutKind}, ObjectMeta: metav1.ObjectMeta{Name: name}}
		namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
		c = CloneSetRolloutController{
			cloneSetController: cloneSetController{
				workloadController: workloadController{
					client: k8sClient,
					rolloutSpec: &v1alpha1.RolloutPlan{
						RolloutBatches: []v1alpha1.RolloutBatch{
							{
								Replicas: intstr.FromInt(1),
							},
						},
					},
					rolloutStatus:    &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
					parentController: &appRollout,
					recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("Rollout")).
						WithAnnotations("controller", "Rollout"),
				},
				targetNamespacedName: namespacedName,
			},
		}

		cloneSet = kruise.CloneSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: kruise.GroupVersion.String(), Kind: "CloneSet"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: kruise.CloneSetSpec{
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
		k8sClient.Delete(ctx, &cloneSet)
	})

	Context("TestNewCloneSetRolloutController", func() {
		It("init a CloneSet Rollout Controller", func() {
			recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout")
			parentController := &v1alpha1.Rollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			rolloutSpec := &v1alpha1.RolloutPlan{
				RolloutBatches: []v1alpha1.RolloutBatch{{
					Replicas: intstr.FromInt(1),
				},
				},
			}
			rolloutStatus := &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState}
			workloadNamespacedName := client.ObjectKey{Name: name, Namespace: namespace}
			got := NewCloneSetRolloutController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus, workloadNamespacedName)
			c := &CloneSetRolloutController{
				cloneSetController: cloneSetController{
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
			Expect(got).Should(Equal(c))
		})
	})

	Context("VerifySpec", func() {
		It("could not fetch CloneSet workload", func() {
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("verify rollout spec hash", func() {
			By("Create a CloneSet")
			cloneSet.Spec.UpdateStrategy.Paused = true
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
		})

		It("the cloneset is in the middle of updating", func() {
			By("Create a CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("setting a dummy pod identifier so it's different")
			c.rolloutStatus.LastAppliedPodTemplateIdentifier = "abc"

			By("verify should fail because it's not paused")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(Equal(fmt.Errorf("the cloneset rollout1 is in the middle of updating, need to be paused first")))
			Expect(consistent).Should(BeFalse())
		})

		It("spec is valid", func() {
			By("Create a CloneSet and set as paused")
			cloneSet.Spec.UpdateStrategy.Paused = true
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("setting a dummy pod identifier so it's different")
			c.rolloutStatus.LastAppliedPodTemplateIdentifier = "abc"

			By("verify should pass and record the size")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(1))
			Expect(c.rolloutStatus.RolloutOriginalSize).Should(BeEquivalentTo(1))
		})
	})

	Context("TestInitialize", func() {
		BeforeEach(func() {
			cloneSet.Spec.UpdateStrategy.Paused = true
		})

		It("could not fetch CloneSet workload", func() {
			consistent, err := c.Initialize(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("failed to patch the owner of CloneSet", func() {
			By("Create a CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("initialize will fail because cloneset has wrong owner reference")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("workload CloneSet is controlled by appRollout already", func() {
			By("Create a CloneSet")
			cloneSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       v1alpha1.RolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.Bool(true),
			}})
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("initialize succeed without patching")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(k8sClient.Get(ctx, c.targetNamespacedName, &cloneSet)).Should(Succeed())
			Expect(len(cloneSet.GetOwnerReferences())).Should(BeEquivalentTo(1))
		})

		It("successfully initialized CloneSet", func() {
			By("create cloneset")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("initialize succeeds")
			c.parentController.SetUID("1231586900")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(k8sClient.Get(ctx, c.targetNamespacedName, &cloneSet)).Should(Succeed())
			Expect(len(cloneSet.GetOwnerReferences())).Should(BeEquivalentTo(1))
		})
	})

	Context("TestRolloutOneBatchPods", func() {
		It("could not fetch CloneSet workload", func() {
			consistent, err := c.RolloutOneBatchPods(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("successfully rollout, current batch number is not equal to the expected one", func() {
			By("Create a CloneSet")
			cloneSet.Spec.Replicas = pointer.Int32(10)
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("rollout the second batch of current cloneset")
			c.rolloutStatus.CurrentBatch = 1
			c.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(1),
				},
				{
					Replicas: intstr.FromString("20%"),
				},
				{
					Replicas: intstr.FromString("80%"),
				},
			}
			done, err := c.RolloutOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(3))
			Expect(k8sClient.Get(ctx, c.targetNamespacedName, &cloneSet)).Should(Succeed())
			Expect(cloneSet.Spec.UpdateStrategy.Partition.IntValue()).Should(BeEquivalentTo(7))
		})
	})

	Context("TestCheckOneBatchPods", func() {
		BeforeEach(func() {
			cloneSet.Spec.Replicas = pointer.Int32(10)
			c.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(2),
				},
				{
					Replicas: intstr.FromString("20%"),
				},
				{
					Replicas: intstr.FromString("80%"),
				},
			}
		})

		It("could not fetch CloneSet workload", func() {
			consistent, err := c.CheckOneBatchPods(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("current ready Pod is less than expected", func() {
			By("Create the CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())
			By("Update the CloneSet status")
			cloneSet.Status.UpdatedReadyReplicas = 3
			cloneSet.Status.UpdatedReplicas = 4
			Expect(k8sClient.Status().Update(ctx, &cloneSet)).Should(Succeed())

			By("checking should fail as not enough pod ready")
			c.rolloutStatus.CurrentBatch = 1
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())
			Expect(c.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(cloneSet.Status.UpdatedReadyReplicas))
		})

		It("failed to check batch Pod when current batch number exceeds the expected ones", func() {
			By("Create a CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("checking")
			c.rolloutStatus.CurrentBatch = 3
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("currentBatch number exceeded the rolloutBatches spec"))
		})

		It("there are enough pods counting the unavailable", func() {
			By("Create the CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())
			By("Update the CloneSet status")
			cloneSet.Status.UpdatedReadyReplicas = 3
			cloneSet.Status.UpdatedReplicas = 4
			Expect(k8sClient.Status().Update(ctx, &cloneSet)).Should(Succeed())
			c.rolloutStatus.CurrentBatch = 1
			// set the rollout batch spec allow unavailable
			perc := intstr.FromString("20%")
			c.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(2),
				},
				{
					Replicas:       perc,
					MaxUnavailable: &perc,
				},
				{
					Replicas: intstr.FromString("80%"),
				},
			}
			By("checking one batch")
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(c.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(cloneSet.Status.UpdatedReadyReplicas))
		})

		It("there are enough pods ready", func() {
			By("Create the CloneSet")
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())
			By("Update the CloneSet status")
			cloneSet.Status.UpdatedReadyReplicas = 10
			cloneSet.Status.UpdatedReplicas = 10
			Expect(k8sClient.Status().Update(ctx, &cloneSet)).Should(Succeed())

			By("the second batch should pass when there are more pods upgraded already")
			c.rolloutStatus.CurrentBatch = 1
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(c.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(cloneSet.Status.UpdatedReadyReplicas))

			By("checking the last batch")
			c.rolloutStatus.CurrentBatch = 2
			done, err = c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
			Expect(c.rolloutStatus.UpgradedReadyReplicas).Should(BeEquivalentTo(cloneSet.Status.UpdatedReadyReplicas))
		})
	})

	Context("TestFinalizeOneBatch", func() {
		BeforeEach(func() {
			c.rolloutStatus.RolloutTargetSize = 10
			c.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(2),
				},
				{
					Replicas: intstr.FromString("20%"),
				},
				{
					Replicas: intstr.FromString("80%"),
				},
			}
		})

		It("test illegal batch partition", func() {
			By("finalizing one batch")
			c.rolloutSpec.BatchPartition = pointer.Int32(2)
			c.rolloutStatus.CurrentBatch = 3
			done, err := c.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("the current batch value in the status is greater than the batch partition"))
		})

		It("test too few upgraded", func() {
			By("finalizing one batch")
			c.rolloutStatus.UpgradedReplicas = 2
			c.rolloutStatus.CurrentBatch = 2
			done, err := c.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("is less than all the pods in the previous batch"))
		})

		It("test too many upgraded", func() {
			By("finalizing one batch")
			c.rolloutStatus.UpgradedReplicas = 5
			c.rolloutStatus.CurrentBatch = 1
			done, err := c.FinalizeOneBatch(ctx)
			Expect(done).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("is greater than all the pods in the current batch"))
		})

		It("test upgraded in the range", func() {
			By("finalizing one batch")
			c.rolloutStatus.UpgradedReplicas = 3
			c.rolloutStatus.CurrentBatch = 1
			done, err := c.FinalizeOneBatch(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalize", func() {
		It("failed to fetch CloneSet", func() {
			By("finalizing")
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeFalse())
		})

		It("Already finalize CloneSet", func() {
			By("Create a CloneSet")
			cloneSet.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       "notRollout",
				Name:       "def",
				UID:        "123456",
			}})
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("finalizing without patch")
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})

		It("successfully to finalize CloneSet", func() {
			By("Create a CloneSet")
			cloneSet.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: v1alpha1.SchemeGroupVersion.String(),
					Kind:       v1alpha1.RolloutKind,
					Name:       "def",
					UID:        "123456",
					Controller: pointer.Bool(true),
				},
				{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Deployment",
					Name:       "def",
					UID:        "998877745",
				},
			})
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("finalizing with patch")
			finalized := c.Finalize(ctx, false)
			Expect(finalized).Should(BeTrue())
			Expect(k8sClient.Get(ctx, c.targetNamespacedName, &cloneSet)).Should(Succeed())
			Expect(len(cloneSet.GetOwnerReferences())).Should(BeEquivalentTo(1))
			Expect(cloneSet.GetOwnerReferences()[0].Kind).Should(Equal("Deployment"))
			Expect(cloneSet.Spec.UpdateStrategy.Paused).Should(BeTrue())
		})
	})
})
