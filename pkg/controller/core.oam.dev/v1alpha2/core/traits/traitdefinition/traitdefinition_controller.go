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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a TraitDefinition object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	record               event.Recorder
	defRevLimit          int
	concurrentReconciles int
}

// Reconcile is the main logic for TraitDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconcile traitDefinition", "traitDefinition", klog.KRef(req.Namespace, req.Name))
	ctx := context.Background()

	var traitdefinition v1beta1.TraitDefinition
	if err := r.Get(ctx, req.NamespacedName, &traitdefinition); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if traitdefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// refresh package discover when traitDefinition is registered
	if traitdefinition.Spec.Reference.Name != "" {
		err := utils.RefreshPackageDiscover(ctx, r.Client, r.dm, r.pd, &traitdefinition)
		if err != nil {
			klog.InfoS("Could not refresh packageDiscover", "err", err)
			r.record.Event(&traitdefinition, event.Warning("cannot refresh packageDiscover", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &traitdefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}

	// generate DefinitionRevision from traitDefinition
	defRev, isNewRevision, err := coredef.GenerateDefinitionRevision(ctx, r.Client, &traitdefinition)
	if err != nil {
		klog.InfoS("Could not generate definitionRevision", "traitDefinition", klog.KObj(&traitdefinition), "err", err)
		r.record.Event(&traitdefinition, event.Warning("Could not generate DefinitionRevision", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &traitdefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, traitdefinition.Name, err)))
	}
	if !isNewRevision {
		if err = r.createOrUpdateTraitDefRevision(ctx, req.Namespace, &traitdefinition, defRev); err != nil {
			klog.InfoS("Could not update DefinitionRevision", "err", err)
			r.record.Event(&(traitdefinition), event.Warning("cannot update DefinitionRevision", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(traitdefinition),
				condition.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully update definitionRevision", "definitionRevision", klog.KObj(defRev))

		if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &traitdefinition, r.defRevLimit); err != nil {
			klog.InfoS("Failed to collect garbage", "err", err)
			r.record.Event(&traitdefinition, event.Warning("failed to garbage collect DefinitionRevision of type TraitDefinition", err))
		}
		return ctrl.Result{}, nil
	}

	def := utils.NewCapabilityTraitDef(&traitdefinition)
	def.Name = req.NamespacedName.Name

	// Store the parameter of traitDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, r.pd, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		klog.InfoS("Could not store capability in ConfigMap", "err", err)
		r.record.Event(&(traitdefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &traitdefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, traitdefinition.Name, err)))
	}
	traitdefinition.Status.ConfigMapRef = cmName
	klog.Info("Successfully stored Capability Schema in ConfigMap")

	if err = r.createOrUpdateTraitDefRevision(ctx, req.Namespace, &traitdefinition, defRev); err != nil {
		klog.InfoS("Could not create DefinitionRevision", "err", err)
		r.record.Event(&(traitdefinition), event.Warning("Could not create definitionRevision", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(traitdefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
	}
	klog.InfoS("Successfully create definitionRevision", "definitionRevision", klog.KObj(defRev))

	traitdefinition.Status.LatestRevision = &common.Revision{
		Name:         defRev.Name,
		Revision:     defRev.Spec.Revision,
		RevisionHash: defRev.Spec.RevisionHash,
	}

	if err := r.UpdateStatus(ctx, &traitdefinition); err != nil {
		klog.InfoS("Could not update TraitDefinition Status", "err", err)
		r.record.Event(&(traitdefinition), event.Warning("Could not update TraitDefinition Status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(traitdefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrUpdateTraitDefinition, traitdefinition.Name, err)))
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &traitdefinition, r.defRevLimit); err != nil {
		klog.InfoS("Failed to collect garbage", "err", err)
		r.record.Event(&traitdefinition, event.Warning("Failed to garbage collect DefinitionRevision of type TraitDefinition", err))
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) createOrUpdateTraitDefRevision(ctx context.Context, namespace string,
	traitDef *v1beta1.TraitDefinition, defRev *v1beta1.DefinitionRevision) error {

	ownerReference := []metav1.OwnerReference{{
		APIVersion:         traitDef.APIVersion,
		Kind:               traitDef.Kind,
		Name:               traitDef.Name,
		UID:                traitDef.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	defRev.SetLabels(traitDef.GetLabels())
	defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels,
		map[string]string{oam.LabelTraitDefinitionName: traitDef.Name}))
	defRev.SetNamespace(namespace)
	defRev.SetAnnotations(traitDef.GetAnnotations())
	defRev.SetOwnerReferences(ownerReference)

	rev := &v1beta1.DefinitionRevision{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: defRev.Name}, rev); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, defRev)
		}
		return err
	}

	rev.SetAnnotations(defRev.GetAnnotations())
	rev.SetLabels(defRev.GetLabels())
	rev.SetOwnerReferences(ownerReference)
	return r.Update(ctx, rev)
}

// UpdateStatus updates v1beta1.TraitDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.TraitDefinition, opts ...client.UpdateOption) error {
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
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("TraitDefinition")).
		WithAnnotations("controller", "TraitDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.TraitDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles TraitDefinition.
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
