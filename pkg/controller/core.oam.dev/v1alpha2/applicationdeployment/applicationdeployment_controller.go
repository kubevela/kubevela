package applicationdeployment

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/slice"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

const appDeployfinalizer = "finalizers.applicationdeployment.oam.dev"

// Reconciler reconciles an ApplicationDeployment object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	record event.Recorder
	Scheme *runtime.Scheme
}

// Reconcile is the main logic of applicationdeployment controller
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationconfigurations,verbs=get;list;watch;create;update;patch;delete
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var appDeploy corev1alpha2.ApplicationDeployment
	if err := r.Get(ctx, req.NamespacedName, &appDeploy); err != nil {
		if apierrors.IsNotFound(err) {
			klog.InfoS("application deployment does not exist", "appDeploy", klog.KRef(req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Start to reconcile ", "application deployment", klog.KObj(&appDeploy))

	r.handleFinalizer(&appDeploy)

	// Get the target application
	var targetApp, sourceApp corev1alpha2.Application
	targetAppName := appDeploy.Spec.TargetApplicationName
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: req.Namespace, Name: targetAppName},
		&targetApp); err != nil {
		klog.ErrorS(err, "cannot locate target application", "target application",
			klog.KRef(req.Namespace, targetAppName))
		return ctrl.Result{}, err
	}
	// Get the source application
	sourceAppName := appDeploy.Spec.SourceApplicationName
	if sourceAppName == nil {
		klog.Info("source app fields not filled, we assume it is deployed for the first time")
	} else if err := r.Get(ctx, ktypes.NamespacedName{Namespace: req.Namespace, Name: *sourceAppName},
		&sourceApp); err != nil {
		klog.ErrorS(err, "cannot locate source application", "source application", klog.KRef(req.Namespace,
			*sourceAppName))
		return ctrl.Result{}, err
	}
	// Get the kubernetes workloads to upgrade from the application
	targetWorkload, sourceWorkload, err := r.extractWorkload(appDeploy.Spec.ComponentList, &targetApp, &sourceApp)
	if err != nil {
		klog.Error(err, "cannot locate the workloads object")
		return ctrl.Result{}, err
	}
	klog.InfoS("get the target workload we need to work on", "targetWorkload", klog.KObj(targetWorkload))
	if sourceWorkload != nil {
		klog.InfoS("get the source workload we need to work on", "sourceWorkload", klog.KObj(sourceWorkload))
	}
	// TODO: pass these two object to the rollout plan
	return ctrl.Result{}, nil
}

func (r *Reconciler) handleFinalizer(appDeploy *corev1alpha2.ApplicationDeployment) {
	if appDeploy.DeletionTimestamp.IsZero() {
		if !slice.ContainsString(appDeploy.Finalizers, appDeployfinalizer, nil) {
			// TODO: add finalizer
			klog.Info("add finalizer")
		}
	} else if slice.ContainsString(appDeploy.Finalizers, appDeployfinalizer, nil) {
		// TODO: perform finalize
		klog.Info("perform clean up")
	}
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ApplicationDeployment")).
		WithAnnotations("controller", "ApplicationDeployment")
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha2.ApplicationDeployment{}).
		Owns(&corev1alpha2.Application{}).
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
