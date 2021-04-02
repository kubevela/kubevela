/*

 Copyright 2021 The KubeVela Authors.

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

package traitdefinition

import (
	"context"
	"fmt"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a TraitDefinition object
type Reconciler struct {
	client.Client
	dm discoverymapper.DiscoveryMapper
	// TODO support package discover refresh in definition
	pd     *definition.PackageDiscover
	Scheme *runtime.Scheme
	record event.Recorder
}

// Reconcile is the main logic for TraitDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling TraitDefinition...", "Name", definitionName, "Namespace", req.Namespace)
	ctx := context.Background()

	var traitdefinition v1alpha2.TraitDefinition
	if err := r.Get(ctx, req.NamespacedName, &traitdefinition); err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if traitdefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if traitdefinition.Spec.Reference.Name != "" {
		err := utils.RefreshPackageDiscover(r.dm, r.pd, common.WorkloadGVK{},
			traitdefinition.Spec.Reference, types.TypeTrait)
		if err != nil {
			klog.ErrorS(err, "cannot refresh packageDiscover")
			r.record.Event(&traitdefinition, event.Warning("cannot refresh packageDiscover", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
				cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}

	var def utils.CapabilityTraitDefinition
	def.Name = req.NamespacedName.Name

	err := def.StoreOpenAPISchema(ctx, r.Client, r.pd, req.Namespace, req.Name)
	if err != nil {
		klog.ErrorS(err, "cannot store capability in ConfigMap")
		r.record.Event(&(def.TraitDefinition), event.Warning("cannot store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &def.TraitDefinition,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}

	if err := r.Status().Update(ctx, &def.TraitDefinition); err != nil {
		klog.ErrorS(err, "cannot update traitDefinition ConfigMapRef Field")
		r.record.Event(&(def.TraitDefinition), event.Warning("cannot update traitDefinition ConfigMapRef Field", err))
		return ctrl.Result{}, err
	}
	klog.Info("Successfully stored Capability Schema in ConfigMap")
	return ctrl.Result{}, nil
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("TraitDefinition")).
		WithAnnotations("controller", "TraitDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.TraitDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles TraitDefinition.
func Setup(mgr ctrl.Manager, args controller.Args, _ logging.Logger) error {
	r := Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		dm:     args.DiscoveryMapper,
		pd:     args.PackageDiscover,
	}
	return r.SetupWithManager(mgr)
}
