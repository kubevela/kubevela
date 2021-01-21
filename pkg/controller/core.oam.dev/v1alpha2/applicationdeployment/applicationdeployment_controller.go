package applicationdeployment

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// Reconciler reconciles a PodSpecWorkload object
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
	var appdeploy v1alpha2.ApplicationDeployment
	if err := r.Get(ctx, req.NamespacedName, &appdeploy); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("applicationdeployment is deleted")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Start to reconcile ", "application deployment", klog.KObj(&appdeploy))

	// TODO add reconcile logic here

	return ctrl.Result{}, nil
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ApplicationDeployment")).
		WithAnnotations("controller", "ApplicationDeployment")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.ApplicationDeployment{}).
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
