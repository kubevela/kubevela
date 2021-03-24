package workloads

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var csRolloutHandler cloneSetHandler = &cloneSetRolloutHandler{}

// cloneSetRolloutController is responsible for handle CloneSet rollout.
type cloneSetRolloutHandler struct {
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
		handler:                csRolloutHandler,
	}
}

// Initial makes sure that the CloneSet keep all old revision pods before start rollout.
func (c *cloneSetRolloutHandler) initialize(ctx context.Context, cloneSet *kruise.CloneSet) error {
	totalReplicas := cloneSet.Spec.Replicas
	cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: *totalReplicas}
	return nil
}

// RolloutOneBatch set the CloneSet partition with newPodTarget, return if we are done
func (c *cloneSetRolloutHandler) rolloutOneBatchPods(ctx context.Context, cloneSet *kruise.CloneSet, newPodTarget int) error {
	// calculate what's the total pods that should be upgraded given the currentBatch in the status
	cloneSetSize := cloneSet.Spec.Replicas

	// set the Partition as the desired number of pods in old revisions.
	cloneSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int,
		IntVal: *cloneSetSize - int32(newPodTarget)}
	return nil
}

// VerifySpecReplicas check if the replicas in all the rollout batches add up to the right number
func (c *cloneSetRolloutHandler) verifySpec(rolloutSpec *v1alpha1.RolloutPlan, cloneSet *kruise.CloneSet) error {
	cloneSetSize := cloneSet.Spec.Replicas
	// the target size has to be the same as the CloneSet size
	if rolloutSpec.TargetSize != nil && *rolloutSpec.TargetSize != *cloneSetSize {
		return fmt.Errorf("the rollout plan is attempting to scale the cloneset, target = %d, cloneset size = %d",
			*rolloutSpec.TargetSize, cloneSetSize)
	}
	// use a common function to check if the sum of all the batches can match the CloneSet size
	err := VerifySumOfBatchSizes(rolloutSpec, *cloneSetSize)
	if err != nil {
		return err
	}
	return nil
}
