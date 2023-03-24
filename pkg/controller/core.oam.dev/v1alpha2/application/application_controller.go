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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlEvent "sigs.k8s.io/controller-runtime/pkg/event"
	ctrlHandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/kubevela/workflow/pkg/executor"
	wffeatures "github.com/kubevela/workflow/pkg/features"

	ctrlrec "github.com/kubevela/pkg/controller/reconciler"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/auth"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
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
)

var (
	// EnableResourceTrackerDeleteOnlyTrigger optimize ResourceTracker mutate event trigger by only receiving deleting events
	EnableResourceTrackerDeleteOnlyTrigger = true
)

// Reconciler reconciles an Application object
type Reconciler struct {
	client.Client
	dm       discoverymapper.DiscoveryMapper
	pd       *packages.PackageDiscover
	Scheme   *runtime.Scheme
	Recorder event.Recorder
	options
}

type options struct {
	appRevisionLimit     int
	concurrentReconciles int
	disableStatusUpdate  bool
	ignoreAppNoCtrlReq   bool
	controllerVersion    string
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
// nolint:gocyclo
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()
	logCtx := monitorContext.NewTraceContext(ctx, "").AddTag("application", req.String(), "controller", "application")
	logCtx.Info("Start reconcile application")
	defer logCtx.Commit("End reconcile application")
	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		if !kerrors.IsNotFound(err) {
			logCtx.Error(err, "get application")
		}
		return r.result(client.IgnoreNotFound(err)).ret()
	}
	ctx = withOriginalApp(ctx, app)
	if ctrlrec.IsPaused(app) {
		return ctrl.Result{}, nil
	}

	if !r.matchControllerRequirement(app) {
		logCtx.Info("skip app: not match the controller requirement of app")
		return ctrl.Result{}, nil
	}

	timeReporter := timeReconcile(app)
	defer timeReporter()

	logCtx.AddTag("resource_version", app.ResourceVersion).AddTag("generation", app.Generation)
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	logCtx.SetContext(ctx)
	if annotations := app.GetAnnotations(); annotations == nil || annotations[oam.AnnotationKubeVelaVersion] == "" {
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, version.VelaVersion)
	}
	logCtx.AddTag("publish_version", app.GetAnnotations()[oam.AnnotationPublishVersion])

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

	if err := handler.ApplyPolicies(logCtx, appFile); err != nil {
		logCtx.Error(err, "[Handle ApplyPolicies]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.PolicyCondition.String(), errors.WithMessage(err, "ApplyPolices")), common.ApplicationPolicyGenerating)
	}
	app.Status.SetConditions(condition.ReadyCondition(common.PolicyCondition.String()))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonPolicyGenerated, velatypes.MessagePolicyGenerated))

	handler.CheckWorkflowRestart(logCtx, app)

	workflowInstance, runners, err := handler.GenerateApplicationSteps(logCtx, app, appParser, appFile)
	if err != nil {
		logCtx.Error(err, "[handle workflow]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.WorkflowCondition.String(), err), common.ApplicationRendering)
	}
	app.Status.SetConditions(condition.ReadyCondition(common.RenderCondition.String()))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))

	executor := executor.New(workflowInstance, r.Client, nil)
	authCtx := logCtx.Fork("execute application workflow")
	defer authCtx.Commit("finish execute application workflow")
	authCtx = auth.MonitorContextWithUserInfo(authCtx, app)
	tBeginWorkflowExecution := time.Now()
	workflowState, err := executor.ExecuteRunners(authCtx, runners)
	metrics.AppReconcileStageDurationHistogram.WithLabelValues("execute-workflow").Observe(time.Since(tBeginWorkflowExecution).Seconds())
	if err != nil {
		logCtx.Error(err, "[handle workflow]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
		return r.endWithNegativeCondition(logCtx, app, condition.ErrorCondition(common.WorkflowCondition.String(), err), common.ApplicationRunningWorkflow)
	}

	handler.addServiceStatus(false, app.Status.Services...)
	handler.addAppliedResource(true, app.Status.AppliedResources...)
	app.Status.AppliedResources = handler.appliedResources
	app.Status.Services = handler.services
	isUpdate := app.Status.Workflow.Message != "" && workflowInstance.Status.Message == ""
	workflowInstance.Status.Phase = workflowState
	app.Status.Workflow = workflow.ConvertWorkflowStatus(workflowInstance.Status, app.Status.Workflow.AppRevision)
	logCtx.Info(fmt.Sprintf("Workflow return state=%s", workflowState))
	switch workflowState {
	case workflowv1alpha1.WorkflowStateSuspending:
		if duration := executor.GetSuspendBackoffWaitTime(); duration > 0 {
			_, err = r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowSuspending, false, isUpdate)
			return r.result(err).requeue(duration).ret()
		}
		if !workflow.IsFailedAfterRetry(app) || !feature.DefaultMutableFeatureGate.Enabled(wffeatures.EnableSuspendOnFailure) {
			r.stateKeep(logCtx, handler, app)
		}
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowSuspending, false, isUpdate)
	case workflowv1alpha1.WorkflowStateTerminated:
		if workflowInstance.Status.EndTime.IsZero() {
			r.doWorkflowFinish(logCtx, app, handler, workflowState)
		}
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowTerminated, false, isUpdate)
	case workflowv1alpha1.WorkflowStateFailed:
		if workflowInstance.Status.EndTime.IsZero() {
			r.doWorkflowFinish(logCtx, app, handler, workflowState)
		}
		return r.gcResourceTrackers(logCtx, handler, common.ApplicationWorkflowFailed, false, isUpdate)
	case workflowv1alpha1.WorkflowStateExecuting:
		_, err = r.gcResourceTrackers(logCtx, handler, common.ApplicationRunningWorkflow, false, isUpdate)
		return r.result(err).requeue(executor.GetBackoffWaitTime()).ret()
	case workflowv1alpha1.WorkflowStateSucceeded:
		if workflowInstance.Status.EndTime.IsZero() {
			r.doWorkflowFinish(logCtx, app, handler, workflowState)
		}
	case workflowv1alpha1.WorkflowStateSkipped:
		return r.result(nil).requeue(executor.GetBackoffWaitTime()).ret()
	default:
	}

	var phase = common.ApplicationRunning
	if !hasHealthCheckPolicy(appFile.PolicyWorkloads) {
		app.Status.Services = handler.services
		if !isHealthy(handler.services) {
			phase = common.ApplicationUnhealthy
		}
	}

	r.stateKeep(logCtx, handler, app)
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
	return r.gcResourceTrackers(logCtx, handler, phase, true, false)
}

func (r *Reconciler) stateKeep(logCtx monitorContext.Context, handler *AppHandler, app *v1beta1.Application) {
	if feature.DefaultMutableFeatureGate.Enabled(features.ApplyOnce) {
		return
	}
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("state-keep").Observe(time.Since(t).Seconds())
	}()
	if err := handler.resourceKeeper.StateKeep(logCtx); err != nil {
		logCtx.Error(err, "Failed to run prevent-configuration-drift")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedStateKeep, err))
		app.Status.SetConditions(condition.ErrorCondition("StateKeep", err))
	}
}

func (r *Reconciler) gcResourceTrackers(logCtx monitorContext.Context, handler *AppHandler, phase common.ApplicationPhase, gcOutdated bool, isUpdate bool) (ctrl.Result, error) {
	subCtx := logCtx.Fork("gc_resourceTrackers", monitorContext.DurationMetric(func(v float64) {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rt").Observe(v)
	}))
	defer subCtx.Commit("finish gc resourceTrackers")

	statusUpdater := r.patchStatus
	if isUpdate {
		statusUpdater = r.updateStatus
	}

	var options []resourcekeeper.GCOption
	if !gcOutdated {
		options = append(options, resourcekeeper.DisableMarkStageGCOption{}, resourcekeeper.DisableGCComponentRevisionOption{}, resourcekeeper.DisableLegacyGCOption{})
	}
	finished, waiting, err := handler.resourceKeeper.GarbageCollect(logCtx, options...)
	if err != nil {
		logCtx.Error(err, "Failed to gc resourcetrackers")
		cond := condition.Deleting()
		cond.Message = fmt.Sprintf("error encountered during garbage collection: %s", err.Error())
		handler.app.Status.SetConditions(cond)
		return r.result(statusUpdater(logCtx, handler.app, phase)).ret()
	}
	if !finished {
		logCtx.Info("GarbageCollecting resourcetrackers unfinished")
		cond := condition.Deleting()
		if len(waiting) > 0 {
			cond.Message = fmt.Sprintf("Waiting for %s to delete. (At least %d resources are deleting.)", waiting[0].DisplayName(), len(waiting))
		}
		handler.app.Status.SetConditions(cond)
		return r.result(statusUpdater(logCtx, handler.app, phase)).requeue(baseGCBackoffWaitTime).ret()
	}
	logCtx.Info("GarbageCollected resourcetrackers")
	return r.result(statusUpdater(logCtx, handler.app, phase)).ret()
}

type reconcileResult struct {
	time.Duration
	err error
}

func (r *reconcileResult) requeue(d time.Duration) *reconcileResult {
	r.Duration = d
	return r
}

func (r *reconcileResult) ret() (ctrl.Result, error) {
	if r.Duration.Seconds() != 0 {
		return ctrl.Result{RequeueAfter: r.Duration}, r.err
	} else if r.err != nil {
		return ctrl.Result{}, r.err
	}
	return ctrl.Result{RequeueAfter: common2.ApplicationReSyncPeriod}, nil
}

func (r *reconcileResult) end(endReconcile bool) (bool, ctrl.Result, error) {
	ret, err := r.ret()
	return endReconcile, ret, err
}

func (r *Reconciler) result(err error) *reconcileResult {
	return &reconcileResult{err: err}
}

// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
// by setting application(namespace-scoped) as their owners.
// We must delete all resource trackers related to an application through finalizer logic.
func (r *Reconciler) handleFinalizers(ctx monitorContext.Context, app *v1beta1.Application, handler *AppHandler) (bool, ctrl.Result, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(app, oam.FinalizerResourceTracker) {
			subCtx := ctx.Fork("handle-finalizers", monitorContext.DurationMetric(func(v float64) {
				metrics.AppReconcileStageDurationHistogram.WithLabelValues("add-finalizer").Observe(v)
			}))
			defer subCtx.Commit("finish add finalizers")
			meta.AddFinalizer(app, oam.FinalizerResourceTracker)
			subCtx.Info("Register new finalizer for application", "finalizer", oam.FinalizerResourceTracker)
			return r.result(errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)).end(true)
		}
	} else {
		if slices.Contains(app.GetFinalizers(), oam.FinalizerResourceTracker) {
			subCtx := ctx.Fork("handle-finalizers", monitorContext.DurationMetric(func(v float64) {
				metrics.AppReconcileStageDurationHistogram.WithLabelValues("remove-finalizer").Observe(v)
			}))
			defer subCtx.Commit("finish remove finalizers")
			rootRT, currentRT, historyRTs, cvRT, err := resourcetracker.ListApplicationResourceTrackers(ctx, r.Client, app)
			if err != nil {
				return r.result(err).end(true)
			}
			result, err := r.gcResourceTrackers(ctx, handler, common.ApplicationDeleting, true, true)
			if err != nil {
				return true, result, err
			}
			if rootRT == nil && currentRT == nil && len(historyRTs) == 0 && cvRT == nil {
				meta.RemoveFinalizer(app, oam.FinalizerResourceTracker)
				meta.RemoveFinalizer(app, oam.FinalizerOrphanResource)
				return r.result(errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)).end(true)
			}
			if wfContext.EnableInMemoryContext {
				wfContext.MemStore.DeleteInMemoryContext(app.Name)
			}
			return true, result, err
		}
	}
	return r.result(nil).end(false)
}

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, app *v1beta1.Application, condition condition.Condition, phase common.ApplicationPhase) (ctrl.Result, error) {
	app.SetConditions(condition)
	if err := r.patchStatus(ctx, app, phase); err != nil {
		return r.result(errors.WithMessage(err, "cannot update application status")).ret()
	}
	return r.result(fmt.Errorf("object level reconcile error, type: %q, msg: %q", string(condition.Type), condition.Message)).ret()
}

func (r *Reconciler) patchStatus(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	if oldApp, ok := originalAppFrom(ctx); ok && oldApp != nil && equality.Semantic.DeepEqual(oldApp.Status, app.Status) {
		return nil
	}
	ctx, cancel := ctrlrec.NewReconcileTerminationContext(ctx)
	defer cancel()
	if err := r.Status().Patch(ctx, app, client.Merge); err != nil {
		// set to -1 to re-run workflow if status is failed to patch
		executor.StepStatusCache.Store(fmt.Sprintf("%s-%s", app.Name, app.Namespace), -1)
		return err
	}
	return nil
}

func (r *Reconciler) updateStatus(ctx context.Context, app *v1beta1.Application, phase common.ApplicationPhase) error {
	app.Status.Phase = phase
	updateObservedGeneration(app)
	if oldApp, ok := originalAppFrom(ctx); ok && oldApp != nil && equality.Semantic.DeepEqual(oldApp.Status, app.Status) {
		return nil
	}
	ctx, cancel := ctrlrec.NewReconcileTerminationContext(ctx)
	defer cancel()
	if !r.disableStatusUpdate {
		return r.Status().Update(ctx, app)
	}
	obj, err := app.Unstructured()
	if err != nil {
		return err
	}
	if err := r.Status().Update(ctx, obj); err != nil {
		// set to -1 to re-run workflow if status is failed to update
		executor.StepStatusCache.Store(fmt.Sprintf("%s-%s", app.Name, app.Namespace), -1)
		return err
	}
	return nil
}

func (r *Reconciler) doWorkflowFinish(logCtx monitorContext.Context, app *v1beta1.Application, handler *AppHandler, state workflowv1alpha1.WorkflowRunPhase) {
	logCtx = logCtx.Fork("do-workflow-finish", monitorContext.DurationMetric(func(v float64) {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("do-workflow-finish").Observe(v)
	}))
	defer logCtx.Commit("do-workflow-finish")
	app.Status.Workflow.Finished = true
	app.Status.Workflow.EndTime = metav1.Now()
	executor.StepStatusCache.Delete(fmt.Sprintf("%s-%s", app.Name, app.Namespace))
	wfContext.CleanupMemoryStore(app.Name, app.Namespace)
	t := time.Since(app.Status.Workflow.StartTime.Time).Seconds()
	metrics.WorkflowFinishedTimeHistogram.WithLabelValues(string(state)).Observe(t)
	if state == workflowv1alpha1.WorkflowStateSucceeded {
		app.Status.SetConditions(condition.ReadyCondition(common.WorkflowCondition.String()))
		r.Recorder.Event(app, event.Normal(velatypes.ReasonApplied, velatypes.MessageWorkflowFinished))
	}
	handler.UpdateApplicationRevisionStatus(logCtx, handler.currentAppRev, app.Status.Workflow)
	logCtx.Info("Application manifests has applied by workflow successfully")
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
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&source.Kind{
			Type: &v1beta1.ResourceTracker{},
		}, ctrlHandler.EnqueueRequestsFromMapFunc(findObjectForResourceTracker)).
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
				old.ManagedFields = nil
				new.ManagedFields = nil

				// if the generation is changed, return true to let the controller handle it
				if old.Generation != new.Generation {
					return true
				}

				// filter the events triggered by initial application status
				if new.Status.Phase == common.ApplicationRendering || (old.Status.Phase == common.ApplicationRendering && new.Status.Phase == common.ApplicationRunningWorkflow) {
					return false
				}

				// ignore the changes in workflow status
				if old.Status.Workflow != nil && new.Status.Workflow != nil {
					// only workflow execution will change the status.workflow
					// let workflow backoff to requeue the event
					new.Status.Workflow.Steps = old.Status.Workflow.Steps
					new.Status.Workflow.ContextBackend = old.Status.Workflow.ContextBackend
					new.Status.Workflow.Message = old.Status.Workflow.Message
					new.Status.Workflow.EndTime = old.Status.Workflow.EndTime
				}

				// appliedResources and Services will be changed during the execution of workflow
				// once the resources is added, the managed fields will also be changed
				new.Status.AppliedResources = old.Status.AppliedResources
				new.Status.Services = old.Status.Services
				// the resource version will be changed if the object is changed
				// ignore this change and let reflect.DeepEqual to compare the rest of the object
				new.ResourceVersion = old.ResourceVersion
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
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("Application")),
		dm:       args.DiscoveryMapper,
		pd:       args.PackageDiscover,
		options:  parseOptions(args),
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
	new.ResourceVersion = old.ResourceVersion
	return !reflect.DeepEqual(new, old)
}

func findObjectForResourceTracker(rt client.Object) []reconcile.Request {
	if EnableResourceTrackerDeleteOnlyTrigger && rt.GetDeletionTimestamp() == nil {
		return nil
	}
	if labels := rt.GetLabels(); labels != nil {
		var request reconcile.Request
		request.Name = labels[oam.LabelAppName]
		request.Namespace = labels[oam.LabelAppNamespace]
		if request.Namespace != "" && request.Name != "" {
			return []reconcile.Request{request}
		}
	}
	return nil
}

func timeReconcile(app *v1beta1.Application) func() {
	t := time.Now()
	beginPhase := string(app.Status.Phase)
	return func() {
		v := time.Since(t).Seconds()
		metrics.ApplicationReconcileTimeHistogram.WithLabelValues(beginPhase, string(app.Status.Phase)).Observe(v)
	}
}

func parseOptions(args core.Args) options {
	return options{
		disableStatusUpdate:  args.EnableCompatibility,
		appRevisionLimit:     args.AppRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
		ignoreAppNoCtrlReq:   args.IgnoreAppWithoutControllerRequirement,
		controllerVersion:    version.VelaVersion,
	}
}

func (r *Reconciler) matchControllerRequirement(app *v1beta1.Application) bool {
	if app.Annotations != nil {
		if requireVersion, ok := app.Annotations[oam.AnnotationControllerRequirement]; ok {
			return requireVersion == r.controllerVersion
		}
	}
	if r.ignoreAppNoCtrlReq {
		return false
	}
	return true
}

const (
	// ComponentNamespaceContextKey is the key in context that defines the override namespace of component
	ComponentNamespaceContextKey contextKey = iota
	// ComponentContextKey is the key in context that records the component
	ComponentContextKey
	// ReplicaKeyContextKey is the key in context that records the replica key
	ReplicaKeyContextKey
	// OriginalAppKey is the key in the context that records the in coming original app
	OriginalAppKey
)

func withOriginalApp(ctx context.Context, app *v1beta1.Application) context.Context {
	return context.WithValue(ctx, OriginalAppKey, app.DeepCopy())
}

func originalAppFrom(ctx context.Context) (*v1beta1.Application, bool) {
	app, ok := ctx.Value(OriginalAppKey).(*v1beta1.Application)
	return app, ok
}
