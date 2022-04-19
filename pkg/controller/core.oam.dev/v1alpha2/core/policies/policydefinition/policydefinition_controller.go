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
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
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
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx, cancel := common2.NewReconcileContext(ctx)
	defer cancel()

	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling PolicyDefinition...", "Name", definitionName, "Namespace", req.Namespace)
	var policydefinition v1beta1.PolicyDefinition
	if err := r.Get(ctx, req.NamespacedName, &policydefinition); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
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
				condition.ReconcileError(fmt.Errorf(util.ErrRefreshPackageDiscover, err)))
		}
	}

	// generate DefinitionRevision from policyDefinition
	defRev, isNewRevision, err := coredef.GenerateDefinitionRevision(ctx, r.Client, &policydefinition)
	if err != nil {
		klog.ErrorS(err, "cannot generate DefinitionRevision", "PolicyDefinitionName", policydefinition.Name)
		r.record.Event(&policydefinition, event.Warning("cannot generate DefinitionRevision", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &policydefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, policydefinition.Name, err)))
	}

	if isNewRevision {
		if err = r.createPolicyDefRevision(ctx, &policydefinition, defRev); err != nil {
			klog.ErrorS(err, "cannot create DefinitionRevision")
			r.record.Event(&(policydefinition), event.Warning("cannot create DefinitionRevision", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &(policydefinition),
				condition.ReconcileError(fmt.Errorf(util.ErrCreateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully created PolicyDefRevision", "name", defRev.Name)

		policydefinition.Status.LatestRevision = &common.Revision{
			Name:         defRev.Name,
			Revision:     defRev.Spec.Revision,
			RevisionHash: defRev.Spec.RevisionHash,
		}

		if err := r.UpdateStatus(ctx, &policydefinition); err != nil {
			klog.ErrorS(err, "cannot update PolicyDefinition Status")
			r.record.Event(&(policydefinition), event.Warning("cannot update PolicyDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &(policydefinition),
				condition.ReconcileError(fmt.Errorf(util.ErrUpdatePolicyDefinition, policydefinition.Name, err)))
		}

		klog.InfoS("Successfully updated the status.latestRevision of the PolicyDefinition", "policyDefinition", klog.KRef(req.Namespace, req.Name),
			"Name", defRev.Name, "Revision", defRev.Spec.Revision, "RevisionHash", defRev.Spec.RevisionHash)
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &policydefinition, r.defRevLimit); err != nil {
		klog.Error("[Garbage collection]")
		r.record.Event(&policydefinition, event.Warning("failed to garbage collect DefinitionRevision of type PolicyDefinition", err))
	}

	def := utils.NewCapabilityPolicyDef(&policydefinition)
	// Store the parameter of policyDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, r.pd, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		klog.InfoS("Could not capability in ConfigMap", "err", err)
		r.record.Event(&(policydefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(policydefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}

	if policydefinition.Status.ConfigMapRef != cmName {
		policydefinition.Status.ConfigMapRef = cmName
		if err := r.UpdateStatus(ctx, &policydefinition); err != nil {
			klog.InfoS("Could not update policyDefinition Status", "err", err)
			r.record.Event(&policydefinition, event.Warning("cannot update PolicyDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &policydefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdatePolicyDefinition, policydefinition.Name, err)))
		}
		klog.InfoS("Successfully updated the status.configMapRef of the PolicyDefinition", "policyDefinition",
			klog.KRef(req.Namespace, req.Name), "status.configMapRef", cmName)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) createPolicyDefRevision(ctx context.Context, def *v1beta1.PolicyDefinition, defRev *v1beta1.DefinitionRevision) error {
	namespace := def.GetNamespace()
	defRev.SetLabels(def.GetLabels())
	defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels,
		map[string]string{oam.LabelPolicyDefinitionName: def.Name}))
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
