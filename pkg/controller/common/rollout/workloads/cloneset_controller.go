package workloads

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// cloneSetHandler is extension point of CloneSetController
type cloneSetHandler interface {
	verifySpec(loneSet *kruise.CloneSet) error

	initialize(cloneSet *kruise.CloneSet)

	rolloutOneBatchPods(cloneSet *kruise.CloneSet) int32

	checkOneBatchPods(cloneSet *kruise.CloneSet) (bool, error)
}

// CloneSetController is responsible for handle Cloneset type of workloads
type CloneSetController struct {
	client           client.Client
	recorder         event.Recorder
	parentController oam.Object

	rolloutSpec            *v1alpha1.RolloutPlan
	rolloutStatus          *v1alpha1.RolloutStatus
	workloadNamespacedName types.NamespacedName
	cloneSet               *kruise.CloneSet

	handler cloneSetHandler
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

	// fetch the cloneset
	verifyErr = c.fetchCloneSet(ctx)
	if verifyErr != nil {
		// do not fail the rollout because we can't get the resource
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		// nolint: nilerr
		return false, nil
	}

	// make sure that the updateRevision is different from what we have already done
	targetHash := c.cloneSet.Status.UpdateRevision
	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		return false, fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
	}
	// record the new pod template hash
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash
	// rollout type specific verification
	if verifyErr = c.handler.verifySpec(c.cloneSet); verifyErr != nil {
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
	if err := c.fetchCloneSet(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}

	// add the parent controller to the owner of the cloneset
	// before kicking start the update and start from every pod in the old version
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	ref := metav1.NewControllerRef(c.parentController, v1beta1.AppRolloutKindVersionKind)
	c.cloneSet.SetOwnerReferences(append(c.cloneSet.GetOwnerReferences(), *ref))
	c.cloneSet.Spec.UpdateStrategy.Paused = false
	// handler specific logic
	c.handler.initialize(c.cloneSet)

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
	if err := c.fetchCloneSet(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	// set the Partition as the desired number of pods in old revisions.
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	// rollout type specific handling
	newPodTarget := c.handler.rolloutOneBatchPods(c.cloneSet)

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
	c.rolloutStatus.UpgradedReplicas = newPodTarget
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (c *CloneSetController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, nil
	}
	verified, err := c.handler.checkOneBatchPods(c.cloneSet)
	if verified && err == nil {
		c.recorder.Event(c.parentController, event.Normal("Batch Available",
			fmt.Sprintf("Batch %d is available", c.rolloutStatus.CurrentBatch)))
	}
	return verified, err
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
// size returns the replicas of a cloneset(not the actual number of pods)
func size(cloneSet *kruise.CloneSet) int32 {
	// default is 1
	if cloneSet.Spec.Replicas == nil {
		return 1
	}
	return *cloneSet.Spec.Replicas
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
