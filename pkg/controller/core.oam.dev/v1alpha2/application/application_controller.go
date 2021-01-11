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

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/builder"
	fclient "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/defclient"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
)

// Reconciler reconciles a Application object
type Reconciler struct {
	dm discoverymapper.DiscoveryMapper
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
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

	if app.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	applog.Info("Start Rendering")

	app.Status.Phase = v1alpha2.ApplicationRendering
	handler := &reter{r.Client, app, applog}

	app.Status.Conditions = []v1alpha1.Condition{}

	applog.Info("parse template")
	// parse template
	appParser := parser.NewParser(fclient.NewDefinitionClient(r.Client, r.dm))

	appfile, err := appParser.Parse(app.Name, app)
	if err != nil {
		handler.l.Error(err, "[Handle Parse]")
		app.Status.SetConditions(errorCondition("Parsed", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Parsed"))

	applog.Info("build template")
	// build template to applicationconfig & component
	ac, comps, err := builder.Build(app.Namespace, appfile, r.Client)
	if err != nil {
		handler.l.Error(err, "[Handle Build]")
		app.Status.SetConditions(errorCondition("Built", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Built"))

	applog.Info("apply applicationconfig & component to the cluster")
	// apply applicationconfig & component to the cluster
	if err := handler.apply(ctx, ac, comps); err != nil {
		handler.l.Error(err, "[Handle apply]")
		app.Status.SetConditions(errorCondition("Applied", err))
		return handler.Err(err)
	}

	app.Status.SetConditions(readyCondition("Applied"))

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
	return ctrl.Result{}, r.Status().Update(ctx, app)
}

// SetupWithManager install to manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.Application{}).
		Owns(&v1alpha2.ApplicationConfiguration{}).Owns(&v1alpha2.Component{}).
		Complete(r)
}

// Setup adds a controller that reconciles ApplicationDeployment.
func Setup(mgr ctrl.Manager, _ core.Args, _ logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("create discovery dm fail %w", err)
	}
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Application"),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return reconciler.SetupWithManager(mgr)
}
