package applicationcontext

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	ac "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconcile error strings.
const (
	errGetAppContex           = "cannot get application context"
	errGetAppRevision         = "cannot get the application revision the context refers to"
	errUpdateAppContextStatus = "cannot update application context status"
)

const reconcileTimeout = 1 * time.Minute

// Reconciler reconciles an Application Context by constructing an in-memory
// application configuration and reuse its reconcile logic
type Reconciler struct {
	client    client.Client
	log       logging.Logger
	record    event.Recorder
	mgr       ctrl.Manager
	applyMode core.ApplyOnceOnlyMode
}

// Reconcile reconcile an application context
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.log.Debug("Reconciling")
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	// fetch the app context
	appContext := &v1alpha2.ApplicationContext{}
	if err := r.client.Get(ctx, request.NamespacedName, appContext); err != nil {
		if apierrors.IsNotFound(err) {
			// stop processing this resource
			return ctrl.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrap(err, errGetAppContex)
	}

	ctx = util.SetNamespaceInCtx(ctx, appContext.Namespace)
	dm, err := discoverymapper.New(r.mgr.GetConfig())
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("create discovery dm fail %w", err)
	}
	// fetch the appRevision it points to
	appRevision := &v1alpha2.ApplicationRevision{}
	key := types.NamespacedName{Namespace: appContext.Namespace, Name: appContext.Spec.ApplicationRevisionName}
	if err := r.client.Get(ctx, key, appRevision); err != nil {
		if apierrors.IsNotFound(err) {
			// stop processing this resource
			return ctrl.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrap(err, errGetAppRevision)
	}

	// copy the status from appContext to appConfig
	appConfig, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
	if err != nil {
		return reconcile.Result{}, err
	}
	appConfig.Status = appContext.Status
	// the name of the appConfig has to be the same as the appContext
	appConfig.Name = appContext.Name
	appConfig.UID = appContext.UID
	appConfig.SetLabels(appContext.GetLabels())
	appConfig.SetAnnotations(appContext.GetAnnotations())
	// makes sure that the appConfig's owner is the same as the appContext
	appConfig.SetOwnerReferences(appContext.GetOwnerReferences())
	// call into the old ac Reconciler and copy the status back
	acReconciler := ac.NewReconciler(r.mgr, dm, r.log, ac.WithRecorder(r.record), ac.WithApplyOnceOnlyMode(r.applyMode))
	reconResult := acReconciler.ACReconcile(ctx, appConfig, r.log)
	appContext.Status = appConfig.Status
	// always update ac status and set the error
	err = errors.Wrap(r.client.Status().Update(ctx, appContext), errUpdateAppContextStatus)
	// use the controller build-in backoff mechanism if an error occurs
	if err != nil {
		reconResult.RequeueAfter = 0
	} else if appContext.Status.RollingStatus == v1alpha2.RollingTemplated {
		// makes sure that we can will reconcile shortly after the annotation is removed
		reconResult.RequeueAfter = time.Second * 5
	}
	return reconResult, err
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, compHandler *ac.ComponentHandler) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
		WithAnnotations("controller", "AppRollout")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.ApplicationContext{}).
		Watches(&source.Kind{Type: &v1alpha2.Component{}}, compHandler).
		Complete(r)
}

// Setup adds a controller that reconciles ApplicationContext
func Setup(mgr ctrl.Manager, args core.Args, l logging.Logger) error {
	name := "oam/" + strings.ToLower(v1alpha2.ApplicationContextGroupKind)
	record := event.NewAPIRecorder(mgr.GetEventRecorderFor(name))
	reconciler := Reconciler{
		client:    mgr.GetClient(),
		mgr:       mgr,
		log:       l.WithValues("controller", name),
		record:    record,
		applyMode: args.ApplyMode,
	}
	compHandler := &ac.ComponentHandler{
		Client:                mgr.GetClient(),
		Logger:                l,
		RevisionLimit:         args.RevisionLimit,
		CustomRevisionHookURL: args.CustomRevisionHookURL,
	}
	return reconciler.SetupWithManager(mgr, compHandler)
}
