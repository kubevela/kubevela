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
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// CloneSetRolloutController is responsible for handle rollout Cloneset type of workloads
type CloneSetRolloutController struct {
	cloneSetController
}

// NewCloneSetRolloutController creates a new Cloneset rollout controller
func NewCloneSetRolloutController(client client.Client, recorder event.Recorder, parentController oam.Object,
	rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) *CloneSetRolloutController {
	return &CloneSetRolloutController{
		cloneSetController: cloneSetController{
			workloadController: workloadController{
				client:           client,
				recorder:         recorder,
				parentController: parentController,
				rolloutSpec:      rolloutSpec,
				rolloutStatus:    rolloutStatus,
			},
			targetNamespacedName: workloadName,
		},
	}
}

// VerifySpec verifies that the target rollout resource is consistent with the rollout spec
func (c *CloneSetRolloutController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error
	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	// fetch the cloneset and get its current size
	currentReplicas, verifyErr := c.size(ctx)
	if verifyErr != nil {
		// do not fail the rollout because we can't get the resource
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint: nilerr
		return false, nil
	}

	// the cloneset size has to be the same as the current size
	if c.cloneSet.Spec.Replicas != nil && *c.cloneSet.Spec.Replicas != c.cloneSet.Status.Replicas {
		verifyErr = fmt.Errorf("the cloneset is still scaling, target = %d, cloneset size = %d",
			*c.cloneSet.Spec.Replicas, c.cloneSet.Status.Replicas)
		// we can wait for the cloneset scale operation to finish
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		return false, nil
	}

	// make sure that the updateRevision is different from what we have already done
	targetHash, verifyErr := utils.ComputeSpecHash(c.cloneSet.Spec)
	if verifyErr != nil {
		// do not fail the rollout because we can't compute the hash value for some reason
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint:nilerr
		return false, nil
	}

	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}

	// check if the rollout batch replicas added up to the Cloneset replicas
	if verifyErr = c.verifyRolloutBatchReplicaValue(currentReplicas); verifyErr != nil {
		return false, verifyErr
	}

	// record the size
	klog.InfoS("record the target size", "total replicas", currentReplicas)
	c.rolloutStatus.RolloutTargetSize = currentReplicas
	c.rolloutStatus.RolloutOriginalSize = currentReplicas

	// check if the cloneset is disabled
	if !c.cloneSet.Spec.UpdateStrategy.Paused {
		return false, fmt.Errorf("the cloneset %s is in the middle of updating, need to be paused first",
			c.cloneSet.GetName())
	}

	// check if the cloneset has any controller
	if controller := metav1.GetControllerOf(c.cloneSet); controller != nil {
		return false, fmt.Errorf("the cloneset %s has a controller owner %s",
			c.cloneSet.GetName(), controller.String())
	}

	// mark the rollout verified
	c.recorder.Event(c.parentController, event.Normal("Rollout Verified",
		"Rollout spec and the CloneSet resource are verified"))
	// record the new pod template hash only if it succeeds
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash
	return true, nil
}

// Initialize makes sure that the cloneset is under our control
func (c *CloneSetRolloutController) Initialize(ctx context.Context) (bool, error) {
	totalReplicas, err := c.size(ctx)
	if err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if controller := metav1.GetControllerOf(c.cloneSet); controller != nil {
		if (controller.Kind == v1beta1.AppRolloutKind && controller.APIVersion == v1beta1.SchemeGroupVersion.String()) ||
			(controller.Kind == v1alpha1.RolloutKind && controller.APIVersion == v1alpha1.SchemeGroupVersion.String()) {
			// it's already there
			return true, nil
		}
	}
	// add the parent controller to the owner of the cloneset
	// before kicking start the update and start from every pod in the old version
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopy())
	ref := metav1.NewControllerRef(c.parentController, c.parentController.GetObjectKind().GroupVersionKind())
	c.cloneSet.SetOwnerReferences(append(c.cloneSet.GetOwnerReferences(), *ref))
	c.cloneSet.Spec.UpdateStrategy.Paused = false
	c.cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: totalReplicas}

	// patch the CloneSet
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the start the cloneset update", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	// mark the rollout initialized
	c.recorder.Event(c.parentController, event.Normal("Rollout Initialized", "Rollout resource are initialized"))
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly, return if we are done
func (c *CloneSetRolloutController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize, err := c.size(ctx)
	if err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	newPodTarget := calculateNewBatchTarget(c.rolloutSpec, 0, int(cloneSetSize), int(c.rolloutStatus.CurrentBatch))
	// set the Partition as the desired number of pods in old revisions.
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopy())
	c.cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int,
		IntVal: cloneSetSize - int32(newPodTarget)}
	// patch the Cloneset
	if err = c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to update the cloneset to upgrade", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	// record the upgrade
	klog.InfoS("upgraded one batch", "current batch", c.rolloutStatus.CurrentBatch)
	c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
		fmt.Sprintf("Submitted upgrade quest for batch %d", c.rolloutStatus.CurrentBatch)))
	c.rolloutStatus.UpgradedReplicas = int32(newPodTarget)
	return true, nil
}

// CheckOneBatchPods checks to see if enough pods are upgraded according to the rollout plan
func (c *CloneSetRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	cloneSetSize, err := c.size(ctx)
	if err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	newPodTarget := calculateNewBatchTarget(c.rolloutSpec, 0, int(cloneSetSize), int(c.rolloutStatus.CurrentBatch))
	// get the number of ready pod from cloneset
	readyPodCount := int(c.cloneSet.Status.UpdatedReadyReplicas)
	if len(c.rolloutSpec.RolloutBatches) <= int(c.rolloutStatus.CurrentBatch) {
		err = errors.New("somehow, currentBatch number exceeded the rolloutBatches spec")
		klog.ErrorS(err, "total batch", len(c.rolloutSpec.RolloutBatches), "current batch",
			c.rolloutStatus.CurrentBatch)
		return false, err
	}
	currentBatch := c.rolloutSpec.RolloutBatches[c.rolloutStatus.CurrentBatch]
	unavail := 0
	if currentBatch.MaxUnavailable != nil {
		unavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(cloneSetSize), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", c.rolloutStatus.CurrentBatch,
		"new pod count target", newPodTarget, "new ready pod count", readyPodCount,
		"max unavailable pod allowed", unavail)
	c.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)
	// we could overshoot in the revert case when many pods are already upgraded
	if unavail+readyPodCount >= newPodTarget {
		// record the successful upgrade
		klog.InfoS("all pods in current batch are ready", "current batch", c.rolloutStatus.CurrentBatch)
		c.recorder.Event(c.parentController, event.Normal("Batch Available",
			fmt.Sprintf("Batch %d is available", c.rolloutStatus.CurrentBatch)))
		return true, nil
	}
	// continue to verify
	klog.InfoS("the batch is not ready yet", "current batch", c.rolloutStatus.CurrentBatch)
	c.rolloutStatus.RolloutRetry("the batch is not ready yet")
	return false, nil
}

// FinalizeOneBatch makes sure that the upgradedReplicas and current batch in the status are valid according to the spec
func (c *CloneSetRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	status := c.rolloutStatus
	spec := c.rolloutSpec
	if spec.BatchPartition != nil && *spec.BatchPartition < status.CurrentBatch {
		err := fmt.Errorf("the current batch value in the status is greater than the batch partition")
		klog.ErrorS(err, "we have moved past the user defined partition", "user specified batch partition",
			*spec.BatchPartition, "current batch we are working on", status.CurrentBatch)
		return false, err
	}
	upgradedReplicas := int(status.UpgradedReplicas)
	currentBatch := int(status.CurrentBatch)
	// calculate the lower bound of the possible pod count just before the current batch
	podCount := calculateNewBatchTarget(c.rolloutSpec, 0, int(c.rolloutStatus.RolloutTargetSize), currentBatch-1)
	// the recorded number should be at least as much as the all the pods before the current batch
	if podCount > upgradedReplicas {
		err := fmt.Errorf("the upgraded replica in the status is less than all the pods in the previous batch")
		klog.ErrorS(err, "rollout status inconsistent", "upgraded num status", upgradedReplicas,
			"pods in all the previous batches", podCount)
		return false, err
	}
	// calculate the upper bound with the current batch
	podCount = calculateNewBatchTarget(c.rolloutSpec, 0, int(c.rolloutStatus.RolloutTargetSize), currentBatch)
	// the recorded number should be not as much as the all the pods including the active batch
	if podCount < upgradedReplicas {
		err := fmt.Errorf("the upgraded replica in the status is greater than all the pods in the current batch")
		klog.ErrorS(err, "rollout status inconsistent", "total target size", c.rolloutStatus.RolloutTargetSize,
			"upgraded num status", upgradedReplicas, "pods in the batches including the current batch", podCount)
		return false, err
	}
	return true, nil
}

// Finalize makes sure the Cloneset is all upgraded
func (c *CloneSetRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := c.fetchCloneSet(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false
	}
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopy())
	// remove the parent controller from the resources' owner list
	var newOwnerList []metav1.OwnerReference
	isOwner := false
	for _, owner := range c.cloneSet.GetOwnerReferences() {
		if owner.Kind == c.parentController.GetObjectKind().GroupVersionKind().Kind &&
			owner.APIVersion == c.parentController.GetObjectKind().GroupVersionKind().GroupVersion().String() &&
			owner.Controller != nil && *owner.Controller {
			isOwner = true
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	if !isOwner {
		// nothing to do if we are already not the owner
		klog.InfoS("the cloneset is already released and not controlled by rollout", "cloneSet", c.cloneSet.Name)
		return true
	}
	c.cloneSet.SetOwnerReferences(newOwnerList)
	// pause the resource when the rollout failed so we can try again next time
	if !succeed {
		c.cloneSet.Spec.UpdateStrategy.Paused = true
	}
	// patch the CloneSet
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the finalize the cloneset", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false
	}
	// mark the resource finalized
	c.recorder.Event(c.parentController, event.Normal("Rollout Finalized",
		fmt.Sprintf("Rollout resource are finalized, succeed := %t", succeed)))
	c.rolloutStatus.LastAppliedPodTemplateIdentifier = c.rolloutStatus.NewPodTemplateIdentifier
	return true
}

// ---------------------------------------------
// The functions below are helper functions
// ---------------------------------------------

// check if the replicas in all the rollout batches add up to the right number
func (c *CloneSetRolloutController) verifyRolloutBatchReplicaValue(currentReplicas int32) error {
	// the target size has to be the same as the cloneset size
	if c.rolloutSpec.TargetSize != nil && *c.rolloutSpec.TargetSize != currentReplicas {
		return fmt.Errorf("the rollout plan is attempting to scale the cloneset, target = %d, cloneset size = %d",
			*c.rolloutSpec.TargetSize, currentReplicas)
	}
	// use a common function to check if the sum of all the batches can match the cloneset size
	err := verifyBatchesWithRollout(c.rolloutSpec, currentReplicas)
	if err != nil {
		return err
	}
	return nil
}
