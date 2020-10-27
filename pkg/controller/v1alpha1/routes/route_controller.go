/*


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

package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/oam-dev/kubevela/api/v1alpha1"
	standardv1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/v1alpha1/routes/ingress"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errApplyNginxIngress = "failed to apply the ingress"
)

var requeueNotReady = 10 * time.Second

// Reconciler reconciles a Route object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	Log    logr.Logger
	record event.Recorder
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=standard.oam.dev,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=routes/status,verbs=get;update;patch
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	mLog := r.Log.WithValues("route", req.NamespacedName)

	mLog.Info("Reconcile route trait")
	// fetch the trait
	var routeTrait standardv1alpha1.Route
	if err := r.Get(ctx, req.NamespacedName, &routeTrait); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	mLog.Info("Get the route trait",
		"host", routeTrait.Spec.Host,
		"workload reference", routeTrait.Spec.WorkloadReference,
		"labels", routeTrait.GetLabels())

	// find the resource object to record the event to, default is the parent appConfig.
	eventObj, err := oamutil.LocateParentAppConfig(ctx, r.Client, &routeTrait)
	if eventObj == nil {
		// fallback to workload itself
		mLog.Error(err, "add events to route trait itself", "name", routeTrait.Name)
		eventObj = &routeTrait
	}

	// Fetch the workload instance to which we want to do routes
	workload, err := oamutil.FetchWorkload(ctx, r, mLog, &routeTrait)
	if err != nil {
		mLog.Error(err, "Error while fetching the workload", "workload reference",
			routeTrait.GetWorkloadReference())
		r.record.Event(eventObj, event.Warning(common.ErrLocatingWorkload, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &routeTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingWorkload)))
	}
	var svc *runtimev1alpha1.TypedReference
	if NeedDiscovery(&routeTrait) {
		if svc, err = r.discoveryAndFillBackend(ctx, mLog, eventObj, workload, &routeTrait); err != nil {
			return oamutil.ReconcileWaitResult, err
		}
	}

	routeIngress, err := ingress.GetRouteIngress(routeTrait.Spec.Provider, r.Client)
	if err != nil {
		mLog.Error(err, "Failed to get routeIngress, use nginx route instead")
		routeIngress = &ingress.Nginx{}
	}

	// Create Ingress
	// construct the serviceMonitor that hooks the service to the prometheus server
	ingresses := routeIngress.Construct(&routeTrait)
	// server side apply the serviceMonitor, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(routeTrait.GetUID())}
	for _, ingress := range ingresses {
		if err := r.Patch(ctx, ingress, client.Apply, applyOpts...); err != nil {
			mLog.Error(err, "Failed to apply to ingress")
			r.record.Event(eventObj, event.Warning(errApplyNginxIngress, err))
			return oamutil.ReconcileWaitResult,
				oamutil.PatchCondition(ctx, r, &routeTrait,
					cpv1alpha1.ReconcileError(errors.Wrap(err, errApplyNginxIngress)))
		}
		r.record.Event(eventObj, event.Normal("nginx ingress created",
			fmt.Sprintf("successfully server side patched a route trait `%s`", routeTrait.Name)))
	}
	// TODO(wonderflow): GC mechanism for no used ingress, service, issuer

	var ingressCreated []runtimev1alpha1.TypedReference
	for _, ingress := range ingresses {
		ingressCreated = append(ingressCreated, runtimev1alpha1.TypedReference{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       reflect.TypeOf(v1beta1.Ingress{}).Name(),
			Name:       ingress.Name,
			UID:        routeTrait.UID,
		})
	}
	routeTrait.Status.Ingresses = ingressCreated
	routeTrait.Status.Service = svc
	var conditions []runtimev1alpha1.Condition
	routeTrait.Status.Status, conditions = routeIngress.CheckStatus(&routeTrait)
	routeTrait.Status.Conditions = conditions
	if routeTrait.Status.Status != ingress.StatusReady {
		return ctrl.Result{RequeueAfter: requeueNotReady}, r.Status().Update(ctx, &routeTrait)
	}
	return ctrl.Result{}, r.Status().Update(ctx, &routeTrait)
}

// discoveryAndFillBackend will automatically discovery backend for route
func (r *Reconciler) discoveryAndFillBackend(ctx context.Context, mLog logr.Logger, eventObj runtime.Object, workload *unstructured.Unstructured,
	routeTrait *v1alpha1.Route) (*runtimev1alpha1.TypedReference, error) {

	// Fetch the child childResources list from the corresponding workload
	childResources, err := oamutil.FetchWorkloadChildResources(ctx, mLog, r, r.dm, workload)
	if err != nil {
		mLog.Error(err, "Error while fetching the workload child childResources", "workload kind", workload.GetKind(),
			"workload name", workload.GetName())
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	// try to see if the workload already has services in child childResources, and match for our route
	err = r.fillBackendByCheckChildResource(mLog, routeTrait, childResources)
	if err != nil && !apierrors.IsNotFound(err) {
		r.record.Event(eventObj, event.Warning(common.ErrLocatingService, err))
		return nil, oamutil.PatchCondition(ctx, r, routeTrait,
			cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingService)))
	}

	// Check if still need discovery after childResource filled.
	if NeedDiscovery(routeTrait) {
		// no service found, we will create service according to rule
		svc, err := r.fillBackendByCreatedService(ctx, mLog, workload, routeTrait, childResources)
		if err != nil {
			r.record.Event(eventObj, event.Warning(common.ErrCreatingService, err))
			return nil, oamutil.PatchCondition(ctx, r, routeTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrCreatingService)))
		}
		r.record.Event(eventObj, event.Normal("Service created",
			fmt.Sprintf("successfully automatically created a service `%s`", svc.Name)))
		return svc, nil
	}
	mLog.Info("workload already has service as child resource, will not create service", "workloadName", workload.GetName())
	return nil, nil

}

// fillBackendByCreatedService will automatically create service by discovery podTemplate or podSpec.
func (r *Reconciler) fillBackendByCreatedService(ctx context.Context, mLog logr.Logger, workload *unstructured.Unstructured,
	routeTrait *v1alpha1.Route, childResources []*unstructured.Unstructured) (*runtimev1alpha1.TypedReference, error) {

	oamService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       common.ServiceKind,
			APIVersion: common.ServiceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeTrait.GetName(),
			Namespace: routeTrait.GetNamespace(),
			Labels:    filterLabels(routeTrait.GetLabels()),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         routeTrait.GetObjectKind().GroupVersionKind().GroupVersion().String(),
					Kind:               routeTrait.GetObjectKind().GroupVersionKind().Kind,
					UID:                routeTrait.GetUID(),
					Name:               routeTrait.GetName(),
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	ports, labels, err := DiscoverPortsLabel(ctx, workload, r, r.dm, childResources)
	if err != nil {
		mLog.Info("[WARN] fail to discovery port and label", "err", err)
		return nil, err
	}
	oamService.Spec.Selector = labels

	// use the same port
	for _, port := range ports {
		oamService.Spec.Ports = append(oamService.Spec.Ports, corev1.ServicePort{
			Port:       int32(port.IntValue()),
			TargetPort: port,
			Protocol:   corev1.ProtocolTCP,
		})
	}
	// server side apply the service, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(routeTrait.GetUID())}
	if err := r.Patch(ctx, oamService, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to service")
		return nil, err
	}
	FillRouteTraitWithService(oamService, routeTrait)
	return &runtimev1alpha1.TypedReference{
		APIVersion: common.ServiceAPIVersion,
		Kind:       common.ServiceKind,
		Name:       oamService.Name,
		UID:        routeTrait.UID,
	}, nil
}

// Assume the workload or it's childResource will always having spec.template as PodTemplate if discoverable
func DiscoverPortsLabel(ctx context.Context, workload *unstructured.Unstructured, r client.Reader, dm discoverymapper.DiscoveryMapper, childResources []*unstructured.Unstructured) ([]intstr.IntOrString, map[string]string, error) {

	// here is the logic follows the design https://github.com/crossplane/oam-kubernetes-runtime/blob/master/design/one-pager-podspecable-workload.md#proposal
	// Get WorkloadDefinition
	workloadDef, err := oamutil.FetchWorkloadDefinition(ctx, r, dm, workload)
	if err != nil {
		return nil, nil, err
	}
	podSpecPath, ok := GetPodSpecPath(workloadDef)
	if podSpecPath != "" {
		ports, err := discoveryFromPodSpec(workload, podSpecPath)
		if err != nil {
			return nil, nil, err
		}
		return ports, filterLabels(workload.GetLabels()), nil
	}
	if ok {
		return discoveryFromPodTemplate(workload, "spec", "template")
	}

	// If workload is not podSpecable, try to detect it's child resource
	var resources = []*unstructured.Unstructured{workload}
	resources = append(resources, childResources...)
	var gatherErrs []error
	for _, w := range resources {
		port, labels, err := discoveryFromPodTemplate(w, "spec", "template")
		if err == nil {
			return port, labels, nil
		}
		gatherErrs = append(gatherErrs, err)
	}
	return nil, nil, fmt.Errorf("fail to automatically discovery backend from workload %v(%v.%v) and it's child resource, errorList: %v", workload.GetName(), workload.GetAPIVersion(), workload.GetKind(), gatherErrs)
}

// fetch the service that is associated with the workload
func (r *Reconciler) fillBackendByCheckChildResource(mLog logr.Logger,
	routeTrait *v1alpha1.Route, childResources []*unstructured.Unstructured) error {
	if len(childResources) == 0 {
		return nil
	}
	// find the service that has the port
	for _, childRes := range childResources {
		if childRes.GetAPIVersion() == corev1.SchemeGroupVersion.String() && childRes.GetKind() == reflect.TypeOf(corev1.Service{}).Name() {
			data, err := json.Marshal(childRes.Object)
			if err != nil {
				mLog.Error(err, "error marshal child childResources as K8s Service, continue to check other resource", "resource name", childRes.GetName())
				continue
			}
			var service corev1.Service
			err = json.Unmarshal(data, &service)
			if err != nil {
				mLog.Error(err, "error unmarshal child childResources as K8s Service, continue to check other resource", "resource name", childRes.GetName())
				continue
			}
			FillRouteTraitWithService(&service, routeTrait)
		}
	}
	return nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Route")).
		WithAnnotations("controller", "route")
	return ctrl.NewControllerManagedBy(mgr).
		For(&standardv1alpha1.Route{}).
		Complete(r)
}

// Setup adds a controller that reconciles MetricsTrait.
func Setup(mgr ctrl.Manager) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Route"),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return reconciler.SetupWithManager(mgr)
}
