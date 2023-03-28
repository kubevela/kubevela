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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("deployment controller", func() {
	var (
		c                    DeploymentRolloutController
		ns                   corev1.Namespace
		namespaceName        string
		sourceName           string
		targetName           string
		sourceDeploy         appsv1.Deployment
		targetDeploy         appsv1.Deployment
		sourceNamespacedName client.ObjectKey
		targetNamespacedName client.ObjectKey
	)

	BeforeEach(func() {
		By("setup before each test")
		namespaceName = "rollout-ns"
		sourceName = "source-dep"
		targetName = "target-dep"
		appRollout := v1alpha1.Rollout{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: v1alpha1.RolloutKind}, ObjectMeta: metav1.ObjectMeta{Name: "test-rollout"}}
		sourceNamespacedName = client.ObjectKey{Name: sourceName, Namespace: namespaceName}
		targetNamespacedName = client.ObjectKey{Name: targetName, Namespace: namespaceName}
		c = DeploymentRolloutController{
			deploymentController: deploymentController{
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

		targetDeploy = appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: targetName},
			Spec: appsv1.DeploymentSpec{
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

		sourceDeploy = appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: sourceName},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(10),
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
				Name: namespaceName,
			},
		}
		By("Create a namespace")
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("clean up after each test")
		k8sClient.Delete(ctx, &sourceDeploy)
		// Delete the target
		k8sClient.Delete(ctx, &targetDeploy)
	})

	Context("TestNewDeploymentRolloutController", func() {
		It("init a Deployment Rollout Controller", func() {
			recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout")
			parentController := &v1alpha1.Rollout{ObjectMeta: metav1.ObjectMeta{Name: sourceName}}
			rolloutSpec := &v1alpha1.RolloutPlan{
				RolloutBatches: []v1alpha1.RolloutBatch{{
					Replicas: intstr.FromInt(1),
				},
				},
			}
			rolloutStatus := &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState}
			workloadNamespacedName := client.ObjectKey{Name: sourceName, Namespace: namespaceName}
			got := NewDeploymentRolloutController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus,
				workloadNamespacedName, workloadNamespacedName)
			c := &DeploymentRolloutController{
				deploymentController: deploymentController{
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
			Expect(got).Should(Equal(c))
		})
	})

	Context("VerifySpec", func() {
		It("Could not fetch both deployment workload", func() {
			By("Create only the source deployment")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("verify target size value", func() {
			By("Create the deployments, source size is 10")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			c.rolloutSpec.TargetSize = pointer.Int32(8)
			consistent, err := c.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("less than source size"))
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(0))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("verify rollout spec hash", func() {
			By("Create the deployments")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Spec.Replicas = pointer.Int32(1)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetHash, _ := utils.ComputeSpecHash(targetDeploy.Spec)
			c.rolloutStatus.LastAppliedPodTemplateIdentifier = targetHash
			By("Verify should fail because the the target hash didn't change")
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("there is no difference between the source and target"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("verify rolloutBatch replica value", func() {
			By("Create the source deployment")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("modify rollout batches")
			c.rolloutSpec.RolloutBatches = []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromInt(2),
				},
				{
					Replicas: intstr.FromInt(13),
				},
			}
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())

			By("set the correct rollout target size")
			c.rolloutSpec.TargetSize = pointer.Int32(15)
			consistent, err = c.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err.Error()).ShouldNot(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(15))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the deployment need to be stable if not paused", func() {
			By("create the source deployment with many pods")
			sourceDeploy.Spec.Replicas = pointer.Int32(50)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should fail b/c source is not stable")
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("is still being reconciled, need to be paused or stable"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(50))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the deployment don't need to be paused if stable", func() {
			By("Create the source deployment with none")
			var sourceReplica int32 = 6
			sourceDeploy.Spec.Replicas = &sourceReplica
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			// test environment doesn't have deployment controller, has to fake it
			sourceDeploy.Status.Replicas = sourceReplica // this has to pass batch check
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())

			targetDeploy.Spec.Replicas = pointer.Int32(0)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should not fail b/c of deployment not stable")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(sourceReplica))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).ShouldNot(BeEmpty())
		})

		It("deployment should not have controller", func() {
			By("Create deployments")
			sourceDeploy.Spec.Paused = true
			sourceDeploy.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       v1alpha1.RolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.Bool(true),
			}})
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("verify should fail because deployment still has a controller")
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("has a controller owner"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("spec is valid", func() {
			By("Create a deployment")
			sourceDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should succeed")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())
		})
	})

	Context("TestInitialize", func() {
		It("failed to fetch Deployment", func() {
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("failed to claim Deployment as the owner reference is ill-formated", func() {
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(Succeed())
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("successfully initialized Deployment", func() {
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(Succeed())
			c.parentController.SetUID("abcdedg")
			c.rolloutStatus.RolloutTargetSize = 12
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is claimed")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(len(sourceDeploy.GetOwnerReferences())).Should(Equal(1))
			Expect(sourceDeploy.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha1.RolloutKindVersionKind.Kind))
			Expect(sourceDeploy.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(c.parentController.GetUID()))
			Expect(sourceDeploy.Spec.Paused).Should(BeFalse())
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(10))
			By("Verify the target deployment is claimed and init to zero")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(len(targetDeploy.GetOwnerReferences())).Should(Equal(1))
			Expect(targetDeploy.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha1.RolloutKindVersionKind.Kind))
			Expect(targetDeploy.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(c.parentController.GetUID()))
			Expect(targetDeploy.Spec.Paused).Should(BeFalse())
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(2))
		})

		It("successfully initialized deployment on resume/revert case", func() {
			sourceDeploy.Spec.Replicas = pointer.Int32(7)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			targetDeploy.Spec.Replicas = pointer.Int32(5)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(Succeed())
			c.parentController.SetUID("abcdedg")
			c.rolloutStatus.RolloutTargetSize = 10
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is claimed")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(len(sourceDeploy.GetOwnerReferences())).Should(Equal(1))
			Expect(sourceDeploy.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha1.RolloutKindVersionKind.Kind))
			Expect(sourceDeploy.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(c.parentController.GetUID()))
			Expect(sourceDeploy.Spec.Paused).Should(BeFalse())
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(7))
			By("Verify the target deployment is claimed with the right amount of replicas")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(len(targetDeploy.GetOwnerReferences())).Should(Equal(1))
			Expect(targetDeploy.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha1.RolloutKindVersionKind.Kind))
			Expect(targetDeploy.GetOwnerReferences()[0].UID).Should(BeEquivalentTo(c.parentController.GetUID()))
			Expect(targetDeploy.Spec.Paused).Should(BeFalse())
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(
				c.rolloutStatus.RolloutTargetSize - *sourceDeploy.Spec.Replicas))
		})
	})

	Context("TestRolloutOneBatchPods", func() {
		It("failed to fetch Deployment", func() {
			initialized, err := c.RolloutOneBatchPods(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("rollout increase first, first batch", func() {
			By("Create the source deployment")
			sourceDeploy.Spec.Replicas = pointer.Int32(10)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(0)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("rollout the first half")
			// rolloutRelaxSpec doesn't set the RolloutStrategy
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutStatus.CurrentBatch = 0
			c.rolloutStatus.RolloutTargetSize = *sourceDeploy.Spec.Replicas
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(10))
			By("Verify the target deployment is scaled up first")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(2))
			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("rollout the second half after fake target status update")
			// Replicas has to be more than ReadyReplicas
			targetDeploy.Status.Replicas = *targetDeploy.Spec.Replicas
			targetDeploy.Status.ReadyReplicas = *targetDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is scaled down")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(8))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(2))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(2))
		})

		It("rollout decrease first, first batch", func() {
			By("Create the source deployment")
			sourceDeploy.Spec.Replicas = pointer.Int32(10)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(0)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("rollout the first half to decrease first")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 0
			c.rolloutStatus.RolloutTargetSize = *sourceDeploy.Spec.Replicas
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is scaled down first")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(8))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(0))
			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("rollout the second half after fake target status update")
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(8))
			By("Verify the target deployment is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(2))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(2))
		})

		It("rollout increase first, last batch", func() {
			By("Create the source deployment")
			sourceDeploy.Spec.Replicas = pointer.Int32(4)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(6)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("rollout the first half, omit strategy")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 3
			c.rolloutStatus.RolloutTargetSize = *sourceDeploy.Spec.Replicas + *targetDeploy.Spec.Replicas
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(4))
			By("Verify the target deployment is scaled up first")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(10))
			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("try to rollout the second half, fail because target status didn't meet")
			targetDeploy.Status.Replicas = *targetDeploy.Spec.Replicas
			targetDeploy.Status.ReadyReplicas = *targetDeploy.Spec.Replicas - 1
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("rollout the second half after fake target status update")
			targetDeploy.Status.Replicas = *targetDeploy.Spec.Replicas
			targetDeploy.Status.ReadyReplicas = *targetDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(err).Should(BeNil())
			Expect(rolloutDone).Should(BeTrue())
			By("Verify the source deployment is scaled down")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(0))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(10))
		})

		It("rollout decrease first, last batch", func() {
			By("Create the source deployment")
			sourceDeploy.Spec.Replicas = pointer.Int32(4)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			// set status as default is 0
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(6)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("rollout the first half to decrease first")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 3
			c.rolloutStatus.RolloutTargetSize = *sourceDeploy.Spec.Replicas + *targetDeploy.Spec.Replicas
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is scaled down first")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(0))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			By("try to rollout the second half, fail because target status didn't change")
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("try to rollout the second half, fail because target status didn't meet")
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas + 1
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("rollout the second half after fake source status update")
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			c.sourceDeploy = appsv1.Deployment{}
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(0))
			By("Verify the target deployment is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(10))
		})

		It("rollout increase first, revert case", func() {
			By("Create the deployments in the middle of rolling out")
			sourceDeploy.Spec.Replicas = pointer.Int32(6)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(14)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Status.Replicas = *targetDeploy.Spec.Replicas
			targetDeploy.Status.ReadyReplicas = *targetDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			By("rollout the first batch to start the revert")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 0
			c.rolloutStatus.RolloutTargetSize = 20
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(4))
			By("rollout the second batch")
			c.rolloutStatus.CurrentBatch = 1
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))
			By("rollout the third batch")
			c.rolloutStatus.CurrentBatch = 2
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(14))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(8))
			By("rollout the fourth batch")
			c.rolloutStatus.CurrentBatch = 3
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			By("Verify the target deployment is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(20))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(8))
		})

		It("rollout decrease first, revert case", func() {
			By("Create the deployments in the middle of rolling out")
			sourceDeploy.Spec.Replicas = pointer.Int32(14)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			sourceDeploy.Status.Replicas = *sourceDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			By("Create the target deployment")
			targetDeploy.Spec.Replicas = pointer.Int32(6)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Status.Replicas = *targetDeploy.Spec.Replicas
			targetDeploy.Status.ReadyReplicas = *targetDeploy.Spec.Replicas
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			By("rollout the first batch to start the revert")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 0
			c.rolloutStatus.RolloutTargetSize = 20
			rolloutDone, err := c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(14))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(4))
			By("rollout the second batch")
			c.rolloutStatus.CurrentBatch = 1
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(14))
			By("Verify the target deployment is not touched")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))
			By("rollout the third batch")
			c.rolloutStatus.CurrentBatch = 2
			rolloutDone, err = c.RolloutOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("Verify the source deployment is not touched")
			Expect(k8sClient.Get(ctx, sourceNamespacedName, &sourceDeploy))
			Expect(*sourceDeploy.Spec.Replicas).Should(BeEquivalentTo(12))
			By("Verify the target deployment is scaled up")
			Expect(k8sClient.Get(ctx, targetNamespacedName, &targetDeploy))
			Expect(*targetDeploy.Spec.Replicas).Should(BeEquivalentTo(6))
			Expect(c.rolloutStatus.UpgradedReplicas).Should(BeEquivalentTo(6))
		})
	})

	Context("TestCheckOneBatchPods", func() {
		It("failed to fetch Deployment", func() {
			initialized, err := c.CheckOneBatchPods(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("check batches with rollout increase first", func() {
			By("Create the source deployment")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Create the target deployment")
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("check first batch")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.IncreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 0
			c.rolloutStatus.RolloutTargetSize = 20
			By("source more than goal")
			sourceDeploy.Status.Replicas = 17
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err := c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("source meet goal")
			sourceDeploy.Status.Replicas = 16
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err = c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("source less than goal")
			sourceDeploy.Status.Replicas = 15
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			rolloutDone, err = c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
		})

		It("check batches with rollout decrease first", func() {
			By("Create the source deployment")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Create the target deployment")
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("check first batch")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 1
			c.rolloutStatus.RolloutTargetSize = 20
			By("target more than goal")
			targetDeploy.Status.Replicas = 9
			targetDeploy.Status.ReadyReplicas = 7
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			rolloutDone, err := c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("target meet goal")
			targetDeploy.Status.Replicas = 7
			targetDeploy.Status.ReadyReplicas = 6
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			rolloutDone, err = c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
			By("target less than goal")
			targetDeploy.Status.Replicas = 6
			targetDeploy.Status.ReadyReplicas = 5
			Expect(k8sClient.Status().Update(ctx, &targetDeploy)).Should(Succeed())
			rolloutDone, err = c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeFalse())
			Expect(err).Should(BeNil())
			By("target less than goal but unavailable allowed")
			unavil := intstr.FromString("10%")
			c.rolloutSpec.RolloutBatches[1].MaxUnavailable = &unavil
			rolloutDone, err = c.CheckOneBatchPods(ctx)
			Expect(rolloutDone).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalizeOneBatch", func() {
		It("failed to fetch Deployment", func() {
			finalized, err := c.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("test rollout batch configured correctly", func() {
			By("Create the deployments")
			sourceDeploy.Spec.Replicas = pointer.Int32(8)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Spec.Replicas = pointer.Int32(5)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("Fail if the targets don't add up")
			c.rolloutSpec = rolloutRelaxSpec
			c.rolloutSpec.RolloutStrategy = v1alpha1.DecreaseFirstRolloutStrategyType
			c.rolloutStatus.CurrentBatch = 1
			c.rolloutStatus.RolloutTargetSize = 10
			finalized, err := c.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeFalse())
			Expect(err.Error()).Should(ContainSubstring("deployment targets don't match total rollout"))
			By("Success if they do")
			// sum of target and source
			c.rolloutStatus.RolloutTargetSize = 13
			finalized, err = c.FinalizeOneBatch(ctx)
			Expect(finalized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalize", func() {
		It("failed to fetch deployment", func() {
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeFalse())
		})

		It("release success without ownership", func() {
			By("Create the deployments")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("no op success if we are not the owner")
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})

		It("release success as the owner", func() {
			By("Create the deployments")
			sourceDeploy.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       v1alpha1.RolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.Bool(true),
			}})
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       v1alpha1.RolloutKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.Bool(true),
			}})
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			By("success if we are the owner")
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())
		})
	})
})
