package rollout

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

type rolloutEvent string

const (
	// rollingSpecVerifiedEvent indicates that we have successfully verified that the rollout spec
	rollingSpecVerifiedEvent rolloutEvent = "rollingSpecVerifiedEvent"

	// rollingInitializedEvent indicates that we have finished initializing all the workload resources
	rollingInitializedEvent rolloutEvent = "rollingInitializedEvent"

	// allBatchFinishedEvent indicates that all batches are upgraded
	allBatchFinishedEvent rolloutEvent = "allBatchFinishedEvent"

	// rollingFailedEvent indicates that the rolling is paused
	rollingPausedEvent rolloutEvent = "rollingFailedEvent"

	// rollingResumedEvent indicates that the rolling is resumed
	rollingResumedEvent rolloutEvent = "rollingResumedEvent"

	// rollingFinalizedEvent indicates that we have finalized the rollout which includes but not
	// limited to the resource garbage collection
	rollingFinalizedEvent rolloutEvent = "allBatchFinishedEvent"

	// rollingFailedEvent indicates that we encountered an unexpected error during upgrading
	rollingFailedEvent rolloutEvent = "rollingFailedEvent"

	// initializedOneBatchEvent indicates that we have successfully rolled out one batch
	initializedOneBatchEvent rolloutEvent = "initializedOneBatchEvent"

	// finishedOneBatchEvent indicates that we have successfully rolled out one batch
	finishedOneBatchEvent rolloutEvent = "finishedOneBatchEvent"

	// oneBatchAvailableEvent indicates that the batch resource is considered available
	// this events comes after we have examine the pod readiness check and traffic shifting if needed
	oneBatchAvailableEvent rolloutEvent = "OneBatchAvailable"

	// batchRolloutWaitingEvent indicates that we are waiting for the approval of resume one batch
	batchRolloutWaitingEvent rolloutEvent = "batchWaitRolloutEvent"

	// batchRolloutApprovedEvent indicates that we are waiting for the approval of the
	batchRolloutApprovedEvent rolloutEvent = "batchWaitRolloutEvent"

	// batchRolloutFailedEvent indicates that we are waiting for the approval of the
	batchRolloutFailedEvent rolloutEvent = "batchRolloutFailedEvent"

	// workloadModifiedEvent indicates that the res
	workloadModifiedEvent rolloutEvent = "workloadModifiedEvent"
)

const invalidRollingStateTransition = "the rollout state transition from `%s` state  with `%s` is invalid"

const invalidBatchRollingStateTransition = "the batch rolling state transition from `%s` state  with `%s` is invalid"

// StateTransition is the center place to do rollout state transition
// it returns an error if the transition is invalid
// it changes the coming rollout state if it's valid
// nolint: gocyclo
func StateTransition(rolloutStatus *v1alpha1.RolloutStatus, event rolloutEvent) error {
	rollingState := rolloutStatus.RollingState
	batchRollingState := rolloutStatus.BatchRollingState
	defer klog.InfoS("try to execute a rollout state transition",
		"pre rolling state", rollingState,
		"pre batch rolling state", batchRollingState,
		"post rolling state", rolloutStatus.RollingState,
		"post batch rolling state", rolloutStatus.BatchRollingState)

	// we first process the global event
	if event == rollingFailedEvent {
		rolloutStatus.RollingState = v1alpha1.RolloutFailedState
		return nil
	}
	if event == rollingPausedEvent {
		rolloutStatus.RollingState = v1alpha1.PausedState
		return nil
	}

	switch rollingState {
	case v1alpha1.VerifyingState:
		if event == rollingSpecVerifiedEvent {
			rolloutStatus.RollingState = v1alpha1.InitializingState
			return nil
		}
		return fmt.Errorf(invalidRollingStateTransition, rollingState, event)

	case v1alpha1.InitializingState:
		if event == rollingInitializedEvent {
			rolloutStatus.RollingState = v1alpha1.RollingInBatchesState
			return nil
		}
		return fmt.Errorf(invalidRollingStateTransition, rollingState, event)

	case v1alpha1.PausedState:
		if event == rollingResumedEvent {
			// we don't know where it was last time, need to start from beginning
			// since we don't change the batch rolling state when we pause
			// we should be able to resume if it was rolling before paused
			rolloutStatus.RollingState = v1alpha1.VerifyingState
			return nil
		}
		return fmt.Errorf(invalidBatchRollingStateTransition, rollingState, event)

	case v1alpha1.RollingInBatchesState:
		switch batchRollingState {
		case v1alpha1.BatchInitializingState:
			if event == initializedOneBatchEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchInRollingState
				return nil
			}
			return fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event)

		case v1alpha1.BatchInRollingState:
			if event == batchRolloutWaitingEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchVerifyingState
				return nil
			}
			if event == batchRolloutApprovedEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchReadyState
				return nil
			}
			return fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event)

		case v1alpha1.BatchVerifyingState:
			if event == batchRolloutApprovedEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchReadyState
				return nil
			}
			if event == batchRolloutFailedEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchVerifyFailedState
				rolloutStatus.RollingState = v1alpha1.RolloutFailedState
				return nil
			}
			return fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event)

		case v1alpha1.BatchReadyState:
			if event == oneBatchAvailableEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchAvailableState
				return nil
			}
			return fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event)

		case v1alpha1.BatchAvailableState:
			if event == finishedOneBatchEvent {
				rolloutStatus.BatchRollingState = v1alpha1.BatchInitializingState
				return nil
			}
			if event == allBatchFinishedEvent {
				// transition out of the batch loop
				rolloutStatus.RollingState = v1alpha1.FinalisingState
				return nil
			}
			return fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event)
		default:
			return fmt.Errorf("invalid batch rolling state %s", batchRollingState)
		}

	case v1alpha1.FinalisingState:
		if event == rollingFinalizedEvent {
			rolloutStatus.RollingState = v1alpha1.RolloutSucceedState
			return nil
		}
		return fmt.Errorf(invalidRollingStateTransition, rollingState, event)

	case v1alpha1.RolloutSucceedState:
		if event == workloadModifiedEvent {
			rolloutStatus.RollingState = v1alpha1.VerifyingState
			return nil
		}
		if event == rollingFinalizedEvent {
			// no op
			return nil
		}
		return fmt.Errorf(invalidRollingStateTransition, rollingState, event)

	case v1alpha1.RolloutFailedState:
		if event == workloadModifiedEvent {
			rolloutStatus.RollingState = v1alpha1.VerifyingState
			return nil
		}
		if event == rollingFailedEvent {
			// no op
			return nil
		}
		return fmt.Errorf(invalidRollingStateTransition, rollingState, event)

	default:
		return fmt.Errorf("invalid rolling state %s", rollingState)
	}
}
