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

package containerizedworkload

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconcile error strings.
const (
	errRenderWorkload  = "cannot render workload"
	errRenderService   = "cannot render service"
	errApplyDeployment = "cannot apply the deployment"
	errApplyConfigMap  = "cannot apply the configmap"
	errApplyService    = "cannot apply the service"
)

// Setup adds a controller that reconciles ContainerizedWorkload.
func Setup(mgr ctrl.Manager, _ controller.Args) error {
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		record: event.NewAPIRecorder(mgr.GetEventRecorderFor("ContainerizedWorkload")),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}

// Reconciler reconciles a ContainerizedWorkload object
type Reconciler struct {
	client.Client
	record event.Recorder
	Scheme *runtime.Scheme
}

// Reconcile reconciles a ContainerizedWorkload object
// +kubebuilder:rbac:groups=core.oam.dev,resources=containerizedworkloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.oam.dev,resources=containerizedworkloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := common2.NewReconcileContext(ctx)
	defer cancel()

	klog.InfoS("Reconcile containerizedworkload", klog.KRef(req.Namespace, req.Name))
	var workload v1alpha2.ContainerizedWorkload
	if err := r.Get(ctx, req.NamespacedName, &workload); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("Container workload is deleted")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Get the workload", "apiVersion", workload.APIVersion, "kind", workload.Kind)
	// find the resource object to record the event to, default is the parent appConfig.
	eventObj, err := util.LocateParentAppConfig(ctx, r.Client, &workload)
	if eventObj == nil {
		// fallback to workload itself
		klog.ErrorS(err, "workload", "name", workload.Name)
		eventObj = &workload
	}
	deploy, err := r.renderDeployment(ctx, &workload)
	if err != nil {
		klog.ErrorS(err, "Failed to render a deployment")
		r.record.Event(eventObj, event.Warning(errRenderWorkload, err))
		return ctrl.Result{},
			util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errRenderWorkload)))
	}
	// server side apply, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(workload.GetUID())}
	if err := r.Patch(ctx, deploy, client.Apply, applyOpts...); err != nil {
		klog.ErrorS(err, "Failed to apply to a deployment")
		r.record.Event(eventObj, event.Warning(errApplyDeployment, err))
		return ctrl.Result{},
			util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errApplyDeployment)))
	}
	r.record.Event(eventObj, event.Normal("Deployment created",
		fmt.Sprintf("Workload `%s` successfully server side patched a deployment `%s`",
			workload.Name, deploy.Name)))

	configMapApplyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(deploy.GetUID())}
	configmaps, err := r.renderConfigMaps(ctx, &workload, deploy)
	if err != nil {
		klog.ErrorS(err, "Failed to render configmaps")
		r.record.Event(eventObj, event.Warning(errRenderWorkload, err))
		return ctrl.Result{},
			util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errRenderWorkload)))
	}
	for _, cm := range configmaps {
		if err := r.Patch(ctx, cm, client.Apply, configMapApplyOpts...); err != nil {
			klog.ErrorS(err, "Failed to apply a configmap")
			r.record.Event(eventObj, event.Warning(errApplyConfigMap, err))
			return ctrl.Result{},
				util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errApplyConfigMap)))
		}
		r.record.Event(eventObj, event.Normal("ConfigMap created",
			fmt.Sprintf("Workload `%s` successfully server side patched a configmap `%s`",
				workload.Name, cm.Name)))
	}
	// create a service for the workload
	// TODO(rz): remove this after we have service trait
	service, err := r.renderService(ctx, &workload, deploy)
	if err != nil {
		klog.ErrorS(err, "Failed to render a service")
		r.record.Event(eventObj, event.Warning(errRenderService, err))
		return ctrl.Result{},
			util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errRenderService)))
	}
	// server side apply the service
	if err := r.Patch(ctx, service, client.Apply, applyOpts...); err != nil {
		klog.ErrorS(err, "Failed to apply a service")
		r.record.Event(eventObj, event.Warning(errApplyDeployment, err))
		return ctrl.Result{},
			util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(errors.Wrap(err, errApplyService)))
	}
	r.record.Event(eventObj, event.Normal("Service created",
		fmt.Sprintf("Workload `%s` successfully server side patched a service `%s`",
			workload.Name, service.Name)))
	// garbage collect the service/deployments that we created but not needed
	if err := r.cleanupResources(ctx, &workload, &deploy.UID, &service.UID); err != nil {
		klog.ErrorS(err, "Failed to clean up resources")
		r.record.Event(eventObj, event.Warning(errApplyDeployment, err))
	}
	workload.Status.Resources = nil
	// record the new deployment, new service
	workload.Status.Resources = append(workload.Status.Resources,
		corev1.ObjectReference{
			APIVersion: deploy.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       deploy.GetObjectKind().GroupVersionKind().Kind,
			Name:       deploy.GetName(),
			UID:        deploy.UID,
		},
		corev1.ObjectReference{
			APIVersion: service.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       service.GetObjectKind().GroupVersionKind().Kind,
			Name:       service.GetName(),
			UID:        service.UID,
		},
	)

	workload.SetConditions(condition.ReconcileSuccess())
	if err := r.UpdateStatus(ctx, &workload); err != nil {
		return ctrl.Result{}, util.EndReconcileWithNegativeCondition(ctx, r, &workload, condition.ReconcileError(err))
	}
	return ctrl.Result{}, nil
}

// UpdateStatus updates v1alpha2.ContainerizedWorkload's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, workload *v1alpha2.ContainerizedWorkload, opts ...client.UpdateOption) error {
	status := workload.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, types.NamespacedName{Namespace: workload.Namespace, Name: workload.Name}, workload); err != nil {
			return
		}
		workload.Status = status
		return r.Status().Update(ctx, workload, opts...)
	})
}

// SetupWithManager setups up k8s controller.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	src := &v1alpha2.ContainerizedWorkload{}
	name := "oam/" + strings.ToLower(v1alpha2.ContainerizedWorkloadKind)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(src).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Service{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
