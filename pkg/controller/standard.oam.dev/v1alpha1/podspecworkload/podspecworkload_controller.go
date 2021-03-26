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

package podspecworkload

import (
	"context"
	"fmt"
	"reflect"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

// Reconcile error strings.
const (
	errRenderDeployment = "cannot render deployment"
	errRenderService    = "cannot render service"
	errApplyDeployment  = "cannot apply the deployment"
	errApplyService     = "cannot apply the service"
)

var (
	deploymentKind       = reflect.TypeOf(appsv1.Deployment{}).Name()
	deploymentAPIVersion = appsv1.SchemeGroupVersion.String()
	serviceKind          = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion    = corev1.SchemeGroupVersion.String()
)

const (
	labelNameKey = "component.oam.dev/name"
)

// Reconciler reconciles a PodSpecWorkload object
type Reconciler struct {
	client.Client
	log    logr.Logger
	record event.Recorder
	Scheme *runtime.Scheme
}

// Reconcile is the main logic for podspecworkload controller
// +kubebuilder:rbac:groups=standard.oam.dev,resources=podspecworkloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=podspecworkloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.log.WithValues("podspecworkload", req.NamespacedName)
	log.Info("Reconcile podspecworkload workload")

	var workload v1alpha1.PodSpecWorkload
	if err := r.Get(ctx, req.NamespacedName, &workload); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Podspec workload is deleted")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info("Get the workload", "apiVersion", workload.APIVersion, "kind", workload.Kind)
	// find the resource object to record the event to, default is the parent appConfig.
	eventObj, err := util.LocateParentAppConfig(ctx, r.Client, &workload)
	if eventObj == nil {
		// fallback to workload itself
		log.Error(err, "workload", "name", workload.Name)
		eventObj = &workload
	}
	deploy, err := r.renderDeployment(&workload)
	if err != nil {
		log.Error(err, "Failed to render a deployment")
		r.record.Event(eventObj, event.Warning(errRenderDeployment, err))
		return util.ReconcileWaitResult,
			util.PatchCondition(ctx, r, &workload, cpv1alpha1.ReconcileError(errors.Wrap(err, errRenderDeployment)))
	}
	// server side apply
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(workload.GetUID())}
	if err := r.Patch(ctx, deploy, client.Apply, applyOpts...); err != nil {
		log.Error(err, "Failed to apply to a deployment")
		r.record.Event(eventObj, event.Warning(errApplyDeployment, err))
		return util.ReconcileWaitResult,
			util.PatchCondition(ctx, r, &workload, cpv1alpha1.ReconcileError(errors.Wrap(err, errApplyDeployment)))
	}
	r.record.Event(eventObj, event.Normal("Deployment created",
		fmt.Sprintf("Workload `%s` successfully patched a deployment `%s`",
			workload.Name, deploy.Name)))

	// record the new deployment
	workload.Status.Resources = []cpv1alpha1.TypedReference{
		{
			APIVersion: deploy.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       deploy.GetObjectKind().GroupVersionKind().Kind,
			Name:       deploy.GetName(),
			UID:        deploy.UID,
		},
	}

	// Determine whether it is necessary to create a service.if container.
	setPorts := r.checkContainerPortsSpecified(&workload)
	if setPorts {
		// create a service for the workload
		service, err := r.renderService(&workload)
		if err != nil {
			log.Error(err, "Failed to render a service")
			r.record.Event(eventObj, event.Warning(errRenderService, err))
			return util.ReconcileWaitResult,
				util.PatchCondition(ctx, r, &workload, cpv1alpha1.ReconcileError(errors.Wrap(err, errRenderService)))
		}
		// server side apply the service
		if err := r.Patch(ctx, service, client.Apply, applyOpts...); err != nil {
			log.Error(err, "Failed to apply a service")
			r.record.Event(eventObj, event.Warning(errApplyDeployment, err))
			return util.ReconcileWaitResult,
				util.PatchCondition(ctx, r, &workload, cpv1alpha1.ReconcileError(errors.Wrap(err, errApplyService)))
		}
		r.record.Event(eventObj, event.Normal("Service created",
			fmt.Sprintf("Workload `%s` successfully server side patched a service `%s`",
				workload.Name, service.Name)))

		// record the new service
		workload.Status.Resources = append(workload.Status.Resources, cpv1alpha1.TypedReference{
			APIVersion: service.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       service.GetObjectKind().GroupVersionKind().Kind,
			Name:       service.GetName(),
			UID:        service.UID,
		})
	}

	if err := r.UpdateStatus(ctx, &workload); err != nil {
		return util.ReconcileWaitResult, err
	}
	return ctrl.Result{}, util.PatchCondition(ctx, r, &workload, cpv1alpha1.ReconcileSuccess())
}

// create a corresponding deployment
func (r *Reconciler) renderDeployment(workload *v1alpha1.PodSpecWorkload) (*appsv1.Deployment, error) {
	// generate the deployment
	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       deploymentKind,
			APIVersion: deploymentAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.GetName(),
			Namespace: workload.GetNamespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: workload.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelNameKey: workload.GetName(),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelNameKey: workload.GetName(),
					},
				},
				Spec: workload.Spec.PodSpec,
			},
		},
	}
	// k8s server-side patch complains if the protocol is not set
	for i := 0; i < len(deploy.Spec.Template.Spec.Containers); i++ {
		for j := 0; j < len(deploy.Spec.Template.Spec.Containers[i].Ports); j++ {
			if len(deploy.Spec.Template.Spec.Containers[i].Ports[j].Protocol) == 0 {
				deploy.Spec.Template.Spec.Containers[i].Ports[j].Protocol = corev1.ProtocolTCP
			}
		}
	}

	// pass through label and annotation from the workload to the deployment
	util.PassLabelAndAnnotation(workload, deploy)
	// pass through label and annotation from the workload to the pod template too
	util.PassLabelAndAnnotation(workload, &deploy.Spec.Template)

	r.log.Info("rendered a deployment", "deploy", deploy.Spec.Template.Spec)

	// set the controller reference so that we can watch this deployment and it will be deleted automatically
	if err := ctrl.SetControllerReference(workload, deploy, r.Scheme); err != nil {
		return nil, err
	}

	return deploy, nil
}

// check whether the container port is specified
func (r *Reconciler) checkContainerPortsSpecified(workload *v1alpha1.PodSpecWorkload) bool {
	if workload == nil {
		return false
	}
	for _, container := range workload.Spec.PodSpec.Containers {
		if len(container.Ports) > 0 {
			return true
		}
	}
	return false
}

// create a service for the deployment
func (r *Reconciler) renderService(workload *v1alpha1.PodSpecWorkload) (*corev1.Service, error) {
	// create a service for the workload
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceKind,
			APIVersion: serviceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.GetName(),
			Namespace: workload.GetNamespace(),
			Labels: map[string]string{
				labelNameKey: workload.GetName(),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				labelNameKey: workload.GetName(),
			},
			Ports: []corev1.ServicePort{},
			Type:  corev1.ServiceTypeClusterIP,
		},
	}
	// create a port for each ports in the all the containers
	var servicePort int32 = 8080
	for _, container := range workload.Spec.PodSpec.Containers {
		for _, port := range container.Ports {
			sp := corev1.ServicePort{
				Name:       port.Name,
				Protocol:   port.Protocol,
				Port:       servicePort,
				TargetPort: intstr.FromInt(int(port.ContainerPort)),
			}
			service.Spec.Ports = append(service.Spec.Ports, sp)
			servicePort++
		}
	}

	// always set the controller reference so that we can watch this service and
	if err := ctrl.SetControllerReference(workload, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

// SetupWithManager will setup controller for podspecworkload
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("PodSpecWorkload")).
		WithAnnotations("controller", "PodSpecWorkload")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PodSpecWorkload{}).
		Complete(r)
}

// UpdateStatus updates *v1alpha1.PodSpecWorkload's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, workload *v1alpha1.PodSpecWorkload, opts ...client.UpdateOption) error {
	status := workload.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, types.NamespacedName{Namespace: workload.Namespace, Name: workload.Name}, workload); err != nil {
			return
		}
		workload.Status = status
		return r.Status().Update(ctx, workload, opts...)
	})
}

// Setup adds a controller that reconciles PodSpecWorkload.
func Setup(mgr ctrl.Manager) error {
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		log:    ctrl.Log.WithName("PodSpecWorkload"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
