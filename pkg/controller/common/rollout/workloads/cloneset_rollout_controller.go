package workloads

import (
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// cloneSetRolloutController is responsible for handle CloneSet rollout.
type cloneSetRolloutHandler struct {
	rolloutStatus *v1alpha1.RolloutStatus
	rolloutSpec   *v1alpha1.RolloutPlan
}

// NewCloneSetRolloutController creates a new CloneSet rollout controller
func NewCloneSetRolloutController(client client.Client, recorder event.Recorder, parentController oam.Object,
	rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) WorkloadController {
	return &CloneSetController{
		client:                 client,
		recorder:               recorder,
		parentController:       parentController,
		rolloutSpec:            rolloutSpec,
		rolloutStatus:          rolloutStatus,
		workloadNamespacedName: workloadName,
		handler: &cloneSetRolloutHandler{
			rolloutSpec:   rolloutSpec,
			rolloutStatus: rolloutStatus,
		},
	}
}

// verifySpec check if the replicas in all the rollout batches add up to the right number
func (r *cloneSetRolloutHandler) verifySpec(cloneSet *kruise.CloneSet) error {
	cloneSetSize := size(cloneSet)
	// record the size
	klog.InfoS("record the rollout target size", "cloneset replicas", cloneSetSize)
	r.rolloutStatus.RolloutTargetTotalSize = cloneSetSize
	// the target size has to be the same as the CloneSet size
	if r.rolloutSpec.TargetSize != nil && *r.rolloutSpec.TargetSize != cloneSetSize {
		return fmt.Errorf("the rollout plan is attempting to scale the cloneset, target = %d, cloneset size = %d",
			*r.rolloutSpec.TargetSize, cloneSetSize)
	}
	// use a common function to check if the sum of all the batches can match the CloneSet size
	return r.verifySumOfBatchSizes(cloneSetSize)
}

// initialize makes sure that the CloneSet keep all old revision pods before start rollout.
func (r *cloneSetRolloutHandler) initialize(cloneSet *kruise.CloneSet) {
	cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: *cloneSet.Spec.Replicas}
}

// rolloutOneBatchPods set the CloneSet partition with newPodTarget, return if we are done
func (r *cloneSetRolloutHandler) rolloutOneBatchPods(cloneSet *kruise.CloneSet) int32 {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize := size(cloneSet)
	newPodTarget := int32(r.calculateNewPodTarget(int(cloneSetSize)))
	// set the Partition as the desired number of pods in old revisions.
	cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int,
		IntVal: cloneSetSize - newPodTarget}
	klog.InfoS("upgrade CloneSet", "upgrade replicas", newPodTarget)
	return newPodTarget
}

func (r *cloneSetRolloutHandler) checkOneBatchPods(cloneSet *kruise.CloneSet) (bool, error) {
	cloneSetSize := size(cloneSet)
	newPodTarget := r.calculateNewPodTarget(int(cloneSetSize))
	// get the number of ready pod from cloneset
	readyPodCount := int(cloneSet.Status.UpdatedReadyReplicas)
	currentBatch := r.rolloutSpec.RolloutBatches[r.rolloutStatus.CurrentBatch]
	unavail := 0
	if currentBatch.MaxUnavailable != nil {
		unavail, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(cloneSetSize), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", r.rolloutStatus.CurrentBatch,
		"new pod count target", newPodTarget, "new ready pod count", readyPodCount,
		"max unavailable pod allowed", unavail)
	r.rolloutStatus.UpgradedReadyReplicas = int32(readyPodCount)
	// we could overshoot in the revert case when many pods are already upgraded
	if unavail+readyPodCount >= newPodTarget {
		// record the successful upgrade
		klog.InfoS("all pods in current batch are ready", "current batch", r.rolloutStatus.CurrentBatch)
		r.rolloutStatus.LastAppliedPodTemplateIdentifier = r.rolloutStatus.NewPodTemplateIdentifier
		return true, nil
	}
	// continue to verify
	klog.InfoS("the batch is not ready yet", "current batch", r.rolloutStatus.CurrentBatch)
	r.rolloutStatus.RolloutRetry("the batch is not ready yet")
	return false, nil
}

// VerifySumOfBatchSizes verifies that the the sum of all the batch replicas is valid given the total replica
// each batch replica can be absolute or a percentage
func (r *cloneSetRolloutHandler) verifySumOfBatchSizes(totalReplicas int32) error {
	// if not set, the sum of all the batch sizes minus the last batch cannot be more than the totalReplicas
	totalRollout := 0
	for i := 0; i < len(r.rolloutSpec.RolloutBatches)-1; i++ {
		rb := r.rolloutSpec.RolloutBatches[i]
		batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, int(totalReplicas), true)
		totalRollout += batchSize
	}
	if totalRollout >= int(totalReplicas) {
		return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
			totalRollout, totalReplicas)
	}

	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := r.rolloutSpec.RolloutBatches[len(r.rolloutSpec.RolloutBatches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		totalRollout += int(lastBatch.Replicas.IntVal)
		// now that they should be the same
		if totalRollout != int(totalReplicas) {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
				totalRollout, totalReplicas)
		}
	}
	return nil
}

func (r *cloneSetRolloutHandler) calculateNewPodTarget(cloneSetSize int) int {
	currentBatch := int(r.rolloutStatus.CurrentBatch)
	newPodTarget := 0
	if currentBatch == len(r.rolloutSpec.RolloutBatches)-1 {
		newPodTarget = cloneSetSize
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.InfoS("use the cloneset size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new version pod target", newPodTarget)
	} else {
		for i, r := range r.rolloutSpec.RolloutBatches {
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
