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

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

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
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// TODO(yangsoon) this is a placeholder for finalizer here
	if init.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	klog.Info("Check the status of the Initializers which you depend on")
	err := r.checkDependsOn(ctx, req.Namespace, init.Spec.DependsOn)
	if err != nil {
		klog.ErrorS(err, "Initializers which you depend on are not ready")
		r.record.Event(init, event.Warning("Initializers which you depend on are not ready", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, init,
			cpv1alpha1.ReconcileError(err))
	}

	if err = r.createOrUpdateApplication(ctx, init); err != nil {
		klog.ErrorS(err, "Could not create resources via application to initialize the env")
		r.record.Event(init, event.Warning("Could not create resources via application", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, init,
			cpv1alpha1.ReconcileError(err))
	}

	if err = r.updateObservedGeneration(ctx, init); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) checkDependsOn(ctx context.Context, ns string, depends []corev1.TypedLocalObjectReference) error {
	for _, depend := range depends {
		dependInit := new(v1beta1.Initializer)
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: depend.Name}, dependInit); err != nil {
			return err
		}
		if dependInit.Status.ObservedGeneration < dependInit.Generation {
			return fmt.Errorf("initializer %s you depend on is not ready", depend.Name)
		}
	}
	return nil
}

func (r *Reconciler) updateObservedGeneration(ctx context.Context, init *v1beta1.Initializer) error {
	if init.Status.ObservedGeneration != init.Generation {
		init.Status.ObservedGeneration = init.Generation
	}
	return r.Client.Status().Update(ctx, init)
}

func (r *Reconciler) createOrUpdateApplication(ctx context.Context, init *v1beta1.Initializer) error {
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
