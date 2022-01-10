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
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlEvent "sigs.k8s.io/controller-runtime/pkg/event"
	ctrlHandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/workflow"
	"github.com/oam-dev/kubevela/version"
)

const (
	errUpdateApplicationFinalizer = "cannot update application finalizer"
)

const (
	// baseWorkflowBackoffWaitTime is the time to wait gc check
	baseGCBackoffWaitTime = 3000 * time.Millisecond

	// resourceTrackerFinalizer is to delete the resource tracker of the latest app revision.
	resourceTrackerFinalizer = "app.oam.dev/resource-tracker-finalizer"
)

// Reconciler reconciles an Application object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	Recorder             event.Recorder
	appRevisionLimit     int
	concurrentReconciles int
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
// nolint:gocyclo
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, common2.ReconcileTimeout)
	defer cancel()

	logCtx := monitorContext.NewTraceContext(ctx, "").AddTag("application", req.String(), "controller", "application")
	logCtx.Info("Reconcile application")
	defer logCtx.Commit("Reconcile application")
	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		if !kerrors.IsNotFound(err) {
			logCtx.Error(err, "get application")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logCtx.AddTag("resource_version", app.ResourceVersion)
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	logCtx.SetContext(ctx)
	if annotations := app.GetAnnotations(); annotations == nil || annotations[oam.AnnotationKubeVelaVersion] == "" {
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, version.VelaVersion)
	}
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	handler, err := NewAppHandler(logCtx, r, app, appParser)
	if err != nil {
		return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), common.ApplicationStarting)
	}
	endReconcile, result, err := r.handleFinalizers(logCtx, app, handler)
	if err != nil {
		if app.GetDeletionTimestamp() == nil {
			return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), common.ApplicationStarting)
		}
		return result, err
	}
	if endReconcile {
		return result, nil
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

	externalPolicies, err := appFile.PrepareWorkflowAndPolicy()
	if err != nil {
		logCtx.Error(err, "[Handle PrepareWorkflowAndPolicy]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.PolicyCondition.String(), errors.WithMessage(err, "PrepareWorkflowAndPolicy")), common.ApplicationPolicyGenerating)
	}

	if len(externalPolicies) > 0 {
		if err := handler.Dispatch(ctx, "", common.PolicyResourceCreator, externalPolicies...); err != nil {
			logCtx.Error(err, "[Handle ApplyPolicyResources]")
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.PolicyCondition.String(), errors.WithMessage(err, "ApplyPolices")), common.ApplicationPolicyGenerating)
		}
		logCtx.Info("Successfully generated application policies")
	}

	app.Status.SetConditions(condition.ReadyCondition(common.PolicyCondition.String()))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonPolicyGenerated, velatypes.MessagePolicyGenerated))

	steps, err := handler.GenerateApplicationSteps(logCtx, app, appParser, appFile, handler.currentAppRev)
	if err != nil {
		logCtx.Error(err, "[handle workflow]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.WorkflowCondition.String(), err), common.ApplicationRendering)
	}
	app.Status.SetConditions(condition.ReadyCondition(common.RenderCondition.String()))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))
	wf := workflow.NewWorkflow(app, r.Client, appFile.WorkflowMode)
	workflowState, err := wf.ExecuteSteps(logCtx.Fork("workflow"), handler.currentAppRev, steps)
	if err != nil {
		logCtx.Error(err, "[handle workflow]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.WorkflowCondition.String(), err), common.ApplicationRunningWorkflow)
	}

	handler.addServiceStatus(false, app.Status.Services...)
	handler.addAppliedResource(true, app.Status.AppliedResources...)
	app.Status.AppliedResources = handler.appliedResources
	app.Status.Services = handler.services
	switch workflowState {
	case common.WorkflowStateInitializing:
		logCtx.Info("Workflow return state=Initializing")
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationRendering, false)
	case common.WorkflowStateSuspended:
		logCtx.Info("Workflow return state=Suspend")
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowSuspending, false)
	case common.WorkflowStateTerminated:
		logCtx.Info("Workflow return state=Terminated")
		if err := r.doWorkflowFinish(app, wf); err != nil {
			return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition(common.WorkflowCondition.String(), errors.WithMessage(err, "DoWorkflowFinish")), common.ApplicationRunningWorkflow)
		}
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowTerminated, false)
	case common.WorkflowStateExecuting:
		logCtx.Info("Workflow return state=Executing")
		_, err = r.gcResourceTrackers(logCtx, handler, common.ApplicationRunningWorkflow, false)
		return reconcile.Result{RequeueAfter: wf.GetBackoffWaitTime()}, err
	case common.WorkflowStateSucceeded:
		logCtx.Info("Workflow return state=Succeeded")
		if err := r.doWorkflowFinish(app, wf); err != nil {
			return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.WorkflowCondition.String(), errors.WithMessage(err, "DoWorkflowFinish")), common.ApplicationRunningWorkflow)
		}
		app.Status.SetConditions(condition.ReadyCondition(common.WorkflowCondition.String()))
		r.Recorder.Event(app, event.Normal(velatypes.ReasonApplied, velatypes.MessageWorkflowFinished))
		logCtx.Info("Application manifests has applied by workflow successfully")
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowFinished, false)
	case common.WorkflowStateFinished:
		logCtx.Info("Workflow state=Finished")
		if status := app.Status.Workflow; status != nil && status.Terminated {
			return ctrl.Result{}, nil
		}
	}

	var phase = common.ApplicationRunning
	if !hasHealthCheckPolicy(appFile.Policies) {
		app.Status.Services = handler.services
		if !isHealthy(handler.services) {
			phase = common.ApplicationUnhealthy
		}
	}

	if err := handler.resourceKeeper.StateKeep(ctx); err != nil {
		logCtx.Error(err, "Failed to run prevent-configuration-drift")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedStateKeep, err))
		app.Status.SetConditions(condition.ErrorCondition("StateKeep", err))
	}
	if err := garbageCollection(logCtx, handler); err != nil {
		logCtx.Error(err, "Failed to run garbage collection")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ReconcileError(err), phase)
	}
	logCtx.Info("Successfully garbage collect")
	app.Status.SetConditions(condition.Condition{
		Type:               condition.ConditionType(common.ReadyCondition.String()),
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             condition.ReasonReconcileSuccess,
	})
	r.Recorder.Event(app, event.Normal(velatypes.ReasonDeployed, velatypes.MessageDeployed))
	return r.gcResourceTrackers(logCtx, handler, phase, true)
}

func (r *Reconciler) gcResourceTrackers(logCtx monitorContext.Context, handler *AppHandler, phase common.ApplicationPhase, gcOutdated bool) (ctrl.Result, error) {
	var options []resourcekeeper.GCOption
	if !gcOutdated {
		options = append(options, resourcekeeper.DisableMarkStageGCOption{}, resourcekeeper.DisableGCComponentRevisionOption{}, resourcekeeper.DisableLegacyGCOption{})
	}
	finished, waiting, err := handler.resourceKeeper.GarbageCollect(logCtx, options...)
	if err != nil {
		logCtx.Error(err, "Failed to gc resourcetrackers")
		r.Recorder.Event(handler.app, event.Warning(velatypes.ReasonFailedGC, err))
		return r.endWithNegativeCondition(logCtx, handler.app, condition.ReconcileError(err), phase)
	}
	if !finished {
		logCtx.Info("GarbageCollecting resourcetrackers")
		cond := condition.Deleting()
		if len(waiting) > 0 {
			cond.Message = fmt.Sprintf("Waiting for %s to delete. (At least %d resources are deleting.)", waiting[0].DisplayName(), len(waiting))
		}
		handler.app.Status.SetConditions(cond)
		return ctrl.Result{RequeueAfter: baseGCBackoffWaitTime}, r.patchStatus(logCtx, handler.app, phase)
	}
	logCtx.Info("GarbageCollected resourcetrackers")
	if phase == common.ApplicationRendering {
		return ctrl.Result{}, r.updateStatus(logCtx, handler.app, common.ApplicationRunningWorkflow)
	}
	return ctrl.Result{}, r.patchStatus(logCtx, handler.app, phase)
}

// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
// by setting application(namespace-scoped) as their owners.
// We must delete all resource trackers related to an application through finalizer logic.
func (r *Reconciler) handleFinalizers(ctx monitorContext.Context, app *v1beta1.Application, handler *AppHandler) (bool, ctrl.Result, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(app, resourceTrackerFinalizer) {
			meta.AddFinalizer(app, resourceTrackerFinalizer)
			ctx.Info("Register new finalizer for application", "finalizer", resourceTrackerFinalizer)
			return true, ctrl.Result{}, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	} else {
		if meta.FinalizerExists(app, resourceTrackerFinalizer) {
			rootRT, currentRT, historyRTs, cvRT, err := resourcetracker.ListApplicationResourceTrackers(ctx, r.Client, app)
			if err != nil {
				return true, ctrl.Result{}, err
			}
			result, err := r.gcResourceTrackers(ctx, handler, common.ApplicationDeleting, true)
			if err != nil {
				return true, result, err
			}
			if rootRT == nil && currentRT == nil && len(historyRTs) == 0 && cvRT == nil {
				meta.RemoveFinalizer(app, resourceTrackerFinalizer)
				return true, ctrl.Result{}, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
			}
			return true, result, err
		}
	}
	return false, ctrl.Result{}, nil
}

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, app *v1beta1.Application, condition condition.Condition, phase common.ApplicationPhase) (ctrl.Result, error) {
	app.SetConditions(condition)
	if err := r.patchStatus(ctx, app, phase); err != nil {
		return ctrl.Result{}, errors.WithMessage(err, "cannot update application status")
	}
	return ctrl.Result{}, fmt.Errorf("object level reconcile error, type: %q, msg: %q", string(condition.Type), condition.Message)
}

func (r *Reconciler) patchStatus(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	return r.Status().Patch(ctx, app, client.Merge)
}

func (r *Reconciler) updateStatus(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	return r.Status().Update(ctx, app)
}

func (r *Reconciler) doWorkflowFinish(app *v1beta1.Application, wf workflow.Workflow) error {
	if err := wf.Trace(); err != nil {
		return errors.WithMessage(err, "record workflow state")
	}
	app.Status.Workflow.Finished = true
	return nil
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
		Watches(&source.Kind{
			Type: &v1beta1.ResourceTracker{},
		}, ctrlHandler.Funcs{
			CreateFunc: func(createEvent ctrlEvent.CreateEvent, limitingInterface workqueue.RateLimitingInterface) {
				handleResourceTracker(createEvent.Object, limitingInterface)
			},
			UpdateFunc: func(updateEvent ctrlEvent.UpdateEvent, limitingInterface workqueue.RateLimitingInterface) {
				handleResourceTracker(updateEvent.ObjectNew, limitingInterface)
			},
			DeleteFunc: func(deleteEvent ctrlEvent.DeleteEvent, limitingInterface workqueue.RateLimitingInterface) {
				handleResourceTracker(deleteEvent.Object, limitingInterface)
			},
			GenericFunc: func(genericEvent ctrlEvent.GenericEvent, limitingInterface workqueue.RateLimitingInterface) {
				handleResourceTracker(genericEvent.Object, limitingInterface)
			},
		}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		WithEventFilter(predicate.Funcs{
			// filter the changes in workflow status
			// let workflow handle its reconcile
			UpdateFunc: func(e ctrlEvent.UpdateEvent) bool {
				new, isNewApp := e.ObjectNew.DeepCopyObject().(*v1beta1.Application)
				old, isOldApp := e.ObjectOld.DeepCopyObject().(*v1beta1.Application)
				if !isNewApp || !isOldApp {
					return filterManagedFieldChangesUpdate(e)
				}

				// We think this event is triggered by resync
				if reflect.DeepEqual(old, new) {
					return true
				}

				// filter managedFields changes
				new.ManagedFields = old.ManagedFields

				// if the generation is changed, return true to let the controller handle it
				if old.Generation != new.Generation {
					return true
				}

				// ignore the changes in workflow status
				if old.Status.Workflow != nil && new.Status.Workflow != nil {
					// only workflow execution will change the status.workflow
					// let workflow backoff to requeue the event
					new.Status.Workflow.Steps = old.Status.Workflow.Steps
					new.Status.Workflow.ContextBackend = old.Status.Workflow.ContextBackend
					new.Status.Workflow.Message = old.Status.Workflow.Message

					// appliedResources and Services will be changed during the execution of workflow
					// once the resources is added, the managed fields will also be changed
					new.Status.AppliedResources = old.Status.AppliedResources
					new.Status.Services = old.Status.Services
					// the resource version will be changed if the object is changed
					// ignore this change and let reflect.DeepEqual to compare the rest of the object
					new.ResourceVersion = old.ResourceVersion
				}
				return !reflect.DeepEqual(old, new)
			},
			CreateFunc: func(e ctrlEvent.CreateEvent) bool {
				return true
			},
			DeleteFunc: func(e ctrlEvent.DeleteEvent) bool {
				return true
			},
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

// filterManagedFieldChangesUpdate filter resourceTracker update event by ignoring managedFields changes
// For old k8s version like 1.18.5, the managedField could always update and cause infinite loop
// this function helps filter those events and prevent infinite loop
func filterManagedFieldChangesUpdate(e ctrlEvent.UpdateEvent) bool {
	new, isNewRT := e.ObjectNew.DeepCopyObject().(*v1beta1.ResourceTracker)
	old, isOldRT := e.ObjectOld.DeepCopyObject().(*v1beta1.ResourceTracker)
	if !isNewRT || !isOldRT {
		return true
	}
	new.ManagedFields = old.ManagedFields
	return !reflect.DeepEqual(new, old)
}

func handleResourceTracker(obj client.Object, limitingInterface workqueue.RateLimitingInterface) {
	rt, ok := obj.(*v1beta1.ResourceTracker)
	if ok {
		if labels := rt.Labels; labels != nil {
			var request reconcile.Request
			request.Name = labels[oam.LabelAppName]
			request.Namespace = labels[oam.LabelAppNamespace]
			if request.Namespace != "" && request.Name != "" {
				limitingInterface.Add(request)
			}
		}
	}
}
