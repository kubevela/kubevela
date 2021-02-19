package v1alpha1

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// RolloutStrategyType defines strategies for pods rollout
type RolloutStrategyType string

const (
	// IncreaseFirstRolloutStrategyType indicates that we increase the target resources first
	IncreaseFirstRolloutStrategyType RolloutStrategyType = "IncreaseFirst"

	// DecreaseFirstRolloutStrategyType indicates that we decrease the source resources first
	DecreaseFirstRolloutStrategyType RolloutStrategyType = "DecreaseFirst"
)

// HookType can be pre, post or during rollout
type HookType string

const (
	// InitializeRolloutHook execute webhook during the rollout initializing phase
	InitializeRolloutHook HookType = "initialize-rollout"
	// PreBatchRolloutHook execute webhook before each batch rollout
	PreBatchRolloutHook HookType = "pre-batch-rollout"
	// PostBatchRolloutHook execute webhook after each batch rollout
	PostBatchRolloutHook HookType = "post-batch-rollout"
	// FinalizeRolloutHook execute the webhook during the rollout finalizing phase
	FinalizeRolloutHook HookType = "finalize-rollout"
)

// RollingState is the overall rollout state
type RollingState string

const (
	// VerifyingState verify that the rollout setting is valid and the controller can locate both the
	// target and the source
	VerifyingState RollingState = "verifying"
	// InitializingState rollout is initializing all the new resources
	InitializingState RollingState = "initializing"
	// RollingInBatchesState rolling out
	RollingInBatchesState RollingState = "rollingInBatches"
	// FinalisingState finalize the rolling, possibly clean up the old resources, adjust traffic
	FinalisingState RollingState = "finalising"
	// RolloutSucceedState rollout successfully completed to match the desired target state
	RolloutSucceedState RollingState = "rolloutSucceed"
	// RolloutFailedState rollout is failed, the target replica is not reached
	// we can not move forward anymore
	// we will let the client to decide when or whether to revert
	RolloutFailedState RollingState = "rolloutFailed"
)

// BatchRollingState is the sub state when the rollout is on the fly
type BatchRollingState string

const (
	// BatchInitializingState still rolling the batch, the batch rolling is not completed yet
	BatchInitializingState BatchRollingState = "batchInitializing"
	// BatchInRollingState still rolling the batch, the batch rolling is not completed yet
	BatchInRollingState BatchRollingState = "batchInRolling"
	// BatchVerifyingState verifying if the application is ready to roll.
	BatchVerifyingState BatchRollingState = "batchVerifying"
	// BatchRolloutFailedState indicates that the batch didn't get the manual or automatic approval
	BatchRolloutFailedState BatchRollingState = "batchVerifyFailed"
	// BatchFinalizingState indicates that all the pods in the are available, we can move on to the next batch
	BatchFinalizingState BatchRollingState = "batchFinalizing"
	// BatchReadyState indicates that all the pods in the are upgraded and its state is ready
	BatchReadyState BatchRollingState = "batchReady"
)

// RolloutPlan fines the details of the rollout plan
type RolloutPlan struct {

	// RolloutStrategy defines strategies for the rollout plan
	// +optional
	RolloutStrategy *RolloutStrategyType `json:"rolloutStrategy,omitempty"`

	// The size of the target resource. The default is the same
	// as the size of the source resource.
	// +optional
	TargetSize *int32 `json:"targetSize,omitempty"`

	// The number of batches, default = 1
	// +optional
	NumBatches *int32 `json:"numBatches,omitempty"`

	// The exact distribution among batches.
	// its size has to be exactly the same as the NumBatches (if set)
	// The total number cannot exceed the targetSize or the size of the source resource
	// We will IGNORE the last batch's replica field if it's a percentage since round errors can lead to inaccurate sum
	// We highly recommend to leave the last batch's replica field empty
	// +optional
	RolloutBatches []RolloutBatch `json:"rolloutBatches,omitempty"`

	// All pods in the batches up to the batchPartition (included) will have
	// the target resource specification while the rest still have the source resource
	// This is designed for the operators to manually rollout
	// Default is the the number of batches which will rollout all the batches
	// +optional
	BatchPartition *int32 `json:"lastBatchToRollout,omitempty"`

	// Paused the rollout, default is false
	// +optional
	Paused bool `json:"paused,omitempty"`

	// RolloutWebhooks provide a way for the rollout to interact with an external process
	// +optional
	RolloutWebhooks []RolloutWebhook `json:"rolloutWebhooks,omitempty"`

	// CanaryMetric provides a way for the rollout process to automatically check certain metrics
	// before complete the process
	// +optional
	CanaryMetric []CanaryMetric `json:"canaryMetric,omitempty"`
}

// RolloutBatch is used to describe how the each batch rollout should be
type RolloutBatch struct {
	// Replicas is the number of pods to upgrade in this batch
	// it can be an absolute number (ex: 5) or a percentage of total pods
	// we will ignore the percentage of the last batch to just fill the gap
	// +optional
	// it is mutually exclusive with the PodList field
	Replicas intstr.IntOrString `json:"replicas,omitempty"`

	// The list of Pods to get upgraded
	// +optional
	// it is mutually exclusive with the Replicas field
	PodList []string `json:"podList,omitempty"`

	// MaxUnavailable is the max allowed number of pods that is unavailable
	// during the upgrade. We will mark the batch as ready as long as there are less
	// or equal number of pods unavailable than this number.
	// default = 0
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// The wait time, in seconds, between instances upgrades, default = 0
	// +optional
	InstanceInterval *int32 `json:"instanceInterval,omitempty"`

	// RolloutWebhooks provides a way for the batch rollout to interact with an external process
	// +optional
	BatchRolloutWebhooks []RolloutWebhook `json:"batchRolloutWebhooks,omitempty"`

	// CanaryMetric provides a way for the batch rollout process to automatically check certain metrics
	// before moving to the next batch
	// +optional
	CanaryMetric []CanaryMetric `json:"canaryMetric,omitempty"`
}

// RolloutWebhook holds the reference to external checks used for canary analysis
type RolloutWebhook struct {
	// Type of this webhook
	Type HookType `json:"type"`

	// Name of this webhook
	Name string `json:"name"`

	// URL address of this webhook
	URL string `json:"url"`

	// ExpectedStatus a
	ExpectedStatus []int `json:"expectedStatus,omitempty"`
	// Metadata (key-value pairs) for this webhook
	// +optional
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// RolloutWebhookPayload holds the info and metadata sent to webhooks
type RolloutWebhookPayload struct {
	// Name of the upgrading resource
	Name string `json:"name"`

	// Namespace of the upgrading resource
	Namespace string `json:"namespace"`

	// Phase of the rollout
	Phase RollingState `json:"phase"`

	// Metadata (key-value pairs) are the extra data send to this webhook
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CanaryMetric holds the reference to metrics used for canary analysis
type CanaryMetric struct {
	// Name of the metric
	Name string `json:"name"`

	// Interval represents the windows size
	Interval string `json:"interval,omitempty"`

	// Range value accepted for this metric
	// +optional
	MetricsRange *MetricsExpectedRange `json:"metricsRange,omitempty"`

	// TemplateRef references a metric template object
	// +optional
	TemplateRef *runtimev1alpha1.TypedReference `json:"templateRef,omitempty"`
}

// MetricsExpectedRange defines the range used for metrics validation
type MetricsExpectedRange struct {
	// Minimum value
	// +optional
	Min *intstr.IntOrString `json:"min,omitempty"`

	// Maximum value
	// +optional
	Max *intstr.IntOrString `json:"max,omitempty"`
}

// RolloutStatus defines the observed state of a rollout plan
type RolloutStatus struct {
	// Conditions represents the latest available observations of a CloneSet's current state.
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// NewPodTemplateIdentifier is a string that uniquely represent the new pod template
	// each workload type could use different ways to identify that so we cannot compare between resources
	NewPodTemplateIdentifier string `json:"targetGeneration,omitempty"`

	// lastAppliedPodTemplateIdentifier is a string that uniquely represent the last pod template
	// each workload type could use different ways to identify that so we cannot compare between resources
	// We update this field only after a successful rollout
	LastAppliedPodTemplateIdentifier string `json:"lastAppliedPodTemplateIdentifier,omitempty"`

	// RollingState is the Rollout State
	RollingState RollingState `json:"rollingState"`

	// BatchRollingState only meaningful when the Status is rolling
	// +optional
	BatchRollingState BatchRollingState `json:"batchRollingState"`

	// The current batch the rollout is working on/blocked
	// it starts from 0
	CurrentBatch int32 `json:"currentBatch"`

	// UpgradedReplicas is the number of Pods upgraded by the rollout controller
	UpgradedReplicas int32 `json:"upgradedReplicas"`

	// UpgradedReplicas is the number of Pods upgraded by the rollout controller that have a Ready Condition.
	UpgradedReadyReplicas int32 `json:"upgradedReadyReplicas"`
}
