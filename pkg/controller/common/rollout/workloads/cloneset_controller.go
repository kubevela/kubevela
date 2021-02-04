package workloads

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// Size fetches the Cloneset and returns the replicas (not the actual number of pods)
func (c *CloneSetController) Size(ctx context.Context) (int32, error) {
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

// Verify verifies that the target rollout resource is consistent with the rollout spec
func (c *CloneSetController) Verify(ctx context.Context) *v1alpha1.RolloutStatus {
	if c.fetchCloneSet(ctx) != nil {
		return c.rolloutStatus
	}

	// make sure that there are changes in the pod template
	targetHash := c.cloneSet.Status.UpdateRevision
	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		err := fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
		klog.Error(err)
		c.rolloutStatus.RolloutFailed(err.Error())
		c.recorder.Event(c.parentController, event.Warning("VerifyFailed", err))
		return c.rolloutStatus
	}
	// record the new pod template hash
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash

	// check if the rollout spec is compatible with the current state
	// 1. the rollout batch is either automatic or zero
	if c.rolloutSpec.BatchPartition != nil && *c.rolloutSpec.BatchPartition != 0 {
		err := fmt.Errorf("the rollout plan has to start from zero, partition= %d", *c.rolloutSpec.BatchPartition)
		klog.Error(err)
		c.rolloutStatus.RolloutFailed(err.Error())
		c.recorder.Event(c.parentController, event.Warning("VerifyFailed", err))
		return c.rolloutStatus
	}
	// 2. the number of old version in the Cloneset equals to the total number
	totalReplicas, _ := c.Size(ctx)
	oldVersionPod, _ := intstr.GetValueFromIntOrPercent(c.cloneSet.Spec.UpdateStrategy.Partition, int(totalReplicas),
		true)
	if oldVersionPod != int(totalReplicas) {
		err := fmt.Errorf("the cloneset was still in the middle of updating, number of old pods= %d", oldVersionPod)
		klog.Error(err)
		c.rolloutStatus.RolloutFailed(err.Error())
		c.recorder.Event(c.parentController, event.Warning("VerifyFailed", err))
		return c.rolloutStatus
	}

	// mark the rollout verified
	c.recorder.Event(c.parentController, event.Normal("Verified",
		"Rollout spec and the CloneSet resource are verified"))
	c.rolloutStatus.StateTransition(v1alpha1.RollingSpecVerifiedEvent)
	return c.rolloutStatus
}

// Initialize makes sure that
func (c *CloneSetController) Initialize(ctx context.Context) *v1alpha1.RolloutStatus {
	if c.fetchCloneSet(ctx) != nil {
		return c.rolloutStatus
	}

	// mark the rollout initialized, there is nothing we need to do for Cloneset for now
	c.recorder.Event(c.parentController, event.Normal("Initialized", "Rollout resource are initialized"))
	c.rolloutStatus.StateTransition(v1alpha1.RollingInitializedEvent)
	return c.rolloutStatus
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (c *CloneSetController) RolloutOneBatchPods(ctx context.Context) *v1alpha1.RolloutStatus {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize, _ := c.Size(ctx)
	newPodTarget := c.calculateNewPodTarget(int(cloneSetSize))
	// set the Partition as the desired number of pods in old revisions.
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	c.cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int,
		IntVal: cloneSetSize - int32(newPodTarget)}
	// patch the Cloneset
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to update the Cloneset", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		c.rolloutStatus.StateTransition(v1alpha1.BatchRolloutContinueEvent)
		return c.rolloutStatus
	}
	// record the upgrade
	c.rolloutStatus.StateTransition(v1alpha1.BatchRolloutVerifyingEvent)
	c.rolloutStatus.UpgradedReplicas = int32(newPodTarget)
	return c.rolloutStatus
}

// CheckOneBatchPods checks to see if the pods are all available according to
func (c *CloneSetController) CheckOneBatchPods(ctx context.Context) *v1alpha1.RolloutStatus {
	cloneSetSize, _ := c.Size(ctx)
	newPodTarget := c.calculateNewPodTarget(int(cloneSetSize))
	// get the number of ready pod from cloneset
	readyPodCount := int(c.cloneSet.Status.UpdatedReadyReplicas)
	currentBatch := c.rolloutSpec.RolloutBatches[c.rolloutStatus.CurrentBatch]
	unavail := 0
	if currentBatch.MaxUnavailable != nil {
		unavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(cloneSetSize), true)
	}
	klog.InfoS("checking the rolling out progress", "new pod count target", newPodTarget,
		"new ready pod count", readyPodCount, "max unavailable pod allowed", unavail)
	c.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)
	if unavail+readyPodCount >= newPodTarget {
		// record the successful upgrade
		c.rolloutStatus.StateTransition(v1alpha1.OneBatchAvailableEvent)
	} else {
		// continue to verify
		c.rolloutStatus.StateTransition(v1alpha1.BatchRolloutVerifyingEvent)
	}
	return c.rolloutStatus
}

// Finalize makes sure the Cloneset is all upgraded and
func (c *CloneSetController) Finalize(ctx context.Context) *v1alpha1.RolloutStatus {
	if c.fetchCloneSet(ctx) != nil {
		return c.rolloutStatus
	}
	// mark the rollout finalized
	c.recorder.Event(c.parentController, event.Normal("Finalized", "Rollout resource are finalized"))
	c.rolloutStatus.StateTransition(v1alpha1.RollingFinalizedEvent)
	return c.rolloutStatus
}

// The functions below are helper functions
func (c *CloneSetController) fetchCloneSet(ctx context.Context) error {
	// get the cloneSet
	workload := kruise.CloneSet{}
	err := c.client.Get(ctx, c.workloadNamespacedName, &workload)
	if err != nil {
		klog.CalculateMaxSize()
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, event.Warning("Failed to get the Cloneset", err))
		}
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}
	c.cloneSet = &workload
	return nil
}

func (c *CloneSetController) calculateNewPodTarget(cloneSetSize int) int {
	currentBatch := c.rolloutStatus.CurrentBatch
	newPodTarget := 0
	for i, r := range c.rolloutSpec.RolloutBatches {
		batchSize, _ := intstr.GetValueFromIntOrPercent(&r.Replicas, cloneSetSize, true)
		if i <= int(currentBatch) {
			newPodTarget += batchSize
		} else {
			break
		}
	}
	klog.InfoS("Calculated the number of new version pod", "new version pod target", newPodTarget)
	return newPodTarget
}
