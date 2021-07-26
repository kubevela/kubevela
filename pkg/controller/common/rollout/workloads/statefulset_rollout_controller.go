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

// StatefulSetRolloutController is responsible for handle rollout StatefulSet type of workloads
type StatefulSetRolloutController struct {
	statefulSetController
	sourceNamespacedName types.NamespacedName
	sourceStatefulSet    *apps.StatefulSet
	targetStatefulSet    *apps.StatefulSet
}

// NewStatefulSetRolloutController creates StatefulSet rollout controller
func NewStatefulSetRolloutController(client client.Client, recorder event.Recorder, parentController oam.Object, rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus,
	sourceNamespacedName, targetNamespacedName types.NamespacedName) *StatefulSetRolloutController {
	return &StatefulSetRolloutController{
		statefulSetController: statefulSetController{
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
func (s *StatefulSetRolloutController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			s.recorder.Event(s.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	if err := s.fetchStatefulSets(ctx); err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		// do not fail the rollout just because we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	// check if the rollout spec is compatible with the current state
	targetTotalReplicas, verifyErr := s.calculateRolloutTotalSize()
	if verifyErr != nil {
		return false, verifyErr
	}
	// record the size and we will use this value to drive the rest of the batches
	s.rolloutStatus.RolloutTargetSize = targetTotalReplicas

	// make sure that the updateRevision is different from what we have already done
	targetHash, verifyErr := utils.ComputeSpecHash(s.targetStatefulSet.Spec)
	if verifyErr != nil {
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint:nilerr
		return false, nil
	}
	if targetHash == s.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}

	// check if the rollout batch replicas added up to the StatefulSet replicas
	if verifyErr = s.verifyRolloutBatchReplicaValue(targetTotalReplicas); verifyErr != nil {
		return false, verifyErr
	}

	if getStatefulSetReplicas(s.sourceStatefulSet) != s.sourceStatefulSet.Status.Replicas {
		return false, fmt.Errorf("the source StatefulSet %s is still being reconciled, need to be stable",
			s.sourceStatefulSet.GetName())
	}

	if getStatefulSetReplicas(s.targetStatefulSet) != s.targetStatefulSet.Status.Replicas {
		return false, fmt.Errorf("the target StatefulSet %s is still being reconciled, need to be stable",
			s.targetStatefulSet.GetName())
	}

	// check if the target StatefulSet has any controller
	if controller := metav1.GetControllerOf(s.targetStatefulSet); controller != nil {
		return false, fmt.Errorf("the target StatefulSet %s has a controller owner %s",
			s.targetStatefulSet.GetName(), controller.String())
	}

	// check if the source StatefulSet has any controller
	if controller := metav1.GetControllerOf(s.sourceStatefulSet); controller != nil {
		return false, fmt.Errorf("the source StatefulSet %s has a controller owner %s",
			s.sourceStatefulSet.GetName(), controller.String())
	}

	// mark the rollout verified
	s.recorder.Event(s.parentController, event.Normal("Rollout Verified",
		"Rollout spec and the StatefulSet resource are verified"))
	// record the new pod template StatefulSet on success
	s.rolloutStatus.NewPodTemplateIdentifier = targetHash
	return true, nil
}

// Initialize makes sure that the source and target StatefulSet is under our control
func (s *StatefulSetRolloutController) Initialize(ctx context.Context) (bool, error) {
	if err := s.fetchStatefulSets(ctx); err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if _, err := s.claimStatefulSet(ctx, s.sourceStatefulSet, nil); err != nil {
		// nolint:nilerr
		return false, nil
	}

	targetInitSize := pointer.Int32Ptr(s.rolloutStatus.RolloutTargetSize - getStatefulSetReplicas(s.sourceStatefulSet))
	if _, err := s.claimStatefulSet(ctx, s.targetStatefulSet, targetInitSize); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// mark the rollout initialized
	s.recorder.Event(s.parentController, event.Normal("Rollout Initialized", "Rollout resource are initialized"))
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (s *StatefulSetRolloutController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	if err := s.fetchStatefulSets(ctx); err != nil {
		// nolint:nilerr
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	currentSizeSetting := *s.sourceStatefulSet.Spec.Replicas + *s.targetStatefulSet.Spec.Replicas
	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstRolloutStrategyType
	if len(s.rolloutSpec.RolloutStrategy) != 0 {
		rolloutStrategy = s.rolloutSpec.RolloutStrategy
	}

	// Determine if we are the first or the second part of the current batch rollout
	if currentSizeSetting == s.rolloutStatus.RolloutTargetSize {
		// we need to finish the first part of the rollout,
		// this may conclude that we've already reached the size (in a rollback case)
		return s.rolloutBatchFirstHalf(ctx, rolloutStrategy)
	}

	// we are at the second half
	targetSize := s.calculateCurrentTarget(s.rolloutStatus.RolloutTargetSize)
	if !s.rolloutBatchSecondHalf(ctx, rolloutStrategy, targetSize) {
		return false, nil
	}

	// record the finished upgrade action
	klog.InfoS("upgraded one batch", "current batch", s.rolloutStatus.CurrentBatch,
		"target StatefulSet size", targetSize)
	s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
		fmt.Sprintf("Finished submiting all upgrade quests for batch %d", s.rolloutStatus.CurrentBatch)))
	s.rolloutStatus.UpgradedReplicas = targetSize
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (s *StatefulSetRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	if err := s.fetchStatefulSets(ctx); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// get the number of ready pod from target
	readyTargetPodCount := s.targetStatefulSet.Status.ReadyReplicas
	sourcePodCount := s.sourceStatefulSet.Status.Replicas
	currentBatch := s.rolloutSpec.RolloutBatches[s.rolloutStatus.CurrentBatch]
	targetGoal := s.calculateCurrentTarget(s.rolloutStatus.RolloutTargetSize)
	sourceGoal := s.calculateCurrentSource(s.rolloutStatus.RolloutTargetSize)
	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstRolloutStrategyType
	if len(s.rolloutSpec.RolloutStrategy) != 0 {
		rolloutStrategy = s.rolloutSpec.RolloutStrategy
	}
	maxUnavail := 0
	if currentBatch.MaxUnavailable != nil {
		maxUnavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(s.rolloutStatus.RolloutTargetSize), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", s.rolloutStatus.CurrentBatch,
		"target pod ready count", readyTargetPodCount, "source pod count", sourcePodCount,
		"max unavailable pod allowed", maxUnavail, "target goal", targetGoal, "source goal", sourceGoal,
		"rolloutStrategy", rolloutStrategy)

	if (rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType && sourcePodCount > sourceGoal) ||
		(rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType &&
			int32(maxUnavail)+readyTargetPodCount < targetGoal) {
		// we haven't met the end goal of this batch, continue to verify
		klog.InfoS("the batch is not ready yet", "current batch", s.rolloutStatus.CurrentBatch)
		s.rolloutStatus.RolloutRetry(fmt.Sprintf(
			"the batch %d is not ready yet with %d target pods ready and %d source pods with %d unavailable allowed",
			s.rolloutStatus.CurrentBatch, readyTargetPodCount, sourcePodCount, maxUnavail))
		return false, nil
	}

	// record the successful upgrade
	s.rolloutStatus.UpgradedReadyReplicas = readyTargetPodCount
	klog.InfoS("all pods in current batch are ready", "current batch", s.rolloutStatus.CurrentBatch)
	s.recorder.Event(s.parentController, event.Normal("Batch Available",
		fmt.Sprintf("Batch %d is available", s.rolloutStatus.CurrentBatch)))
	return true, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (s *StatefulSetRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	if err := s.fetchStatefulSets(ctx); err != nil {
		// nolint:nilerr
		return false, nil
	}

	sourceTarget := getStatefulSetReplicas(s.sourceStatefulSet)
	targetTarget := getStatefulSetReplicas(s.targetStatefulSet)
	if sourceTarget+targetTarget != s.rolloutStatus.RolloutTargetSize {
		err := fmt.Errorf("StatefulSet targets don't match total rollout, sourceTarget = %d, targetTarget = %d, "+
			"rolloutTargetSize = %d", sourceTarget, targetTarget, s.rolloutStatus.RolloutTargetSize)
		klog.ErrorS(err, "the batch is not valid", "current batch", s.rolloutStatus.CurrentBatch)
		return false, err
	}
	return true, nil
}

// Finalize makes sure the StatefulSet is all upgraded
func (s *StatefulSetRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := s.fetchStatefulSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		return false
	}

	// release source StatefulSet
	if _, err := s.releaseStatefulSet(ctx, s.sourceStatefulSet); err != nil {
		return false
	}

	// release target StatefulSet
	if _, err := s.releaseStatefulSet(ctx, s.targetStatefulSet); err != nil {
		return false
	}

	// mark the resource finalized
	s.rolloutStatus.LastAppliedPodTemplateIdentifier = s.rolloutStatus.NewPodTemplateIdentifier
	s.recorder.Event(s.parentController, event.Normal("Rollout Finalized",
		fmt.Sprintf("Rollout resource are finalized, succeed := %t", succeed)))
	return true
}

func (s *StatefulSetRolloutController) fetchStatefulSets(ctx context.Context) error {
	sourceWorkload := apps.StatefulSet{}
	if err := s.client.Get(ctx, s.sourceNamespacedName, &sourceWorkload); err != nil {
		if !apierrors.IsNotFound(err) {
			s.recorder.Event(s.parentController, event.Warning("Failed to get the source StatefulSet", err))
		}
		return err
	}

	targetWorkload := apps.StatefulSet{}
	if err := s.client.Get(ctx, s.targetNamespacedName, &targetWorkload); err != nil {
		if !apierrors.IsNotFound(err) {
			s.recorder.Event(s.parentController, event.Warning("Failed to get the target StatefulSet", err))
		}
		return err
	}

	s.sourceStatefulSet = &sourceWorkload
	s.targetStatefulSet = &targetWorkload
	return nil
}

// get the target size of the rollout
func (s *StatefulSetRolloutController) calculateRolloutTotalSize() (int32, error) {
	sourceSize := getStatefulSetReplicas(s.sourceStatefulSet)

	if s.rolloutSpec.TargetSize != nil {
		targetSize := *s.rolloutSpec.TargetSize
		if targetSize < sourceSize {
			return -1, fmt.Errorf("target size `%d` less than source size `%d`", targetSize, sourceSize)
		}
		return targetSize, nil
	}

	return sourceSize, nil
}

// check if the replicas in all the rollout batches add up to the right number
func (s *StatefulSetRolloutController) verifyRolloutBatchReplicaValue(totalReplicas int32) error {
	return verifyBatchesWithRollout(s.rolloutSpec, totalReplicas)
}

// the target StatefulSet size for the current batch
func (s *StatefulSetRolloutController) calculateCurrentTarget(totalSize int32) int32 {
	targetSize := int32(calculateNewBatchTarget(s.rolloutSpec, 0, int(totalSize), int(s.rolloutStatus.CurrentBatch)))
	klog.InfoS("Calculated the number of pods in the target StatefulSet after current batch",
		"current batch", s.rolloutStatus.CurrentBatch, "target StatefulSet size", targetSize)
	return targetSize
}

// the source StatefulSet size for the current batch
func (s *StatefulSetRolloutController) calculateCurrentSource(totalSize int32) int32 {
	sourceSize := totalSize - s.calculateCurrentTarget(totalSize)
	klog.InfoS("Calculated the number of pods in the source StatefulSet after current batch",
		"current batch", s.rolloutStatus.CurrentBatch, "source StatefulSet size", sourceSize)
	return sourceSize
}

func (s *StatefulSetRolloutController) rolloutBatchFirstHalf(ctx context.Context,
	rolloutStrategy v1alpha1.RolloutStrategyType) (finished bool, rolloutError error) {
	targetSize := s.calculateCurrentTarget(s.rolloutStatus.RolloutTargetSize)
	defer func() {
		if finished {
			// record the finished upgrade action
			klog.InfoS("one batch is done already, no need to upgrade", "current batch", s.rolloutStatus.CurrentBatch)
			s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("upgrade quests for batch %d is already reached, no need to upgrade",
					s.rolloutStatus.CurrentBatch)))
			s.rolloutStatus.UpgradedReplicas = targetSize
		}
	}()

	if rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType {
		// set the target replica first which should increase its size
		if getStatefulSetReplicas(s.targetStatefulSet) < targetSize {
			klog.InfoS("set target StatefulSet replicas", "StatefulSet", s.targetStatefulSet.Name, "targetSize", targetSize)
			_ = s.scaleStatefulSet(ctx, s.targetStatefulSet, targetSize)
			s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the increase part of upgrade quests for batch %d, target size = %d",
					s.rolloutStatus.CurrentBatch, targetSize)))
			return false, nil
		}

		// do nothing if the target is already reached
		klog.InfoS("target StatefulSet replicas overshoot the size already", "StatefulSet", s.targetStatefulSet.Name,
			"StatefulSet size", getStatefulSetReplicas(s.targetStatefulSet), "targetSize", targetSize)
		return true, nil
	}

	if rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType {
		// set the source replicas first which should shrink its size
		sourceSize := s.calculateCurrentSource(s.rolloutStatus.RolloutTargetSize)
		if getStatefulSetReplicas(s.sourceStatefulSet) > sourceSize {
			klog.InfoS("set source StatefulSet replicas", "source StatefulSet", s.sourceStatefulSet.Name, "sourceSize", sourceSize)
			_ = s.scaleStatefulSet(ctx, s.sourceStatefulSet, sourceSize)
			s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the decrease part of upgrade quests for batch %d, source size = %d",
					s.rolloutStatus.CurrentBatch, sourceSize)))
			return false, nil
		}

		// do nothing if the reduce target is already reached
		klog.InfoS("source StatefulSet replicas overshoot the size already", "source StatefulSet", s.sourceStatefulSet.Name,
			"StatefulSet size", getStatefulSetReplicas(s.sourceStatefulSet), "sourceSize", sourceSize)
		return true, nil
	}

	return false, fmt.Errorf("encountered an unknown rolloutStrategy `%s`", rolloutStrategy)
}

func (s *StatefulSetRolloutController) rolloutBatchSecondHalf(ctx context.Context,
	rolloutStrategy v1alpha1.RolloutStrategyType, targetSize int32) bool {
	sourceSize := s.calculateCurrentSource(s.rolloutStatus.RolloutTargetSize)

	if rolloutStrategy == v1alpha1.IncreaseFirstRolloutStrategyType {
		// calculate the max unavailable given the target size
		maxUnavail := 0
		currentBatch := s.rolloutSpec.RolloutBatches[s.rolloutStatus.CurrentBatch]
		if currentBatch.MaxUnavailable != nil {
			maxUnavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(s.rolloutStatus.RolloutTargetSize), true)
		}
		// make sure that the target StatefulSet has enough ready pods before reducing the source
		if s.targetStatefulSet.Status.ReadyReplicas+int32(maxUnavail) >= targetSize {
			// set the source replicas now which should shrink its size
			klog.InfoS("set source StatefulSet replicas", "StatefulSet", s.sourceStatefulSet.Name, "sourceSize", sourceSize)
			if err := s.scaleStatefulSet(ctx, s.sourceStatefulSet, sourceSize); err != nil {
				return false
			}
			s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the decrease part of upgrade quests for batch %d, source size = %d",
					s.rolloutStatus.CurrentBatch, sourceSize)))
		} else {
			// continue to verify
			klog.InfoS("the batch is not ready yet", "current batch", s.rolloutStatus.CurrentBatch,
				"target ready pod", s.targetStatefulSet.Status.ReadyReplicas)
			s.rolloutStatus.RolloutRetry(fmt.Sprintf("the batch %d is not ready yet with %d target pods ready",
				s.rolloutStatus.CurrentBatch, s.targetStatefulSet.Status.ReadyReplicas))
			return false
		}
	} else if rolloutStrategy == v1alpha1.DecreaseFirstRolloutStrategyType {
		// make sure that the source StatefulSet has the correct pods before moving the target
		if s.sourceStatefulSet.Status.Replicas == sourceSize {
			// we can increase the target StatefulSet as soon as the source StatefulSet's replica is correct
			// no need to wait for them to be ready
			klog.InfoS("set target StatefulSet replicas", "StatefulSet", s.targetStatefulSet.Name, "targetSize", targetSize)
			if err := s.scaleStatefulSet(ctx, s.targetStatefulSet, targetSize); err != nil {
				return false
			}
			s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
				fmt.Sprintf("Submitted the increase part of upgrade quests for batch %d, target size = %d",
					s.rolloutStatus.CurrentBatch, targetSize)))
		} else {
			// continue to verify
			klog.InfoS("the batch is not ready yet", "current batch", s.rolloutStatus.CurrentBatch,
				"source StatefulSet pod", s.sourceStatefulSet.Status.Replicas)
			s.rolloutStatus.RolloutRetry(fmt.Sprintf("the batch %d is not ready yet with %d source pods",
				s.rolloutStatus.CurrentBatch, s.sourceStatefulSet.Status.Replicas))
			return false
		}
	}

	return true
}
