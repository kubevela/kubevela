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

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/version"
)

const (
	errUpdateApplicationStatus    = "cannot update application status"
	errUpdateApplicationFinalizer = "cannot update application finalizer"
)

const (
	legacyResourceTrackerFinalizer = "resourceTracker.finalizer.core.oam.dev"
	// resourceTrackerFinalizer is to delete the resource tracker of the latest app revision.
	resourceTrackerFinalizer = "app.oam.dev/resource-tracker-finalizer"
	// onlyRevisionFinalizer is to delete all resource trackers of app revisions which may be used
	// out of the domain of app controller, e.g., AppRollout controller.
	onlyRevisionFinalizer = "app.oam.dev/only-revision-finalizer"
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
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	klog.InfoS("Reconcile application", "application", klog.KRef(req.Namespace, req.Name))

	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	// this annotation will be propogated to all resources created by the application
	if len(app.GetAnnotations()[oam.AnnotationKubeVelaVersion]) == 0 {
		oamutil.AddAnnotations(app, map[string]string{
			oam.AnnotationKubeVelaVersion: version.VelaVersion,
		})
	}
	if endReconcile, err := r.handleFinalizers(ctx, app); endReconcile {
		return ctrl.Result{}, err
	}

	handler := &appHandler{
		r:   r,
		app: app,
	}
	if app.Status.LatestRevision != nil {
		// record previous app revision name
		handler.previousRevisionName = app.Status.LatestRevision.Name
	}

	app.Status.Phase = common.ApplicationRendering
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	generatedAppfile, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		klog.ErrorS(err, "Failed to parse application", "application", klog.KObj(app))
		app.Status.SetConditions(errorCondition("Parsed", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return handler.handleErr(err)
	}
	app.Status.SetConditions(readyCondition("Parsed"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonParsed, velatypes.MessageParsed))

	handler.appfile = generatedAppfile
	appRev, err := handler.GenerateAppRevision(ctx)
	if err != nil {
		klog.ErrorS(err, "Failed to calculate appRevision", "application", klog.KObj(app))
		app.Status.SetConditions(errorCondition("Parsed", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return handler.handleErr(err)
	}
	klog.Info("Successfully calculate appRevision", "revisionName", appRev.Name,
		"revisionHash", handler.revisionHash, "isNewRevision", handler.isNewRevision)

	// pass appRevision to appfile, so it can be used to render data in context.appRevision
	generatedAppfile.RevisionName = appRev.Name
	// build template to applicationconfig & component
	ac, comps, err := generatedAppfile.GenerateApplicationConfiguration()
	if err != nil {
		klog.ErrorS(err, "Failed to generate applicationConfiguration", "application", klog.KObj(app))
		app.Status.SetConditions(errorCondition("Built", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return handler.handleErr(err)
	}
	app.Status.SetConditions(readyCondition("Built"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))
	klog.Info("Successfully render application resources", "application", klog.KObj(app))

	// pass application's labels and annotations to ac
	oamutil.PassLabelAndAnnotation(app, ac)
	// apply application resources' manifests to the cluster
	if err := handler.apply(ctx, appRev, ac, comps); err != nil {
		klog.ErrorS(err, "Failed to apply application resources' manifests",
			"application", klog.KObj(app))
		app.Status.SetConditions(errorCondition("Applied", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
		return handler.handleErr(err)
	}
	klog.Info("Successfully apply application resources' manifests", "application", klog.KObj(app))

	// if inplace is false and rolloutPlan is nil, it means the user will use an outer AppRollout object to rollout the application
	if handler.app.Spec.RolloutPlan != nil {
		res, err := handler.handleRollout(ctx)
		if err != nil {
			klog.ErrorS(err, "Failed to handle rollout", "application", klog.KObj(app))
			app.Status.SetConditions(errorCondition("Rollout", err))
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRollout, err))
			return handler.handleErr(err)
		}
		// skip health check and garbage collection if rollout have not finished
		// start next reconcile immediately
		if res.Requeue || res.RequeueAfter > 0 {
			app.Status.Phase = common.ApplicationRollingOut
			return res, r.UpdateStatus(ctx, app)
		}

		// there is no need reconcile immediately, that means the rollout operation have finished
		r.Recorder.Event(app, event.Normal(velatypes.ReasonRollout, velatypes.MessageRollout))
		app.Status.SetConditions(readyCondition("Rollout"))
		klog.Info("Finished rollout ")
	}

	// The following logic will be skipped if rollout have not finished
	app.Status.SetConditions(readyCondition("Applied"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonFailedApply, velatypes.MessageApplied))
	app.Status.Phase = common.ApplicationHealthChecking
	klog.Info("Check application health status")
	// check application health status
	appCompStatus, healthy, err := handler.statusAggregate(generatedAppfile)
	if err != nil {
		klog.ErrorS(err, "Failed to aggregate status", "application", klog.KObj(app))
		app.Status.SetConditions(errorCondition("HealthCheck", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedHealthCheck, err))
		return handler.handleErr(err)
	}
	if !healthy {
		app.Status.SetConditions(errorCondition("HealthCheck", errors.New("not healthy")))

		app.Status.Services = appCompStatus
		// unhealthy will check again after 10s
		return ctrl.Result{RequeueAfter: time.Second * 10}, r.Status().Update(ctx, app)
	}
	app.Status.Services = appCompStatus
	app.Status.SetConditions(readyCondition("HealthCheck"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonHealthCheck, velatypes.MessageHealthCheck))
	app.Status.Phase = common.ApplicationRunning

	if err := garbageCollection(ctx, handler); err != nil {
		klog.ErrorS(err, "Failed to run Garbage collection")
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
		return handler.handleErr(err)
	}
	klog.Info("Successfully garbage collect", "application", klog.KObj(app))

	// Gather status of components
	var refComps []v1alpha1.TypedReference
	for _, comp := range comps {
		refComps = append(refComps, v1alpha1.TypedReference{
			APIVersion: comp.APIVersion,
			Kind:       comp.Kind,
			Name:       comp.Name,
			UID:        app.UID,
		})
	}
	app.Status.Components = refComps
	r.Recorder.Event(app, event.Normal(velatypes.ReasonDeployed, velatypes.MessageDeployed))
	return ctrl.Result{}, r.UpdateStatus(ctx, app)
}

// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
// by setting application(namespace-scoped) as their owner.
// We delete all resource trackers related to an application through below finalizer logic.
func (r *Reconciler) handleFinalizers(ctx context.Context, app *v1beta1.Application) (bool, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(app, resourceTrackerFinalizer) {
			meta.AddFinalizer(app, resourceTrackerFinalizer)
			klog.InfoS("Register new finalizer for application", "application", klog.KObj(app), "finalizer", resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		if appWillReleaseByRollout(app) {
			klog.InfoS("Found an application which will be released by rollout", "application", klog.KObj(app))
			if !meta.FinalizerExists(app, onlyRevisionFinalizer) {
				meta.AddFinalizer(app, onlyRevisionFinalizer)
				klog.InfoS("Register new finalizer for application", "application", klog.KObj(app), "finalizer", onlyRevisionFinalizer)
				return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
			}
		}
	} else {
		if meta.FinalizerExists(app, legacyResourceTrackerFinalizer) {
			// TODO(roywang) legacyResourceTrackerFinalizer will be deprecated in the future
			// this is for backward compatibility
			rt := &v1beta1.ResourceTracker{}
			rt.SetName(fmt.Sprintf("%s-%s", app.Namespace, app.Name))
			if err := r.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete legacy resource tracker", "name", rt.Name)
				app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
				return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
			}
			meta.RemoveFinalizer(app, legacyResourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		if meta.FinalizerExists(app, resourceTrackerFinalizer) {
			if app.Status.LatestRevision != nil && len(app.Status.LatestRevision.Name) != 0 {
				latestTracker := &v1beta1.ResourceTracker{}
				latestTracker.SetName(dispatch.ConstructResourceTrackerName(app.Status.LatestRevision.Name, app.Namespace))
				if err := r.Client.Delete(ctx, latestTracker); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete latest resource tracker", "name", latestTracker.Name)
					app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
					return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
				}
			}
			meta.RemoveFinalizer(app, resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		if meta.FinalizerExists(app, onlyRevisionFinalizer) {
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}
			rtList := &v1beta1.ResourceTrackerList{}
			if err := r.Client.List(ctx, rtList, listOpts...); err != nil {
				klog.ErrorS(err, "Failed to list resource tracker of app", "name", app.Name)
				app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
				return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
			}
			for _, rt := range rtList.Items {
				if err := r.Client.Delete(ctx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete resource tracker", "name", rt.Name)
					app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
					return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
				}
			}
			meta.RemoveFinalizer(app, onlyRevisionFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	}
	return false, nil
}

// appWillReleaseByRollout judge whether the application will be released by rollout.
// If it's true, application controller will only create or update application revision but not emit any other K8s
// resources into the cluster. Rollout controller will do real release works.
func appWillReleaseByRollout(app *v1beta1.Application) bool {
	return len(app.GetAnnotations()[oam.AnnotationAppRollout]) != 0 || app.Spec.RolloutPlan != nil
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

// UpdateStatus updates v1beta1.Application's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, app *v1beta1.Application, opts ...client.UpdateOption) error {
	status := app.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: app.Name}, app); err != nil {
			return
		}
		app.Status = status
		return r.Status().Update(ctx, app, opts...)
	})
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
