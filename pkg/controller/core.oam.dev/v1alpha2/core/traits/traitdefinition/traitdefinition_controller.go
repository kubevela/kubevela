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
	"github.com/oam-dev/kubevela/version"
)

// Reconciler reconciles a TraitDefinition object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	pd     *packages.PackageDiscover
	Scheme *runtime.Scheme
	record event.Recorder
	options
}

type options struct {
	defRevLimit          int
	concurrentReconciles int
	ignoreDefNoCtrlReq   bool
	controllerVersion    string
}

// Reconcile is the main logic for TraitDefinition controller
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := common2.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile traitDefinition", "traitDefinition", klog.KRef(req.Namespace, req.Name))

	var traitdefinition v1beta1.TraitDefinition
	if err := r.Get(ctx, req.NamespacedName, &traitdefinition); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !r.matchControllerRequirement(&traitdefinition) {
		klog.InfoS("skip traitDefinition: not match the controller requirement of traitDefinition", "traitDefinition", klog.KObj(&traitdefinition))
		return ctrl.Result{}, nil
	}

	// this is a placeholder for finalizer here in the future
	if traitdefinition.DeletionTimestamp != nil {
		klog.InfoS("The TraitDefinition is being deleted", "traitDefinition", klog.KRef(req.Namespace, req.Name))
		return ctrl.Result{}, nil
	}

	// refresh package discover when traitDefinition is registered
	if traitdefinition.Spec.Reference.Name != "" {
		err := utils.RefreshPackageDiscover(ctx, r.Client, r.dm, r.pd, &traitdefinition)
		if err != nil {
			klog.ErrorS(err, "Could not refresh packageDiscover", "traitDefinition", klog.KRef(req.Namespace, req.Name))
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
		return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, traitdefinition.Name, err)))
	}

	if isNewRevision {
		if err := r.createTraitDefRevision(ctx, &traitdefinition, defRev); err != nil {
			klog.ErrorS(err, "Could not create DefinitionRevision")
			r.record.Event(&traitdefinition, event.Warning("Could not create definitionRevision", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrCreateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully created definitionRevision", "definitionRevision", klog.KObj(defRev))

		traitdefinition.Status.LatestRevision = &common.Revision{
			Name:         defRev.Name,
			Revision:     defRev.Spec.Revision,
			RevisionHash: defRev.Spec.RevisionHash,
		}
		if err := r.UpdateStatus(ctx, &traitdefinition); err != nil {
			klog.ErrorS(err, "Could not update TraitDefinition Status", "traitDefinition", klog.KRef(req.Namespace, req.Name))
			r.record.Event(&traitdefinition, event.Warning("Could not update TraitDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdateTraitDefinition, traitdefinition.Name, err)))
		}
		klog.InfoS("Successfully updated the status.latestRevision of the TraitDefinition", "traitDefinition", klog.KRef(req.Namespace, req.Name),
			"Name", defRev.Name, "Revision", defRev.Spec.Revision, "RevisionHash", defRev.Spec.RevisionHash)
	}

	if err := coredef.CleanUpDefinitionRevision(ctx, r.Client, &traitdefinition, r.defRevLimit); err != nil {
		klog.InfoS("Failed to collect garbage", "err", err)
		r.record.Event(&traitdefinition, event.Warning("Failed to garbage collect DefinitionRevision of type TraitDefinition", err))
	}

	def := utils.NewCapabilityTraitDef(&traitdefinition)
	def.Name = req.NamespacedName.Name
	// Store the parameter of traitDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, r.pd, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		klog.InfoS("Could not store capability in ConfigMap", "err", err)
		r.record.Event(&(traitdefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, traitdefinition.Name, err)))
	}

	if traitdefinition.Status.ConfigMapRef != cmName {
		traitdefinition.Status.ConfigMapRef = cmName
		if err := r.UpdateStatus(ctx, &traitdefinition); err != nil {
			klog.ErrorS(err, "Could not update TraitDefinition Status", "traitDefinition", klog.KRef(req.Namespace, req.Name))
			r.record.Event(&traitdefinition, event.Warning("Could not update TraitDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &traitdefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdateTraitDefinition, traitdefinition.Name, err)))
		}
		klog.InfoS("Successfully updated the status.configMapRef of the TraitDefinition", "traitDefinition",
			klog.KRef(req.Namespace, req.Name), "status.configMapRef", cmName)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) createTraitDefRevision(ctx context.Context, traitDef *v1beta1.TraitDefinition, defRev *v1beta1.DefinitionRevision) error {
	namespace := traitDef.GetNamespace()

	defRev.SetLabels(traitDef.GetLabels())
	defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels,
		map[string]string{oam.LabelTraitDefinitionName: traitDef.Name}))
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
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		dm:      args.DiscoveryMapper,
		pd:      args.PackageDiscover,
		options: parseOptions(args),
	}
	return r.SetupWithManager(mgr)
}

func parseOptions(args oamctrl.Args) options {
	return options{
		defRevLimit:          args.DefRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
		ignoreDefNoCtrlReq:   args.IgnoreDefinitionWithoutControllerRequirement,
		controllerVersion:    version.VelaVersion,
	}
}

func (r *Reconciler) matchControllerRequirement(traitDefinition *v1beta1.TraitDefinition) bool {
	if traitDefinition.Annotations != nil {
		if requireVersion, ok := traitDefinition.Annotations[oam.AnnotationControllerRequirement]; ok {
			return requireVersion == r.controllerVersion
		}
	}
	if r.ignoreDefNoCtrlReq {
		return false
	}
	return true
}
