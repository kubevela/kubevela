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
	apps "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// DeploymentRolloutController is responsible for handling rollout deployment type of workloads
type DeploymentRolloutController struct {
	deploymentController
	sourceNamespacedName types.NamespacedName
	sourceDeploy         apps.Deployment
	targetDeploy         apps.Deployment
}

// NewDeploymentRolloutController creates a new deployment rollout controller
func NewDeploymentRolloutController(client client.Client, recorder event.Recorder, parentController oam.Object,
	rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, sourceNamespacedName,
	targetNamespacedName types.NamespacedName) *DeploymentRolloutController {
	return &DeploymentRolloutController{
		deploymentController: deploymentController{
			workloadController: workloadController{
				client:           client,
				recorder:         recorder,
				parentController: parentController,
				rolloutSpec:      rolloutSpec,
				rolloutStatus:    rolloutStatus,
			},
			targetNamespacedName: targetNamespacedName,
		},
		sourceNamespacedName: sourceNamespacedName,
	}
}

// VerifySpec verifies that the rollout resource is consistent with the rollout spec
func (c *DeploymentRolloutController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	if err := c.fetchDeployments(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		// do not fail the rollout just because we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	// check if the rollout spec is compatible with the current state
	targetTotalReplicas, verifyErr := c.calculateRolloutTotalSize()
	if verifyErr != nil {
		return false, verifyErr
	}
	// record the size and we will use this value to drive the rest of the batches
	// we do not handle scale case in this controller
	c.rolloutStatus.RolloutTargetSize = targetTotalReplicas

	// make sure that the updateRevision is different from what we have already done
	targetHash, verifyErr := utils.ComputeSpecHash(c.targetDeploy.Spec)
	if verifyErr != nil {
		// do not fail the rollout because we can't compute the hash value for some reason
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint:nilerr
		return false, nil
	}
	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}

	// check if the rollout batch replicas added up to the Deployment replicas
	// we don't handle scale case in this controller
	if verifyErr = c.verifyRolloutBatchReplicaValue(targetTotalReplicas); verifyErr != nil {
		return false, verifyErr
	}
	if !c.sourceDeploy.Spec.Paused && getDeploymentReplicas(&c.sourceDeploy) != c.sourceDeploy.Status.Replicas {
		return false, fmt.Errorf("the source deployment %s is still being reconciled, need to be paused or stable",
			c.sourceDeploy.GetName())
	}

	if !c.targetDeploy.Spec.Paused && getDeploymentReplicas(&c.targetDeploy) != c.targetDeploy.Status.Replicas {
		return false, fmt.Errorf("the target deployment %s is still being reconciled, need to be paused or stable",
			c.targetDeploy.GetName())
	}

	// check if the targetDeploy has any controller
	if controller := metav1.GetControllerOf(&c.targetDeploy); controller != nil {
		return false, fmt.Errorf("the target deployment %s has a controller owner %s",
			c.targetDeploy.GetName(), controller.String())
	}

	// check if the sourceDeploy has any controller
	if controller := metav1.GetControllerOf(&c.sourceDeploy); controller != nil {
		return false, fmt.Errorf("the source deployment %s has a controller owner %s",
			c.sourceDeploy.GetName(), controller.String())
	}

	// mark the rollout verified
	c.recorder.Event(c.parentController, event.Normal("Rollout Verified",
		"Rollout spec and the Deployment resource are verified"))
	// record the new pod template hash on success
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash
	return true, nil
}

// Initialize makes sure that the source and target deployment is under our control
func (c *DeploymentRolloutController) Initialize(ctx context.Context) (bool, error) {
	if err := c.fetchDeployments(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	// claim source deployment
	if _, err := c.claimDeployment(ctx, &c.sourceDeploy, nil); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// claim target deployment
	// make sure we start with the matching replicas and target
	targetInitSize := pointer.Int32(c.rolloutStatus.RolloutTargetSize - getDeploymentReplicas(&c.sourceDeploy))
	if _, err := c.claimDeployment(ctx, &c.targetDeploy, targetInitSize); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// mark the rollout initialized
	c.recorder.Event(c.parentController, event.Normal("Rollout Initialized", "Rollout resource are initialized"))
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (c *DeploymentRolloutController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.fetchDeployments(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	currentSizeSetting := *c.sourceDeploy.Spec.Replicas + *c.targetDeploy.Spec.Replicas
	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstRolloutStrategyType
	if len(c.rolloutSpec.RolloutStrategy) != 0 {
		rolloutStrategy = c.rolloutSpec.RolloutStrategy
	}

	// Determine if we are the first or the second part of the current batch rollout
	if currentSizeSetting == c.rolloutStatus.RolloutTargetSize {
		// we need to finish the first part of the rollout,
		// this may conclude that we've already reached the size (in a rollback case)
		return c.rolloutBatchFirstHalf(ctx, rolloutStrategy)
	}

	// we are at the second half
	targetSize := c.calculateCurrentTarget(c.rolloutStatus.RolloutTargetSize)
	if !c.rolloutBatchSecondHalf(ctx, rolloutStrategy, targetSize) {
		return false, nil
	}

	// record the finished upgrade action
	klog.InfoS("upgraded one batch", "current batch", c.rolloutStatus.CurrentBatch,
		"target deployment size", targetSize)
	c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
		fmt.Sprintf("Finished submiting all upgrade quests for batch %d", c.rolloutStatus.CurrentBatch)))
	c.rolloutStatus.UpgradedReplicas = targetSize
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (c *DeploymentRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.fetchDeployments(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	// get the number of ready pod from target
	readyTargetPodCount := c.targetDeploy.Status.ReadyReplicas
	sourcePodCount := c.sourceDeploy.Status.Replicas
	currentBatch := c.rolloutSpec.RolloutBatches[c.rolloutStatus.CurrentBatch]
	targetGoal := c.calculateCurrentTarget(c.rolloutStatus.RolloutTargetSize)
	sourceGoal := c.calculateCurrentSource(c.rolloutStatus.RolloutTargetSize)
	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstRolloutStrategyType
	if len(c.rolloutSpec.RolloutStrategy) != 0 {
		rolloutStrategy = c.rolloutSpec.RolloutStrategy
	}
	maxUnavail := 0
	if currentBatch.MaxUnavailable != nil {
		maxUnavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(c.rolloutStatus.RolloutTargetSize), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", c.rolloutStatus.CurrentBatch,
		"target pod ready count", readyTargetPodCount, "source pod count", sourcePodCount,
		"max unavailable pod allowed", maxUnavail, "target goal", targetGoal, "source goal", sourceGoal,
		"rolloutStrategy", rolloutStrategy)

	if (rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType && sourcePodCount > sourceGoal) ||
		(rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType &&
			int32(maxUnavail)+readyTargetPodCount < targetGoal) {
		// we haven't met the end goal of this batch, continue to verify
		klog.InfoS("the batch is not ready yet", "current batch", c.rolloutStatus.CurrentBatch)
		c.rolloutStatus.RolloutRetry(fmt.Sprintf(
			"the batch %d is not ready yet with %d target pods ready and %d source pods with %d unavailable allowed",
			c.rolloutStatus.CurrentBatch, readyTargetPodCount, sourcePodCount, maxUnavail))
		return false, nil
	}

	// record the successful upgrade
	c.rolloutStatus.UpgradedReadyReplicas = readyTargetPodCount
	klog.InfoS("all pods in current batch are ready", "current batch", c.rolloutStatus.CurrentBatch)
	c.recorder.Event(c.parentController, event.Normal("Batch Available",
		fmt.Sprintf("Batch %d is available", c.rolloutStatus.CurrentBatch)))
	return true, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (c *DeploymentRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	if err := c.fetchDeployments(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	sourceTarget := getDeploymentReplicas(&c.sourceDeploy)
	targetTarget := getDeploymentReplicas(&c.targetDeploy)
	if sourceTarget+targetTarget != c.rolloutStatus.RolloutTargetSize {
		err := fmt.Errorf("deployment targets don't match total rollout, sourceTarget = %d, targetTarget = %d, "+
			"rolloutTargetSize = %d", sourceTarget, targetTarget, c.rolloutStatus.RolloutTargetSize)
		klog.ErrorS(err, "the batch is not valid", "current batch", c.rolloutStatus.CurrentBatch)
		return false, err
	}
	return true, nil
}

// Finalize makes sure the Deployment is all upgraded
func (c *DeploymentRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := c.fetchDeployments(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		return false
	}

	// release source deployment
	if _, err := c.releaseDeployment(ctx, &c.sourceDeploy); err != nil {
		return false
	}

	// release target deployment
	if _, err := c.releaseDeployment(ctx, &c.targetDeploy); err != nil {
		return false
	}

	// mark the resource finalized
	c.rolloutStatus.LastAppliedPodTemplateIdentifier = c.rolloutStatus.NewPodTemplateIdentifier
	c.recorder.Event(c.parentController, event.Normal("Rollout Finalized",
		fmt.Sprintf("Rollout resource are finalized, succeed := %t", succeed)))
	return true
}

/*
	----------------------------------

The functions below are helper functions
-------------------------------------
*/
func (c *DeploymentRolloutController) fetchDeployments(ctx context.Context) error {
	if err := c.client.Get(ctx, c.sourceNamespacedName, &c.sourceDeploy); err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, event.Warning("Failed to get the source Deployment", err))
		}
		return err
	}
	if err := c.client.Get(ctx, c.targetNamespacedName, &c.targetDeploy); err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, event.Warning("Failed to get the target Deployment", err))
		}
		return err
	}

	return nil
}

// calculateRolloutTotalSize fetches the Deployment and returns the replicas (not the actual number of pods)
func (c *DeploymentRolloutController) calculateRolloutTotalSize() (int32, error) {
	sourceSize := getDeploymentReplicas(&c.sourceDeploy)
	// the spec target size is the truth if it's set
	if c.rolloutSpec.TargetSize != nil {
		targetSize := *c.rolloutSpec.TargetSize
		if targetSize < sourceSize {
			return -1, fmt.Errorf("target size `%d` less than source size `%d`", targetSize, sourceSize)
		}
		return targetSize, nil
	}
	// otherwise, we assume that the source is the total
	return sourceSize, nil
}

// check if the replicas in all the rollout batches add up to the right number
func (c *DeploymentRolloutController) verifyRolloutBatchReplicaValue(totalReplicas int32) error {
	// use a common function to check if the sum of all the batches can match the Deployment size
	return verifyBatchesWithRollout(c.rolloutSpec, totalReplicas)
}

// the target deploy size for the current batch
func (c *DeploymentRolloutController) calculateCurrentTarget(totalSize int32) int32 {
	targetSize := int32(calculateNewBatchTarget(c.rolloutSpec, 0, int(totalSize), int(c.rolloutStatus.CurrentBatch)))
	klog.InfoS("Calculated the number of pods in the target deployment after current batch",
		"current batch", c.rolloutStatus.CurrentBatch, "target deploy size", targetSize)
	return targetSize
}

// the source deploy size for the current batch
func (c *DeploymentRolloutController) calculateCurrentSource(totalSize int32) int32 {
	sourceSize := totalSize - c.calculateCurrentTarget(totalSize)
	klog.InfoS("Calculated the number of pods in the source deployment after current batch",
		"current batch", c.rolloutStatus.CurrentBatch, "source deploy size", sourceSize)
	return sourceSize
}

func (c *DeploymentRolloutController) rolloutBatchFirstHalf(ctx context.Context,
	rolloutStrategy v1alpha1.RolloutStrategyType) (finished bool, rolloutError error) {
	targetSize := c.calculateCurrentTarget(c.rolloutStatus.RolloutTargetSize)
	defer func() {
		if finished {
			// record the finished upgrade action
			klog.InfoS("one batch is done already, no need to upgrade", "current batch", c.rolloutStatus.CurrentBatch)
			c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("upgrade quests for batch %d is already reached, no need to upgrade",
					c.rolloutStatus.CurrentBatch)))
			c.rolloutStatus.UpgradedReplicas = targetSize
		}
	}()

	if rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType {
		// set the target replica first which should increase its size
		if getDeploymentReplicas(&c.targetDeploy) < targetSize {
			klog.InfoS("set target deployment replicas", "deploy", c.targetDeploy.Name, "targetSize", targetSize)
			_ = c.scaleDeployment(ctx, &c.targetDeploy, targetSize)
			c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the increase part of upgrade quests for batch %d, target size = %d",
					c.rolloutStatus.CurrentBatch, targetSize)))
			return false, nil
		}

		// do nothing if the target is already reached
		klog.InfoS("target deployment replicas overshoot the size already", "deploy", c.targetDeploy.Name,
			"deployment size", getDeploymentReplicas(&c.targetDeploy), "targetSize", targetSize)
		return true, nil
	}

	if rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType {
		// set the source replicas first which should shrink its size
		sourceSize := c.calculateCurrentSource(c.rolloutStatus.RolloutTargetSize)
		if getDeploymentReplicas(&c.sourceDeploy) > sourceSize {
			klog.InfoS("set source deployment replicas", "source deploy", c.sourceDeploy.Name, "sourceSize", sourceSize)
			_ = c.scaleDeployment(ctx, &c.sourceDeploy, sourceSize)
			c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the decrease part of upgrade quests for batch %d, source size = %d",
					c.rolloutStatus.CurrentBatch, sourceSize)))
			return false, nil
		}

		// do nothing if the reduce target is already reached
		klog.InfoS("source deployment replicas overshoot the size already", "source deploy", c.sourceDeploy.Name,
			"deployment size", getDeploymentReplicas(&c.sourceDeploy), "sourceSize", sourceSize)
		return true, nil
	}

	return false, fmt.Errorf("encountered an unknown rolloutStrategy `%s`", rolloutStrategy)
}

func (c *DeploymentRolloutController) rolloutBatchSecondHalf(ctx context.Context,
	rolloutStrategy v1alpha1.RolloutStrategyType, targetSize int32) bool {
	sourceSize := c.calculateCurrentSource(c.rolloutStatus.RolloutTargetSize)

	if rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType {
		// calculate the max unavailable given the target size
		maxUnavail := 0
		currentBatch := c.rolloutSpec.RolloutBatches[c.rolloutStatus.CurrentBatch]
		if currentBatch.MaxUnavailable != nil {
			maxUnavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(c.rolloutStatus.RolloutTargetSize), true)
		}
		// make sure that the target deployment has enough ready pods before reducing the source
		if c.targetDeploy.Status.ReadyReplicas+int32(maxUnavail) >= targetSize {
			// set the source replicas now which should shrink its size
			klog.InfoS("set source deployment replicas", "deploy", c.sourceDeploy.Name, "sourceSize", sourceSize)
			if err := c.scaleDeployment(ctx, &c.sourceDeploy, sourceSize); err != nil {
				return false
			}
			c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the decrease part of upgrade quests for batch %d, source size = %d",
					c.rolloutStatus.CurrentBatch, sourceSize)))
		} else {
			// continue to verify
			klog.InfoS("the batch is not ready yet", "current batch", c.rolloutStatus.CurrentBatch,
				"target ready pod", c.targetDeploy.Status.ReadyReplicas)
			c.rolloutStatus.RolloutRetry(fmt.Sprintf("the batch %d is not ready yet with %d target pods ready",
				c.rolloutStatus.CurrentBatch, c.targetDeploy.Status.ReadyReplicas))
			return false
		}
	} else if rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType {
		// make sure that the source deployment has the correct pods before moving the target
		if c.sourceDeploy.Status.Replicas == sourceSize {
			// we can increase the target deployment as soon as the source deployment's replica is correct
			// no need to wait for them to be ready
			klog.InfoS("set target deployment replicas", "deploy", c.targetDeploy.Name, "targetSize", targetSize)
			if err := c.scaleDeployment(ctx, &c.targetDeploy, targetSize); err != nil {
				return false
			}
			c.recorder.Event(c.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the increase part of upgrade quests for batch %d, target size = %d",
					c.rolloutStatus.CurrentBatch, targetSize)))
		} else {
			// continue to verify
			klog.InfoS("the batch is not ready yet", "current batch", c.rolloutStatus.CurrentBatch,
				"source deploy pod", c.sourceDeploy.Status.Replicas)
			c.rolloutStatus.RolloutRetry(fmt.Sprintf("the batch %d is not ready yet with %d source pods",
				c.rolloutStatus.CurrentBatch, c.sourceDeploy.Status.Replicas))
			return false
		}
	}

	return true
}
