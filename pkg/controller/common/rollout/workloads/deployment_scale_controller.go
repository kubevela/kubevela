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
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DeploymentScaleController is responsible for handle scale Deployment type of workloads
type DeploymentScaleController struct {
	workloadController
	targetNamespacedName types.NamespacedName
	deploy               *appsv1.Deployment
}

// NewDeploymentScaleController creates Deployment scale controller
func NewDeploymentScaleController(client client.Client, recorder event.Recorder, parentController oam.Object, rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) *DeploymentScaleController {
	return &DeploymentScaleController{
		workloadController: workloadController{
			client:           client,
			recorder:         recorder,
			parentController: parentController,
			rolloutSpec:      rolloutSpec,
			rolloutStatus:    rolloutStatus,
		},
		targetNamespacedName: workloadName,
	}
}

// VerifySpec verifies that the deployment is stable and can be scaled
func (s *DeploymentScaleController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error
	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			s.recorder.Event(s.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	// the rollout has to have a target size in the scale case
	if s.rolloutSpec.TargetSize == nil {
		return false, fmt.Errorf("the rollout plan is attempting to scale the deployment %s without a target",
			s.targetNamespacedName.Name)
	}
	// record the target size
	s.rolloutStatus.RolloutTargetSize = *s.rolloutSpec.TargetSize
	klog.InfoS("record the target size", "target size", *s.rolloutSpec.TargetSize)

	// fetch the deployment and get its current size
	originalSize, verifyErr := s.size(ctx)
	if verifyErr != nil {
		// do not fail the rollout because we can't get the resource
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint: nilerr
		return false, nil
	}
	s.rolloutStatus.RolloutOriginalSize = originalSize
	klog.InfoS("record the original size", "original size", originalSize)

	// check if the rollout batch replicas scale up/down to the replicas target
	if verifyErr = verifyBatchesWithScale(s.rolloutSpec, int(originalSize),
		int(s.rolloutStatus.RolloutTargetSize)); verifyErr != nil {
		return false, verifyErr
	}

	// check if the deployment is scaling
	if originalSize != s.deploy.Status.Replicas {
		verifyErr = fmt.Errorf("the deployment %s is in the middle of scaling, target size = %d, real size = %d",
			s.deploy.GetName(), originalSize, s.deploy.Status.Replicas)
		// do not fail the rollout, we can wait
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		return false, nil
	}

	// check if the deployment is upgrading
	if !s.deploy.Spec.Paused && s.deploy.Status.UpdatedReplicas != originalSize {
		verifyErr = fmt.Errorf("the deployment %s is in the middle of updating, target size = %d, updated pod = %d",
			s.deploy.GetName(), originalSize, s.deploy.Status.UpdatedReplicas)
		// do not fail the rollout, we can wait
		s.rolloutStatus.RolloutRetry(verifyErr.Error())
		return false, nil
	}

	// check if the deployment has any controller
	if controller := metav1.GetControllerOf(s.deploy); controller != nil {
		return false, fmt.Errorf("the deployment %s has a controller owner %s",
			s.deploy.GetName(), controller.String())
	}

	// mark the scale verified
	s.recorder.Event(s.parentController, event.Normal("Scale Verified",
		"Rollout spec and the deployment resource are verified"))
	return true, nil
}

// Initialize makes sure that the deployment is under our control
func (s *DeploymentScaleController) Initialize(ctx context.Context) (bool, error) {
	err := s.fetchDeployment(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		// nolint: nilerr
		return false, nil
	}

	if controller := metav1.GetControllerOf(s.deploy); controller != nil {
		if controller.Kind == v1beta1.AppRolloutKind && controller.APIVersion == v1beta1.SchemeGroupVersion.String() {
			// it's already there
			return true, nil
		}
	}
	// add the parent controller to the owner of the deployment
	deployPatch := client.MergeFrom(s.deploy.DeepCopyObject())
	ref := metav1.NewControllerRef(s.parentController, v1beta1.AppRolloutKindVersionKind)
	s.deploy.SetOwnerReferences(append(s.deploy.GetOwnerReferences(), *ref))
	s.deploy.Spec.Paused = false

	// patch the deployment
	if err := s.client.Patch(ctx, s.deploy, deployPatch, client.FieldOwner(s.parentController.GetUID())); err != nil {
		s.recorder.Event(s.parentController, event.Warning("Failed to the start the deployment update", err))
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	// mark the rollout initialized
	s.recorder.Event(s.parentController, event.Normal("Scale Initialized", "deployment is initialized"))
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can scale to according to the rollout spec
func (s *DeploymentScaleController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	err := s.fetchDeployment(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		// nolint: nilerr
		return false, nil
	}

	deployPatch := client.MergeFrom(s.deploy.DeepCopyObject())
	// set the replica according to the batch
	newPodTarget := calculateNewBatchTarget(s.rolloutSpec, int(s.rolloutStatus.RolloutOriginalSize),
		int(s.rolloutStatus.RolloutTargetSize), int(s.rolloutStatus.CurrentBatch))
	s.deploy.Spec.Replicas = pointer.Int32Ptr(int32(newPodTarget))
	// patch the deployment
	if err := s.client.Patch(ctx, s.deploy, deployPatch, client.FieldOwner(s.parentController.GetUID())); err != nil {
		s.recorder.Event(s.parentController, event.Warning("Failed to update the deployment to upgrade", err))
		s.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	// record the scale
	klog.InfoS("scale one batch", "current batch", s.rolloutStatus.CurrentBatch)
	s.recorder.Event(s.parentController, event.Normal("Batch Rollout",
		fmt.Sprintf("Submitted scale quest for batch %d", s.rolloutStatus.CurrentBatch)))
	s.rolloutStatus.UpgradedReplicas = int32(newPodTarget)
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are scaled according to the rollout plan
func (s *DeploymentScaleController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	err := s.fetchDeployment(ctx)
	if err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		// nolint:nilerr
		return false, nil
	}
	newPodTarget := calculateNewBatchTarget(s.rolloutSpec, int(s.rolloutStatus.RolloutOriginalSize),
		int(s.rolloutStatus.RolloutTargetSize), int(s.rolloutStatus.CurrentBatch))
	// get the number of ready pod from deployment
	// TODO: should we use the replica number when we shrink?
	readyPodCount := int(s.deploy.Status.ReadyReplicas)
	currentBatch := s.rolloutSpec.RolloutBatches[s.rolloutStatus.CurrentBatch]
	unavail := 0
	if currentBatch.MaxUnavailable != nil {
		unavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable,
			util.Abs(int(s.rolloutStatus.RolloutTargetSize-s.rolloutStatus.RolloutOriginalSize)), true)
	}
	klog.InfoS("checking the scaling progress", "current batch", s.rolloutStatus.CurrentBatch,
		"new pod count target", newPodTarget, "new ready pod count", readyPodCount,
		"max unavailable pod allowed", unavail)
	s.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)
	targetReached := false
	// nolint
	if s.rolloutStatus.RolloutOriginalSize <= s.rolloutStatus.RolloutTargetSize && unavail+readyPodCount >= newPodTarget {
		targetReached = true
	} else if s.rolloutStatus.RolloutOriginalSize > s.rolloutStatus.RolloutTargetSize && readyPodCount <= newPodTarget {
		targetReached = true
	}
	if targetReached {
		// record the successful upgrade
		klog.InfoS("the current batch is ready", "current batch", s.rolloutStatus.CurrentBatch,
			"target", newPodTarget, "readyPodCount", readyPodCount, "max unavailable allowed", unavail)
		s.recorder.Event(s.parentController, event.Normal("Batch Available",
			fmt.Sprintf("Batch %d is available", s.rolloutStatus.CurrentBatch)))
		return true, nil
	}
	// continue to verify
	klog.InfoS("the batch is not ready yet", "current batch", s.rolloutStatus.CurrentBatch,
		"target", newPodTarget, "readyPodCount", readyPodCount, "max unavailable allowed", unavail)
	s.rolloutStatus.RolloutRetry("the batch is not ready yet")
	return false, nil
}

// FinalizeOneBatch makes sure that the current batch and replica count in the status are validate
func (s *DeploymentScaleController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	status := s.rolloutStatus
	spec := s.rolloutSpec
	if spec.BatchPartition != nil && *spec.BatchPartition < status.CurrentBatch {
		err := fmt.Errorf("the current batch value in the status is greater than the batch partition")
		klog.ErrorS(err, "we have moved past the user defined partition", "user specified batch partition",
			*spec.BatchPartition, "current batch we are working on", status.CurrentBatch)
		return false, err
	}
	// special case the equal case
	if s.rolloutStatus.RolloutOriginalSize == s.rolloutStatus.RolloutTargetSize {
		return true, nil
	}
	// we just make sure the target is right
	finishedPodCount := int(status.UpgradedReplicas)
	currentBatch := int(status.CurrentBatch)
	// calculate the pod target just before the current batch
	preBatchTarget := calculateNewBatchTarget(s.rolloutSpec, int(s.rolloutStatus.RolloutOriginalSize),
		int(s.rolloutStatus.RolloutTargetSize), currentBatch-1)
	// calculate the pod target with the current batch
	curBatchTarget := calculateNewBatchTarget(s.rolloutSpec, int(s.rolloutStatus.RolloutOriginalSize),
		int(s.rolloutStatus.RolloutTargetSize), currentBatch)
	// the recorded number should be at least as much as the all the pods before the current batch
	if finishedPodCount < util.Min(preBatchTarget, curBatchTarget) {
		err := fmt.Errorf("the upgraded replica in the status is less than the lower bound")
		klog.ErrorS(err, "rollout status inconsistent", "existing pod target", finishedPodCount,
			"the lower bound", util.Min(preBatchTarget, curBatchTarget))
		return false, err
	}
	// the recorded number should be not as much as the all the pods including the active batch
	if finishedPodCount > util.Max(preBatchTarget, curBatchTarget) {
		err := fmt.Errorf("the upgraded replica in the status is greater than the upper bound")
		klog.ErrorS(err, "rollout status inconsistent", "existing pod target", finishedPodCount,
			"the upper bound", util.Max(preBatchTarget, curBatchTarget))
		return false, err
	}
	return true, nil
}

// Finalize makes sure the deployment is scaled and ready to use
func (s *DeploymentScaleController) Finalize(ctx context.Context, succeed bool) bool {
	if err := s.fetchDeployment(ctx); err != nil {
		s.rolloutStatus.RolloutRetry(err.Error())
		return false
	}
	deployPatch := client.MergeFrom(s.deploy.DeepCopyObject())
	// remove the parent controller from the resources' owner list
	var newOwnerList []metav1.OwnerReference
	isOwner := false
	for _, owner := range s.deploy.GetOwnerReferences() {
		if owner.Kind == v1beta1.AppRolloutKind && owner.APIVersion == v1beta1.SchemeGroupVersion.String() {
			isOwner = true
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	if !isOwner {
		// nothing to do if we are already not the owner
		klog.InfoS("the deployment is already released and not controlled by rollout", "deployment", s.deploy.Name)
		return true
	}

	s.deploy.SetOwnerReferences(newOwnerList)
	// patch the deployment
	if err := s.client.Patch(ctx, s.deploy, deployPatch, client.FieldOwner(s.parentController.GetUID())); err != nil {
		s.recorder.Event(s.parentController, event.Warning("Failed to the finalize the deployment", err))
		s.rolloutStatus.RolloutRetry(err.Error())
		return false
	}
	// mark the resource finalized
	s.recorder.Event(s.parentController, event.Normal("Scale Finalized",
		fmt.Sprintf("Scale resource are finalized, succeed := %t", succeed)))
	return true
}

// size fetches the Deloyment and returns the replicas (not the actual number of pods)
func (s *DeploymentScaleController) size(ctx context.Context) (int32, error) {
	if s.deploy == nil {
		err := s.fetchDeployment(ctx)
		if err != nil {
			return 0, err
		}
	}
	// default is 1
	if s.deploy.Spec.Replicas == nil {
		return 1, nil
	}
	return *s.deploy.Spec.Replicas, nil
}

func (s *DeploymentScaleController) fetchDeployment(ctx context.Context) error {
	// get the deployment
	workload := appsv1.Deployment{}
	err := s.client.Get(ctx, s.targetNamespacedName, &workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			s.recorder.Event(s.parentController, event.Warning("Failed to get the Deployment", err))
		}
		return err
	}
	s.deploy = &workload
	return nil
}
