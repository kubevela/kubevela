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
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("deployment controller", func() {
	var (
		c                    DeploymentController
		ns                   corev1.Namespace
		namespaceName        string
		sourceName           string
		targetName           string
		sourceDeploy         v1.Deployment
		targetDeploy         v1.Deployment
		sourceNamespacedName client.ObjectKey
		targetNamespacedName client.ObjectKey
	)

	BeforeEach(func() {
		By("setup before each test")
		namespaceName = "rollout-ns"
		sourceName = "source-dep"
		targetName = "target-dep"
		appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: "test-rollout"}}
		sourceNamespacedName = client.ObjectKey{Name: sourceName, Namespace: namespaceName}
		targetNamespacedName = client.ObjectKey{Name: targetName, Namespace: namespaceName}
		c = DeploymentController{
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
			rolloutStatus:        &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
			parentController:     &appRollout,
			sourceNamespacedName: sourceNamespacedName,
			targetNamespacedName: targetNamespacedName,
			recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout"),
		}

		targetDeploy = v1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: targetName},
			Spec: v1.DeploymentSpec{
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

		sourceDeploy = v1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: sourceName},
			Spec: v1.DeploymentSpec{
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

	Context("TestNewDeploymentController", func() {
		It("init a Deployment Controller", func() {
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
			workloadNamespacedName := client.ObjectKey{Name: sourceName, Namespace: namespaceName}
			got := NewDeploymentController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus,
				workloadNamespacedName, workloadNamespacedName)
			c := &DeploymentController{
				client:               k8sClient,
				recorder:             recorder,
				parentController:     parentController,
				rolloutSpec:          rolloutSpec,
				rolloutStatus:        rolloutStatus,
				sourceNamespacedName: workloadNamespacedName,
				targetNamespacedName: workloadNamespacedName,
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

		It("verify rollout spec hash", func() {
			By("Create the deployments")
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			targetDeploy.Spec.Replicas = pointer.Int32Ptr(1)
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
					Replicas: intstr.FromInt(3),
				},
			}
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).Should(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(10))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())

			By("set the correct rollout target size")
			c.rolloutSpec.TargetSize = pointer.Int32Ptr(5)
			consistent, err = c.VerifySpec(ctx)
			Expect(consistent).Should(BeFalse())
			Expect(err.Error()).ShouldNot(ContainSubstring("the rollout plan batch size mismatch"))
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(5))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("the deployment need to be stable", func() {
			By("create the source deployment with many pods")
			sourceDeploy.Spec.Replicas = pointer.Int32Ptr(50)
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
			sourceDeploy.Spec.Replicas = pointer.Int32Ptr(5)
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
			// test environment doesn't have deployment controller, has to fake it
			c.sourceDeploy.Status.Replicas = 5
			Expect(k8sClient.Status().Update(ctx, &sourceDeploy)).Should(Succeed())
			targetDeploy.Spec.Replicas = pointer.Int32Ptr(0)
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify should not fail b/c of deployment not stable")
			consistent, err := c.VerifySpec(ctx)
			Expect(err.Error()).ShouldNot(ContainSubstring("is still being reconciled, need to be paused or stable"))
			Expect(consistent).Should(BeFalse())
			Expect(c.rolloutStatus.RolloutTargetSize).Should(BeEquivalentTo(5))
			Expect(c.rolloutStatus.NewPodTemplateIdentifier).Should(BeEmpty())
		})

		It("deployment should not have controller", func() {
			By("Create deployments")
			sourceDeploy.Spec.Paused = true
			sourceDeploy.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ApplicationKind,
				Name:       "def",
				UID:        "123456",
				Controller: pointer.BoolPtr(true),
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
		BeforeEach(func() {
			By("Create paused deployments")

		})

		It("failed to fetch Deployment", func() {
			sourceDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("failed to claim Deployment", func() {
			sourceDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			targetDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(Succeed())
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		FIt("successfully initialized Deployment", func() {
			sourceDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &sourceDeploy)).Should(Succeed())
			targetDeploy.Spec.Paused = true
			Expect(k8sClient.Create(ctx, &targetDeploy)).Should(Succeed())
			c.parentController.SetUID("abcdedg")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})

	})
})
