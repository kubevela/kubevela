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
	"strconv"

	"github.com/oam-dev/kubevela/api/v1alpha1"
	standardv1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/go-logr/logr"
	certmanager "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha2"
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
	errCreateIssuer      = "failed to create cert-manager Issuer"
)

var (
	// oamServiceLabel is the pre-defined labels for any serviceMonitor
	// created by the RouteTrait
	oamServiceLabel = map[string]string{
		"k8s-app":    "oam",
		"controller": "routeTrait",
	}
)

// Reconciler reconciles a Route object
type Reconciler struct {
	client.Client
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
		"path", routeTrait.Spec.Path,
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

	// try to see if the workload already has services as child resources, and match for our route
	svc, exist, svcPort, err := r.fetchService(ctx, mLog, workload, &routeTrait)
	if err != nil && !apierrors.IsNotFound(err) {
		r.record.Event(eventObj, event.Warning(common.ErrLocatingService, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &routeTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingService)))
	}

	// Create Services
	if !exist {
		// no service found, we will create service according to rule
		svc, svcPort, err = r.createService(ctx, mLog, workload, &routeTrait)
		if err != nil {
			r.record.Event(eventObj, event.Warning(common.ErrCreatingService, err))
			return oamutil.ReconcileWaitResult,
				oamutil.PatchCondition(ctx, r, &routeTrait,
					cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrCreatingService)))
		}
		r.record.Event(eventObj, event.Normal("Service created",
			fmt.Sprintf("successfully automatically created a service `%s`", svc.Name)))
	} else {
		mLog.Info("workload already has service as child resource, will not create new", "workloadName", workload.GetName())
	}

	// Create Issuers
	var issuer standardv1alpha1.TLS
	if routeTrait.Spec.TLS == nil || routeTrait.Spec.TLS.IssuerName == "" {
		issuerName, err := r.createSelfsignedIssuer(ctx, &routeTrait)
		if err != nil {
			r.record.Event(eventObj, event.Warning(errCreateIssuer, err))
			return oamutil.ReconcileWaitResult,
				oamutil.PatchCondition(ctx, r, &routeTrait,
					cpv1alpha1.ReconcileError(errors.Wrap(err, errCreateIssuer)))
		}
		r.record.Event(eventObj, event.Normal("Issuer created",
			fmt.Sprintf("successfully automatically created a Issuer for route TLS `%s`", issuerName)))
		issuer.Type = standardv1alpha1.NamespaceIssuer
		issuer.IssuerName = issuerName
	} else {
		issuer = *routeTrait.Spec.TLS
	}

	// Create Ingress
	// construct the serviceMonitor that hooks the service to the prometheus server
	ingress := constructNginxIngress(&routeTrait, issuer, svc, svcPort)
	// server side apply the serviceMonitor, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(routeTrait.GetUID())}
	if err := r.Patch(ctx, ingress, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to ingress")
		r.record.Event(eventObj, event.Warning(errApplyNginxIngress, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &routeTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, errApplyNginxIngress)))
	}
	r.record.Event(eventObj, event.Normal("Nginx Ingress created",
		fmt.Sprintf("successfully server side patched a route trait `%s`", routeTrait.Name)))

	// TODO(wonderflow): GC mechanism for no used ingress, service, issuer

	routeTrait.Status.Service = svc
	routeTrait.Status.Ingress = &runtimev1alpha1.TypedReference{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       reflect.TypeOf(v1beta1.Ingress{}).Name(),
		Name:       ingress.Name,
		UID:        routeTrait.UID,
	}
	return ctrl.Result{}, oamutil.PatchCondition(ctx, r, &routeTrait)
}

// create a service that targets the exposed workload pod
func (r *Reconciler) createService(ctx context.Context, mLog logr.Logger, workload *unstructured.Unstructured,
	routeTrait *v1alpha1.Route) (*runtimev1alpha1.TypedReference, int32, error) {

	oamService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       common.ServiceKind,
			APIVersion: common.ServiceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-" + workload.GetName(),
			Namespace: workload.GetNamespace(),
			Labels:    oamServiceLabel,
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
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}
	// assign selector
	if routeTrait.Spec.Backend != nil && len(routeTrait.Spec.Backend.SelectLabels) != 0 {
		oamService.Spec.Selector = routeTrait.Spec.Backend.SelectLabels
	}

	port, labels, err := DiscoverPortLabel(ctx, mLog, workload, r)
	if err == nil {
		if len(oamService.Spec.Selector) == 0 {
			oamService.Spec.Selector = labels
		}
		if routeTrait.Spec.Backend == nil {
			routeTrait.Spec.Backend = &standardv1alpha1.Backend{Port: port}
		} else if routeTrait.Spec.Backend.Port.String() == "0" {
			routeTrait.Spec.Backend.Port = port
		}
	} else {
		mLog.Info("[WARN] fail to discovery port and label", "err", err)
	}

	var servicePort int32 = 443
	oamService.Spec.Ports = []corev1.ServicePort{
		{
			Port:       servicePort,
			TargetPort: routeTrait.Spec.Backend.Port,
			Protocol:   corev1.ProtocolTCP,
		},
	}
	// server side apply the service, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(routeTrait.GetUID())}
	if err := r.Patch(ctx, oamService, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to service")
		return nil, servicePort, err
	}
	return &runtimev1alpha1.TypedReference{
		APIVersion: common.ServiceAPIVersion,
		Kind:       common.ServiceKind,
		Name:       oamService.Name,
		UID:        routeTrait.UID,
	}, servicePort, nil
}

// Assume the workload or it's childResource will always having spec.template as PodTemplate if discoverable
func DiscoverPortLabel(ctx context.Context, mLog logr.Logger, workload *unstructured.Unstructured, r client.Reader) (intstr.IntOrString, map[string]string, error) {
	var resources = []*unstructured.Unstructured{workload}
	// Fetch the child resources list from the corresponding workload
	childResources, err := oamutil.FetchWorkloadChildResources(ctx, mLog, r, workload)
	if err == nil {
		resources = append(resources, childResources...)
	} else {
		mLog.Info("[WARN] fail to fetch workload child resource", "name", workload.GetName(), "err", err)
	}
	var gatherErrs []error
	for _, w := range resources {
		port, labels, err := discoveryFromObject(w)
		if err == nil {
			return port, labels, nil
		}
		gatherErrs = append(gatherErrs, err)
	}
	return intstr.IntOrString{}, nil, fmt.Errorf("can't discovery port from workload %v %v.%v and it's child resource, errorList: %v", workload.GetName(), workload.GetAPIVersion(), workload.GetKind(), gatherErrs)
}

func discoveryFromObject(w *unstructured.Unstructured) (intstr.IntOrString, map[string]string, error) {
	obj, found, _ := unstructured.NestedMap(w.Object, "spec", "template")
	if !found {
		return intstr.IntOrString{}, nil, fmt.Errorf("not have spec.template in workload %v", w.GetName())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return intstr.IntOrString{}, nil, fmt.Errorf("workload %v convert object err %v", w.GetName(), err)
	}
	var template corev1.PodTemplate
	err = json.Unmarshal(data, &template)
	if err != nil {
		return intstr.IntOrString{}, nil, fmt.Errorf("workload %v convert object to PodTemplate err %v", w.GetName(), err)
	}
	port := getFirstPort(template.Template.Spec.Containers)
	if port == 0 {
		return intstr.IntOrString{}, nil, fmt.Errorf("no port found in workload %v", w.GetName())
	}
	return intstr.FromInt(int(port)), template.Labels, nil
}

func getFirstPort(cs []corev1.Container) int32 {
	//TODO(wonderflow): exclude some sidecars
	for _, container := range cs {
		for _, p := range container.Ports {
			return p.ContainerPort
		}
	}
	return 0
}

func (r *Reconciler) createSelfsignedIssuer(ctx context.Context, routeTrait *v1alpha1.Route) (string, error) {
	var selfSigned = "selfsigned"
	var issuer = certmanager.Issuer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: certmanager.SchemeGroupVersion.String(),
			Kind:       certmanager.IssuerKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfSigned,
			Namespace: routeTrait.Namespace,
		},
	}
	err := r.Client.Get(ctx, client.ObjectKey{Name: selfSigned, Namespace: routeTrait.Namespace}, &issuer)
	if err == nil {
		return selfSigned, nil
	}
	issuer.Spec.SelfSigned = &certmanager.SelfSignedIssuer{}
	if apierrors.IsNotFound(err) {
		return selfSigned, r.Client.Create(ctx, &issuer)
	}
	return selfSigned, fmt.Errorf("get %s err %v", selfSigned, err)
}

func constructNginxIngress(routeTrait *standardv1alpha1.Route, issuer standardv1alpha1.TLS, service *runtimev1alpha1.TypedReference, port int32) *v1beta1.Ingress {

	var annotations = make(map[string]string)

	// Use nginx-ingress as implementation
	annotations["kubernetes.io/ingress.class"] = "nginx"

	// SSL
	var issuerAnn = "cert-manager.io/issuer"
	if issuer.Type == standardv1alpha1.ClusterIssuer {
		issuerAnn = "cert-manager.io/cluster-issuer"
	}
	annotations[issuerAnn] = issuer.IssuerName

	// Rewrite
	if routeTrait.Spec.RewriteTarget != "" {
		annotations["ingress.kubernetes.io/rewrite-target"] = routeTrait.Spec.RewriteTarget
	}

	// Custom headers
	var headerSnippet string
	for k, v := range routeTrait.Spec.CustomHeaders {
		headerSnippet += fmt.Sprintf("more_set_headers \"%s: %s\";\n", k, v)
	}
	if headerSnippet != "" {
		annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = headerSnippet
	}
	backend := routeTrait.Spec.Backend
	if backend != nil {
		// Backend protocol
		if backend.Protocol != "" {
			annotations["nginx.ingress.kubernetes.io/backend-protocol"] = backend.Protocol
		}

		//Send timeout
		if backend.SendTimeout != 0 {
			annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = strconv.Itoa(backend.SendTimeout)
		}

		//Read timeout
		if backend.ReadTimeout != 0 {
			annotations["nginx.ingress.kubernetes.io/proxy‑read‑timeout"] = strconv.Itoa(backend.ReadTimeout)
		}
	}

	ingress := &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       reflect.TypeOf(v1beta1.Ingress{}).Name(),
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        routeTrait.Name,
			Namespace:   routeTrait.Namespace,
			Annotations: annotations,
			Labels:      oamServiceLabel,
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
	}
	ingress.Spec.TLS = []v1beta1.IngressTLS{
		{
			Hosts:      []string{routeTrait.Spec.Host},
			SecretName: routeTrait.Name + "-cert",
		},
	}
	if routeTrait.Spec.DefaultBackend != nil {
		ingress.Spec.Backend = routeTrait.Spec.DefaultBackend
	}
	ingress.Spec.Rules = []v1beta1.IngressRule{
		{
			Host: routeTrait.Spec.Host,
			IngressRuleValue: v1beta1.IngressRuleValue{HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: []v1beta1.HTTPIngressPath{
					{
						Path: routeTrait.Spec.Path,
						Backend: v1beta1.IngressBackend{
							ServiceName: service.Name,
							ServicePort: intstr.FromInt(int(port)),
						},
					},
				},
			}},
		},
	}
	return ingress
}

// fetch the service that is associated with the workload
func (r *Reconciler) fetchService(ctx context.Context, mLog logr.Logger,
	workload *unstructured.Unstructured, routeTrait *v1alpha1.Route) (*runtimev1alpha1.TypedReference, bool, int32, error) {
	// Fetch the child resources list from the corresponding workload
	resources, err := oamutil.FetchWorkloadChildResources(ctx, mLog, r, workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			mLog.Error(err, "Error while fetching the workload child resources", "workload kind", workload.GetKind(),
				"workload name", workload.GetName())
		}
		return nil, false, 0, err
	}

	// find the service that has the port
	for _, childRes := range resources {
		if childRes.GetAPIVersion() == corev1.SchemeGroupVersion.String() && childRes.GetKind() == reflect.TypeOf(corev1.Service{}).Name() {
			svc := &runtimev1alpha1.TypedReference{
				APIVersion: common.ServiceAPIVersion,
				Kind:       common.ServiceKind,
				Name:       childRes.GetName(),
				UID:        childRes.GetUID(),
			}
			ports, _, _ := unstructured.NestedSlice(childRes.Object, "spec", "ports")
			for _, port := range ports {
				data, _ := json.Marshal(port)
				var servicePort corev1.ServicePort
				_ = json.Unmarshal(data, &servicePort)
				if routeTrait.Spec.Backend == nil || routeTrait.Spec.Backend.Port.IntValue() == 0 || servicePort.TargetPort == routeTrait.Spec.Backend.Port {
					return svc, true, servicePort.Port, nil
				}
			}
		}
	}
	return nil, false, 0, nil
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
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Route"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
