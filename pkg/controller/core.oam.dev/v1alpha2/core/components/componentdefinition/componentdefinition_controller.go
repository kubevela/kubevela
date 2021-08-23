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

package componentdefinition

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a ComponentDefinition object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	record               event.Recorder
	defRevLimit          int
	concurrentReconciles int
}

// Reconcile is the main logic for ComponentDefinition controller
func (r *Reconciler) Reconcile(_ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx := common2.NewReconcileContext(_ctx, req.NamespacedName)
	ctx.BeginReconcile()
	defer ctx.EndReconcile()

	var componentDefinition v1beta1.ComponentDefinition
	if err := r.Get(ctx, req.NamespacedName, &componentDefinition); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if componentDefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// refresh package discover when componentDefinition is registered
	if componentDefinition.Spec.Workload.Type != types.AutoDetectWorkloadDefinition {
		err := utils.RefreshPackageDiscover(ctx, r.Client, r.dm, r.pd, &componentDefinition)
		if err != nil {
			ctx.Info("Could not discover the open api of the CRD", "err", err)
			r.record.Event(&componentDefinition, event.Warning("Could not discover the open api of the CRD", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &componentDefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}

	// generate DefinitionRevision from componentDefinition
	defRev, isNewRevision, err := coredef.GenerateDefinitionRevision(ctx, r.Client, &componentDefinition)
	if err != nil {
		ctx.Info("Could not generate DefinitionRevision", "componentDefinition", klog.KObj(&componentDefinition), "err", err)
		r.record.Event(&componentDefinition, event.Warning("Could not generate DefinitionRevision", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &componentDefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, componentDefinition.Name, err)))
	}

	if isNewRevision {
		if err := r.createComponentDefRevision(ctx, &componentDefinition, defRev.DeepCopy()); err != nil {
			ctx.Info("Could not create DefinitionRevision", "err", err)
			r.record.Event(&componentDefinition, event.Warning("cannot create DefinitionRevision", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &componentDefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrCreateDefinitionRevision, defRev.Name, err)))
		}
		ctx.Info("Successfully create definitionRevision", "definitionRevision", klog.KObj(defRev))
	}

	componentDefinition.Status.LatestRevision = &common.Revision{
		Name:         defRev.Name,
		Revision:     defRev.Spec.Revision,
		RevisionHash: defRev.Spec.RevisionHash,
	}

	if err := r.UpdateStatus(ctx, &componentDefinition); err != nil {
		ctx.Info("Could not update componentDefinition Status", "err", err)
		r.record.Event(&componentDefinition, event.Warning("cannot update ComponentDefinition Status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &componentDefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrUpdateComponentDefinition, componentDefinition.Name, err)))
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &componentDefinition, r.defRevLimit); err != nil {
		ctx.Info("Failed to collect garbage", "err", err)
		r.record.Event(&componentDefinition, event.Warning("failed to garbage collect DefinitionRevision of type ComponentDefinition", err))
	}

	def := utils.NewCapabilityComponentDef(&componentDefinition)
	// Store the parameter of componentDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, r.pd, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		ctx.Info("Could not capability in ConfigMap", "err", err)
		r.record.Event(&(componentDefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(componentDefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}

	componentDefinition.Status.ConfigMapRef = cmName
	if err := r.UpdateStatus(ctx, &componentDefinition); err != nil {
		ctx.Info("Could not update componentDefinition Status", "err", err)
		r.record.Event(&componentDefinition, event.Warning("cannot update ComponentDefinition Status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &componentDefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrUpdateComponentDefinition, componentDefinition.Name, err)))
	}
	ctx.Info("Successfully stored Capability Schema in ConfigMap", "configMap", klog.KRef(componentDefinition.Namespace, cmName))
	return ctrl.Result{}, nil
}

func (r *Reconciler) createComponentDefRevision(ctx context.Context, componentDef *v1beta1.ComponentDefinition, defRev *v1beta1.DefinitionRevision) error {
	namespace := componentDef.GetNamespace()
	defRev.SetLabels(componentDef.GetLabels())
	defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels,
		map[string]string{oam.LabelComponentDefinitionName: componentDef.Name}))
	defRev.SetNamespace(namespace)

	rev := &v1beta1.DefinitionRevision{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: defRev.Name}, rev)
	if apierrors.IsNotFound(err) {
		err = r.Create(ctx, defRev)
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}
	return err
}

// UpdateStatus updates v1beta1.ComponentDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.ComponentDefinition, opts ...client.UpdateOption) error {
	status := def.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, def); err != nil {
			return
		}
		def.Status = status
		return r.Status().Update(ctx, def, opts...)
	})
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ComponentDefinition")).
		WithAnnotations("controller", "ComponentDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.ComponentDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles ComponentDefinition.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		dm:                   args.DiscoveryMapper,
		pd:                   args.PackageDiscover,
		defRevLimit:          args.DefRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return r.SetupWithManager(mgr)
}
