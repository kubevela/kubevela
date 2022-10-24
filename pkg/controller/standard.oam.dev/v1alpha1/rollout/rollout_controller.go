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
	"encoding/json"
	"math"

	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	rolloutplan "github.com/oam-dev/kubevela/pkg/controller/common/rollout"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"

	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	rolloutFinalizer = "finalizers.rollout.standard.oam.dev"

	errUpdateRollout = "failed to update the rollout"
)

type reconciler struct {
	client.Client
	applicator           apply.Applicator
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
}

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	rollout := new(v1alpha1.Rollout)
	ctx = oamutil.SetNamespaceInCtx(ctx, req.Namespace)
	if err := r.Get(ctx, req.NamespacedName, rollout); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	doneReconcile, res, retErr := r.handleFinalizer(ctx, rollout)
	if doneReconcile {
		return res, retErr
	}

	klog.InfoS("start rollout reconcile", "rollout",
		klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)

	if len(rollout.Status.RollingState) == 0 {
		rollout.Status.ResetStatus()
	}

	// no need to proceed if rollout is already in a terminal state and there is no source/target change
	doneReconcile = checkRollingTerminated(*rollout)
	if doneReconcile {
		return reconcile.Result{}, nil
	}

	h := handler{
		reconciler: r,
		rollout:    rollout,
		compName:   rollout.Spec.ComponentName,
	}
	// handle rollout target/source change (only if it's not deleting already)
	if h.isRolloutModified(*rollout) {
		h.handleRolloutModified()
	} else {
		// except modified in middle of one rollout, in most cases use last source/target in rollout and revision as this round reconcile
		h.sourceRevName = rollout.Status.LastSourceRevision
		h.targetRevName = rollout.Spec.TargetRevisionName
	}

	// this means user trigger a scale operation by modify targetSize,so empty sourceRevision
	if h.sourceRevName != "" && h.targetRevName == h.sourceRevName {
		h.sourceRevName = ""
	}

	if err := h.extractWorkloadFromCompRevision(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// set base info to workload
	h.setWorkloadBaseInfo()

	if err := h.assembleWorkload(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if rollout.Status.RollingState == v1alpha1.LocatingTargetAppState {
		if rollout.GetAnnotations() == nil || rollout.GetAnnotations()[oam.AnnotationWorkloadName] != h.targetWorkload.GetName() {
			// this is a update operation, the target workload will change so modify annotation
			gvk := map[string]string{"apiVersion": h.targetWorkload.GetAPIVersion(), "kind": h.targetWorkload.GetKind()}
			gvkValue, _ := json.Marshal(gvk)
			rollout.SetAnnotations(oamutil.MergeMapOverrideWithDst(rollout.GetAnnotations(),
				map[string]string{oam.AnnotationWorkloadName: h.targetWorkload.GetName(), oam.AnnotationWorkloadGVK: string(gvkValue)}))
			klog.InfoS("rollout controller set targetWorkload ", h.targetWorkload.GetName(),
				"in annotation in rollout namespace: ", rollout.Namespace, " name", rollout.Name, "gvk", gvkValue)
			// exit current reconcile before create target workload, this reconcile don't update status just modify annotation
			// next round reconcile will create workload and pass `LocatingTargetAppState` phase
			return ctrl.Result{}, h.Update(ctx, rollout)
		}

		// this is a scale operation, if user don't fill rolloutBatches, fill it with default value
		if len(h.sourceRevName) == 0 && len(rollout.Spec.RolloutPlan.RolloutBatches) == 0 {
			// logic reach here means cannot get an error, so ignore it
			replicas, _ := getWorkloadReplicasNum(*h.targetWorkload)
			rollout.Spec.RolloutPlan.RolloutBatches = []v1alpha1.RolloutBatch{{
				Replicas: intstr.FromInt(int(math.Abs(float64(*rollout.Spec.RolloutPlan.TargetSize - replicas))))},
			}
			klog.InfoS("rollout controller set default rollout  batches ", h.rollout.GetName(),
				" namespace: ", rollout.Namespace, "targetSize", rollout.Spec.RolloutPlan.TargetSize)
			return ctrl.Result{}, h.Update(ctx, rollout)
		}
	}

	switch rollout.Status.RollingState {
	case v1alpha1.RolloutDeletingState:
		removed, err := h.checkWorkloadNotExist(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if removed {
			klog.InfoS(" the target workload is gone, no need do anything", "rollout",
				klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)
			rollout.Status.StateTransition(v1alpha1.RollingFinalizedEvent)
			// update the appRollout status
			return ctrl.Result{}, h.updateStatus(ctx, rollout)
		}
	case v1alpha1.LocatingTargetAppState:
		if err := h.applyTargetWorkload(ctx); err != nil {
			return ctrl.Result{}, err
		}
		rollout.Status.StateTransition(v1alpha1.AppLocatedEvent)
		klog.InfoS("finished  rollout apply targetWorkload, passed LocatingTargetApp phase", "rollout",
			klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)
		return ctrl.Result{}, r.updateStatus(ctx, rollout)
	default:
		// we should do nothing
	}
	rolloutPlanController := rolloutplan.NewRolloutPlanController(r.Client, rollout, r.record, &rollout.Spec.RolloutPlan,
		&rollout.Status.RolloutStatus, h.targetWorkload, h.sourceWorkload)
	result, rolloutStatus := rolloutPlanController.Reconcile(ctx)
	rollout.Status.RolloutStatus = *rolloutStatus

	switch rolloutStatus.RollingState {
	case v1alpha1.LocatingTargetAppState:
		// when revert in middle of rollout, will meet this situation
		rollout.Status.LastUpgradedTargetRevision = rollout.Spec.TargetRevisionName
		rollout.Status.LastSourceRevision = h.targetRevName
	case v1alpha1.RolloutAbandoningState:
		// do noting
		// do not update the last with new revision if we are still trying to abandon the previous rollout
	default:
		rollout.Status.LastUpgradedTargetRevision = h.targetRevName
		rollout.Status.LastSourceRevision = h.sourceRevName
	}

	if rolloutStatus.RollingState == v1alpha1.RolloutSucceedState {
		if err := h.handleFinalizeSucceed(ctx); err != nil {
			return reconcile.Result{}, err
		}
		klog.InfoS("rollout succeeded, record the source and target  revision", "source", rollout.Spec.SourceRevisionName,
			"target", rollout.Spec.TargetRevisionName)
	} else if rolloutStatus.RollingState == v1alpha1.RolloutFailedState {
		if err := h.handleFinalizeFailed(ctx); err != nil {
			return reconcile.Result{}, err
		}
		klog.InfoS("rollout failed, record the source and target app revision", "source", rollout.Spec.SourceRevisionName,
			"target", rollout.Spec.TargetRevisionName)
	}
	return result, r.updateStatus(ctx, rollout)
}

// SetupWithManager will setup with event recorder
func (r *reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Rollout")).
		WithAnnotations("controller", "Rollout")

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1alpha1.Rollout{}).
		Complete(r)
}

// Setup adds a controller that reconciles ComponentDefinition.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		concurrentReconciles: args.ConcurrentReconciles,
	}
	r.applicator = apply.NewAPIApplicator(r.Client)
	return r.SetupWithManager(mgr)
}

func (r *reconciler) updateStatus(ctx context.Context, rollout *v1alpha1.Rollout) error {
	return r.Status().Update(ctx, rollout)
}

// handle adding and handle finalizer logic, it turns if we should continue to reconcile
func (r *reconciler) handleFinalizer(ctx context.Context, rollout *v1alpha1.Rollout) (bool, reconcile.Result, error) {
	if rollout.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(&rollout.ObjectMeta, rolloutFinalizer) {
			meta.AddFinalizer(&rollout.ObjectMeta, rolloutFinalizer)
			klog.InfoS("Register new app rollout finalizers", "rollout", rollout.Name,
				"finalizers", rollout.ObjectMeta.Finalizers)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, rollout), errUpdateRollout)
		}
	} else if meta.FinalizerExists(&rollout.ObjectMeta, rolloutFinalizer) {
		if rollout.Status.RollingState == v1alpha1.RolloutSucceedState {
			klog.InfoS("Safe to delete the succeeded rollout", "rollout", rollout.Name)
			meta.RemoveFinalizer(&rollout.ObjectMeta, rolloutFinalizer)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, rollout), errUpdateRollout)
		}
		if rollout.Status.RollingState == v1alpha1.RolloutFailedState {
			klog.InfoS("delete the rollout in failed state", "rollout", rollout.Name)
			meta.RemoveFinalizer(&rollout.ObjectMeta, rolloutFinalizer)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, rollout), errUpdateRollout)
		}
		// still need to finalize
		klog.Info("perform clean up", "app rollout", rollout.Name)
		r.record.Event(rollout, event.Normal("Rollout ", "rollout target deleted, release the resources"))
		rollout.Status.StateTransition(v1alpha1.RollingDeletedEvent)
	}
	return false, reconcile.Result{}, nil
}
