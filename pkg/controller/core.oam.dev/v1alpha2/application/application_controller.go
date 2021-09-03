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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
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
	// WorkflowReconcileWaitTime is the time to wait before reconcile again workflow running
	WorkflowReconcileWaitTime      = time.Second * 3
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
	ctx, cancel := common2.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile application", "application", klog.KRef(req.Namespace, req.Name))

	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	if len(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]) == 0 {
		oamutil.AddAnnotations(app, map[string]string{
			oam.AnnotationKubeVelaVersion: version.VelaVersion,
		})
	}
	handler := &AppHandler{
		r:   r,
		app: app,
	}
	endReconcile, err := r.handleFinalizers(ctx, app)
	if err != nil {
		return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
	}
	if endReconcile {
		return ctrl.Result{}, nil
	}

	// parse application to appfile
	app.Status.Phase = common.ApplicationRendering
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	appFile, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		klog.ErrorS(err, "Failed to parse application", "application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Parsed", err))
	}
	app.Status.SetConditions(condition.ReadyCondition("Parsed"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonParsed, velatypes.MessageParsed))

	if err := handler.PrepareCurrentAppRevision(ctx, appFile); err != nil {
		klog.ErrorS(err, "Failed to prepare app revision", "application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Revision", err))
	}
	klog.Info("Successfully prepare current app revision", "revisionName", handler.currentAppRev.Name,
		"revisionHash", handler.currentRevHash, "isNewRevision", handler.isNewRevision)

	var comps []*velatypes.ComponentManifest
	comps, err = appFile.GenerateComponentManifests()
	if err != nil {
		klog.ErrorS(err, "Failed to render components", "application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Render", err))
	}

	handler.handleCheckManageWorkloadTrait(handler.currentAppRev.Spec.TraitDefinitions, comps)

	if err := handler.HandleComponentsRevision(ctx, comps); err != nil {
		klog.ErrorS(err, "Failed to handle compoents revision", "application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Render", err))
	}

	if err := handler.FinalizeAndApplyAppRevision(ctx, comps); err != nil {
		klog.ErrorS(err, "Failed to apply app revision", "application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRevision, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Revision", err))
	}
	app.Status.SetConditions(condition.ReadyCondition("Revision"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRevisoned, velatypes.MessageRevisioned))
	klog.Info("Successfully apply application revision", "application", klog.KObj(app))

	policies, wfSteps, err := appFile.GenerateWorkflowAndPolicy(ctx, r.dm, r.Client, r.pd, handler.Dispatch)
	if err != nil {
		klog.Error(err, "[Handle GenerateWorkflowAndPolicy]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Render", err))
	}
	app.Status.SetConditions(condition.ReadyCondition("Render"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))
	klog.InfoS("Successfully render application resources", "application", klog.KObj(app))

	if err := handler.ApplyAppManifests(ctx, comps, policies); err != nil {
		klog.ErrorS(err, "Failed to apply application manifests",
			"application", klog.KObj(app))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Applied", err))
	}
	if err := handler.UpdateAppLatestRevisionStatus(ctx); err != nil {
		klog.ErrorS(err, "Failed to update application status", "application", klog.KObj(app))
		return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
	}
	app.Status.SetConditions(condition.ReadyCondition("Applied"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonApplied, velatypes.MessageApplied))
	klog.Info("Successfully apply application manifests", "application", klog.KObj(app))

	done, pause, err := workflow.NewWorkflow(app, r.Client).ExecuteSteps(ctx, handler.currentAppRev.Name, wfSteps)
	if err != nil {
		klog.Error(err, "[handle workflow]")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedWorkflow, err))
		return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Workflow", err))
	}

	if pause {
		if err := r.patchStatus(ctx, app); err != nil {
			return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
		}
		return ctrl.Result{}, nil
	}

	if !done {
		return reconcile.Result{RequeueAfter: WorkflowReconcileWaitTime}, r.patchStatus(ctx, app)
	}

	wfStatus := app.Status.Workflow
	if wfStatus != nil {
		if wfStatus.Terminated && app.Status.Phase == common.ApplicationWorkflowTerminated {
			if err := r.patchStatus(ctx, app); err != nil {
				return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
			}
			return ctrl.Result{}, nil
		}

		if !wfStatus.Terminated {
			ref, err := handler.DispatchAndGC(ctx)
			if err != nil {
				klog.ErrorS(err, "Failed to gc after workflow",
					"application", klog.KObj(app))
				r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
				return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("GCAfterWorkflow", err))
			}
			wfStatus.Terminated = true
			app.Status.ResourceTracker = ref
			return r.endWithNegativeCondition(ctx, app, condition.ReadyCondition("GCAfterWorkflow"))
		}
	}

	// if inplace is false and rolloutPlan is nil, it means the user will use an outer AppRollout object to rollout the application
	if handler.app.Spec.RolloutPlan != nil {
		res, err := handler.handleRollout(ctx)
		if err != nil {
			klog.ErrorS(err, "Failed to handle rollout", "application", klog.KObj(app))
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRollout, err))
			return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("Rollout", err))
		}
		// skip health check and garbage collection if rollout have not finished
		// start next reconcile immediately
		if res.Requeue || res.RequeueAfter > 0 {
			app.Status.Phase = common.ApplicationRollingOut
			if err := r.patchStatus(ctx, app); err != nil {
				return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
			}
			return res, nil
		}

		// there is no need reconcile immediately, that means the rollout operation have finished
		r.Recorder.Event(app, event.Normal(velatypes.ReasonRollout, velatypes.MessageRollout))
		app.Status.SetConditions(condition.ReadyCondition("Rollout"))
		klog.Info("Finished rollout ")
	}

	app.Status.Phase = common.ApplicationHealthChecking
	klog.Info("Check application health status")

	// check application health status if no health check policy is applied
	if !hasHealthCheckPolicy(appFile.Policies) {
		appCompStatus, healthy, err := handler.aggregateHealthStatus(appFile)
		if err != nil {
			klog.ErrorS(err, "application", klog.KObj(app))
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedHealthCheck, err))
			app.Status.SetConditions(condition.Condition{
				Type:               v1beta1.TypeHealthy,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             v1beta1.ReasonHealthCheckErr,
			})
			return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("HealthCheck", err))
		}
		app.Status.Services = appCompStatus
		if !healthy {
			app.Status.SetConditions(condition.Condition{
				Type:               v1beta1.TypeHealthy,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             v1beta1.ReasonUnhealthy,
			})
			if err := r.patchStatus(ctx, app); err != nil {
				return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
			}
			return r.endWithNegativeCondition(ctx, app, condition.ErrorCondition("HealthCheck", errors.New("not healthy")))
		}
		app.Status.SetConditions(condition.Condition{
			Type:               v1beta1.TypeHealthy,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             v1beta1.ReasonHealthy,
		})
		r.Recorder.Event(app, event.Normal(velatypes.ReasonHealthCheck, velatypes.MessageHealthCheck))
	}
	app.Status.Phase = common.ApplicationRunning

	if err := garbageCollection(ctx, handler); err != nil {
		klog.ErrorS(err, "Failed to run garbage collection")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
		return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
	}
	klog.Info("Successfully garbage collect", "application", klog.KObj(app))

	r.Recorder.Event(app, event.Normal(velatypes.ReasonDeployed, velatypes.MessageDeployed))
	if err := r.patchStatus(ctx, app); err != nil {
		return r.endWithNegativeCondition(ctx, app, condition.ReconcileError(err))
	}
	return ctrl.Result{}, nil
}

// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
// by setting application(namespace-scoped) as their owners.
// We must delete all resource trackers related to an application through finalizer logic.
func (r *Reconciler) handleFinalizers(ctx context.Context, app *v1beta1.Application) (bool, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(app, resourceTrackerFinalizer) {
			meta.AddFinalizer(app, resourceTrackerFinalizer)
			klog.InfoS("Register new finalizer for application", "application", klog.KObj(app), "finalizer", resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	} else {
		if meta.FinalizerExists(app, legacyResourceTrackerFinalizer) {
			// TODO(roywang) legacyResourceTrackerFinalizer will be deprecated in the future
			// this is for backward compatibility
			rt := &v1beta1.ResourceTracker{}
			rt.SetName(fmt.Sprintf("%s-%s", app.Namespace, app.Name))
			if err := r.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete legacy resource tracker", "name", rt.Name)
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
				klog.ErrorS(err, "Failed to list resource tracker of app", "name", app.Name)
				return true, errors.WithMessage(err, "cannot remove finalizer")
			}
			for _, rt := range rtList.Items {
				if err := r.Client.Delete(ctx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete resource tracker", "name", rt.Name)
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

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, app *v1beta1.Application, condition condition.Condition) (ctrl.Result, error) {
	app.SetConditions(condition)
	if err := r.patchStatus(ctx, app); err != nil {
		return ctrl.Result{}, errors.WithMessage(err, "cannot update application status")
	}
	return ctrl.Result{}, fmt.Errorf("object level reconcile error, type: %q, msg: %q", string(condition.Type), condition.Message)
}

func (r *Reconciler) patchStatus(ctx context.Context, app *v1beta1.Application) error {
	updateObservedGeneration(app)
	return r.Client.Status().Patch(ctx, app, client.Merge)
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
