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

package workloaddefinition

import (
	"context"
	"fmt"
	"time"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a WorkloadDefinition object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	Scheme *runtime.Scheme
	record event.Recorder
}

// Reconcile is the main logic for WorkloadDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	reconcileWaitResult := reconcile.Result{RequeueAfter: 120 * time.Second}
	definitionName := req.NamespacedName.Name
	if definitionName == "containerizedworkloads.core.oam.dev" {
		return ctrl.Result{}, nil
	}

	klog.Infof("Reconciling WorkloadDefinition %s...", definitionName)
	ctx := context.Background()
	var def utils.CapabilityWorkloadDefinition
	def.Name = req.NamespacedName.Name

	err := def.StoreOpenAPISchema(ctx, r, req.Namespace, req.Name)
	if err != nil {
		klog.Error(err)
		r.record.Event(&(def.WorkloadDefinition), event.Warning("cannot store capability in ConfigMap", err))
		// TODO(zzxwill) The error message should also be patched into Status
		return reconcileWaitResult, util.PatchCondition(ctx, r, &(def.WorkloadDefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}
	klog.Info("Successfully stored Capability Schema in ConfigMap")
	return ctrl.Result{}, nil
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("WorkloadDefinition")).
		WithAnnotations("controller", "WorkloadDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.WorkloadDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles WorkloadDefinition.
func Setup(mgr ctrl.Manager, _ controller.Args, _ logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	r := Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return r.SetupWithManager(mgr)
}
