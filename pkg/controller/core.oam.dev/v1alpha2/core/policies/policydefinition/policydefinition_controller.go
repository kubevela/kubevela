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
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/version"
)

// Reconciler reconciles a PolicyDefinition object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	Scheme               *runtime.Scheme
	record               event.Recorder
	defRevLimit          int
	concurrentReconciles int
	ignoreDefNoCtrlReq   bool
	controllerVersion    string
}

// Reconcile is the main logic for PolicyDefinition controller
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling PolicyDefinition...", "Name", definitionName, "Namespace", req.Namespace)
	var policyDefinition v1beta1.PolicyDefinition
	if err := r.Get(ctx, req.NamespacedName, &policyDefinition); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// this is a placeholder for finalizer here in the future
	if policyDefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if !coredef.MatchControllerRequirement(&policyDefinition, r.controllerVersion, r.ignoreDefNoCtrlReq) {
		klog.InfoS("skip definition: not match the controller requirement of definition", "policyDefinition", klog.KObj(&policyDefinition))
		return ctrl.Result{}, nil
	}

	defRev, result, err := coredef.ReconcileDefinitionRevision(ctx, r.Client, r.record, &policyDefinition, r.defRevLimit, func(revision *common.Revision) error {
		policyDefinition.Status.LatestRevision = revision
		if err := r.UpdateStatus(ctx, &policyDefinition); err != nil {
			return err
		}
		return nil
	})
	if result != nil {
		return *result, err
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	def := utils.NewCapabilityPolicyDef(&policyDefinition)
	def.Name = req.NamespacedName.Name
	// Store the parameter of policyDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		klog.InfoS("Could not capability in ConfigMap", "err", err)
		r.record.Event(&(policyDefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(policyDefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}

	if policyDefinition.Status.ConfigMapRef != cmName {
		policyDefinition.Status.ConfigMapRef = cmName
		// Override the conditions, which maybe include the error info.
		policyDefinition.Status.Conditions = []condition.Condition{condition.ReconcileSuccess()}

		if err := r.UpdateStatus(ctx, &policyDefinition); err != nil {
			klog.InfoS("Could not update policyDefinition Status", "err", err)
			r.record.Event(&policyDefinition, event.Warning("cannot update PolicyDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &policyDefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdatePolicyDefinition, policyDefinition.Name, err)))
		}
		klog.InfoS("Successfully updated the status.configMapRef of the PolicyDefinition", "policyDefinition",
			klog.KRef(req.Namespace, req.Name), "status.configMapRef", cmName)
	}

	return ctrl.Result{}, nil
}

// UpdateStatus updates v1beta1.PolicyDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.PolicyDefinition, opts ...client.SubResourceUpdateOption) error {
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
		defRevLimit:          args.DefRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
		ignoreDefNoCtrlReq:   args.IgnoreDefinitionWithoutControllerRequirement,
		controllerVersion:    version.VelaVersion,
	}
	return r.SetupWithManager(mgr)
}
