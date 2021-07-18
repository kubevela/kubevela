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

package initializer

import (
	"context"
	"fmt"
	"time"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// InitializerReconcileWaitTime is the time to wait before reconcile again
const InitializerReconcileWaitTime = time.Second * 5

// Reconciler reconciles a Initializer object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
}

// Reconcile is the main logic for Initializer controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconcile initializer", "initializer", klog.KRef(req.Namespace, req.Name))
	ctx := context.Background()

	init := new(v1beta1.Initializer)
	if err := r.Client.Get(ctx, req.NamespacedName, init); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO(yangsoon) this is a placeholder for finalizer here
	if init.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	klog.Info("Check the status of the Initializers which you depend on")
	init.Status.Phase = v1beta1.InitializerCheckingDependsOn
	dependsOnInitReady, err := r.checkOrInstallDependsOn(ctx, init.Spec.DependsOn)
	if err != nil {
		klog.ErrorS(err, "Initializers which you depend on are not ready")
		r.record.Event(init, event.Warning("Initializers which you depend on are not ready", err))
		return r.endWithNegativeCondition(ctx, init, cpv1alpha1.ReconcileError(err))
	}
	if !dependsOnInitReady {
		klog.Info("Wait for dependent Initializer to be ready")
		return reconcile.Result{RequeueAfter: InitializerReconcileWaitTime}, nil
	}

	init.Status.Phase = v1beta1.InitializerInitializing
	appReady, err := r.applyResources(ctx, init)
	if err != nil {
		klog.ErrorS(err, "Could not create resources via application to initialize the env")
		r.record.Event(init, event.Warning("Could not create resources via application", err))
		return r.endWithNegativeCondition(ctx, init, cpv1alpha1.ReconcileError(err))
	}
	if !appReady {
		klog.Info("Wait for the Application created by Initializer to be ready")
		return reconcile.Result{RequeueAfter: InitializerReconcileWaitTime}, nil
	}

	init.Status.Phase = v1beta1.InitializerSuccess
	if err = r.patchStatus(ctx, init); err != nil {
		klog.ErrorS(err, "Could not update status")
		r.record.Event(init, event.Warning("Could not update status", err))
		return ctrl.Result{}, oamutil.EndReconcileWithNegativeCondition(ctx, r, init, cpv1alpha1.ReconcileError(err))
	}
	return ctrl.Result{}, nil
}

// checkOrInstallDependsOn check the status of dependOn Initializer or help install the build-in Initializer.
// If the dependOn Initializer is not found and the namespace is default or vela-system, we will try to find
// and install the build-in Initializer from ConfigMap.
func (r *Reconciler) checkOrInstallDependsOn(ctx context.Context, depends []v1beta1.DependsOn) (bool, error) {
	for _, depend := range depends {
		dependInit, err := utils.GetInitializer(ctx, r.Client, depend.Ref.Namespace, depend.Ref.Name)
		if err != nil {
			// if Initializer is not found and the namespace is default or vela-system,
			// try to install build-in initializer from ConfigMap
			if apierrors.IsNotFound(err) && (depend.Ref.Namespace == "default" || depend.Ref.Namespace == velatypes.DefaultKubeVelaNS) {
				init, err := utils.GetBuildInInitializer(ctx, r.Client, depend.Ref.Name)
				if err != nil {
					return false, err
				}
				return false, r.Client.Create(ctx, init)
			}
			return false, err
		}

		if dependInit.Status.Phase != v1beta1.InitializerSuccess {
			klog.InfoS("Initializer you depend on is not ready",
				"initializer", klog.KObj(dependInit), "phase", dependInit.Status.Phase)
			return false, nil
		}
	}
	return true, nil
}

func (r *Reconciler) endWithNegativeCondition(ctx context.Context, init *v1beta1.Initializer, condition cpv1alpha1.Condition) (ctrl.Result, error) {
	init.SetConditions(condition)
	if err := r.patchStatus(ctx, init); err != nil {
		return ctrl.Result{}, errors.WithMessage(err, "cannot update initializer status")
	}
	return ctrl.Result{}, fmt.Errorf("object level reconcile error, type: %q, msg: %q", string(condition.Type), condition.Message)
}

func (r *Reconciler) patchStatus(ctx context.Context, init *v1beta1.Initializer) error {
	updateObservedGeneration(init)
	return r.Client.Status().Patch(ctx, init, client.Merge)
}

func updateObservedGeneration(init *v1beta1.Initializer) {
	if init.Status.ObservedGeneration != init.Generation {
		init.Status.ObservedGeneration = init.Generation
	}
}

func (r *Reconciler) applyResources(ctx context.Context, init *v1beta1.Initializer) (bool, error) {
	// set ownerReference for system adddons(application)
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         init.APIVersion,
		Kind:               init.Kind,
		Name:               init.Name,
		UID:                init.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	app := init.Spec.AppTemplate.DeepCopy()
	app.SetNamespace(init.Namespace)
	app.SetName(init.Name)
	app.SetOwnerReferences(ownerReference)
	app.SetAnnotations(map[string]string{
		"app.oam.dev/initializer-name": init.Name,
	})

	if err := r.createOrUpdateResource(ctx, app); err != nil {
		return false, err
	}

	err := r.Client.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, app)
	if err != nil {
		return false, err
	}

	klog.InfoS("Check the status of Application", "app", klog.KObj(app), "phase", app.Status.Phase)
	if app.Status.Phase != common.ApplicationRunning {
		return false, nil
	}
	return true, nil
}

func (r *Reconciler) createOrUpdateResource(ctx context.Context, app *v1beta1.Application) error {
	klog.InfoS("Create or update resources", "app", klog.KObj(app))
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, app)
		}
		return err
	}
	return r.Update(ctx, app)
}

// UpdateStatus updates v1beta1.Initializer's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, def *v1beta1.Initializer, opts ...client.UpdateOption) error {
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
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Initializer")).
		WithAnnotations("controller", "Initializer")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.Initializer{}).
		Complete(r)
}

// Setup adds a controller that reconciles Initializer.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		dm:                   args.DiscoveryMapper,
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return r.SetupWithManager(mgr)
}
