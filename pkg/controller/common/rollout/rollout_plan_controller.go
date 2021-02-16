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
	rolloutStatus v1alpha1.RolloutStatus

	targetWorkload *unstructured.Unstructured
	sourceWorkload *unstructured.Unstructured
}

// NewRolloutPlanController creates a RolloutPlanController
func NewRolloutPlanController(client client.Client, parentController oam.Object, recorder event.Recorder,
	rolloutSpec *v1alpha1.RolloutPlan,
	rolloutStatus v1alpha1.RolloutStatus, targetWorkload,
	sourceWorkload *unstructured.Unstructured) *Controller {
	return &Controller{
		client:           client,
		parentController: parentController,
		recorder:         recorder,
		rolloutSpec:      rolloutSpec,
		rolloutStatus:    rolloutStatus,
		targetWorkload:   targetWorkload,
		sourceWorkload:   sourceWorkload,
	}
}

// Reconcile reconciles a rollout plan
func (r *Controller) Reconcile(ctx context.Context) (res reconcile.Result, status v1alpha1.RolloutStatus) {
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
		status = *workloadController.Verify(ctx)

	case v1alpha1.InitializingState:
		// TODO: call the pre-rollout webhooks
		status = *workloadController.Initialize(ctx)

	case v1alpha1.RollingInBatchesState:
		status = r.reconcileBatchInRolling(ctx, workloadController)

	case v1alpha1.FinalisingState:
		// TODO: call the post-rollout webhooks
		status = *workloadController.Finalize(ctx)

	case v1alpha1.RolloutSucceedState:
		// Nothing to do

	case v1alpha1.RolloutFailedState:
		// Nothing to do

	default:
		panic(fmt.Sprintf("illegal rollout status %+v", r.rolloutStatus))
	}

	return res, status
}

// reconcile logic when we are in the middle of rollout
func (r *Controller) reconcileBatchInRolling(ctx context.Context, workloadController workloads.WorkloadController) (
	status v1alpha1.RolloutStatus) {

	if r.rolloutSpec.Paused {
		r.recorder.Event(r.parentController, event.Normal("Rollout paused", "Rollout paused"))
		r.rolloutStatus.SetConditions(v1alpha1.NewPositiveCondition("Paused"))
		return r.rolloutStatus
	}

	// makes sure that the current batch and replica count in the status are validate
	replicas, err := workloadController.Size(ctx)
	if err != nil {
		r.rolloutStatus.RolloutRetry(err.Error())
		return r.rolloutStatus
	}
	r.validateRollingBatchStatus(int(replicas))

	switch r.rolloutStatus.BatchRollingState {
	case v1alpha1.BatchInitializingState:
		// TODO:  call the pre-batch webhook

	case v1alpha1.BatchInRollingState:
		//  still rolling the batch, the batch rolling is not completed yet
		status = *workloadController.RolloutOneBatchPods(ctx)

	case v1alpha1.BatchVerifyingState:
		// verifying if the application is ready to roll
		// need to check if they meet the availability requirements in the rollout spec.
		// TODO: evaluate any metrics/analysis
		status = *workloadController.CheckOneBatchPods(ctx)

	case v1alpha1.BatchFinalizingState:
		// all the pods in the are available
		r.finalizeOneBatch()

	case v1alpha1.BatchReadyState:
		// all the pods in the are upgraded and their state are ready
		// wait to move to the next batch if there are any
		r.tryMovingToNextBatch()

	default:
		panic(fmt.Sprintf("illegal status %+v", r.rolloutStatus))
	}

	return status
}

// check if we can move to the next batch
func (r *Controller) tryMovingToNextBatch() {
	if r.rolloutSpec.BatchPartition == nil || *r.rolloutSpec.BatchPartition > r.rolloutStatus.CurrentBatch {
		klog.InfoS("ready to rollout the next batch", "current batch", r.rolloutStatus.CurrentBatch)
		r.rolloutStatus.CurrentBatch++
		r.rolloutStatus.StateTransition(v1alpha1.BatchRolloutApprovedEvent)
	} else {
		klog.V(common.LogDebug).InfoS("the current batch is waiting to move on", "current batch",
			r.rolloutStatus.CurrentBatch)
	}
}

func (r *Controller) finalizeOneBatch() {
	// TODO: call the post-batch webhooks if there are any
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
			r.rolloutSpec, &r.rolloutStatus, target), nil

	default:
		return nil, fmt.Errorf("the workload kind `%s` is not supported", kind)
	}
}
