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

package application

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha1/envbinding"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/workflow"
	"github.com/oam-dev/kubevela/version"
)

const (
	errUpdateApplicationFinalizer = "cannot update application finalizer"
)

const (
	// baseWorkflowBackoffWaitTime is the time to wait before reconcile workflow again
	baseWorkflowBackoffWaitTime = 3000 * time.Millisecond

	legacyResourceTrackerFinalizer = "resourceTracker.finalizer.core.oam.dev"
	// resourceTrackerFinalizer is to delete the resource tracker of the latest app revision.
	resourceTrackerFinalizer = "app.oam.dev/resource-tracker-finalizer"
	// legacyOnlyRevisionFinalizer is to delete all resource trackers of app revisions which may be used
	// out of the domain of app controller, e.g., AppRollout controller.
	legacyOnlyRevisionFinalizer = "app.oam.dev/only-revision-finalizer"
)

// Reconciler reconciles a Application object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	Recorder             event.Recorder
	applicator           apply.Applicator
	appRevisionLimit     int
	concurrentReconciles int
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
// nolint:gocyclo
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	logCtx := monitorContext.NewTraceContext(ctx, "").AddTag("application", req.String(), "controller", "application")
	logCtx.Info("Reconcile application")
	defer logCtx.Commit("Reconcile application")
	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		logCtx.Error(err, "get application")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logCtx.AddTag("resource_version", app.ResourceVersion)
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	logCtx.SetContext(ctx)
	if len(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]) == 0 {
		oamutil.AddAnnotations(app, map[string]string{
			oam.AnnotationKubeVelaVersion: version.VelaVersion,
		})
	}
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	handler := &AppHandler{
		r:      r,
		app:    app,
		parser: appParser,
	}
	endReconcile, err := r.handleFinalizers(logCtx, app)
	if err != nil {
		return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), common.ApplicationStarting)
	}
	if endReconcile {
		return ctrl.Result{}, nil
	}

	appFile, err := appParser.GenerateAppFile(logCtx, app)
	if err != nil {
		logCtx.Error(err, "Failed to parse application")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Parsed", err), common.ApplicationRendering)
	}
	app.Status.SetConditions(condition.ReadyCondition("Parsed"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonParsed, velatypes.MessageParsed))

	if err := handler.PrepareCurrentAppRevision(logCtx, appFile); err != nil {
		logCtx.Error(err, "Failed to prepare app revision")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Revision", err), common.ApplicationRendering)
	}
	if err := handler.FinalizeAndApplyAppRevision(logCtx); err != nil {
		logCtx.Error(err, "Failed to apply app revision")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Revision", err), common.ApplicationRendering)
	}
	logCtx.Info("Successfully prepare current app revision", "revisionName", handler.currentAppRev.Name,
		"revisionHash", handler.currentRevHash, "isNewRevision", handler.isNewRevision)
	app.Status.SetConditions(condition.ReadyCondition("Revision"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRevisoned, velatypes.MessageRevisioned))

	if err := handler.UpdateAppLatestRevisionStatus(logCtx); err != nil {
		logCtx.Error(err, "Failed to update application status")
		return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), common.ApplicationRendering)
	}
	logCtx.Info("Successfully apply application revision")

	policies, err := appFile.PrepareWorkflowAndPolicy()
	if err != nil {
		logCtx.Error(err, "[Handle PrepareWorkflowAndPolicy]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("PrepareWorkflowAndPolicy", err), common.ApplicationPolicyGenerating)
	}

	if len(policies) > 0 {
		if err := handler.Dispatch(ctx, "", common.PolicyResourceCreator, policies...); err != nil {
			logCtx.Error(err, "[Handle ApplyPolicyResources]")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("ApplyPolices", err), common.ApplicationPolicyGenerating)
		}
		logCtx.Info("Successfully generated application policies")
	}

	app.Status.SetConditions(condition.ReadyCondition("Render"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))

	if !appWillRollout(app) {
		steps, err := handler.GenerateApplicationSteps(logCtx, app, appParser, appFile, handler.currentAppRev, r.Client, r.dm, r.pd)
		if err != nil {
			logCtx.Error(err, "[handle workflow]")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Workflow", err), common.ApplicationRunningWorkflow)
		}

		workflowState, err := workflow.NewWorkflow(app, r.Client, appFile.WorkflowMode).ExecuteSteps(logCtx.Fork("workflow"), handler.currentAppRev, steps)
		if err != nil {
			logCtx.Error(err, "[handle workflow]")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Workflow", err), common.ApplicationRunningWorkflow)
		}

		handler.addServiceStatus(false, app.Status.Services...)
		handler.addAppliedResource(app.Status.AppliedResources...)
		app.Status.AppliedResources = handler.appliedResources
		switch workflowState {
		case common.WorkflowStateSuspended:
			logCtx.Info("Workflow return state=Suspend")
			return ctrl.Result{}, r.patchStatusWithRetryOnConflict(logCtx, app, common.ApplicationWorkflowSuspending)
		case common.WorkflowStateTerminated:
			return ctrl.Result{}, r.patchStatusWithRetryOnConflict(logCtx, app, common.ApplicationWorkflowTerminated)
		case common.WorkflowStateExecuting:
			return reconcile.Result{RequeueAfter: baseWorkflowBackoffWaitTime}, r.patchStatusWithRetryOnConflict(ctx, app, common.ApplicationRunningWorkflow)
		case common.WorkflowStateFinished:
			wfStatus := app.Status.Workflow
			if wfStatus != nil {
				ref, err := handler.DispatchAndGC(ctx)
				if err == nil {
					err = envbinding.GarbageCollectionForOutdatedResourcesInSubClusters(ctx, r.Client, policies, func(c context.Context) error {
						_, e := handler.DispatchAndGC(c)
						return e
					})
				}
				if err != nil {
					logCtx.Error(err, "Failed to gc after workflow")
					r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
					return r.endWithNegativeConditionWithRetry(logCtx, app, condition.ErrorCondition("GCAfterWorkflow", err), common.ApplicationRunningWorkflow)
				}
				app.Status.ResourceTracker = ref
			}
		}
		app.Status.SetConditions(condition.ReadyCondition("WorkflowFinished"))
		r.Recorder.Event(app, event.Normal(velatypes.ReasonApplied, velatypes.MessageWorkflowFinished))
		klog.Info("Application manifests has applied by workflow successfully", "application", klog.KObj(app))
	} else {
		var comps []*velatypes.ComponentManifest
		comps, err = appFile.GenerateComponentManifests()
		if err != nil {
			logCtx.Error(err, "Failed to render components")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
			return r.endWithNegativeConditionWithRetry(logCtx, app, condition.ErrorCondition("Render", err), common.ApplicationRendering)
		}

		assemble.HandleCheckManageWorkloadTrait(*handler.currentAppRev, comps)

		if err := handler.HandleComponentsRevision(logCtx, comps); err != nil {
			logCtx.Error(err, "Failed to handle components revision")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
			return r.endWithNegativeConditionWithRetry(logCtx, app, condition.ErrorCondition("Render", err), common.ApplicationRendering)
		}
		klog.Info("Application manifests has prepared and ready for appRollout to handle", "application", klog.KObj(app))
	}
	// if inplace is false and rolloutPlan is nil, it means the user will use an outer AppRollout object to rollout the application
	if handler.app.Spec.RolloutPlan != nil {
		res, err := handler.handleRollout(logCtx)
		if err != nil {
			logCtx.Error(err, "Failed to handle rollout")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRollout, err))
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition("Rollout", err), common.ApplicationRollingOut)
		}
		// skip health check and garbage collection if rollout have not finished
		// start next reconcile immediately
		if res.Requeue || res.RequeueAfter > 0 {
			if err := r.patchStatus(logCtx, app, common.ApplicationRollingOut); err != nil {
				return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), common.ApplicationRollingOut)
			}
			return res, nil
		}

		// there is no need reconcile immediately, that means the rollout operation have finished
		r.Recorder.Event(app, event.Normal(velatypes.ReasonRollout, velatypes.MessageRollout))
		app.Status.SetConditions(condition.ReadyCondition("Rollout"))
		logCtx.Info("Finished rollout ")
	}
	var phase = common.ApplicationRunning
	if !hasHealthCheckPolicy(appFile.Policies) {
		app.Status.Services = handler.services
		if !isHealthy(handler.services) {
			phase = common.ApplicationUnhealthy
		}
	}

	if err := garbageCollection(ctx, handler); err != nil {
		logCtx.Error(err, "Failed to run garbage collection")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), phase)
	}
	logCtx.Info("Successfully garbage collect")
	app.Status.SetConditions(condition.Condition{
		Type:               condition.TypeReady,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             condition.ReasonReconcileSuccess,
	})
	r.Recorder.Event(app, event.Normal(velatypes.ReasonDeployed, velatypes.MessageDeployed))
	return ctrl.Result{}, r.patchStatus(logCtx, app, phase)
}

// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
// by setting application(namespace-scoped) as their owners.
// We must delete all resource trackers related to an application through finalizer logic.
func (r *Reconciler) handleFinalizers(ctx monitorContext.Context, app *v1beta1.Application) (bool, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(app, resourceTrackerFinalizer) {
			meta.AddFinalizer(app, resourceTrackerFinalizer)
			ctx.Info("Register new finalizer for application", "finalizer", resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	} else {
		if meta.FinalizerExists(app, legacyResourceTrackerFinalizer) {
			// TODO(roywang) legacyResourceTrackerFinalizer will be deprecated in the future
			// this is for backward compatibility
			rt := &v1beta1.ResourceTracker{}
			rt.SetName(fmt.Sprintf("%s-%s", app.Namespace, app.Name))
			if err := r.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				ctx.Error(err, "Failed to delete legacy resource tracker", "name", rt.Name)
				return true, errors.WithMessage(err, "cannot remove finalizer")
			}
			meta.RemoveFinalizer(app, legacyResourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		if meta.FinalizerExists(app, resourceTrackerFinalizer) || meta.FinalizerExists(app, legacyOnlyRevisionFinalizer) {
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}
			rtList := &v1beta1.ResourceTrackerList{}
			if err := r.Client.List(ctx, rtList, listOpts...); err != nil {
				ctx.Error(err, "Failed to list resource tracker of app", "name", app.Name)
				return true, errors.WithMessage(err, "cannot remove finalizer")
			}
			for _, rt := range rtList.Items {
				if err := r.Client.Delete(ctx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
					ctx.Error(err, "Failed to delete resource tracker", "name", rt.Name)
					return true, errors.WithMessage(err, "cannot remove finalizer")
				}
			}
			meta.RemoveFinalizer(app, resourceTrackerFinalizer)
			// legacyOnlyRevisionFinalizer will be deprecated in the future
			// this is for backward compatibility
			meta.RemoveFinalizer(app, legacyOnlyRevisionFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	}
	return false, nil
}

func (r *Reconciler) _endWithNegativeCondition(ctx context.Context, app *v1beta1.Application, condition condition.Condition, phase common.ApplicationPhase, retry bool) (ctrl.Result, error) {
	app.SetConditions(condition)
	handler := r.patchStatus
	if retry {
		handler = r.patchStatusWithRetryOnConflict
	}
	if err := handler(ctx, app, phase); err != nil {
		return ctrl.Result{}, errors.WithMessage(err, "cannot update application status")
	}
	return ctrl.Result{}, fmt.Errorf("object level reconcile error, type: %q, msg: %q", string(condition.Type), condition.Message)
}

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, app *v1beta1.Application, condition condition.Condition, phase common.ApplicationPhase) (ctrl.Result, error) {
	return r._endWithNegativeCondition(ctx, app, condition, phase, false)
}

// Note: Only operations that must override the status should use this function, it should only focus on workflow operations by now.
func (r *Reconciler) endWithNegativeConditionWithRetry(ctx context.Context, app *v1beta1.Application, condition condition.Condition, phase common.ApplicationPhase) (ctrl.Result, error) {
	return r._endWithNegativeCondition(ctx, app, condition, phase, true)
}

func (r *Reconciler) patchStatus(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	return r.Client.Status().Patch(ctx, app, client.Merge)
}

// Note: Only operations that must override the status should use this function, it should only focus on workflow operations by now.
func (r *Reconciler) patchStatusWithRetryOnConflict(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		status := app.Status.DeepCopy()
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
			return errors.WithMessage(err, "failed to get application while patching status")
		}
		app.Status = *status
		err := r.Client.Status().Patch(ctx, app, client.Merge)
		if err != nil {
			return errors.WithMessage(err, "failed to re-patch status")
		}
		return nil
	})
}

// appWillRollout judge whether the application will be released by rollout.
// If it's true, application controller will only create or update application revision but not emit any other K8s
// resources into the cluster. Rollout controller will do real release works.
func appWillRollout(app *v1beta1.Application) bool {
	return len(app.GetAnnotations()[oam.AnnotationAppRollout]) != 0 || app.Spec.RolloutPlan != nil
}

func hasHealthCheckPolicy(policies []*appfile.Workload) bool {
	for _, p := range policies {
		if p.FullTemplate != nil && p.FullTemplate.PolicyDefinition != nil &&
			p.FullTemplate.PolicyDefinition.Spec.ManageHealthCheck {
			return true
		}
	}
	return false
}

func isHealthy(services []common.ApplicationComponentStatus) bool {
	for _, service := range services {
		if !service.Healthy {
			return false
		}
		for _, tr := range service.Traits {
			if !tr.Healthy {
				return false
			}
		}
	}
	return true
}

// SetupWithManager install to manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// If Application Own these two child objects, AC status change will notify application controller and recursively update AC again, and trigger application event again...
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.Application{}).
		Complete(r)
}

// Setup adds a controller that reconciles AppRollout.
func Setup(mgr ctrl.Manager, args core.Args) error {
	reconciler := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             event.NewAPIRecorder(mgr.GetEventRecorderFor("Application")),
		dm:                   args.DiscoveryMapper,
		pd:                   args.PackageDiscover,
		applicator:           apply.NewAPIApplicator(mgr.GetClient()),
		appRevisionLimit:     args.AppRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return reconciler.SetupWithManager(mgr)
}

func updateObservedGeneration(app *v1beta1.Application) {
	if app.Status.ObservedGeneration != app.Generation {
		app.Status.ObservedGeneration = app.Generation
	}
}
