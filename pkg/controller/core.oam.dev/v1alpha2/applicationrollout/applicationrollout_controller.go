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

package applicationrollout

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common/rollout"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	errUpdateAppRollout = "failed to update the app rollout"

	appRolloutFinalizer = "finalizers.approllout.oam.dev"

	reconcileTimeOut = 60 * time.Second
)

// Reconciler reconciles an AppRollout object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	record event.Recorder
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=approllouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=approllouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile is the main logic of appRollout controller
// nolint:gocyclo
func (r *Reconciler) Reconcile(req ctrl.Request) (res reconcile.Result, retErr error) {
	var appRollout v1beta1.AppRollout
	ctx, cancel := context.WithTimeout(context.TODO(), reconcileTimeOut)
	defer cancel()
	ctx = oamutil.SetNamespaceInCtx(ctx, req.Namespace)

	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished reconciling appRollout", "controller request", req, "time spent",
					time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished reconcile appRollout", "controller  request", req, "time spent",
					time.Since(startTime))
			}
		} else {
			klog.Errorf("Failed to reconcile appRollout %s: %v", req, retErr)
		}
	}()
	if err := r.Get(ctx, req.NamespacedName, &appRollout); err != nil {
		if apierrors.IsNotFound(err) {
			klog.InfoS("appRollout does not exist", "appRollout", klog.KRef(req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Start to reconcile ", "appRollout", klog.KObj(&appRollout))

	// handle app Finalizer
	doneReconcile, res, retErr := r.handleFinalizer(ctx, &appRollout)
	if doneReconcile {
		return res, retErr
	}

	reconRes, err := r.DoReconcile(ctx, &appRollout)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconRes, r.updateStatus(ctx, &appRollout)
}

// DoReconcile is real reconcile logic for appRollout.
// 1.prepare rollout info: use assemble module in application pkg to generate manifest with appRevision
// 2.determine witch component is the common component between source and target AppRevision
// 3.if target workload isn't exist yet, template the targetAppRevision to apply target manifest
// 4.extract target workload and source workload(if sourceAppRevision not empty)
// 5.generate a rolloutPlan controller with source and target workload and call rolloutPlan's reconcile func
// 6.handle output status
// !!! Note the AppRollout object should not be updated in this function as it could be logically used in Application reconcile loop which does not have real AppRollout object.
func (r *Reconciler) DoReconcile(ctx context.Context, appRollout *v1beta1.AppRollout) (reconcile.Result, error) {
	if len(appRollout.Status.RollingState) == 0 {
		appRollout.Status.ResetStatus()
	}
	var err error

	// no need to proceed if rollout is already in a terminal state and there is no source/target change
	doneReconcile := r.handleRollingTerminated(*appRollout)
	if doneReconcile {
		return reconcile.Result{}, nil
	}

	h := rolloutHandler{Reconciler: r, appRollout: appRollout}
	// handle rollout target/source change (only if it's not deleting already)
	if isRolloutModified(*appRollout) {
		h.handleRolloutModified()
	} else {
		// except modified in middle of one rollout, in most cases use real source/target in appRollout and revision as this round reconcile
		h.sourceRevName = appRollout.Spec.SourceAppRevisionName
		h.targetRevName = appRollout.Spec.TargetAppRevisionName
	}

	// call assemble func generate source and target manifest
	if err = h.prepareRollout(ctx); err != nil {
		return reconcile.Result{}, err
	}

	// we only support one workload rollout now, so here is determine witch component is need to rollout
	if err = h.determineRolloutComponent(); err != nil {
		return reconcile.Result{}, err
	}

	var sourceWorkload, targetWorkload *unstructured.Unstructured

	// we should handle two special cases before call rolloutPlan Reconcile
	switch h.appRollout.Status.RollingState {
	case v1alpha1.RolloutDeletingState:
		//  application has been deleted, the related appRev haven't removed
		if h.sourceAppRevision == nil && h.targetAppRevision == nil {
			klog.InfoS("Both the target and the source app are gone", "appRollout",
				klog.KRef(appRollout.Namespace, appRollout.Name), "rolling state", appRollout.Status.RollingState)
			h.appRollout.Status.StateTransition(v1alpha1.RollingFinalizedEvent)
			// update the appRollout status
			return ctrl.Result{}, nil
		}
	case v1alpha1.LocatingTargetAppState:
		// target manifest haven't template yet, call dispatch template target manifest firstly
		err = h.templateTargetManifest(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		// this ensures that we template workload only once
		h.appRollout.Status.StateTransition(v1alpha1.AppLocatedEvent)
		return reconcile.Result{RequeueAfter: 3 * time.Second}, nil
	default:
		// in other cases there is no need do anything
	}

	sourceWorkload, targetWorkload, err = h.fetchSourceAndTargetWorkload(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	klog.InfoS("get the target workload we need to work on", "targetWorkload", klog.KObj(targetWorkload))
	if sourceWorkload != nil {
		klog.InfoS("get the source workload we need to work on", "sourceWorkload", klog.KObj(sourceWorkload))
	}

	// reconcile the rollout part of the spec given the target and source workload
	rolloutPlanController := rollout.NewRolloutPlanController(r, appRollout, r.record,
		&appRollout.Spec.RolloutPlan, &appRollout.Status.RolloutStatus, targetWorkload, sourceWorkload)
	result, rolloutStatus := rolloutPlanController.Reconcile(ctx)
	// make sure that the new status is copied back
	appRollout.Status.RolloutStatus = *rolloutStatus
	// do not update the last with new revision if we are still trying to abandon the previous rollout
	if rolloutStatus.RollingState != v1alpha1.RolloutAbandoningState {
		appRollout.Status.LastUpgradedTargetAppRevision = appRollout.Spec.TargetAppRevisionName
		appRollout.Status.LastSourceAppRevision = appRollout.Spec.SourceAppRevisionName
	}

	if rolloutStatus.RollingState == v1alpha1.RolloutSucceedState {
		err = h.finalizeRollingSucceeded(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		klog.InfoS("rollout succeeded, record the source and target app revision", "source", appRollout.Spec.SourceAppRevisionName,
			"target", appRollout.Spec.TargetAppRevisionName)
	} else if rolloutStatus.RollingState == v1alpha1.RolloutFailedState {
		klog.InfoS("rollout failed, record the source and target app revision", "source", appRollout.Spec.SourceAppRevisionName,
			"target", appRollout.Spec.TargetAppRevisionName, "revert on deletion", appRollout.Spec.RevertOnDelete)

	}
	return result, nil
}

// check if either the source or the target of the appRollout has changed
func isRolloutModified(appRollout v1beta1.AppRollout) bool {
	return appRollout.Status.RollingState != v1alpha1.RolloutDeletingState &&
		((appRollout.Status.LastUpgradedTargetAppRevision != "" &&
			appRollout.Status.LastUpgradedTargetAppRevision != appRollout.Spec.TargetAppRevisionName) ||
			(appRollout.Status.LastSourceAppRevision != "" &&
				appRollout.Status.LastSourceAppRevision != appRollout.Spec.SourceAppRevisionName))
}

// handle adding and handle finalizer logic, it turns if we should continue to reconcile
func (r *Reconciler) handleFinalizer(ctx context.Context, appRollout *v1beta1.AppRollout) (bool, reconcile.Result, error) {
	if appRollout.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(&appRollout.ObjectMeta, appRolloutFinalizer) {
			meta.AddFinalizer(&appRollout.ObjectMeta, appRolloutFinalizer)
			klog.InfoS("Register new app rollout finalizers", "rollout", appRollout.Name,
				"finalizers", appRollout.ObjectMeta.Finalizers)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, appRollout), errUpdateAppRollout)
		}
	} else if meta.FinalizerExists(&appRollout.ObjectMeta, appRolloutFinalizer) {
		if appRollout.Status.RollingState == v1alpha1.RolloutSucceedState {
			klog.InfoS("Safe to delete the succeeded rollout", "rollout", appRollout.Name)
			meta.RemoveFinalizer(&appRollout.ObjectMeta, appRolloutFinalizer)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, appRollout), errUpdateAppRollout)
		}
		if appRollout.Status.RollingState == v1alpha1.RolloutFailedState {
			klog.InfoS("delete the rollout in deleted state", "rollout", appRollout.Name)
			if appRollout.Spec.RevertOnDelete {
				klog.InfoS("need to revert the failed rollout", "rollout", appRollout.Name)
			}
			meta.RemoveFinalizer(&appRollout.ObjectMeta, appRolloutFinalizer)
			return true, reconcile.Result{}, errors.Wrap(r.Update(ctx, appRollout), errUpdateAppRollout)
		}
		// still need to finalize
		klog.Info("perform clean up", "app rollout", appRollout.Name)
		r.record.Event(appRollout, event.Normal("Rollout ", "rollout target deleted, release the resources"))
		appRollout.Status.StateTransition(v1alpha1.RollingDeletedEvent)
	}
	return false, reconcile.Result{}, nil
}

func (r *Reconciler) handleRollingTerminated(appRollout v1beta1.AppRollout) bool {
	// handle rollout completed
	if appRollout.Status.RollingState == v1alpha1.RolloutSucceedState ||
		appRollout.Status.RollingState == v1alpha1.RolloutFailedState {
		if appRollout.Status.LastUpgradedTargetAppRevision == appRollout.Spec.TargetAppRevisionName &&
			appRollout.Status.LastSourceAppRevision == appRollout.Spec.TargetAppRevisionName {
			klog.InfoS("rollout completed, no need to reconcile", "source", appRollout.Spec.SourceAppRevisionName,
				"target", appRollout.Spec.TargetAppRevisionName)
			return true
		}
	}
	return false
}

// UpdateStatus updates v1alpha2.AppRollout's Status with retry.RetryOnConflict
func (r *Reconciler) updateStatus(ctx context.Context, appRollout *v1beta1.AppRollout) error {
	status := appRollout.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: appRollout.Namespace, Name: appRollout.Name}, appRollout); err != nil {
			return
		}
		appRollout.Status = status
		return r.Status().Update(ctx, appRollout)
	})
}

// NewReconciler render a applicationRollout reconciler
func NewReconciler(c client.Client, dm discoverymapper.DiscoveryMapper, record event.Recorder, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		c,
		dm,
		record,
		scheme,
	}
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
		WithAnnotations("controller", "AppRollout")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.AppRollout{}).
		Owns(&v1beta1.Application{}).
		Complete(r)
}

// Setup adds a controller that reconciles AppRollout.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		dm:     args.DiscoveryMapper,
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
