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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
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
func (c *CloneSetController) Verify(ctx context.Context) (status *v1alpha1.RolloutStatus) {
	var verifyErr error
	status = c.rolloutStatus

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, event.Warning("VerifyFailed", verifyErr))
		}
	}()

	if verifyErr = c.fetchCloneSet(ctx); verifyErr != nil {
		return
	}

	// make sure that the updateRevision is different from what we have already done
	targetHash := c.cloneSet.Status.UpdateRevision
	if targetHash == c.rolloutStatus.LastAppliedPodTemplateIdentifier {
		verifyErr = fmt.Errorf("there is no difference between the source and target, hash = %s", targetHash)
		c.rolloutStatus.RolloutFailed(verifyErr.Error())
		return
	}
	// record the new pod template hash
	c.rolloutStatus.NewPodTemplateIdentifier = targetHash

	// check if the rollout spec is compatible with the current state
	totalReplicas, _ := c.Size(ctx)

	// check if the rollout batch replicas added up to the Cloneset replicas
	if verifyErr = c.verifyRolloutBatchReplicaValue(totalReplicas); verifyErr != nil {
		c.rolloutStatus.RolloutFailed(verifyErr.Error())
		return
	}

	if !c.cloneSet.Spec.UpdateStrategy.Paused {
		verifyErr = fmt.Errorf("the cloneset %s is in the middle of updating, need to be paused first",
			c.cloneSet.GetName())
		c.rolloutStatus.RolloutRetry(verifyErr.Error())
		return
	}

	// mark the rollout verified
	c.recorder.Event(c.parentController, event.Normal("Verified",
		"Rollout spec and the CloneSet resource are verified"))
	c.rolloutStatus.StateTransition(v1alpha1.RollingSpecVerifiedEvent)
	return c.rolloutStatus
}

// Initialize makes sure that
func (c *CloneSetController) Initialize(ctx context.Context) *v1alpha1.RolloutStatus {
	totalReplicas, err := c.Size(ctx)
	if err != nil {
		return c.rolloutStatus
	}
	// add the parent controller to the owner of the cloneset
	// before kicking start the update and start from every pod in the old version
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	ref := metav1.NewControllerRef(c.parentController, v1alpha2.AppRolloutKindVersionKind)
	c.cloneSet.SetOwnerReferences(append(c.cloneSet.GetOwnerReferences(), *ref))
	c.cloneSet.Spec.UpdateStrategy.Paused = false
	c.cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: totalReplicas}

	// patch the CloneSet
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the start the cloneset update", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return c.rolloutStatus
	}
	// mark the rollout initialized
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
		c.recorder.Event(c.parentController, event.Warning("Failed to update the cloneset to upgrade", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return c.rolloutStatus
	}
	// record the upgrade
	klog.InfoS("upgraded one batch", "current batch", c.rolloutStatus.CurrentBatch)
	c.recorder.Event(c.parentController, event.Normal("Rollout",
		fmt.Sprintf("upgraded the batch num = %d", c.rolloutStatus.CurrentBatch)))
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
	klog.V(common.LogDebug).InfoS("checking the rolling out progress", "current batch", currentBatch,
		"new pod count target", newPodTarget, "new ready pod count", readyPodCount,
		"max unavailable pod allowed", unavail)
	c.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)
	if unavail+readyPodCount >= newPodTarget {
		// record the successful upgrade
		klog.InfoS("pods are ready", "current batch", currentBatch)
		c.recorder.Event(c.parentController, event.Normal("Batch Available",
			fmt.Sprintf("the batch num = %d is available", c.rolloutStatus.CurrentBatch)))
		c.rolloutStatus.StateTransition(v1alpha1.OneBatchAvailableEvent)
		c.rolloutStatus.LastAppliedPodTemplateIdentifier = c.rolloutStatus.NewPodTemplateIdentifier
	} else {
		// continue to verify
		klog.V(common.LogDebug).InfoS("the batch is not ready yet", "current batch", currentBatch)
		c.rolloutStatus.RolloutRetry("the batch is not ready yet")
	}
	return c.rolloutStatus
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (c *CloneSetController) FinalizeOneBatch(ctx context.Context) *v1alpha1.RolloutStatus {
	// nothing to do for now
	return c.rolloutStatus
}

// Finalize makes sure the Cloneset is all upgraded
func (c *CloneSetController) Finalize(ctx context.Context) (*v1alpha1.RolloutStatus, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		return c.rolloutStatus, err
	}
	clonePatch := client.MergeFrom(c.cloneSet.DeepCopyObject())
	// remove the parent controller from the resources' owner list
	var newOwnerList []metav1.OwnerReference
	for _, owner := range c.cloneSet.GetOwnerReferences() {
		if owner.Kind == v1alpha2.AppRolloutKind && owner.APIVersion == v1alpha2.SchemeGroupVersion.String() {
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	c.cloneSet.SetOwnerReferences(newOwnerList)
	// patch the CloneSet
	if err := c.client.Patch(ctx, c.cloneSet, clonePatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the finalize the cloneset", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return c.rolloutStatus, err
	}
	// mark the resource finalized
	c.recorder.Event(c.parentController, event.Normal("Finalized", "Rollout resource are finalized"))
	return c.rolloutStatus, nil
}

/* --------------------
The functions below are helper functions
--------------------- */
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
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}
	c.cloneSet = &workload
	return nil
}

func (c *CloneSetController) calculateNewPodTarget(cloneSetSize int) int {
	currentBatch := int(c.rolloutStatus.CurrentBatch)
	newPodTarget := 0
	if currentBatch == len(c.rolloutSpec.RolloutBatches)-1 {
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.InfoS("use the cloneset size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new version pod target", newPodTarget)
		newPodTarget = cloneSetSize
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
