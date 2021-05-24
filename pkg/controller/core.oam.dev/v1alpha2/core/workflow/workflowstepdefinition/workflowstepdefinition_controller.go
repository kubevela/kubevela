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

package workflowstepdefinition

import (
	"context"
	"fmt"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a WorkflowStepDefinition object
type Reconciler struct {
	client.Client
	dm          discoverymapper.DiscoveryMapper
	pd          *definition.PackageDiscover
	Scheme      *runtime.Scheme
	record      event.Recorder
	defRevLimit int
}

// Reconcile is the main logic for WorkflowStepDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling WorkflowStepDefinition...", "Name", definitionName, "Namespace", req.Namespace)
	ctx := context.Background()

	var wfstepdefinition v1beta1.WorkflowStepDefinition
	if err := r.Get(ctx, req.NamespacedName, &wfstepdefinition); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if wfstepdefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// refresh package discover when WorkflowStepDefinition is registered
	if wfstepdefinition.Spec.Reference.Name != "" {
		err := utils.RefreshPackageDiscover(ctx, r.Client, r.dm, r.pd, &wfstepdefinition)
		if err != nil {
			klog.ErrorS(err, "cannot refresh packageDiscover")
			r.record.Event(&wfstepdefinition, event.Warning("cannot refresh packageDiscover", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &wfstepdefinition,
				cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}
	// generate DefinitionRevision from WorkflowStepDefinition
	defRev, isNewRevision, err := coredef.GenerateDefinitionRevision(ctx, r.Client, &wfstepdefinition)
	if err != nil {
		klog.ErrorS(err, "cannot generate DefinitionRevision", "WorkflowStepDefinitionName", wfstepdefinition.Name)
		r.record.Event(&wfstepdefinition, event.Warning("cannot generate DefinitionRevision", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &wfstepdefinition,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, wfstepdefinition.Name, err)))
	}
	if !isNewRevision {
		if err = r.createOrUpdateWFStepDefRevision(ctx, req.Namespace, &wfstepdefinition, defRev); err != nil {
			klog.ErrorS(err, "cannot update DefinitionRevision")
			r.record.Event(&(wfstepdefinition), event.Warning("cannot update DefinitionRevision", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &(wfstepdefinition),
				cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully update DefinitionRevision", "name", defRev.Name)

		if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &wfstepdefinition, r.defRevLimit); err != nil {
			klog.Error("[Garbage collection]")
			r.record.Event(&wfstepdefinition, event.Warning("failed to garbage collect DefinitionRevision of type WorkflowStepDefinition", err))
		}
		return ctrl.Result{}, nil
	}

	if err = r.createOrUpdateWFStepDefRevision(ctx, req.Namespace, &wfstepdefinition, defRev); err != nil {
		klog.ErrorS(err, "cannot create DefinitionRevision")
		r.record.Event(&(wfstepdefinition), event.Warning("cannot create DefinitionRevision", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(wfstepdefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrCreateOrUpdateDefinitionRevision, defRev.Name, err)))
	}
	klog.InfoS("Successfully create DefinitionRevision", "name", defRev.Name)

	wfstepdefinition.Status.LatestRevision = &common.Revision{
		Name:         defRev.Name,
		Revision:     defRev.Spec.Revision,
		RevisionHash: defRev.Spec.RevisionHash,
	}

	if err := r.UpdateStatus(ctx, &wfstepdefinition); err != nil {
		klog.ErrorS(err, "cannot update WorkflowStepDefinition Status")
		r.record.Event(&(wfstepdefinition), event.Warning("cannot update WorkflowStepDefinition Status", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(wfstepdefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrUpdateWorkflowStepDefinition, wfstepdefinition.Name, err)))
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &wfstepdefinition, r.defRevLimit); err != nil {
		klog.Error("[Garbage collection]")
		r.record.Event(&wfstepdefinition, event.Warning("failed to garbage collect DefinitionRevision of type WorkflowStepDefinition", err))
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) createOrUpdateWFStepDefRevision(ctx context.Context, ns string,
	def *v1beta1.WorkflowStepDefinition, defRev *v1beta1.DefinitionRevision) error {

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
		map[string]string{oam.LabelWorkflowStepDefinitionName: def.Name}))
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

// UpdateStatus updates v1beta1.WorkflowStepDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.WorkflowStepDefinition, opts ...client.UpdateOption) error {
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
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("WorkflowStepDefinition")).
		WithAnnotations("controller", "WorkflowStepDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.WorkflowStepDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles WorkflowStepDefinition.
func Setup(mgr ctrl.Manager, args controller.Args, _ logging.Logger) error {
	r := Reconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		dm:          args.DiscoveryMapper,
		pd:          args.PackageDiscover,
		defRevLimit: args.DefRevisionLimit,
	}
	return r.SetupWithManager(mgr)
}
