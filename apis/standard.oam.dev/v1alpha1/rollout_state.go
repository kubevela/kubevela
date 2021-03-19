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

	// RolloutOneBatchEvent indicates that we have rollout one batch
	RolloutOneBatchEvent RolloutEvent = "RolloutOneBatchEvent"

	// OneBatchAvailableEvent indicates that the batch resource is considered available
	// this events comes after we have examine the pod readiness check and traffic shifting if needed
	OneBatchAvailableEvent RolloutEvent = "OneBatchAvailable"

	// BatchRolloutApprovedEvent indicates that we got the approval manually
	BatchRolloutApprovedEvent RolloutEvent = "BatchRolloutApprovedEvent"

	// BatchRolloutFailedEvent indicates that we are waiting for the approval of the
	BatchRolloutFailedEvent RolloutEvent = "BatchRolloutFailedEvent"
)

// These are valid conditions of the rollout.
const (
	// RolloutSpecVerifying indicates that the rollout just started with verification
	RolloutSpecVerifying runtimev1alpha1.ConditionType = "RolloutSpecVerifying"
	// RolloutInitializing means we start to initialize the cluster
	RolloutInitializing runtimev1alpha1.ConditionType = "RolloutInitializing"
	// RolloutInProgress means we are upgrading resources.
	RolloutInProgress runtimev1alpha1.ConditionType = "RolloutInProgress"
	// RolloutFinalizing means the rollout is finalizing
	RolloutFinalizing runtimev1alpha1.ConditionType = "RolloutFinalizing"
	// RolloutFailing means the rollout is failing
	RolloutFailing runtimev1alpha1.ConditionType = "RolloutFailing"
	// RolloutFailed means that the rollout failed.
	RolloutFailed runtimev1alpha1.ConditionType = "RolloutFailed"
	// RolloutSucceed means that the rollout is done.
	RolloutSucceed runtimev1alpha1.ConditionType = "RolloutSucceed"
	// BatchInitializing
	BatchInitializing runtimev1alpha1.ConditionType = "BatchInitializing"
	// BatchPaused
	BatchPaused runtimev1alpha1.ConditionType = "BatchPaused"
	// BatchVerifying
	BatchVerifying runtimev1alpha1.ConditionType = "BatchVerifying"
	// BatchRolloutFailed
	BatchRolloutFailed runtimev1alpha1.ConditionType = "BatchRolloutFailed"
	// BatchFinalizing
	BatchFinalizing runtimev1alpha1.ConditionType = "BatchFinalizing"
	// BatchReady
	BatchReady runtimev1alpha1.ConditionType = "BatchReady"
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
	case VerifyingSpecState:
		return RolloutSpecVerifying

	case InitializingState:
		return RolloutInitializing

	case RollingInBatchesState:
		switch r.BatchRollingState {
		case BatchInitializingState:
			return BatchInitializing

		case BatchVerifyingState:
			return BatchVerifying

		case BatchFinalizingState:
			return BatchFinalizing

		case BatchRolloutFailedState:
			return BatchRolloutFailed

		case BatchReadyState:
			return BatchReady

		default:
			return RolloutInProgress
		}

	case FinalisingState:
		return RolloutFinalizing

	case RolloutFailingState:
		return RolloutFailing

	case RolloutFailedState:
		return RolloutFailed

	case RolloutSucceedState:
		return RolloutSucceed

	default:
		return RolloutFailed
	}
}

// RolloutRetry is a special state transition since we need an error message
func (r *RolloutStatus) RolloutRetry(reason string) {
	// we can still retry, no change on the state
	r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), reason))
}

// RolloutModified is special state transition as we allow it to happen at any time
func (r *RolloutStatus) RolloutModified() {
	r.SetRolloutCondition(NewNegativeCondition(r.getRolloutConditionType(), "Rollout Spec is modified"))
	r.ResetStatus()
}

// RolloutFailed is a special state transition since we need an error message
func (r *RolloutStatus) RolloutFailed(reason string) {
	// set the condition first which depends on the state
	r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), reason))
	r.RollingState = RolloutFailedState
}

// RolloutFailing is a special state transition that always moves the rollout state to the failing state
func (r *RolloutStatus) RolloutFailing(reason string) {
	// set the condition first which depends on the state
	r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), reason))
	r.RollingState = RolloutFailingState
	r.BatchRollingState = BatchInitializingState
}

// ResetStatus resets the status of the rollout to start from beginning
func (r *RolloutStatus) ResetStatus() {
	r.NewPodTemplateIdentifier = ""
	r.RolloutTargetTotalSize = -1
	r.LastAppliedPodTemplateIdentifier = ""
	r.RollingState = VerifyingSpecState
	r.BatchRollingState = BatchInitializingState
	r.CurrentBatch = 0
	r.UpgradedReplicas = 0
	r.UpgradedReadyReplicas = 0
}

// SetRolloutCondition sets the supplied condition, replacing any existing condition
// of the same type unless they are identical.
func (r *RolloutStatus) SetRolloutCondition(new runtimev1alpha1.Condition) {
	exists := false
	for i, existing := range r.Conditions {
		if existing.Type != new.Type {
			continue
		}
		// we want to update the condition when the LTT changes
		if existing.Type == new.Type &&
			existing.Status == new.Status &&
			existing.Reason == new.Reason &&
			existing.Message == new.Message &&
			existing.LastTransitionTime == new.LastTransitionTime {
			exists = true
			continue
		}

		r.Conditions[i] = new
		exists = true
	}
	if !exists {
		r.Conditions = append(r.Conditions, new)
	}

}

// StateTransition is the center place to do rollout state transition
// it returns an error if the transition is invalid
// it changes the coming rollout state if it's valid
func (r *RolloutStatus) StateTransition(event RolloutEvent) {
	rollingState := r.RollingState
	batchRollingState := r.BatchRollingState
	defer func() {
		klog.InfoS("try to execute a rollout state transition",
			"pre rolling state", rollingState,
			"pre batch rolling state", batchRollingState,
			"post rolling state", r.RollingState,
			"post batch rolling state", r.BatchRollingState)
	}()

	// we have special transition for these two types of event
	if event == RollingFailedEvent || event == RollingRetriableFailureEvent {
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))
	}

	switch rollingState {
	case VerifyingSpecState:
		if event == RollingSpecVerifiedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.RollingState = InitializingState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case InitializingState:
		if event == RollingInitializedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.RollingState = RollingInBatchesState
			r.BatchRollingState = BatchInitializingState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RollingInBatchesState:
		r.batchStateTransition(event)
		return

	case FinalisingState:
		if event == RollingFinalizedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.RollingState = RolloutSucceedState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RolloutFailingState:
		if event == RollingFinalizedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.RollingState = RolloutFailedState
			return
		}
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RolloutSucceedState:
		panic(fmt.Errorf(invalidRollingStateTransition, rollingState, event))

	case RolloutFailedState:
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
		r.SetConditions(NewNegativeCondition(r.getRolloutConditionType(), "failed"))
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
		if event == RolloutOneBatchEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.BatchRollingState = BatchVerifyingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchVerifyingState:
		if event == OneBatchAvailableEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.BatchRollingState = BatchFinalizingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchFinalizingState:
		if event == FinishedOneBatchEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.BatchRollingState = BatchReadyState
			return
		}
		if event == AllBatchFinishedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			// transition out of the batch loop
			r.BatchRollingState = BatchReadyState
			r.RollingState = FinalisingState
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	case BatchReadyState:
		if event == BatchRolloutApprovedEvent {
			r.SetRolloutCondition(NewPositiveCondition(r.getRolloutConditionType()))
			r.BatchRollingState = BatchInitializingState
			r.CurrentBatch++
			return
		}
		panic(fmt.Errorf(invalidBatchRollingStateTransition, batchRollingState, event))

	default:
		panic(fmt.Errorf("invalid batch rolling state %s", batchRollingState))
	}
}
