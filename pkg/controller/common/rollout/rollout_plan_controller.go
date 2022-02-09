/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rollout

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/common/rollout/workloads"
	"github.com/oam-dev/kubevela/pkg/dependency/kruiseapi"
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
	klog.InfoS("Reconcile the rollout plan", "rollout status", r.rolloutStatus,
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
			"upgraded Replicas", status.UpgradedReplicas, "ready Replicas", status.UpgradedReadyReplicas,
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
		verified, err := workloadController.VerifySpec(ctx)
		if err != nil {
			// we can fail it right away, everything after initialized need to be finalized
			r.rolloutStatus.RolloutFailed(err.Error())
		} else if verified {
			r.rolloutStatus.StateTransition(v1alpha1.RollingSpecVerifiedEvent)
		}

	case v1alpha1.InitializingState:
		if err = r.initializeRollout(ctx); err == nil {
			initialized, err := workloadController.Initialize(ctx)
			if err != nil {
				r.rolloutStatus.RolloutFailing(err.Error())
			} else if initialized {
				r.rolloutStatus.StateTransition(v1alpha1.RollingInitializedEvent)
			}
		}

	case v1alpha1.RollingInBatchesState:
		r.reconcileBatchInRolling(ctx, workloadController)

	case v1alpha1.RolloutFailingState, v1alpha1.RolloutAbandoningState, v1alpha1.RolloutDeletingState:
		if succeed := workloadController.Finalize(ctx, false); succeed {
			r.finalizeRollout(ctx)
		}

	case v1alpha1.FinalisingState:
		if succeed := workloadController.Finalize(ctx, true); succeed {
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

	switch r.rolloutStatus.BatchRollingState {
	case v1alpha1.BatchInitializingState:
		r.initializeOneBatch(ctx)

	case v1alpha1.BatchInRollingState:
		//  still rolling the batch, the batch rolling is not completed yet
		upgradeDone, err := workloadController.RolloutOneBatchPods(ctx)
		if err != nil {
			r.rolloutStatus.RolloutFailing(err.Error())
		} else if upgradeDone {
			r.rolloutStatus.StateTransition(v1alpha1.RolloutOneBatchEvent)
		}

	case v1alpha1.BatchVerifyingState:
		// verifying if the application is ready to roll
		// need to check if they meet the availability requirements in the rollout spec.
		// TODO: evaluate any metrics/analysis
		// TODO: We may need to go back to rollout again if the size of the resource can change behind our back
		verified, err := workloadController.CheckOneBatchPods(ctx)
		if err != nil {
			r.rolloutStatus.RolloutFailing(err.Error())
		} else if verified {
			r.rolloutStatus.StateTransition(v1alpha1.OneBatchAvailableEvent)
		}

	case v1alpha1.BatchFinalizingState:
		// finalize one batch
		finalized, err := workloadController.FinalizeOneBatch(ctx)
		if err != nil {
			r.rolloutStatus.RolloutFailing(err.Error())
		} else if finalized {
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

// GetWorkloadController pick the right workload controller to work on the workload
func (r *Controller) GetWorkloadController() (workloads.WorkloadController, error) {
	kind := r.targetWorkload.GetObjectKind().GroupVersionKind().Kind
	target := types.NamespacedName{
		Namespace: r.targetWorkload.GetNamespace(),
		Name:      r.targetWorkload.GetName(),
	}
	var source types.NamespacedName
	if r.sourceWorkload != nil {
		source.Namespace = r.sourceWorkload.GetNamespace()
		source.Name = r.sourceWorkload.GetName()
	}

	if r.targetWorkload.GroupVersionKind().Group == kruiseapi.GroupVersion.Group {
		// check if the target workload is CloneSet
		if r.targetWorkload.GetKind() == kruiseapi.CloneSet {
			// check whether current rollout plan is for workload rolling or scaling
			if r.sourceWorkload != nil {
				klog.InfoS("using cloneset rollout controller for this rolloutplan", "source workload name", source.Name, "namespace",
					source.Namespace, "target workload name", target.Name, "namespace",
					target.Namespace)
				return workloads.NewCloneSetRolloutController(r.client, r.recorder, r.parentController,
					r.rolloutSpec, r.rolloutStatus, target), nil
			}
			klog.InfoS("using cloneset scale controller for this rolloutplan", "target workload name", target.Name, "namespace",
				target.Namespace)
			return workloads.NewCloneSetScaleController(r.client, r.recorder, r.parentController,
				r.rolloutSpec, r.rolloutStatus, target), nil
		}
	}

	if r.targetWorkload.GroupVersionKind().Group == apps.GroupName {
		// check if the target workload is Deployment
		if r.targetWorkload.GetKind() == reflect.TypeOf(apps.Deployment{}).Name() {
			// check whether current rollout plan is for workload rolling or scaling
			if r.sourceWorkload != nil {
				klog.InfoS("using deployment rollout controller for this rolloutplan", "source workload name", source.Name, "namespace",
					source.Namespace, "target workload name", target.Name, "namespace",
					target.Namespace)
				return workloads.NewDeploymentRolloutController(r.client, r.recorder, r.parentController,
					r.rolloutSpec, r.rolloutStatus, source, target), nil
			}
			klog.InfoS("using deployment scale controller for this rolloutplan", "target workload name", target.Name, "namespace",
				target.Namespace)
			return workloads.NewDeploymentScaleController(r.client, r.recorder, r.parentController,
				r.rolloutSpec, r.rolloutStatus, target), nil
		}

		// check if the target workload is StatefulSet
		if r.targetWorkload.GetKind() == reflect.TypeOf(apps.StatefulSet{}).Name() {
			// check whether current rollout plan is for workload rolling or scaling
			if r.sourceWorkload != nil {
				return workloads.NewStatefulSetRolloutController(r.client, r.recorder, r.parentController,
					r.rolloutSpec, r.rolloutStatus, target), nil
			}
			return workloads.NewStatefulSetScaleController(r.client, r.recorder, r.parentController,
				r.rolloutSpec, r.rolloutStatus, target), nil
		}
	}

	return nil, fmt.Errorf("the workload kind `%s` is not supported", kind)
}
