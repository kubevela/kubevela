package applicationdeployment

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
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

const appDeployFinalizer = "finalizers.applicationdeployment.oam.dev"
const reconcileTimeOut = 60 * time.Second

// Reconciler reconciles an ApplicationDeployment object
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

// Reconcile is the main logic of applicationdeployment controller
func (r *Reconciler) Reconcile(req ctrl.Request) (res reconcile.Result, retErr error) {
	var appRollout oamv1alpha2.AppRollout
	ctx, cancel := context.WithTimeout(context.TODO(), reconcileTimeOut)
	defer cancel()

	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished reconciling appDeployment", "deployment", req, "time spent",
					time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished reconcile appDeployment", "deployment", req, "time spent", time.Since(startTime))
			}
		} else {
			klog.Errorf("Failed to reconcile appDeployment %s: %v", req, retErr)
		}
	}()

	if err := r.Get(ctx, req.NamespacedName, &appRollout); err != nil {
		if apierrors.IsNotFound(err) {
			klog.InfoS("application deployment does not exist", "appRollout", klog.KRef(req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Start to reconcile ", "application deployment", klog.KObj(&appRollout))

	// TODO: check if the target/source has changed
	r.handleFinalizer(&appRollout)

	ctx = oamutil.SetNamespaceInCtx(ctx, appRollout.Namespace)

	// Get the target application
	var targetApp oamv1alpha2.ApplicationConfiguration
	sourceApp := &oamv1alpha2.ApplicationConfiguration{}
	targetAppName := appRollout.Spec.TargetAppRevisionName
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: req.Namespace, Name: targetAppName},
		&targetApp); err != nil {
		klog.ErrorS(err, "cannot locate target application", "target application",
			klog.KRef(req.Namespace, targetAppName))
		return ctrl.Result{}, err
	}

	// Get the source application
	sourceAppName := appRollout.Spec.SourceApplicationName
	if sourceAppName == "" {
		klog.Info("source app fields not filled, we assume it is deployed for the first time")
		sourceApp = nil
	} else if err := r.Get(ctx, ktypes.NamespacedName{Namespace: req.Namespace, Name: sourceAppName}, sourceApp); err != nil {
		klog.ErrorS(err, "cannot locate source application", "source application", klog.KRef(req.Namespace,
			sourceAppName))
		return ctrl.Result{}, err
	}

	targetWorkload, sourceWorkload, err := r.extractWorkloads(ctx, appRollout.Spec.ComponentList, &targetApp, sourceApp)
	if err != nil {
		klog.ErrorS(err, "cannot fetch the workloads to upgrade", "target application",
			klog.KRef(req.Namespace, targetAppName), "source application", klog.KRef(req.Namespace, sourceAppName),
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
	if rolloutStatus.RollingState == v1alpha1.RolloutSucceedState {
		// remove the rollout annotation so that the target appConfig controller can take over the rest of the work
		oamutil.RemoveAnnotations(&targetApp, []string{oam.AnnotationAppRollout})
		if err := r.Update(ctx, &targetApp); err != nil {
			klog.ErrorS(err, "cannot remove the rollout annotation", "target application",
				klog.KRef(req.Namespace, targetAppName))
			return ctrl.Result{}, err
		}
	}
	// update the appRollout status
	return result, r.updateStatus(ctx, &appRollout)
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
		if !slice.ContainsString(appRollout.Finalizers, appDeployFinalizer, nil) {
			// TODO: add finalizer
			klog.Info("add finalizer")
		}
	} else if slice.ContainsString(appRollout.Finalizers, appDeployFinalizer, nil) {
		// TODO: perform finalize
		klog.Info("perform clean up")
	}
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ApplicationDeployment")).
		WithAnnotations("controller", "ApplicationDeployment")
	return ctrl.NewControllerManagedBy(mgr).
		For(&oamv1alpha2.AppRollout{}).
		Owns(&oamv1alpha2.Application{}).
		Complete(r)
}

// Setup adds a controller that reconciles ApplicationDeployment.
func Setup(mgr ctrl.Manager, _ controller.Args, _ logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("create discovery dm fail %w", err)
	}
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		dm:     dm,
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
