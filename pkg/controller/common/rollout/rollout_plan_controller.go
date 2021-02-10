package rollout

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/common/rollout/workloads"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// the default time to check back if we still have work to do
const rolloutReconcileRequeueTime = 5 * time.Second

// Controller is the controller that controls the rollout plan resource
type Controller struct {
	client           client.Client
	recorder         event.Recorder
	parentController oam.Object

	rolloutSpec   *v1alpha1.RolloutPlan
	rolloutStatus *v1alpha1.RolloutStatus

	targetWorkload *unstructured.Unstructured
	sourceWorkload *unstructured.Unstructured
}

// NewRolloutPlanController creates a RolloutPlanController
func NewRolloutPlanController(client client.Client, parentController oam.Object, recorder event.Recorder,
	rolloutSpec *v1alpha1.RolloutPlan,
	rolloutStatus *v1alpha1.RolloutStatus, targetWorkload,
	sourceWorkload *unstructured.Unstructured) *Controller {
	return &Controller{
		client:           client,
		parentController: parentController,
		recorder:         recorder,
		rolloutSpec:      rolloutSpec.DeepCopy(),
		rolloutStatus:    rolloutStatus.DeepCopy(),
		targetWorkload:   targetWorkload,
		sourceWorkload:   sourceWorkload,
	}
}

// Reconcile reconciles a rollout plan
func (r *Controller) Reconcile(ctx context.Context) (res reconcile.Result, status *v1alpha1.RolloutStatus) {
	klog.InfoS("Reconcile the rollout plan", "rollout Spec", r.rolloutSpec,
		"target workload", klog.KObj(r.targetWorkload))
	if r.sourceWorkload != nil {
		klog.InfoS("we will do rolling upgrades", "source workload", klog.KObj(r.sourceWorkload))
	}
	klog.InfoS("rollout spec ", "rollout state", r.rolloutStatus.RollingState, "batch rolling state",
		r.rolloutStatus.BatchRollingState, "current batch", r.rolloutStatus.CurrentBatch, "upgraded Replicas",
		r.rolloutStatus.UpgradedReplicas)

	defer klog.InfoS("Finished reconciling rollout plan", "rollout state", status.RollingState,
		"batch rolling state", status.BatchRollingState, "current batch", status.CurrentBatch,
		"upgraded Replicas", status.UpgradedReplicas, "reconcile result ", res)

	status = r.rolloutStatus

	defer func() {
		if status.RollingState == v1alpha1.RolloutFailedState ||
			status.RollingState == v1alpha1.RolloutSucceedState {
			// no need to requeue if we reach the terminal states
			res = reconcile.Result{}
		} else {
			res = reconcile.Result{
				RequeueAfter: rolloutReconcileRequeueTime,
			}
		}
	}()

	workloadController, err := r.GetWorkloadController()
	if err != nil {
		r.rolloutStatus.RolloutFailed(err.Error())
		r.recorder.Event(r.parentController, event.Warning("Unsupported workload", err))
		return
	}

	switch r.rolloutStatus.RollingState {
	case v1alpha1.VerifyingState:
		r.rolloutStatus = workloadController.Verify(ctx)

	case v1alpha1.InitializingState:
		if err := r.initializeRollout(ctx); err == nil {
			r.rolloutStatus = workloadController.Initialize(ctx)
		}

	case v1alpha1.RollingInBatchesState:
		r.reconcileBatchInRolling(ctx, workloadController)

	case v1alpha1.FinalisingState:
		r.rolloutStatus = workloadController.Finalize(ctx)
		// if we are still going to finalize it
		if r.rolloutStatus.RollingState == v1alpha1.FinalisingState {
			r.finalizeRollout(ctx)
		}

	case v1alpha1.RolloutSucceedState:
		// Nothing to do

	case v1alpha1.RolloutFailedState:
		// Nothing to do

	default:
		panic(fmt.Sprintf("illegal rollout status %+v", r.rolloutStatus))
	}

	return res, r.rolloutStatus
}

// reconcile logic when we are in the middle of rollout
func (r *Controller) reconcileBatchInRolling(ctx context.Context, workloadController workloads.WorkloadController) {

	if r.rolloutSpec.Paused {
		r.recorder.Event(r.parentController, event.Normal("Rollout paused", "Rollout paused"))
		r.rolloutStatus.SetConditions(v1alpha1.NewPositiveCondition("Paused"))
		return
	}

	// makes sure that the current batch and replica count in the status are validate
	replicas, err := workloadController.Size(ctx)
	if err != nil {
		r.rolloutStatus.RolloutRetry(err.Error())
		return
	}
	r.validateRollingBatchStatus(int(replicas))

	switch r.rolloutStatus.BatchRollingState {
	case v1alpha1.BatchInitializingState:
		r.initializeOneBatch(ctx)

	case v1alpha1.BatchInRollingState:
		//  still rolling the batch, the batch rolling is not completed yet
		r.rolloutStatus = workloadController.RolloutOneBatchPods(ctx)

	case v1alpha1.BatchVerifyingState:
		// verifying if the application is ready to roll
		// need to check if they meet the availability requirements in the rollout spec.
		// TODO: evaluate any metrics/analysis
		r.rolloutStatus = workloadController.CheckOneBatchPods(ctx)

	case v1alpha1.BatchFinalizingState:
		// all the pods in the are available
		r.finalizeOneBatch(ctx)

	case v1alpha1.BatchReadyState:
		// all the pods in the are upgraded and their state are ready
		// wait to move to the next batch if there are any
		r.tryMovingToNextBatch()

	default:
		panic(fmt.Sprintf("illegal status %+v", r.rolloutStatus))
	}
}

// all the common initialize work before we rollout
func (r *Controller) initializeRollout(ctx context.Context) error {
	// call the pre-rollout webhooks
	for _, rw := range r.rolloutSpec.RolloutWebhooks {
		if rw.Type == v1alpha1.InitializeRolloutHook {
			err := callWebhook(ctx, r.parentController, v1alpha1.InitializingState, rw)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rw.Name, "webhook end point", rw.URL)
				r.rolloutStatus.RolloutFailed("failed to invoke a webhook")
				return err
			}
			klog.InfoS("successfully invoked a pre rollout webhook", "webhook name", rw.Name, "webhook end point",
				rw.URL)
		}
	}

	return nil
}

// all the common initialize work before we rollout one batch of resources
func (r *Controller) initializeOneBatch(ctx context.Context) {
	rolloutHooks := r.gatherAllWebhooks()
	// call all the pre-batch rollout webhooks
	for _, rh := range rolloutHooks {
		if rh.Type == v1alpha1.PreBatchRolloutHook {
			err := callWebhook(ctx, r.parentController, v1alpha1.InitializingState, rh)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rh.Name, "webhook end point", rh.URL)
				r.rolloutStatus.RolloutFailed("failed to invoke a webhook")
				return
			}
			klog.InfoS("successfully invoked a pre batch webhook", "webhook name", rh.Name, "webhook end point",
				rh.URL)
		}
	}
	r.rolloutStatus.StateTransition(v1alpha1.InitializedOneBatchEvent)
}

func (r *Controller) gatherAllWebhooks() []v1alpha1.RolloutWebhook {
	// we go through the rollout level webhooks first
	rolloutHooks := r.rolloutSpec.RolloutWebhooks
	// we then append the batch specific rollout webhooks to the overall webhooks
	// order matters here
	currentBatch := int(r.rolloutStatus.CurrentBatch)
	rolloutHooks = append(rolloutHooks, r.rolloutSpec.RolloutBatches[currentBatch].BatchRolloutWebhooks...)
	return rolloutHooks
}

// check if we can move to the next batch
func (r *Controller) tryMovingToNextBatch() {
	if r.rolloutSpec.BatchPartition == nil || *r.rolloutSpec.BatchPartition > r.rolloutStatus.CurrentBatch {
		klog.InfoS("ready to rollout the next batch", "current batch", r.rolloutStatus.CurrentBatch)
		r.rolloutStatus.StateTransition(v1alpha1.BatchRolloutApprovedEvent)
	} else {
		klog.V(common.LogDebug).InfoS("the current batch is waiting to move on", "current batch",
			r.rolloutStatus.CurrentBatch)
	}
}

func (r *Controller) finalizeOneBatch(ctx context.Context) {
	rolloutHooks := r.gatherAllWebhooks()
	// call all the post-batch rollout webhooks
	for _, rh := range rolloutHooks {
		if rh.Type == v1alpha1.PostBatchRolloutHook {
			err := callWebhook(ctx, r.parentController, v1alpha1.FinalisingState, rh)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rh.Name, "webhook end point", rh.URL)
				r.rolloutStatus.RolloutFailed("failed to invoke a webhook")
				return
			}
			klog.InfoS("successfully invoked a post batch webhook", "webhook name", rh.Name, "webhook end point",
				rh.URL)
		}
	}
	// calculate the next phase
	currentBatch := int(r.rolloutStatus.CurrentBatch)
	if currentBatch == len(r.rolloutSpec.RolloutBatches)-1 {
		// this is the last batch, mark the rollout finalized
		r.rolloutStatus.StateTransition(v1alpha1.AllBatchFinishedEvent)
		r.recorder.Event(r.parentController, event.Normal("all batches rolled out",
			fmt.Sprintf("upgrade pod = %d, total ready pod = %d", r.rolloutStatus.UpgradedReplicas,
				r.rolloutStatus.UpgradedReadyReplicas)))
	} else {
		klog.InfoS("finished one batch rollout", "current batch", r.rolloutStatus.CurrentBatch)
		// th
		r.recorder.Event(r.parentController, event.Normal("Batch finalized",
			fmt.Sprintf("the batch num = %d is ready", r.rolloutStatus.CurrentBatch)))
		r.rolloutStatus.StateTransition(v1alpha1.FinishedOneBatchEvent)
	}
}

// all the common finalize work after we rollout
func (r *Controller) finalizeRollout(ctx context.Context) {
	// call the post-rollout webhooks
	for _, rw := range r.rolloutSpec.RolloutWebhooks {
		if rw.Type == v1alpha1.FinalizeRolloutHook {
			err := callWebhook(ctx, r.parentController, v1alpha1.FinalisingState, rw)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rw.Name, "webhook end point", rw.URL)
				r.rolloutStatus.RolloutFailed("failed to invoke a post rollout webhook")
			}
			klog.InfoS("successfully invoked a post rollout webhook", "webhook name", rw.Name, "webhook end point",
				rw.URL)
		}
	}
	r.rolloutStatus.StateTransition(v1alpha1.RollingFinalizedEvent)
}

// verify that the upgradedReplicas and current batch in the status are valid according to the spec
func (r *Controller) validateRollingBatchStatus(totalSize int) bool {
	status := r.rolloutStatus
	spec := r.rolloutSpec
	podCount := 0
	if spec.BatchPartition != nil && *spec.BatchPartition < status.CurrentBatch {
		klog.ErrorS(fmt.Errorf("the current batch value in the status is greater than the batch partition"),
			"batch partition", *spec.BatchPartition, "current batch status", status.CurrentBatch)
		return false
	}
	upgradedReplicas := int(status.UpgradedReplicas)
	currentBatch := int(status.CurrentBatch)
	// calculate the lower bound of the possible pod count just before the current batch
	for i, r := range spec.RolloutBatches {
		if i < currentBatch {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&r.Replicas, totalSize, true)
			podCount += batchSize
		}
	}
	// the recorded number should be at least as much as the all the pods before the current batch
	if podCount > upgradedReplicas {
		klog.ErrorS(fmt.Errorf("the upgraded replica in the status is too small"), "upgraded num status",
			upgradedReplicas, "pods in all the previous batches", podCount)
		return false
	}
	// calculate the upper bound with the current batch
	if currentBatch == len(spec.RolloutBatches)-1 {
		// avoid round up problems
		podCount = totalSize
	} else {
		batchSize, _ := intstr.GetValueFromIntOrPercent(&spec.RolloutBatches[currentBatch].Replicas,
			totalSize, true)
		podCount += batchSize
	}
	// the recorded number should be not as much as the all the pods including the active batch
	if podCount < upgradedReplicas {
		klog.ErrorS(fmt.Errorf("the upgraded replica in the status is too large"), "upgraded num status",
			upgradedReplicas, "pods in the batches including the current batch", podCount)
		return false
	}
	return true
}

// GetWorkloadController pick the right workload controller to work on the workload
func (r *Controller) GetWorkloadController() (workloads.WorkloadController, error) {
	kind := r.targetWorkload.GetObjectKind().GroupVersionKind().Kind
	target := types.NamespacedName{
		Namespace: r.targetWorkload.GetNamespace(),
		Name:      r.targetWorkload.GetName(),
	}

	switch kind {
	case "CloneSet":
		return workloads.NewCloneSetController(r.client, r.recorder, r.parentController,
			r.rolloutSpec, r.rolloutStatus, target), nil

	default:
		return nil, fmt.Errorf("the workload kind `%s` is not supported", kind)
	}
}
