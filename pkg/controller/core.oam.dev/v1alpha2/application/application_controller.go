/*


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
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	apply "github.com/oam-dev/kubevela/pkg/utils/apply"
)

// RolloutReconcileWaitTime is the time to wait before reconcile again an application still in rollout phase
const RolloutReconcileWaitTime = time.Second * 3

// Reconciler reconciles a Application object
type Reconciler struct {
	client.Client
	dm         discoverymapper.DiscoveryMapper
	Log        logr.Logger
	Scheme     *runtime.Scheme
	applicator apply.Applicator
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	applog := r.Log.WithValues("application", req.NamespacedName)
	app := new(v1alpha2.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// TODO: check finalizer
	if app.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	applog.Info("Start Rendering")

	app.Status.Phase = v1alpha2.ApplicationRendering
	handler := &appHandler{r, app, applog}

	applog.Info("parse template")
	// parse template
	appParser := appfile.NewApplicationParser(r.Client, r.dm)

	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	appfile, err := appParser.GenerateAppFile(ctx, app.Name, app)
	if err != nil {
		applog.Error(err, "[Handle Parse]")
		app.Status.SetConditions(errorCondition("Parsed", err))
		return handler.handleErr(err)
	}

	app.Status.SetConditions(readyCondition("Parsed"))

	applog.Info("build template")
	// build template to applicationconfig & component
	ac, comps, err := appParser.GenerateApplicationConfiguration(appfile, app.Namespace)
	if err != nil {
		applog.Error(err, "[Handle GenerateApplicationConfiguration]")
		app.Status.SetConditions(errorCondition("Built", err))
		return handler.handleErr(err)
	}
	// pass the App label and annotation to ac except some app specific ones
	oamutil.PassLabelAndAnnotation(app, ac)
	app.Status.SetConditions(readyCondition("Built"))
	applog.Info("apply appConfig & component to the cluster")
	// apply appConfig & component to the cluster
	if err := handler.apply(ctx, ac, comps); err != nil {
		applog.Error(err, "[Handle apply]")
		app.Status.SetConditions(errorCondition("Applied", err))
		return handler.handleErr(err)
	}

	app.Status.SetConditions(readyCondition("Applied"))
	app.Status.Phase = v1alpha2.ApplicationHealthChecking
	applog.Info("check application health status")
	// check application health status
	appCompStatus, healthy, err := handler.statusAggregate(appfile)
	if err != nil {
		applog.Error(err, "[status aggregate]")
		app.Status.SetConditions(errorCondition("HealthCheck", err))
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
	app.Status.Phase = v1alpha2.ApplicationRunning
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
	return ctrl.Result{}, r.UpdateStatus(ctx, app)
}

// SetupWithManager install to manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// If Application Own these two child objects, AC status change will notify application controller and recursively update AC again, and trigger application event again...
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.Application{}).
		Complete(r)
}

// UpdateStatus updates v1alpha2.Application's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, app *v1alpha2.Application, opts ...client.UpdateOption) error {
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
func Setup(mgr ctrl.Manager, _ core.Args, _ logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("create discovery dm fail %w", err)
	}
	reconciler := Reconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("Application"),
		Scheme:     mgr.GetScheme(),
		dm:         dm,
		applicator: apply.NewAPIApplicator(mgr.GetClient()),
	}
	return reconciler.SetupWithManager(mgr)
}
