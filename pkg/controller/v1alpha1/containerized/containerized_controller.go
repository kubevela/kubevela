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

package containerized

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/api/v1alpha1"
)

// ContainerizedReconciler reconciles a Containerized object
type ContainerizedReconciler struct {
	client.Client
	Log    logr.Logger
	record event.Recorder
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=standard.oam.dev,resources=containerizeds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=containerizeds/status,verbs=get;update;patch

func (r *ContainerizedReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("containerized", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *ContainerizedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Containerized")).
		WithAnnotations("controller", "Containerized")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Containerized{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// Setup adds a controller that reconciles MetricsTrait.
func Setup(mgr ctrl.Manager) error {
	reconciler := ContainerizedReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Containerized"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
