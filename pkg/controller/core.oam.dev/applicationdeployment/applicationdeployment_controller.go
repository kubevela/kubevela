package applicationdeployment

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/oam-dev/kubevela/api/core.oam.dev/v1alpha2"
)

// Reconciler reconciles a PodSpecWorkload object
type Reconciler struct {
	client.Client
	log    logr.Logger
	record event.Recorder
	Scheme *runtime.Scheme
}

// Reconcile is the main logci of applicationdeployment controller
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=applicationconfigurations,verbs=get;list;watch;create;update;patch;delete
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.log.WithValues("applicationdeployments", req.NamespacedName)
	log.Info("Reconcile applicationdeployment")

	var appdeploy v1alpha2.ApplicationDeployment
	if err := r.Get(ctx, req.NamespacedName, &appdeploy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("applicationdeployment is deleted")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info("Get the applicationdeployment", "apiVersion", appdeploy.APIVersion, "kind", appdeploy.Kind)

	//TODO add reconcile logic here

	return ctrl.Result{}, nil
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ApplicationDeployment")).
		WithAnnotations("controller", "ApplicationDeployment")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.ApplicationDeployment{}).
		Owns(&corev1alpha2.ApplicationConfiguration{}).
		Complete(r)
}

// Setup adds a controller that reconciles ApplicationDeployment.
func Setup(mgr ctrl.Manager) error {
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		log:    ctrl.Log.WithName("ApplicationDeployment"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
