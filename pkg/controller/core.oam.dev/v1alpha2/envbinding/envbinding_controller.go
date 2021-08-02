/*
 Copyright 2021. The KubeVela Authors.

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

package envbinding

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a EnvBinding object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
}

// Reconcile is the main logic for EnvBinding controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconcile EnvBinding", "envbinding", klog.KRef(req.Namespace, req.Name))

	ctx := context.Background()
	envBinding := new(v1alpha1.EnvBinding)
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, envBinding); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// this is a placeholder for finalizer here in the future
	if envBinding.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	baseApp, err := util.RawExtension2Application(envBinding.Spec.AppTemplate.RawExtension)
	if err != nil {
		klog.ErrorS(err, "Failed to parse AppTemplate of EnvBinding")
		r.record.Event(envBinding, event.Warning("Failed to parse AppTemplate of EnvBinding", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	var engine ClusterManagerEngine
	switch envBinding.Spec.Engine {
	case v1alpha1.OCMEngine:
		engine = NewOCMEngine(r.Client, baseApp.Name, baseApp.Namespace)
	default:
		engine = NewOCMEngine(r.Client, baseApp.Name, baseApp.Namespace)
	}

	// 1. prepare the pre-work for cluster scheduling
	envBinding.Status.Phase = v1alpha1.EnvBindingPrepare
	if err = engine.Prepare(ctx, envBinding.Spec.Envs); err != nil {
		klog.ErrorS(err, "Failed to prepare the pre-work for cluster scheduling")
		r.record.Event(envBinding, event.Warning("Failed to prepare the pre-work for cluster scheduling", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// 2. patch the component parameters for application in different envs
	envBinding.Status.Phase = v1alpha1.EnvBindingRendering
	envBindApps, err := CreateEnvBindApps(envBinding, baseApp)
	if err != nil {
		klog.ErrorS(err, "Failed to patch the parameters for application in different envs")
		r.record.Event(envBinding, event.Warning("Failed to patch the parameters for application in different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// 3. render applications for different envs
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	if err = RenderEnvBindApps(ctx, envBindApps, appParser); err != nil {
		klog.ErrorS(err, "Failed to render the application for different envs")
		r.record.Event(envBinding, event.Warning("Failed to render the application for different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// 4. assemble resources for applications in different envs
	if err = AssembleEnvBindApps(envBindApps); err != nil {
		klog.ErrorS(err, "Failed to assemble resources for application in different envs")
		r.record.Event(envBinding, event.Warning("Failed to assemble resources for application in different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// 5. schedule resource of applications in different envs
	envBinding.Status.Phase = v1alpha1.EnvBindingScheduling
	if err = engine.Schedule(ctx, envBindApps); err != nil {
		klog.ErrorS(err, "Failed to schedule resource of applications in different envs")
		r.record.Event(envBinding, event.Warning("Failed to schedule resource of applications in different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// 6. store manifest of different envs to configmap
	if err = StoreManifest2ConfigMap(ctx, r, envBinding, envBindApps); err != nil {
		klog.ErrorS(err, "Failed to store manifest of different envs to configmap")
		r.record.Event(envBinding, event.Warning("Failed to store manifest of different envs to configmap", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	envBinding.Status.Phase = v1alpha1.EnvBindingFinished
	envBinding.Status.ClusterDecisions = engine.GetClusterDecisions()
	if err = r.Client.Status().Patch(ctx, envBinding, client.Merge); err != nil {
		klog.ErrorS(err, "Failed to update status")
		r.record.Event(envBinding, event.Warning("Failed to update status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, envBinding, condition.ReconcileError(err))
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, envBinding *v1alpha1.EnvBinding, cond condition.Condition) (ctrl.Result, error) {
	envBinding.SetConditions(cond)
	if err := r.Client.Status().Patch(ctx, envBinding, client.Merge); err != nil {
		return ctrl.Result{}, errors.WithMessage(err, "cannot update initializer status")
	}
	// if any condition is changed, patching status can trigger requeue the resource and we should return nil to
	// avoid requeue it again
	if util.IsConditionChanged([]condition.Condition{cond}, envBinding) {
		return ctrl.Result{}, nil
	}
	// if no condition is changed, patching status can not trigger requeue, so we must return an error to
	// requeue the resource
	return ctrl.Result{}, errors.Errorf("object level reconcile error, type: %q, msg: %q", string(cond.Type), cond.Message)
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("EnvBinding")).
		WithAnnotations("controller", "EnvBinding")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1alpha1.EnvBinding{}).
		Complete(r)
}

// Setup adds a controller that reconciles EnvBinding.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:               mgr.GetClient(),
		dm:                   args.DiscoveryMapper,
		pd:                   args.PackageDiscover,
		Scheme:               mgr.GetScheme(),
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return r.SetupWithManager(mgr)
}
