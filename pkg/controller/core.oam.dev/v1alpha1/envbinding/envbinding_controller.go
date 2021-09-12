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
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	resourceTrackerFinalizer = "envbinding.oam.dev/resource-tracker-finalizer"
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
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := common2.NewReconcileContext(ctx)
	defer cancel()
	klog.InfoS("Reconcile EnvBinding", "envbinding", klog.KRef(req.Namespace, req.Name))

	envBinding := new(v1alpha1.EnvBinding)
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, envBinding); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	endReconcile, err := r.handleFinalizers(ctx, envBinding)
	if err != nil {
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}
	if endReconcile {
		return ctrl.Result{}, nil
	}

	if err := validatePlacement(envBinding); err != nil {
		klog.ErrorS(err, "The placement is not compliant")
		r.record.Event(envBinding, event.Warning("The placement is not compliant", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
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
		engine = NewOCMEngine(r.Client, baseApp.Name, baseApp.Namespace, envBinding.Name)
	case v1alpha1.SingleClusterEngine:
		engine = NewSingleClusterEngine(r.Client, baseApp.Name, baseApp.Namespace, envBinding.Name)
	case v1alpha1.ClusterGatewayEngine:
		engine = NewClusterGatewayEngine(r.Client, envBinding.Name)
	default:
		engine = NewClusterGatewayEngine(r.Client, envBinding.Name)
	}

	// prepare the pre-work for cluster scheduling
	envBinding.Status.Phase = v1alpha1.EnvBindingPrepare
	if err = engine.prepare(ctx, envBinding.Spec.Envs); err != nil {
		klog.ErrorS(err, "Failed to prepare the pre-work for cluster scheduling")
		r.record.Event(envBinding, event.Warning("Failed to prepare the pre-work for cluster scheduling", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// patch the component parameters for application in different envs
	envBinding.Status.Phase = v1alpha1.EnvBindingRendering
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)
	envBindApps, err := engine.initEnvBindApps(ctx, envBinding, baseApp, appParser)
	if err != nil {
		klog.ErrorS(err, "Failed to patch the parameters for application in different envs")
		r.record.Event(envBinding, event.Warning("Failed to patch the parameters for application in different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	// schedule resource of applications in different envs
	envBinding.Status.Phase = v1alpha1.EnvBindingScheduling
	clusterDecisions, err := engine.schedule(ctx, envBindApps)
	if err != nil {
		klog.ErrorS(err, "Failed to schedule resource of applications in different envs")
		r.record.Event(envBinding, event.Warning("Failed to schedule resource of applications in different envs", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	if err = garbageCollect(ctx, r.Client, envBinding, envBindApps); err != nil {
		klog.ErrorS(err, "Failed to garbage collect old resource for envBinding")
		r.record.Event(envBinding, event.Warning("Failed to garbage collect old resource for envBinding", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	if err = r.applyOrRecordManifests(ctx, envBinding, envBindApps); err != nil {
		klog.ErrorS(err, "Failed to apply or record manifests")
		r.record.Event(envBinding, event.Warning("Failed to apply or record manifests", err))
		return r.endWithNegativeCondition(ctx, envBinding, condition.ReconcileError(err))
	}

	envBinding.Status.Phase = v1alpha1.EnvBindingFinished
	envBinding.Status.ClusterDecisions = clusterDecisions
	if err = r.Client.Status().Patch(ctx, envBinding, client.Merge); err != nil {
		klog.ErrorS(err, "Failed to update status")
		r.record.Event(envBinding, event.Warning("Failed to update status", err))
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, envBinding, condition.ReconcileError(err))
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) applyOrRecordManifests(ctx context.Context, envBinding *v1alpha1.EnvBinding, envBindApps []*EnvBindApp) error {
	if envBinding.Spec.OutputResourcesTo != nil && len(envBinding.Spec.OutputResourcesTo.Name) != 0 {
		if err := StoreManifest2ConfigMap(ctx, r.Client, envBinding, envBindApps); err != nil {
			klog.ErrorS(err, "Failed to store manifest of different envs to configmap")
			r.record.Event(envBinding, event.Warning("Failed to store manifest of different envs to configmap", err))
		}
		envBinding.Status.ResourceTracker = nil
		return nil
	}

	rt, err := r.createOrGetResourceTracker(ctx, envBinding)
	if err != nil {
		return err
	}
	if err = r.dispatchManifests(ctx, rt, envBindApps); err != nil {
		klog.ErrorS(err, "Failed to dispatch resources of different envs to cluster")
		r.record.Event(envBinding, event.Warning("Failed to dispatch resources of different envs to cluster", err))
		return err
	}

	if err = r.updateResourceTrackerStatus(ctx, rt.Name, envBindApps); err != nil {
		return err
	}
	envBinding.Status.ResourceTracker = &v1.ObjectReference{
		Kind:       rt.Kind,
		APIVersion: rt.APIVersion,
		Name:       rt.Name,
	}
	return nil
}

func (r *Reconciler) dispatchManifests(ctx context.Context, resourceTracker *v1beta1.ResourceTracker, envBindApps []*EnvBindApp) error {
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         resourceTracker.APIVersion,
		Kind:               resourceTracker.Kind,
		Name:               resourceTracker.Name,
		UID:                resourceTracker.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	applicator := apply.NewAPIApplicator(r.Client)
	for _, app := range envBindApps {
		for _, obj := range app.ScheduledManifests {
			obj.SetOwnerReferences(ownerReference)
			if err := applicator.Apply(ctx, obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) createOrGetResourceTracker(ctx context.Context, envBinding *v1alpha1.EnvBinding) (*v1beta1.ResourceTracker, error) {
	rt := new(v1beta1.ResourceTracker)
	rtName := constructResourceTrackerName(envBinding.Name, envBinding.Namespace)
	err := r.Client.Get(ctx, client.ObjectKey{Name: rtName}, rt)
	if err == nil {
		return rt, nil
	}
	if !kerrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "cannot get resource tracker")
	}
	klog.InfoS("Going to create a resource tracker", "resourceTracker", rtName)
	rt.SetName(rtName)
	if err = r.Client.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *Reconciler) updateResourceTrackerStatus(ctx context.Context, rtName string, envBindApps []*EnvBindApp) error {
	var refs []v1.ObjectReference
	for _, app := range envBindApps {
		for _, obj := range app.ScheduledManifests {
			refs = append(refs, v1.ObjectReference{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
				Name:       obj.GetName(),
				Namespace:  obj.GetNamespace(),
			})
		}
	}

	resourceTracker := new(v1beta1.ResourceTracker)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Client.Get(ctx, client.ObjectKey{Name: rtName}, resourceTracker); err != nil {
			return
		}
		resourceTracker.Status.TrackedResources = refs
		return r.Client.Status().Update(ctx, resourceTracker)
	}); err != nil {
		return err
	}
	klog.InfoS("Successfully update resource tracker status", "resourceTracker", rtName)
	return nil
}

func (r *Reconciler) handleFinalizers(ctx context.Context, envBinding *v1alpha1.EnvBinding) (bool, error) {
	if envBinding.ObjectMeta.DeletionTimestamp.IsZero() {
		if !meta.FinalizerExists(envBinding, resourceTrackerFinalizer) {
			meta.AddFinalizer(envBinding, resourceTrackerFinalizer)
			klog.InfoS("Register new finalizer for envBinding", "envBinding", klog.KObj(envBinding), "finalizer", resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, envBinding), "cannot update envBinding finalizer")
		}
	} else {
		if meta.FinalizerExists(envBinding, resourceTrackerFinalizer) {
			rt := new(v1beta1.ResourceTracker)
			rt.SetName(constructResourceTrackerName(envBinding.Name, envBinding.Namespace))
			if err := r.Client.Get(ctx, client.ObjectKey{Name: rt.Name}, rt); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to get resource tracker of envBinding", "envBinding", klog.KObj(envBinding))
				return true, errors.WithMessage(err, "cannot remove finalizer")
			}

			if err := r.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete resource tracker of envBinding", "envBinding", klog.KObj(envBinding))
				return true, errors.WithMessage(err, "cannot remove finalizer")
			}

			if err := GarbageCollectionForAllResourceTrackersInSubCluster(ctx, r.Client, envBinding); err != nil {
				return true, err
			}
			meta.RemoveFinalizer(envBinding, resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, envBinding), "cannot update envBinding finalizer")
		}
	}
	return false, nil
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
