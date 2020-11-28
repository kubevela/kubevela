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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/builder"
	fclient "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/template"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplicationReconciler reconciles a Application object
type applicationReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
func (r *applicationReconciler) Reconcile(req ctrl.Request) (result ctrl.Result, gerr error) {

	ctx := context.Background()
	_log := r.Log.WithValues("application", req.NamespacedName)
	app:=new(v1alpha2.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	},app);err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	if app.DeletionTimestamp != nil {
		_log.Info("Handle delete")
		owns := false

		matchLabels:=client.MatchingLabels{
			builder.OamApplicationLable: app.Name,
		}

		aclist:=&v1alpha2.ApplicationConfigurationList{}
		if err:=r.List(ctx,aclist,matchLabels);err != nil && !kerrors.IsNotFound(err) {
			return ctrl.Result{}, errors.WithMessage(err, "list appConfigs")
		}
		for _, ac := range aclist.Items {
			owns = true
			if ac.DeletionTimestamp != nil {
				continue
			}
			if err := r.Client.Delete(ctx, &ac); err != nil && !kerrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Errorf("delete ApplicationConfig: %s", ac.Name)
			}
		}

		compList:=&v1alpha2.ComponentList{}
		if err:=r.List(ctx,compList,client.MatchingLabels{
			builder.OamApplicationLable: app.Name,
		});err != nil && !kerrors.IsNotFound(err) {
			return ctrl.Result{}, errors.WithMessage(err, "list componetes")
		}
		for _, comp := range compList.Items {
			owns = true
			if comp.DeletionTimestamp != nil {
				continue
			}
			if err := r.Client.Delete(ctx, &comp); err != nil && !kerrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Errorf("delete Component: %s", comp.Name)
			}
		}
		if !owns {
			_log.Info("Remove finalizer")
			removeFinalizers(app)
			return ctrl.Result{}, r.Update(ctx, app)
		}
	}

	if app.Status.Phase == "finished" {
		return ctrl.Result{}, nil
	}

	_log.Info("Start Rendering")
	registerFinalizers(app)
	app.Status.Phase = "rendering"
	handler := &reter{r, app, _log}

	_log.Info("Parse Template")
	//parse template
	appParser := parser.NewParser(template.GetHanler(fclient.NewDefinitionClient(r.Client)))

	expr, err := app.Spec.Maps()
	if err != nil {
		app.Status.SetConditions(errorCondition("Parser", err))
		return handler.Err(err)
	}

	appfile, err := appParser.Parse(app.Name, expr)

	if err != nil {
		app.Status.SetConditions(errorCondition("Parser", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Parser"))

	_log.Info("Build Template")
	// build template to applicationconfig & component
	ac, comps, err := builder.Build(app.Namespace, appfile)
	if err != nil {
		app.Status.SetConditions(errorCondition("Build", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Build"))

	_log.Info("Apply Output RC")
	// apply applicationconfig & component to the cluster
	if err := handler.apply(ac, comps...); err != nil {
		app.Status.SetConditions(errorCondition("Apply", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Apply"))

	app.Status.Phase = "finished"
	_log.Info("Finish Rendering")
	return ctrl.Result{}, r.Status().Update(ctx, app)
}

// SetupWithManager install to manager
func (r *applicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.Application{}).
		Owns(&v1alpha2.ApplicationConfiguration{}).Owns(&v1alpha2.Component{}).
		Complete(r)
}


// Setup adds a controller that reconciles ApplicationDeployment.
func Setup(mgr ctrl.Manager,_ core.Args, _ logging.Logger) error {
	reconciler := applicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Application"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
