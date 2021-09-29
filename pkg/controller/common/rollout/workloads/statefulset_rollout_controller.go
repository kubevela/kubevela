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
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// StatefulSetRolloutController is responsible for handle rollout StatefulSet type of workloads
type StatefulSetRolloutController struct {
	statefulSetController
	statefulSet *appsv1.StatefulSet
}

// NewStatefulSetRolloutController creates StatefulSet rollout controller
func NewStatefulSetRolloutController(client client.Client, recorder event.Recorder, parentController oam.Object, rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus,
	targetNamespacedName types.NamespacedName) *StatefulSetRolloutController {
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

	currentReplicas, verifyErr := s.size(ctx)
	if verifyErr != nil {
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint: nilerr
		return false, nil
	}
	// record the size and we will use this value to drive the rest of the batches
	klog.InfoS("record the target size", "total replicas", currentReplicas)
	s.rolloutStatus.RolloutTargetSize = currentReplicas
	s.rolloutStatus.RolloutOriginalSize = currentReplicas

	// make sure that the updateRevision is different from what we have already done
	targetHash := s.statefulSet.Status.UpdateRevision
	if targetHash == s.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}

	if s.statefulSet.Spec.Replicas != nil && currentReplicas != s.statefulSet.Status.Replicas {
		verifyErr = fmt.Errorf("the StatefulSet is still scaling, target = %d, statefulSet size = %d",
			currentReplicas, s.statefulSet.Status.Replicas)
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		return false, verifyErr
	}

	// check if the rollout batch replicas added up to the StatefulSet replicas
	if verifyErr = s.verifyRolloutBatchReplicaValue(currentReplicas); verifyErr != nil {
		return false, verifyErr
	}

	// check if the StatefulSet has any controller
	if controller := metav1.GetControllerOf(s.statefulSet); controller != nil {
		return false, fmt.Errorf("the StatefulSet %s has a controller owner %s",
			s.statefulSet.GetName(), controller.String())
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
	currentReplicas, err := s.size(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if _, err := s.claimStatefulSet(ctx, s.statefulSet); err != nil {
		// nolint:nilerr
		return false, nil
	}

	if err := s.setPartition(ctx, s.statefulSet, currentReplicas); err != nil {
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
	currentReplicas, err := s.size(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	newPodTarget := s.calculateCurrentTarget(currentReplicas)
	if err := s.setPartition(ctx, s.statefulSet, currentReplicas-newPodTarget); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// record the finished upgrade action
	klog.InfoS("upgraded one batch", "current batch", s.rolloutStatus.CurrentBatch,
		"target size", newPodTarget)
	s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
		fmt.Sprintf("Finished submiting all upgrade quests for batch %d", s.rolloutStatus.CurrentBatch)))
	s.rolloutStatus.UpgradedReplicas = newPodTarget
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (s *StatefulSetRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	currentReplicas, err := s.size(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	newPodTarget := s.calculateCurrentTarget(currentReplicas)
	readyPodCount := int(s.statefulSet.Status.ReadyReplicas)

	if len(s.rolloutSpec.RolloutBatches) <= int(s.rolloutStatus.CurrentBatch) {
		err := errors.New("somehow, currentBatch number exceeded the rolloutBatches spec")
		klog.ErrorS(err, "total batch", len(s.rolloutSpec.RolloutBatches), "current batch",
			s.rolloutStatus.CurrentBatch)
		return false, err
	}

	currentBatch := s.rolloutSpec.RolloutBatches[s.rolloutStatus.CurrentBatch]
	maxUnavail := 0
	if currentBatch.MaxUnavailable != nil {
		maxUnavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(currentReplicas), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", s.rolloutStatus.CurrentBatch,
		"new pod count target", newPodTarget, "new ready pod count", readyPodCount,
		"max unavailable pod allowed", maxUnavail)
	s.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)

	if maxUnavail+readyPodCount >= int(newPodTarget) {
		// record the successful upgrade
		klog.InfoS("all pods in current batch are ready", "current batch", s.rolloutStatus.CurrentBatch)
		s.recorder.Event(s.parentController, event.Normal("Batch Available",
			fmt.Sprintf("Batch %d is available", s.rolloutStatus.CurrentBatch)))
		return true, nil
	}

	// continue to verify
	klog.InfoS("the batch is not ready yet", "current batch", s.rolloutStatus.CurrentBatch)
	s.rolloutStatus.RolloutRetry("the batch is not ready yet")
	return false, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (s *StatefulSetRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	status := s.rolloutStatus
	spec := s.rolloutSpec

	if spec.BatchPartition != nil && *spec.BatchPartition < status.CurrentBatch {
		err := fmt.Errorf("the current batch value in the status is greater than the batch partition")
		klog.ErrorS(err, "we have moved past the user defined partition", "user specified batch partition",
			*spec.BatchPartition, "current batch we are working on", status.CurrentBatch)
		return false, err
	}

	upgradedReplicas := int(status.UpgradedReplicas)
	currentBatch := int(status.CurrentBatch)
	// calculate the lower bound of the possible pod count just before the current batch
	podCount := calculateNewBatchTarget(s.rolloutSpec, 0, int(s.rolloutStatus.RolloutTargetSize), currentBatch-1)
	// the recorded number should be at least as much as the all the pods before the current batch
	if podCount > upgradedReplicas {
		err := fmt.Errorf("the upgraded replica in the status is less than all the pods in the previous batch")
		klog.ErrorS(err, "rollout status inconsistent", "upgraded num status", upgradedReplicas,
			"pods in all the previous batches", podCount)
		return false, err
	}

	// calculate the upper bound with the current batch
	podCount = calculateNewBatchTarget(s.rolloutSpec, 0, int(s.rolloutStatus.RolloutTargetSize), currentBatch)
	// the recorded number should be not as much as the all the pods including the active batch
	if podCount < upgradedReplicas {
		err := fmt.Errorf("the upgraded replica in the status is greater than all the pods in the current batch")
		klog.ErrorS(err, "rollout status inconsistent", "total target size", s.rolloutStatus.RolloutTargetSize,
			"upgraded num status", upgradedReplicas, "pods in the batches including the current batch", podCount)
		return false, err
	}
	return true, nil
}

// Finalize makes sure the StatefulSet is all upgraded
func (s *StatefulSetRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := s.fetchStatefulSet(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		return false
	}

	// release StatefulSet
	if _, err := s.releaseStatefulSet(ctx, s.statefulSet); err != nil {
		return false
	}

	// mark the resource finalized
	s.rolloutStatus.LastAppliedPodTemplateIdentifier = s.rolloutStatus.NewPodTemplateIdentifier
	s.recorder.Event(s.parentController, event.Normal("Rollout Finalized",
		fmt.Sprintf("Rollout resource are finalized, succeed := %t", succeed)))
	return true
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

func (s *StatefulSetRolloutController) fetchStatefulSet(ctx context.Context) error {
	workload := appsv1.StatefulSet{}
	if err := s.client.Get(ctx, s.targetNamespacedName, &workload); err != nil {
		if !apierrors.IsNotFound(err) {
			s.recorder.Event(s.parentController, event.Warning("Failed to get the StatefulSet", err))
		}
		return err
	}
	s.statefulSet = &workload
	return nil
}

func (s *StatefulSetRolloutController) size(ctx context.Context) (int32, error) {
	if s.statefulSet == nil {
		if err := s.fetchStatefulSet(ctx); err != nil {
			return 0, err
		}
	}
	// default is 1
	return getStatefulSetReplicas(s.statefulSet), nil
}
