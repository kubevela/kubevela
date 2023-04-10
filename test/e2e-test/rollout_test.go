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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/pkg/controller/utils"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/oam-dev/kubevela/pkg/oam"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("rollout related e2e-test,Cloneset component rollout tests", func() {
	ctx := context.Background()
	var namespaceName, componentName, rolloutName string
	var ns corev1.Namespace
	var rollout v1alpha1.Rollout
	var kc kruise.CloneSet

	createNamespace := func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		// delete the namespaceName with all its resources
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespaceName,
		}
		res := &corev1.Namespace{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	verifyRolloutSucceeded := func(compRevName string) {
		By("Wait for the rollout  to succeed")
		Eventually(
			func() error {
				rollout = v1alpha1.Rollout{}
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: rolloutName}, &rollout)
				if err != nil {
					return err
				}
				if rollout.Status.LastUpgradedTargetRevision != compRevName {
					return fmt.Errorf("component revision name error %s", compRevName)
				}
				if rollout.Status.RollingState != v1alpha1.RolloutSucceedState {
					return fmt.Errorf("rollout isn't succeed acctauly %s", rollout.Status.RollingState)
				}
				return nil
			},
			time.Second*300, 300*time.Millisecond).Should(BeNil())
		Expect(rollout.Status.UpgradedReadyReplicas).Should(BeEquivalentTo(rollout.Status.RolloutTargetSize))
		Expect(rollout.Status.UpgradedReplicas).Should(BeEquivalentTo(rollout.Status.RolloutTargetSize))
		clonesetName := rollout.Spec.ComponentName

		By("Wait for resourceTracker to resume the control of cloneset")

		Eventually(
			func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clonesetName}, &kc)
				if err != nil {
					return err
				}
				if kc.Status.UpdatedReplicas != *kc.Spec.Replicas {
					return fmt.Errorf("expect cloneset updated replicas %d, but got %d",
						kc.Status.UpdatedReplicas, *kc.Spec.Replicas)
				}
				return nil
			},
			time.Second*60, time.Millisecond*500).Should(BeNil())
		// make sure all pods are upgraded
		image := kc.Spec.Template.Spec.Containers[0].Image
		podList := corev1.PodList{}
		Eventually(func() error {
			if err := k8sClient.List(ctx, &podList, client.MatchingLabels(kc.Spec.Template.Labels),
				client.InNamespace(namespaceName)); err != nil {
				return err
			}
			if len(podList.Items) != int(*kc.Spec.Replicas) {
				return fmt.Errorf("expect pod numbers %q, got %q", int(*kc.Spec.Replicas), len(podList.Items))
			}
			for _, pod := range podList.Items {
				gotImage := pod.Spec.Containers[0].Image
				if gotImage != image {
					return fmt.Errorf("expect pod container image %q, got %q", image, gotImage)
				}
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("expect pod phase %q, got %q", corev1.PodRunning, pod.Status.Phase)
				}
			}
			return nil
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed())
	}

	initialScale := func() {
		By("Apply the component scale to deploy")
		var newRollout v1alpha1.Rollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/comp-rollout.yaml", &newRollout)).Should(BeNil())
		newRollout.Namespace = namespaceName
		compRevName := utils.ConstructRevisionName(componentName, 1)
		newRollout.Spec.TargetRevisionName = compRevName
		Expect(k8sClient.Create(ctx, &newRollout)).Should(BeNil())
		rolloutName = newRollout.Name
		verifyRolloutSucceeded(compRevName)
	}

	applyTwoComponentRevision := func() {
		var compRev1, compRev2 appsv1.ControllerRevision
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/compRevSource.yaml", &compRev1)).Should(BeNil())
		compRev1.SetNamespace(namespaceName)
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/compRevTarget.yaml", &compRev2)).Should(BeNil())
		compRev2.SetNamespace(namespaceName)
		Expect(k8sClient.Create(ctx, &compRev1)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &compRev2)).Should(BeNil())
	}

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespaceName = randomNamespaceName("comp-rollout-e2e-test")
		createNamespace()
		componentName = "metrics-provider"
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Eventually(func() error {
			err := k8sClient.Delete(ctx, &ns)
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}, 15*time.Second, 300*time.Microsecond).Should(BeNil())
		Eventually(func() error {
			err := k8sClient.Delete(ctx, &rollout)
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}, 15*time.Second, 300*time.Microsecond).Should(BeNil())
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("Test component rollout cloneset", func() {
		var err error
		applyTwoComponentRevision()
		By("verify generate two controller revisions")
		ctlRevList := appsv1.ControllerRevisionList{}
		Eventually(func() error {
			if err = k8sClient.List(ctx, &ctlRevList, client.InNamespace(namespaceName),
				client.MatchingLabels(map[string]string{oam.LabelControllerRevisionComponent: componentName})); err != nil {
				return err
			}
			if len(ctlRevList.Items) < 2 {
				return fmt.Errorf("component revision missmatch actually %d", len(ctlRevList.Items))
			}
			return nil
		}, time.Second*30, 300*time.Millisecond).Should(BeNil())
		By("initial scale component revision")
		initialScale()
		clonesetName := rollout.Spec.ComponentName
		By("rollout to compRev 2")
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			// we needn't specify sourceRevision, rollout use lastTarget as source
			checkRollout.Spec.TargetRevisionName = utils.ConstructRevisionName(componentName, 2)
			checkRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32(0)
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		By("verify rollout pause in first batch")
		checkRollout := new(v1alpha1.Rollout)
		Eventually(func() error {
			checkRollout = new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			if checkRollout.Status.LastUpgradedTargetRevision != utils.ConstructRevisionName(componentName, 2) {
				return fmt.Errorf("last target error")
			}
			if checkRollout.Status.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("rollout state error")
			}
			if checkRollout.Status.CurrentBatch != 0 {
				return fmt.Errorf("current batch missmatch")
			}
			return nil
		}, 60*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(
			func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clonesetName}, &kc)
				if err != nil {
					return err
				}
				if len(kc.OwnerReferences) == 0 {
					return fmt.Errorf("cloneset owner missmatch")
				}
				if kc.OwnerReferences[0].UID != checkRollout.UID || kc.OwnerReferences[0].Kind != v1alpha1.RolloutKind {
					return fmt.Errorf("cloneset owner missmatch not rollout Uid %s", checkRollout.UID)
				}
				if kc.Status.UpdatedReplicas != 3 {
					return fmt.Errorf("expect cloneset updated replicas %d, but got %d",
						3, *kc.Spec.Replicas)
				}
				return nil
			},
			time.Second*120, time.Millisecond*500).Should(BeNil())
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			checkRollout.Spec.RolloutPlan.BatchPartition = nil
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(componentName, 2))
		By("continue rollout forward")
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			// we needn't specify sourceRevision, rollout use lastTarget as source
			checkRollout.Spec.TargetRevisionName = utils.ConstructRevisionName(componentName, 1)
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(componentName, 1))
	})

	It("Test component rollout cloneset revert in middle of rollout", func() {
		var err error
		applyTwoComponentRevision()
		By("verify generate two controller revisions")
		ctlRevList := appsv1.ControllerRevisionList{}
		Eventually(func() error {
			if err = k8sClient.List(ctx, &ctlRevList, client.InNamespace(namespaceName),
				client.MatchingLabels(map[string]string{oam.LabelControllerRevisionComponent: componentName})); err != nil {
				return err
			}
			if len(ctlRevList.Items) < 2 {
				return fmt.Errorf("component revision missmatch acctually %d", len(ctlRevList.Items))
			}
			return nil
		}, time.Second*30, 300*time.Millisecond).Should(BeNil())
		By("initial scale component revision")
		initialScale()
		clonesetName := rollout.Spec.ComponentName
		By("rollout to compRev 2")
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			// we needn't specify sourceRevision, rollout use lastTarget as source
			checkRollout.Spec.TargetRevisionName = utils.ConstructRevisionName(componentName, 2)
			checkRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32(0)
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		By("verify rollout pause in first batch")
		checkRollout := new(v1alpha1.Rollout)
		Eventually(func() error {
			checkRollout = new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			if checkRollout.Status.LastUpgradedTargetRevision != utils.ConstructRevisionName(componentName, 2) {
				return fmt.Errorf("last target error")
			}
			if checkRollout.Status.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("rollout state error")
			}
			if checkRollout.Status.CurrentBatch != 0 {
				return fmt.Errorf("current batch missmatch")
			}
			return nil
		}, 60*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(
			func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clonesetName}, &kc)
				if err != nil {
					return err
				}
				if len(kc.OwnerReferences) == 0 {
					return fmt.Errorf("cloneset owner missmatch")
				}
				if kc.OwnerReferences[0].UID != checkRollout.UID || kc.OwnerReferences[0].Kind != v1alpha1.RolloutKind {
					return fmt.Errorf("cloneset owner missmatch not rollout Uid %s", checkRollout.UID)
				}
				if kc.Status.UpdatedReplicas != 3 {
					return fmt.Errorf("expect cloneset updated replicas %d, but got %d",
						3, *kc.Spec.Replicas)
				}
				return nil
			},
			time.Second*120, time.Millisecond*500).Should(BeNil())
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			checkRollout.Spec.TargetRevisionName = utils.ConstructRevisionName(componentName, 1)
			checkRollout.Spec.RolloutPlan.BatchPartition = nil
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(componentName, 1))
		By("continue rollout forward")
		Eventually(func() error {
			checkRollout := new(v1alpha1.Rollout)
			if err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: rolloutName}, checkRollout); err != nil {
				return err
			}
			// we needn't specify sourceRevision, rollout use lastTarget as source
			checkRollout.Spec.TargetRevisionName = utils.ConstructRevisionName(componentName, 2)
			if err = k8sClient.Update(ctx, checkRollout); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 15*time.Millisecond).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(componentName, 2))
	})
})
