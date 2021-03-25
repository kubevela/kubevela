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

// cloneSetScaleHandler is responsible for CloneSet scale
type cloneSetScaleHandler struct {
	rolloutStatus *v1alpha1.RolloutStatus
	rolloutSpec   *v1alpha1.RolloutPlan
}

// NewCloneSetScaleController creates CloneSet scale controller
func NewCloneSetScaleController(client client.Client, recorder event.Recorder, parentController oam.Object, rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus, workloadName types.NamespacedName) WorkloadController {
	return &CloneSetController{
		client:                 client,
		recorder:               recorder,
		parentController:       parentController,
		rolloutSpec:            rolloutSpec,
		rolloutStatus:          rolloutStatus,
		workloadNamespacedName: workloadName,
		handler: &cloneSetScaleHandler{
			rolloutSpec:   rolloutSpec,
			rolloutStatus: rolloutStatus,
		},
	}
}

func (s *cloneSetScaleHandler) verifySpec(cloneSet *kruise.CloneSet) error {
	cloneSetSize := *cloneSet.Spec.Replicas
	// the rollout has to have a target size in the scale case
	if s.rolloutSpec.TargetSize == nil {
		return fmt.Errorf("the rollout plan is attempting to scale the cloneset %s without a target", cloneSet.Name)
	}

	// record the size
	targetReplicas := *s.rolloutSpec.TargetSize
	klog.InfoS("record the rollout target size", "cloneset replicas", targetReplicas)
	s.rolloutStatus.RolloutTargetTotalSize = targetReplicas
	if cloneSetSize <= targetReplicas {
		return s.verifyIncrease(int(cloneSetSize), int(targetReplicas))
	}
	return s.verifyDecrease(int(cloneSetSize), int(targetReplicas))
}

func (s *cloneSetScaleHandler) initialize(cloneSet *kruise.CloneSet) {
	// make sure that all pod are upgraded to the desired version
	cloneSet.Spec.UpdateStrategy.Partition = nil
}

// rolloutOneBatchPods update CloneSet spec replicas directly
func (s *cloneSetScaleHandler) rolloutOneBatchPods(cloneSet *kruise.CloneSet) int32 {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize := size(cloneSet)
	newPodTarget := int32(s.calculateNewPodTarget(int(cloneSetSize)))
	// set the replicas as the desired number of pods
	cloneSet.Spec.Replicas = &newPodTarget
	klog.InfoS("scale CloneSet", "scale replicas target", newPodTarget)
	return newPodTarget
}

func (s *cloneSetScaleHandler) checkOneBatchPods(cloneSet *kruise.CloneSet) (bool, error) {
	return false, nil
}

// TODO: try to combine the increase and decrease verification
func (s *cloneSetScaleHandler) verifyIncrease(originalSize, targetSize int) error {
	// verify that all the batches add up to the target size starting from original size
	totalRollout := originalSize
	for i := 0; i < len(s.rolloutSpec.RolloutBatches)-1; i++ {
		rb := s.rolloutSpec.RolloutBatches[i]
		batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, targetSize, true)
		totalRollout += batchSize
	}
	if totalRollout >= targetSize {
		return fmt.Errorf("the rollout plan increased too much, total batch size = %d, targetSize size = %d",
			totalRollout, targetSize)
	}
	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := s.rolloutSpec.RolloutBatches[len(s.rolloutSpec.RolloutBatches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		totalRollout += int(lastBatch.Replicas.IntVal)
		// now that they should be the same
		if totalRollout != targetSize {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, targetSize size = %d",
				totalRollout, targetSize)
		}
	}
	return nil
}

func (s *cloneSetScaleHandler) verifyDecrease(originalSize, targetSize int) error {
	// verify that all the batches reduce to the target size starting from original size
	totalRollout := originalSize
	for i := 0; i < len(s.rolloutSpec.RolloutBatches)-1; i++ {
		rb := s.rolloutSpec.RolloutBatches[i]
		batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, targetSize, true)
		totalRollout -= batchSize
	}
	if totalRollout <= targetSize {
		return fmt.Errorf("the rollout plan reduced too much, total batch size = %d, targetSize size = %d",
			totalRollout, originalSize)
	}
	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := s.rolloutSpec.RolloutBatches[len(s.rolloutSpec.RolloutBatches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		totalRollout -= int(lastBatch.Replicas.IntVal)
		// now that they should be the same
		if totalRollout != targetSize {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, targetSize size = %d",
				totalRollout, targetSize)
		}
	}
	return nil
}

// TODO: Fix this
func (s *cloneSetScaleHandler) calculateNewPodTarget(cloneSetSize int) int {
	currentBatch := int(s.rolloutStatus.CurrentBatch)
	newPodTarget := 0
	if currentBatch == len(s.rolloutSpec.RolloutBatches)-1 {
		newPodTarget = cloneSetSize
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.InfoS("use the cloneset size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new version pod target", newPodTarget)
	} else {
		for i, r := range s.rolloutSpec.RolloutBatches {
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
