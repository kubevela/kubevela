package applicationrollout

import (
	"context"
	"strconv"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/slice"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oamv1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common/rollout"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
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
func (r *Reconciler) Reconcile(req ctrl.Request) (res reconcile.Result, retErr error) {
	var appRollout oamv1alpha2.AppRollout

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

	r.handleFinalizer(&appRollout)
	targetAppRevisionName := appRollout.Spec.TargetAppRevisionName
	sourceAppRevisionName := appRollout.Spec.SourceAppRevisionName

	// handle rollout target/source change
	if appRollout.Status.RollingState == v1alpha1.RolloutSucceedState ||
		appRollout.Status.RollingState == v1alpha1.RolloutFailedState {
		if appRollout.Status.LastUpgradedTargetAppRevision == targetAppRevisionName &&
			appRollout.Status.LastSourceAppRevision == sourceAppRevisionName {
			klog.InfoS("rollout completed, no need to reconcile", "source", sourceAppRevisionName,
				"target", targetAppRevisionName)
			return ctrl.Result{}, nil
		}
	}

	if appRollout.Status.LastUpgradedTargetAppRevision != "" &&
		appRollout.Status.LastUpgradedTargetAppRevision != targetAppRevisionName ||
		(appRollout.Status.LastSourceAppRevision != "" && appRollout.Status.LastSourceAppRevision != sourceAppRevisionName) {
		klog.InfoS("rollout target changed, restart the rollout", "new source", sourceAppRevisionName,
			"new target", targetAppRevisionName)
		r.record.Event(&appRollout, event.Normal("Rollout Restarted",
			"rollout target changed, restart the rollout", "new source", sourceAppRevisionName,
			"new target", targetAppRevisionName))

		if err := r.finalizeRollingAborted(ctx, appRollout.Status.LastUpgradedTargetAppRevision,
			appRollout.Status.LastSourceAppRevision); err != nil {
			klog.ErrorS(err, "failed to finalize the previous rolling resources ", "old source",
				appRollout.Status.LastSourceAppRevision, "old target", appRollout.Status.LastUpgradedTargetAppRevision)
		}
		appRollout.Status.RolloutModified()
	}

	// Get the source application
	var sourceApRev *oamv1alpha2.ApplicationRevision
	var sourceApp *oamv1alpha2.ApplicationContext
	var err error
	if sourceAppRevisionName == "" {
		klog.Info("source app fields not filled, this is a scale operation")
		sourceApp = nil
	} else {
		sourceApRev, sourceApp, err = r.getSourceAppContexts(ctx, sourceAppRevisionName)
		if err != nil {
			return ctrl.Result{}, err
		}
		// check if the source app is templated
		if sourceApp.Status.RollingStatus != oamv1alpha2.RollingTemplated {
			r.record.Event(&appRollout, event.Normal("Rollout Paused",
				"source app revision is not ready for rolling yet", "application revision", sourceApp.GetName()))
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
	}

	// Get the target application revision
	targetAppRev, targetApp, err := r.getTargetApps(ctx, targetAppRevisionName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// check if the app is templated
	if targetApp.Status.RollingStatus != oamv1alpha2.RollingTemplated {
		r.record.Event(&appRollout, event.Normal("Rollout Paused",
			"target app revision is not ready for rolling yet", "application revision", targetApp.GetName()))
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// we get the real workloads from the spec of the revisions
	targetWorkload, sourceWorkload, err := r.extractWorkloads(ctx, appRollout.Spec.ComponentList, targetAppRev, sourceApRev)
	if err != nil {
		klog.ErrorS(err, "cannot fetch the workloads to upgrade", "target application",
			klog.KRef(req.Namespace, targetAppRevisionName), "source application", klog.KRef(req.Namespace, sourceAppRevisionName),
			"commonComponent", appRollout.Spec.ComponentList)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, client.IgnoreNotFound(err)
	}
	klog.InfoS("get the target workload we need to work on", "targetWorkload", klog.KObj(targetWorkload))
	if sourceWorkload != nil {
		klog.InfoS("get the source workload we need to work on", "sourceWorkload", klog.KObj(sourceWorkload))
	}

	// reconcile the rollout part of the spec given the target and source workload
	rolloutPlanController := rollout.NewRolloutPlanController(r, &appRollout, r.record,
		&appRollout.Spec.RolloutPlan, &appRollout.Status.RolloutStatus, targetWorkload, sourceWorkload)
	result, rolloutStatus := rolloutPlanController.Reconcile(ctx)
	// make sure that the new status is copied back
	appRollout.Status.RolloutStatus = *rolloutStatus
	appRollout.Status.LastUpgradedTargetAppRevision = targetAppRevisionName
	appRollout.Status.LastSourceAppRevision = sourceAppRevisionName
	if rolloutStatus.RollingState == v1alpha1.RolloutSucceedState {
		if err = r.finalizeRollingSucceeded(ctx, sourceApp, targetApp); err != nil {
			return ctrl.Result{}, err
		}
		klog.InfoS("rollout succeeded, record the source and target app revision", "source", sourceAppRevisionName,
			"target", targetAppRevisionName)
	}
	// update the appRollout status
	return result, r.updateStatus(ctx, &appRollout)
}

func (r *Reconciler) finalizeRollingSucceeded(ctx context.Context, sourceApp *oamv1alpha2.ApplicationContext,
	targetApp *oamv1alpha2.ApplicationContext) error {
	if sourceApp != nil {
		// mark the source app as an application revision only so that it stop being reconciled
		oamutil.RemoveAnnotations(sourceApp, []string{oam.AnnotationAppRollout})
		oamutil.AddAnnotations(sourceApp, map[string]string{oam.AnnotationAppRevision: strconv.FormatBool(true)})
		if err := r.Update(ctx, sourceApp); err != nil {
			klog.ErrorS(err, "cannot add the app revision annotation", "source application",
				klog.KRef(sourceApp.Namespace, sourceApp.GetName()))
			return err
		}
	}
	// remove the rollout annotation so that the target appConfig controller can take over the rest of the work
	oamutil.RemoveAnnotations(targetApp, []string{oam.AnnotationAppRollout})
	if err := r.Update(ctx, targetApp); err != nil {
		klog.ErrorS(err, "cannot remove the rollout annotation", "target application",
			klog.KRef(targetApp.Namespace, targetApp.GetName()))
		return err
	}
	return nil
}

func (r *Reconciler) finalizeRollingAborted(ctx context.Context, sourceRevision, targetRevision string) error {
	// TODO:  finalize the previous appcontext the best we can
	return nil
}

// UpdateStatus updates v1alpha2.AppRollout's Status with retry.RetryOnConflict
func (r *Reconciler) updateStatus(ctx context.Context, appRollout *oamv1alpha2.AppRollout, opts ...client.UpdateOption) error {
	status := appRollout.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: appRollout.Namespace, Name: appRollout.Name}, appRollout); err != nil {
			return
		}
		appRollout.Status = status
		return r.Status().Update(ctx, appRollout, opts...)
	})
}

func (r *Reconciler) handleFinalizer(appRollout *oamv1alpha2.AppRollout) {
	if appRollout.DeletionTimestamp.IsZero() {
		if !slice.ContainsString(appRollout.Finalizers, appRolloutFinalizer, nil) {
			// TODO: add finalizer
			klog.Info("add finalizer")
		}
	} else if slice.ContainsString(appRollout.Finalizers, appRolloutFinalizer, nil) {
		// TODO: perform finalize
		klog.Info("perform clean up")
	}
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
		WithAnnotations("controller", "AppRollout")
	return ctrl.NewControllerManagedBy(mgr).
		For(&oamv1alpha2.AppRollout{}).
		Owns(&oamv1alpha2.Application{}).
		Complete(r)
}

// Setup adds a controller that reconciles AppRollout.
func Setup(mgr ctrl.Manager, args controller.Args, _ logging.Logger) error {
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		dm:     args.DiscoveryMapper,
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
