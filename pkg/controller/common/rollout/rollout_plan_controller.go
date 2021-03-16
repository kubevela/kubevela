package rollout

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruisev1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
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
	rolloutSpec *v1alpha1.RolloutPlan, rolloutStatus *v1alpha1.RolloutStatus,
	targetWorkload, sourceWorkload *unstructured.Unstructured) *Controller {
	initializedRolloutStatus := rolloutStatus.DeepCopy()
	// use Mutation webhook?
	if len(initializedRolloutStatus.RollingState) == 0 {
		initializedRolloutStatus.ResetStatus()
	}
	if len(initializedRolloutStatus.BatchRollingState) == 0 {
		initializedRolloutStatus.BatchRollingState = v1alpha1.BatchInitializingState
	}
	return &Controller{
		client:           client,
		parentController: parentController,
		recorder:         recorder,
		rolloutSpec:      rolloutSpec.DeepCopy(),
		rolloutStatus:    initializedRolloutStatus,
		targetWorkload:   targetWorkload,
		sourceWorkload:   sourceWorkload,
	}
}

// Reconcile reconciles a rollout plan
func (r *Controller) Reconcile(ctx context.Context) (res reconcile.Result, status *v1alpha1.RolloutStatus) {
	klog.InfoS("Reconcile the rollout plan", "rollout Spec", r.rolloutSpec,
		"target workload", klog.KObj(r.targetWorkload))
	if r.sourceWorkload != nil {
		klog.InfoS("We will do rolling upgrades", "source workload", klog.KObj(r.sourceWorkload))
	}
	klog.InfoS("rollout status", "rollout state", r.rolloutStatus.RollingState, "batch rolling state",
		r.rolloutStatus.BatchRollingState, "current batch", r.rolloutStatus.CurrentBatch, "upgraded Replicas",
		r.rolloutStatus.UpgradedReplicas, "ready Replicas", r.rolloutStatus.UpgradedReadyReplicas)

	defer func() {
		klog.InfoS("Finished one round of reconciling rollout plan", "rollout state", status.RollingState,
			"batch rolling state", status.BatchRollingState, "current batch", status.CurrentBatch,
			"upgraded Replicas", status.UpgradedReplicas, "ready Replicas", r.rolloutStatus.UpgradedReadyReplicas,
			"reconcile result ", res)
	}()
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
	case v1alpha1.VerifyingSpecState:
		if err = workloadController.VerifySpec(ctx); err != nil {
			// we can fail it right away, everything after initialized need to be finalized
			r.rolloutStatus.RolloutFailed(err.Error())
		} else {
			r.rolloutStatus.StateTransition(v1alpha1.RollingSpecVerifiedEvent)
		}

	case v1alpha1.InitializingState:
		if err := r.initializeRollout(ctx); err == nil {
			if err = workloadController.Initialize(ctx); err == nil {
				r.rolloutStatus.StateTransition(v1alpha1.RollingInitializedEvent)
			}
		}

	case v1alpha1.RollingInBatchesState:
		r.reconcileBatchInRolling(ctx, workloadController)

	case v1alpha1.RolloutFailingState:
		if err = workloadController.Finalize(ctx, false); err == nil {
			r.finalizeRollout(ctx)
		}

	case v1alpha1.FinalisingState:
		if err = workloadController.Finalize(ctx, true); err == nil {
			// if we are still going to finalize it
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

// reconcile logic when we are in the middle of rollout, we have to go through finalizing state before succeed or fail
func (r *Controller) reconcileBatchInRolling(ctx context.Context, workloadController workloads.WorkloadController) {

	if r.rolloutSpec.Paused {
		r.recorder.Event(r.parentController, event.Normal("Rollout paused", "Rollout paused"))
		r.rolloutStatus.SetConditions(v1alpha1.NewPositiveCondition(v1alpha1.BatchPaused))
		return
	}

	// makes sure that the current batch and replica count in the status are validate
	err := r.validateRollingBatchStatus(int(r.rolloutStatus.RolloutTargetSize))
	if err != nil {
		r.rolloutStatus.RolloutFailing(err.Error())
		return
	}

	switch r.rolloutStatus.BatchRollingState {
	case v1alpha1.BatchInitializingState:
		r.initializeOneBatch(ctx)

	case v1alpha1.BatchInRollingState:
		//  still rolling the batch, the batch rolling is not completed yet
		if err = workloadController.RolloutOneBatchPods(ctx); err != nil {
			r.rolloutStatus.RolloutFailing(err.Error())
		} else {
			r.rolloutStatus.StateTransition(v1alpha1.RolloutOneBatchEvent)
		}

	case v1alpha1.BatchVerifyingState:
		// verifying if the application is ready to roll
		// need to check if they meet the availability requirements in the rollout spec.
		// TODO: evaluate any metrics/analysis
		// TODO: We may need to go back to rollout again if the size of the resource can change behind our back
		finished := false
		if finished = workloadController.CheckOneBatchPods(ctx); finished {
			r.rolloutStatus.StateTransition(v1alpha1.OneBatchAvailableEvent)
		}

	case v1alpha1.BatchFinalizingState:
		// finalize one batch
		if err = workloadController.FinalizeOneBatch(ctx); err != nil {
			r.rolloutStatus.RolloutFailing(err.Error())
		} else {
			r.finalizeOneBatch(ctx)
		}

	case v1alpha1.BatchReadyState:
		// all the pods in the are upgraded and their state are ready
		// wait to move to the next batch if there are any
		r.tryMovingToNextBatch()

	default:
		panic(fmt.Sprintf("illegal status %+v", r.rolloutStatus))
	}
}

// all the common initialize work before we rollout
// TODO: fail the rollout if the webhook call is explicitly rejected (through http status code)
func (r *Controller) initializeRollout(ctx context.Context) error {
	// call the pre-rollout webhooks
	for _, rw := range r.rolloutSpec.RolloutWebhooks {
		if rw.Type == v1alpha1.InitializeRolloutHook {
			err := callWebhook(ctx, r.parentController, string(v1alpha1.InitializingState), rw)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rw.Name, "webhook end point", rw.URL)
				r.rolloutStatus.RolloutRetry("failed to invoke a webhook")
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
			err := callWebhook(ctx, r.parentController, string(v1alpha1.BatchInitializingState), rh)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rh.Name, "webhook end point", rh.URL)
				r.rolloutStatus.RolloutRetry("failed to invoke a webhook")
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
			err := callWebhook(ctx, r.parentController, string(v1alpha1.BatchFinalizingState), rh)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rh.Name, "webhook end point", rh.URL)
				r.rolloutStatus.RolloutRetry("failed to invoke a webhook")
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
		r.recorder.Event(r.parentController, event.Normal("All batches rolled out",
			fmt.Sprintf("upgrade pod = %d, total ready pod = %d", r.rolloutStatus.UpgradedReplicas,
				r.rolloutStatus.UpgradedReadyReplicas)))
	} else {
		klog.InfoS("finished one batch rollout", "current batch", r.rolloutStatus.CurrentBatch)
		// th
		r.recorder.Event(r.parentController, event.Normal("Batch Finalized",
			fmt.Sprintf("Batch %d is finalized and ready to go", r.rolloutStatus.CurrentBatch)))
		r.rolloutStatus.StateTransition(v1alpha1.FinishedOneBatchEvent)
	}
}

// all the common finalize work after we rollout
func (r *Controller) finalizeRollout(ctx context.Context) {
	// call the post-rollout webhooks
	for _, rw := range r.rolloutSpec.RolloutWebhooks {
		if rw.Type == v1alpha1.FinalizeRolloutHook {
			err := callWebhook(ctx, r.parentController, string(r.rolloutStatus.RollingState), rw)
			if err != nil {
				klog.ErrorS(err, "failed to invoke a webhook",
					"webhook name", rw.Name, "webhook end point", rw.URL)
				r.rolloutStatus.RolloutRetry("failed to invoke a post rollout webhook")
				return
			}
			klog.InfoS("successfully invoked a post rollout webhook", "webhook name", rw.Name, "webhook end point",
				rw.URL)
		}
	}
	r.rolloutStatus.StateTransition(v1alpha1.RollingFinalizedEvent)
}

// verify that the upgradedReplicas and current batch in the status are valid according to the spec
func (r *Controller) validateRollingBatchStatus(totalSize int) error {
	status := r.rolloutStatus
	spec := r.rolloutSpec
	podCount := 0
	if spec.BatchPartition != nil && *spec.BatchPartition < status.CurrentBatch {
		err := fmt.Errorf("the current batch value in the status is greater than the batch partition")
		klog.ErrorS(err, "we have moved past the user defined partition", "user specified batch partition",
			*spec.BatchPartition, "current batch we are working on", status.CurrentBatch)
		return err
	}
	upgradedReplicas := int(status.UpgradedReplicas)
	currentBatch := int(status.CurrentBatch)
	// calculate the lower bound of the possible pod count just before the current batch
	for i, r := range spec.RolloutBatches {
		if i < currentBatch {
			batchSize, _ := intstr.GetValueFromIntOrPercent(&r.Replicas, totalSize, true)
			podCount += batchSize
		} else {
			break
		}
	}
	// the recorded number should be at least as much as the all the pods before the current batch
	if podCount > upgradedReplicas {
		err := fmt.Errorf("the upgraded replica in the status is less than all the pods in the previous batch")
		klog.ErrorS(err, "rollout status inconsistent", "upgraded num status", upgradedReplicas, "pods in all the previous batches", podCount)
		return err
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
		err := fmt.Errorf("the upgraded replica in the status is greater than all the pods in the current batch")
		klog.ErrorS(err, "rollout status inconsistent", "total target size", totalSize,
			"upgraded num status", upgradedReplicas, "pods in the batches including the current batch", podCount)
		return err
	}
	return nil
}

// GetWorkloadController pick the right workload controller to work on the workload
func (r *Controller) GetWorkloadController() (workloads.WorkloadController, error) {
	kind := r.targetWorkload.GetObjectKind().GroupVersionKind().Kind
	target := types.NamespacedName{
		Namespace: r.targetWorkload.GetNamespace(),
		Name:      r.targetWorkload.GetName(),
	}
	var source types.NamespacedName
	if r.sourceWorkload != nil {
		source.Namespace = r.targetWorkload.GetNamespace()
		source.Name = r.targetWorkload.GetName()
	}

	if r.targetWorkload.GroupVersionKind().Group == kruisev1.GroupVersion.Group {
		if r.targetWorkload.GetKind() == reflect.TypeOf(kruisev1.CloneSet{}).Name() {
			return workloads.NewCloneSetController(r.client, r.recorder, r.parentController,
				r.rolloutSpec, r.rolloutStatus, target), nil
		}
	}

	if r.targetWorkload.GroupVersionKind().Group == apps.GroupName {
		if r.targetWorkload.GetKind() == reflect.TypeOf(apps.Deployment{}).Name() {
			return workloads.NewDeploymentController(r.client, r.recorder, r.parentController,
				r.rolloutSpec, r.rolloutStatus, source, target), nil
		}
	}
	return nil, fmt.Errorf("the workload kind `%s` is not supported", kind)
}
