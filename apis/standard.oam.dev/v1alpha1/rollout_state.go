package v1alpha1

import (
	"fmt"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// RolloutEvent is used to describe the events during rollout
type RolloutEvent string

const (
	// RollingFailedEvent indicates that we encountered an unexpected error during upgrading and can't be retried
	RollingFailedEvent RolloutEvent = "RollingFailedEvent"

	// RollingRetriableFailureEvent indicates that we encountered an unexpected but retriable error
	RollingRetriableFailureEvent RolloutEvent = "RollingRetriableFailureEvent"

	// RollingSpecVerifiedEvent indicates that we have successfully verified that the rollout spec
	RollingSpecVerifiedEvent RolloutEvent = "RollingSpecVerifiedEvent"

	// RollingInitializedEvent indicates that we have finished initializing all the workload resources
	RollingInitializedEvent RolloutEvent = "RollingInitializedEvent"

	// AllBatchFinishedEvent indicates that all batches are upgraded
	AllBatchFinishedEvent RolloutEvent = "AllBatchFinishedEvent"

	// RollingFinalizedEvent indicates that we have finalized the rollout which includes but not
	// limited to the resource garbage collection
	RollingFinalizedEvent RolloutEvent = "AllBatchFinishedEvent"

	// InitializedOneBatchEvent indicates that we have successfully rolled out one batch
	InitializedOneBatchEvent RolloutEvent = "InitializedOneBatchEvent"

	// FinishedOneBatchEvent indicates that we have successfully rolled out one batch
	FinishedOneBatchEvent RolloutEvent = "FinishedOneBatchEvent"

	// BatchRolloutContinueEvent indicates that we need to continue to upgrade the pods in the batch
	BatchRolloutContinueEvent RolloutEvent = "BatchRolloutContinueEvent"

	// BatchRolloutVerifyingEvent indicates that we are waiting for the approval of resume one batch
	BatchRolloutVerifyingEvent RolloutEvent = "BatchRolloutVerifyingEvent"

	// OneBatchAvailableEvent indicates that the batch resource is considered available
	// this events comes after we have examine the pod readiness check and traffic shifting if needed
	OneBatchAvailableEvent RolloutEvent = "OneBatchAvailable"

	// BatchRolloutApprovedEvent indicates that we are waiting for the approval of the
	BatchRolloutApprovedEvent RolloutEvent = "BatchRolloutApprovedEvent"

	// BatchRolloutFailedEvent indicates that we are waiting for the approval of the
	BatchRolloutFailedEvent RolloutEvent = "BatchRolloutFailedEvent"

	// WorkloadModifiedEvent indicates that the res
	WorkloadModifiedEvent RolloutEvent = "WorkloadModifiedEvent"
)

// These are valid conditions of pod.
const (
	// RolloutSpecVerified indicates that the rollout spec matches the resource we have in the cluster
	RolloutSpecVerified runtimev1alpha1.ConditionType = "RolloutSpecVerified"
	// RolloutInitialized means all the needed initialization work is done
	RolloutInitialized runtimev1alpha1.ConditionType = "Initialized"
	// RolloutInProgress means we are upgrading resources.
	RolloutInProgress runtimev1alpha1.ConditionType = "Ready"
	// RolloutSucceed means that the rollout is done.
	RolloutSucceed runtimev1alpha1.ConditionType = "Succeed"
)

// NewPositiveCondition creates a positive condition type
func NewPositiveCondition(condType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               condType,
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

// NewNegativeCondition creates a false condition type
func NewNegativeCondition(condType runtimev1alpha1.ConditionType, message string) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               condType,
		Status:             v1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
	}
}

const invalidRollingStateTransition = "the rollout state transition from `%s` state  with `%s` is invalid"

const invalidBatchRollingStateTransition = "the batch rolling state transition from `%s` state  with `%s` is invalid"

func (r *RolloutStatus) getRolloutConditionType() runtimev1alpha1.ConditionType {
	// figure out which condition type should we put in the condition depends on its state
	switch r.RollingState {
	case VerifyingState:
		return RolloutSpecVerified

	case InitializingState:
		return RolloutInitialized

	case RollingInBatchesState:
		return RolloutInProgress

	case FinalisingState:
		return RolloutSucceed

	default:
		return RolloutSucceed
	}
}

// RolloutRetry is a special state transition since we need an error message
func (r *RolloutStatus) RolloutRetry(reason string) {
	// we can still retry, no change on the state
	r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), reason))
}

// RolloutFailed is a special state transition since we need an error message
func (r *RolloutStatus) RolloutFailed(reason string) {
	// set the condition first which depends on the state
	r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), reason))
	r.RollingState = RolloutFailedState
}

// StateTransition is the center place to do rollout state transition
// it returns an error if the transition is invalid
// it changes the coming rollout state if it's valid
func (r *RolloutStatus) StateTransition(event RolloutEvent) {
	rollingState := r.RollingState
	batchRollingState := r.BatchRollingState
	defer klog.InfoS("try to execute a rollout state transition",
		"pre rolling state", rollingState,
		"pre batch rolling state", batchRollingState,
		"post rolling state", r.RollingState,
		"post batch rolling state", r.BatchRollingState)

	// we have special transition for these two types of event
	if event == RollingFailedEvent || event == RollingRetriableFailureEvent {
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))
	}

	switch rollingState {
	case VerifyingState:
		if event == RollingSpecVerifiedEvent {
			r.RollingState = InitializingState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case InitializingState:
		if event == RollingInitializedEvent {
			r.RollingState = RollingInBatchesState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RollingInBatchesState:
		r.batchStateTransition(event)
		return

	case FinalisingState:
		if event == RollingFinalizedEvent {
			r.RollingState = RolloutSucceedState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RolloutSucceedState:
		if event == WorkloadModifiedEvent {
			r.RollingState = VerifyingState
			return
		}
		if event == RollingFinalizedEvent {
			// no op
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RolloutFailedState:
		if event == WorkloadModifiedEvent {
			r.RollingState = VerifyingState
			return
		}
		if event == RollingFailedEvent {
			// no op
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	default:
		panic(fmt.Errorf("invalid rolling state %s", rollingState))
	}
}

// batchStateTransition handles the state transition when the rollout is in action
func (r *RolloutStatus) batchStateTransition(event RolloutEvent) {
	batchRollingState := r.BatchRollingState
	if event == BatchRolloutFailedEvent {
		r.BatchRollingState = BatchRolloutFailedState
		r.RollingState = RolloutFailedState
		return
	}
	switch batchRollingState {
	case BatchInitializingState:
		if event == InitializedOneBatchEvent {
			r.BatchRollingState = BatchInRollingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchInRollingState:
		if event == BatchRolloutContinueEvent {
			// no op
			return
		}
		if event == BatchRolloutVerifyingEvent {
			r.BatchRollingState = BatchVerifyingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchVerifyingState:
		if event == OneBatchAvailableEvent {
			r.BatchRollingState = BatchReadyState
			return
		}
		if event == BatchRolloutVerifyingEvent {
			// no op
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchReadyState:
		if event == BatchRolloutApprovedEvent {
			r.BatchRollingState = BatchFinalizeState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchFinalizeState:
		if event == FinishedOneBatchEvent {
			r.BatchRollingState = BatchInitializingState
			return
		}
		if event == AllBatchFinishedEvent {
			// transition out of the batch loop
			r.RollingState = FinalisingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	default:
		panic(fmt.Errorf("invalid batch rolling state %s", batchRollingState))
	}
}
