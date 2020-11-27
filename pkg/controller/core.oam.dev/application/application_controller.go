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

	v1alpha22 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/builder"
	fclient "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/cache-client"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/lister"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/parser"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application/template"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kcache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// ApplicationReconciler reconciles a Application object
type applicationReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	appConfigLister lister.ApplicationConfigurationLister
	componentLister lister.ComponentLister
	reader          fclient.FastClient
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
func (r *applicationReconciler) Reconcile(req ctrl.Request) (result ctrl.Result, gerr error) {

	ctx := context.Background()
	_log := r.Log.WithValues("application", req.NamespacedName)

	app, err := r.reader.GetApplication(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	})
	if err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	if app.DeletionTimestamp != nil {
		_selector, _ := labels.Parse(fmt.Sprintf("%s=%s", builder.OamApplicationLable, app.Name))

		owns := false
		acs, err := r.appConfigLister.AppConfigs(req.Namespace).List(_selector)
		if err != nil && !kerrors.IsNotFound(err) {
			return ctrl.Result{}, errors.WithMessage(err, "list appConfigs")
		}
		for _, ac := range acs {
			owns = true
			if ac.DeletionTimestamp != nil {
				continue
			}
			if err := r.Client.Delete(ctx, ac); err != nil && !kerrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Errorf("delete ApplicationConfig: %s", ac.Name)
			}
		}

		comps, err := r.componentLister.Components(req.Namespace).List(_selector)
		if err != nil && !kerrors.IsNotFound(err) {
			return ctrl.Result{}, errors.WithMessage(err, "list componetes")
		}
		for _, comp := range comps {
			owns = true
			if comp.DeletionTimestamp != nil {
				continue
			}
			if err := r.Client.Delete(ctx, comp); err != nil && !kerrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Errorf("delete Component: %s", comp.Name)
			}
		}
		if !owns {
			removeFinalizers(app)
			return ctrl.Result{}, r.Update(ctx, app)
		}
	}

	if app.Status.Phase == "finished" {
		return ctrl.Result{}, nil
	}

	registerFinalizers(app)
	app.Status.Phase = "rendering"
	handler := &reter{r, app, _log}

	//parse template
	appParser := parser.NewParser(template.GetHanler(req.Namespace, r.reader))

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

	// build template to applicationconfig & component
	ac, comps, err := builder.Build(app.Namespace, appfile)
	if err != nil {
		app.Status.SetConditions(errorCondition("Build", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Build"))

	// apply applicationconfig & component to the cluster
	if err := handler.apply(ac, comps...); err != nil {
		app.Status.SetConditions(errorCondition("Apply", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Apply"))

	app.Status.Phase = "finished"
	return ctrl.Result{}, r.Update(ctx, app)
}

// SetupWithManager install to manager
func (r *applicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha22.Application{}).
		Owns(&core.ApplicationConfiguration{}).Owns(&core.Component{}).
		Complete(r)
}

// InjectCache setup cache
func (r *applicationReconciler) InjectCache(_cache cache.Cache) error {
	ctx := context.Background()

	r.reader = fclient.NewFastClient(_cache, r.Client)

	componentInformer, err := _cache.GetInformer(ctx, &core.Component{})
	if err != nil {
		return err
	}
	r.componentLister = lister.NewComponentLister(componentInformer.(kcache.SharedIndexInformer).GetIndexer())

	appConfigInformer, err := _cache.GetInformer(ctx, &core.ApplicationConfiguration{})
	if err != nil {
		return err
	}

	r.appConfigLister = lister.NewApplicationConfigurationLister(appConfigInformer.(kcache.SharedIndexInformer).GetIndexer())
	return nil
}

// Setup adds a controller that reconciles ApplicationDeployment.
func Setup(mgr ctrl.Manager) error {
	reconciler := applicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Application"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
