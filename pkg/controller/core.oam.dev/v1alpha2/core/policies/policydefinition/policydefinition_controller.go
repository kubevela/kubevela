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

package policydefinition

import (
	"context"
	"fmt"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a PolicyDefinition object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	record               event.Recorder
	defRevLimit          int
	concurrentReconciles int
}

// Reconcile is the main logic for PolicyDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling PolicyDefinition...", "Name", definitionName, "Namespace", req.Namespace)
	ctx := context.Background()

	var policydefinition v1beta1.PolicyDefinition
	if err := r.Get(ctx, req.NamespacedName, &policydefinition); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if policydefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// refresh package discover when policyDefinition is registered
	if policydefinition.Spec.Reference.Name != "" {
		err := utils.RefreshPackageDiscover(ctx, r.Client, r.dm, r.pd, &policydefinition)
		if err != nil {
			klog.ErrorS(err, "cannot refresh packageDiscover")
			r.record.Event(&policydefinition, event.Warning("cannot refresh packageDiscover", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &policydefinition,
				cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}

	// generate DefinitionRevision from policyDefinition
	defRev, isNewRevision, err := coredef.GenerateDefinitionRevision(ctx, r.Client, &policydefinition)
	if err != nil {
		klog.ErrorS(err, "cannot generate DefinitionRevision", "PolicyDefinitionName", policydefinition.Name)
		r.record.Event(&policydefinition, event.Warning("cannot generate DefinitionRevision", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &policydefinition,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, policydefinition.Name, err)))
	}
	if !isNewRevision {
		if err = r.createOrUpdatePolicyDefRevision(ctx, req.Namespace, &policydefinition, defRev); err != nil {
			klog.ErrorS(err, "cannot update DefinitionRevision")
			r.record.Event(&(policydefinition), event.Warning("cannot update DefinitionRevision", err))
			return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(policydefinition),
				cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully update DefinitionRevision", "name", defRev.Name)

		if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &policydefinition, r.defRevLimit); err != nil {
			klog.Error("[Garbage collection]")
			r.record.Event(&policydefinition, event.Warning("failed to garbage collect DefinitionRevision of type PolicyDefinition", err))
		}
		return ctrl.Result{}, nil
	}

	if err = r.createOrUpdatePolicyDefRevision(ctx, req.Namespace, &policydefinition, defRev); err != nil {
		klog.ErrorS(err, "cannot create DefinitionRevision")
		r.record.Event(&(policydefinition), event.Warning("cannot create DefinitionRevision", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(policydefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
	}
	klog.InfoS("Successfully createOrUpdatePolicyDefRevision", "name", defRev.Name)

	policydefinition.Status.LatestRevision = &common.Revision{
		Name:         defRev.Name,
		Revision:     defRev.Spec.Revision,
		RevisionHash: defRev.Spec.RevisionHash,
	}

	if err := r.UpdateStatus(ctx, &policydefinition); err != nil {
		klog.ErrorS(err, "cannot update PolicyDefinition Status")
		r.record.Event(&(policydefinition), event.Warning("cannot update PolicyDefinition Status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &(policydefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrUpdatePolicyDefinition, policydefinition.Name, err)))
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &policydefinition, r.defRevLimit); err != nil {
		klog.Error("[Garbage collection]")
		r.record.Event(&policydefinition, event.Warning("failed to garbage collect DefinitionRevision of type PolicyDefinition", err))
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) createOrUpdatePolicyDefRevision(ctx context.Context, ns string,
	def *v1beta1.PolicyDefinition, defRev *v1beta1.DefinitionRevision) error {

	ownerReference := []metav1.OwnerReference{{
		APIVersion:         def.APIVersion,
		Kind:               def.Kind,
		Name:               def.Name,
		UID:                def.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	defRev.SetLabels(def.GetLabels())
	defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels,
		map[string]string{oam.LabelPolicyDefinitionName: def.Name}))
	defRev.SetNamespace(ns)
	defRev.SetAnnotations(def.GetAnnotations())
	defRev.SetOwnerReferences(ownerReference)

	rev := &v1beta1.DefinitionRevision{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: defRev.Name}, rev); err != nil {
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

// UpdateStatus updates v1beta1.PolicyDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.PolicyDefinition, opts ...client.UpdateOption) error {
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
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("PolicyDefinition")).
		WithAnnotations("controller", "PolicyDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.PolicyDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles PolicyDefinition.
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
