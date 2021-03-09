package workloads

import (
	"context"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// WorkloadController is the interface that all type of cloneSet controller implements
type WorkloadController interface {
	// Size returns the total number of pods in the resources according to the spec
	Size(ctx context.Context) (int32, error)

	// Verify makes sure that the resources can be upgraded according to the rollout plan
	// it returns new rollout status
	Verify(ctx context.Context) *v1alpha1.RolloutStatus

	// Initialize make sure that the resource is ready to be upgraded
	// this function is tasked to change rollout status
	Initialize(ctx context.Context) *v1alpha1.RolloutStatus

	// RolloutOneBatchPods tries to upgrade pods in the resources following the rollout plan
	// it will upgrade as many pods as the rollout plan allows at once, the routine does not block on any operations.
	// Instead, we rely on the go-client's requeue mechanism to drive this towards the spec goal
	// it returns the number of pods upgraded in this round
	RolloutOneBatchPods(ctx context.Context) *v1alpha1.RolloutStatus

	// CheckOneBatchPods tries to upgrade pods in the resources following the rollout plan
	// it will upgrade as many pods as the rollout plan allows at once, the routine does not block on any operations.
	// Instead, we rely on the go-client's requeue mechanism to drive this towards the spec goal
	// it returns the number of pods upgraded in this round
	CheckOneBatchPods(ctx context.Context) *v1alpha1.RolloutStatus

	// FinalizeOneBatch makes sure that the rollout can start the next batch
	// it also needs to handle the corner cases around the very last batch
	FinalizeOneBatch(ctx context.Context) *v1alpha1.RolloutStatus

	// Finalize makes sure the resources are in a good final state.
	// For example, we may remove the source object to prevent scalar traits to ever work
	// and the finalize rollout web hooks will be called after this call succeeds
	Finalize(ctx context.Context) (*v1alpha1.RolloutStatus, error)
}
