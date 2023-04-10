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

// Reconciler reconciles a ComponentDefinition object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
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

// Reconcile is the main logic for ComponentDefinition controller
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile componentDefinition", "componentDefinition", klog.KRef(req.Namespace, req.Name))

	var componentDefinition v1beta1.ComponentDefinition
	if err := r.Get(ctx, req.NamespacedName, &componentDefinition); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !coredef.MatchControllerRequirement(&componentDefinition, r.controllerVersion, r.ignoreDefNoCtrlReq) {
		klog.InfoS("skip definition: not match the controller requirement of definition", "componentDefinition", klog.KObj(&componentDefinition))
		return ctrl.Result{}, nil
	}

	defRev, result, err := coredef.ReconcileDefinitionRevision(ctx, r.Client, r.record, &componentDefinition, r.defRevLimit, func(revision *common.Revision) error {
		componentDefinition.Status.LatestRevision = revision
		if err := r.UpdateStatus(ctx, &componentDefinition); err != nil {
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

	def := utils.NewCapabilityComponentDef(&componentDefinition)
	// Store the parameter of componentDefinition to configMap
	cmName, err := def.StoreOpenAPISchema(ctx, r.Client, req.Namespace, req.Name, defRev.Name)
	if err != nil {
		klog.InfoS("Could not store capability in ConfigMap", "err", err)
		r.record.Event(&(componentDefinition), event.Warning("Could not store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(componentDefinition),
			condition.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}
	if componentDefinition.Status.ConfigMapRef != cmName {
		componentDefinition.Status.ConfigMapRef = cmName
		// Override the conditions, which maybe include the error info.
		componentDefinition.Status.Conditions = []condition.Condition{condition.ReconcileSuccess()}

		if err := r.UpdateStatus(ctx, &componentDefinition); err != nil {
			klog.InfoS("Could not update componentDefinition Status", "err", err)
			r.record.Event(&componentDefinition, event.Warning("cannot update ComponentDefinition Status", err))
			return ctrl.Result{}, util.PatchCondition(ctx, r, &componentDefinition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdateComponentDefinition, componentDefinition.Name, err)))
		}
		klog.InfoS("Successfully updated the status.configMapRef of the ComponentDefinition", "componentDefinition",
			klog.KRef(req.Namespace, req.Name), "status.configMapRef", cmName)
	}
	return ctrl.Result{}, nil
}

// UpdateStatus updates v1beta1.ComponentDefinition's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.ComponentDefinition, opts ...client.SubResourceUpdateOption) error {
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
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		dm:      args.DiscoveryMapper,
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
