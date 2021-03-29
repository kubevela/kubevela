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
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// CloneSetController is responsible for handle Cloneset type of workloads
type CloneSetController struct {
	client           client.Client
	recorder         event.Recorder
	parentController oam.Object

	rolloutSpec            *v1alpha1.RolloutPlan
	rolloutStatus          *v1alpha1.RolloutStatus
	workloadNamespacedName types.NamespacedName
	cloneSet               *kruise.CloneSet
}

// NewCloneSetController creates a new Cloneset controller
func NewCloneSetController(client client.Client, recorder event.Recorder, parentController oam.Object,
	rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) *CloneSetController {
	return &CloneSetController{
		client:                 client,
		recorder:               recorder,
		parentController:       parentController,
		rolloutSpec:            rolloutSpec,
		rolloutStatus:          rolloutStatus,
		workloadNamespacedName: workloadName,
	}
}

// VerifySpec verifies that the target rollout resource is consistent with the rollout spec
func (c *CloneSetController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	// fetch the cloneset and get its current size
	totalReplicas, verifyErr := c.size(ctx)
	if verifyErr != nil {
		// do not fail the rollout because we can't get the resource
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint: nilerr
		return false, nil
	}
	// record the size
	klog.InfoS("record the target size", "total replicas", totalReplicas)
	c.rolloutStatus.RolloutTargetTotalSize = totalReplicas

	// make sure that the updateRevision is different from what we have already done
	targetHash := c.cloneSet.Status.UpdateRevision
	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}
	// record the new pod template hash
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash

	// check if the rollout batch replicas added up to the Cloneset replicas
	if verifyErr = c.verifyRolloutBatchReplicaValue(totalReplicas); verifyErr != nil {
		return false, verifyErr
	}

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
	return true, nil
}

// Initialize makes sure that the cloneset is under our control
func (c *CloneSetController) Initialize(ctx context.Context) (bool, error) {
	totalReplicas, err := c.size(ctx)
	if err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if controller := metav1.GetControllerOf(c.cloneSet); controller != nil {
		if controller.Kind == v1beta1.AppRolloutKind && controller.APIVersion == v1beta1.SchemeGroupVersion.String() {
			// it's already there
			return true, nil
		}
	}
	// add the parent controller to the owner of the cloneset
	// before kicking start the update and start from every pod in the old version
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	ref := metav1.NewControllerRef(c.parentController, v1beta1.AppRolloutKindVersionKind)
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
func (c *CloneSetController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize, _ := c.size(ctx)
	newPodTarget := c.calculateNewPodTarget(int(cloneSetSize))
	// set the Partition as the desired number of pods in old revisions.
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	c.cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int,
		IntVal: cloneSetSize - int32(newPodTarget)}
	// patch the Cloneset
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
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

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (c *CloneSetController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	cloneSetSize, _ := c.size(ctx)
	newPodTarget := c.calculateNewPodTarget(int(cloneSetSize))
	// get the number of ready pod from cloneset
	readyPodCount := int(c.cloneSet.Status.UpdatedReadyReplicas)
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
		c.rolloutStatus.LastAppliedPodTemplateIdentifier = c.rolloutStatus.NewPodTemplateIdentifier
		return true, nil
	}
	// continue to verify
	klog.InfoS("the batch is not ready yet", "current batch", c.rolloutStatus.CurrentBatch)
	c.rolloutStatus.RolloutRetry("the batch is not ready yet")
	return false, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (c *CloneSetController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	// nothing to do for cloneset for now
	return true, nil
}

// Finalize makes sure the Cloneset is all upgraded
func (c *CloneSetController) Finalize(ctx context.Context, succeed bool) bool {
	if err := c.fetchCloneSet(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false
	}
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	// remove the parent controller from the resources' owner list
	var newOwnerList []metav1.OwnerReference
	for _, owner := range c.cloneSet.GetOwnerReferences() {
		if owner.Kind == v1beta1.AppRolloutKind && owner.APIVersion == v1beta1.SchemeGroupVersion.String() {
			continue
		}
		newOwnerList = append(newOwnerList, owner)
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
	return true
}

// ---------------------------------------------
// The functions below are helper functions
// ---------------------------------------------
// size fetches the Cloneset and returns the replicas (not the actual number of pods)
func (c *CloneSetController) size(ctx context.Context) (int32, error) {
	if c.cloneSet == nil {
		err := c.fetchCloneSet(ctx)
		if err != nil {
			return 0, err
		}
	}
	// default is 1
	if c.cloneSet.Spec.Replicas == nil {
		return 1, nil
	}
	return *c.cloneSet.Spec.Replicas, nil
}

// check if the replicas in all the rollout batches add up to the right number
func (c *CloneSetController) verifyRolloutBatchReplicaValue(totalReplicas int32) error {
	// the target size has to be the same as the cloneset size
	if c.rolloutSpec.TargetSize != nil && *c.rolloutSpec.TargetSize != totalReplicas {
		return fmt.Errorf("the rollout plan is attempting to scale the cloneset, target = %d, cloneset size = %d",
			*c.rolloutSpec.TargetSize, totalReplicas)
	}
	// use a common function to check if the sum of all the batches can match the cloneset size
	err := VerifySumOfBatchSizes(c.rolloutSpec, totalReplicas)
	if err != nil {
		return err
	}
	return nil
}

func (c *CloneSetController) fetchCloneSet(ctx context.Context) error {
	// get the cloneSet
	workload := kruise.CloneSet{}
	err := c.client.Get(ctx, c.workloadNamespacedName, &workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, event.Warning("Failed to get the Cloneset", err))
		}
		return err
	}
	c.cloneSet = &workload
	return nil
}

func (c *CloneSetController) calculateNewPodTarget(cloneSetSize int) int {
	currentBatch := int(c.rolloutStatus.CurrentBatch)
	newPodTarget := 0
	if currentBatch == len(c.rolloutSpec.RolloutBatches)-1 {
		newPodTarget = cloneSetSize
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.InfoS("use the cloneset size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new version pod target", newPodTarget)
	} else {
		for i, r := range c.rolloutSpec.RolloutBatches {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&r.Replicas, cloneSetSize, true)
			if i <= currentBatch {
				newPodTarget += batchSize
			} else {
				break
			}
		}
		klog.InfoS("Calculated the number of new version pod", "current batch", currentBatch,
			"new version pod target", newPodTarget)
	}
	return newPodTarget
}
