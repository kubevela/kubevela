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

package metrics

import (
	"context"
	"fmt"
	"reflect"

	monitoring "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/api/v1alpha1"
)

const (
	errApplyServiceMonitor = "failed to apply the service monitor"
	errLocatingWorkload    = "failed to locate the workload"
	errLocatingService     = "failed to locate any the services"
	errCreatingService     = "failed to create the services"
	servicePort            = 4848
)

var (
	serviceMonitorKind       = reflect.TypeOf(monitoring.ServiceMonitor{}).Name()
	serviceMonitorAPIVersion = monitoring.SchemeGroupVersion.String()
	serviceKind              = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion        = corev1.SchemeGroupVersion.String()
)

var (
	// oamServiceLabel is the pre-defined labels for any serviceMonitor
	// created by the MetricsTrait,  prometheus operator listens on this
	oamServiceLabel = map[string]string{
		"k8s-app":    "oam",
		"controller": "metricsTrait",
	}
	// ServiceMonitorNSName is the name of the namespace in which the serviceMonitor resides
	// it must be the same that the prometheus operator is listening to
	ServiceMonitorNSName = "monitoring"
)

// MetricsTraitReconciler reconciles a MetricsTrait object
type MetricsTraitReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	record event.Recorder
}

// +kubebuilder:rbac:groups=standard.oam.dev,resources=metricstraits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=metricstraits/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=*/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=*/status,verbs=get;
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;create;update;patch

func (r *MetricsTraitReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	mLog := r.Log.WithValues("metricstrait", req.NamespacedName)
	mLog.Info("Reconcile metricstrait trait")
	// fetch the trait
	var metricsTrait v1alpha1.MetricsTrait
	if err := r.Get(ctx, req.NamespacedName, &metricsTrait); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	mLog.Info("Get the metricsTrait trait",
		"metrics end point", metricsTrait.Spec.ScrapeService,
		"workload reference", metricsTrait.Spec.WorkloadReference,
		"labels", metricsTrait.GetLabels())

	// find the resource object to record the event to, default is the parent appConfig.
	eventObj, err := oamutil.LocateParentAppConfig(ctx, r.Client, &metricsTrait)
	if eventObj == nil {
		// fallback to workload itself
		mLog.Error(err, "add events to metricsTrait itself", "name", metricsTrait.Name)
		eventObj = &metricsTrait
	}
	if metricsTrait.Spec.ScrapeService.Enabled != nil && !*metricsTrait.Spec.ScrapeService.Enabled {
		r.record.Event(eventObj, event.Normal("Metrics Trait disabled", "no op"))
		r.gcOrphanServiceMonitor(ctx, mLog, &metricsTrait)
		return ctrl.Result{}, oamutil.PatchCondition(ctx, r, &metricsTrait, cpv1alpha1.ReconcileSuccess())
	}

	// Fetch the workload instance to which we want to expose metrics
	workload, err := oamutil.FetchWorkload(ctx, r, mLog, &metricsTrait)
	if err != nil {
		mLog.Error(err, "Error while fetching the workload", "workload reference",
			metricsTrait.GetWorkloadReference())
		r.record.Event(eventObj, event.Warning(errLocatingWorkload, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &metricsTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, errLocatingWorkload)))
	}
	// try to see if the workload already has services as child resources
	serviceLabel, err := r.fetchServicesLabel(ctx, mLog, workload, metricsTrait.Spec.ScrapeService.TargetPort)
	if err != nil && !apierrors.IsNotFound(err) {
		r.record.Event(eventObj, event.Warning(errLocatingService, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &metricsTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, errLocatingService)))
	} else if serviceLabel == nil {
		// TODO: use podMonitor instead?
		// no service with the targetPort found, we will create a service that talks to the targetPort
		serviceLabel, err = r.createService(ctx, mLog, workload, &metricsTrait)
		if err != nil {
			r.record.Event(eventObj, event.Warning(errCreatingService, err))
			return oamutil.ReconcileWaitResult,
				oamutil.PatchCondition(ctx, r, &metricsTrait,
					cpv1alpha1.ReconcileError(errors.Wrap(err, errCreatingService)))
		}
	}
	// construct the serviceMonitor that hooks the service to the prometheus server
	serviceMonitor := constructServiceMonitor(&metricsTrait, serviceLabel)
	// server side apply the serviceMonitor, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(metricsTrait.GetUID())}
	if err := r.Patch(ctx, serviceMonitor, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to serviceMonitor")
		r.record.Event(eventObj, event.Warning(errApplyServiceMonitor, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &metricsTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, errApplyServiceMonitor)))
	}
	r.record.Event(eventObj, event.Normal("ServiceMonitor created",
		fmt.Sprintf("successfully server side patched a serviceMonitor `%s`", serviceMonitor.Name)))

	r.gcOrphanServiceMonitor(ctx, mLog, &metricsTrait)

	return ctrl.Result{}, oamutil.PatchCondition(ctx, r, &metricsTrait, cpv1alpha1.ReconcileSuccess())
}

// fetch the label of the service that is associated with the workload
func (r *MetricsTraitReconciler) fetchServicesLabel(ctx context.Context, mLog logr.Logger,
	workload *unstructured.Unstructured, targetPort intstr.IntOrString) (map[string]string, error) {
	// Fetch the child resources list from the corresponding workload
	resources, err := oamutil.FetchWorkloadChildResources(ctx, mLog, r, workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			mLog.Error(err, "Error while fetching the workload child resources", "workload kind", workload.GetKind(),
				"workload name", workload.GetName())
		}
		return nil, err
	}
	// find the service that has the port
	for _, childRes := range resources {
		if childRes.GetAPIVersion() == serviceAPIVersion && childRes.GetKind() == serviceKind {
			ports, _, _ := unstructured.NestedSlice(childRes.Object, "spec", "ports")
			for _, port := range ports {
				servicePort, _ := port.(corev1.ServicePort)
				if servicePort.TargetPort == targetPort {
					return childRes.GetLabels(), nil
				}
			}
		}
	}
	return nil, nil
}

// create a service that targets the exposed workload pod
func (r *MetricsTraitReconciler) createService(ctx context.Context, mLog logr.Logger, workload *unstructured.Unstructured,
	metricsTrait *v1alpha1.MetricsTrait) (map[string]string, error) {
	oamService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceKind,
			APIVersion: serviceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oam-" + workload.GetName(),
			Namespace: workload.GetNamespace(),
			Labels:    oamServiceLabel,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	// assign selector
	if len(metricsTrait.Spec.ScrapeService.TargetSelector) == 0 {
		// default is that we assumed that the pods have the same label as the workload
		// we might be able to find the podSpec label but it is more complicated
		oamService.Spec.Selector = workload.GetLabels()
	} else {
		oamService.Spec.Selector = metricsTrait.Spec.ScrapeService.TargetSelector
	}
	oamService.Spec.Ports = []corev1.ServicePort{
		{
			Port:       servicePort,
			TargetPort: metricsTrait.Spec.ScrapeService.TargetPort,
			Protocol:   corev1.ProtocolTCP,
		},
	}
	// server side apply the service, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(metricsTrait.GetUID())}
	if err := r.Patch(ctx, oamService, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to service")
		return nil, err
	}
	return oamServiceLabel, nil
}

// remove all service monitors that are no longer used
func (r *MetricsTraitReconciler) gcOrphanServiceMonitor(ctx context.Context, mLog logr.Logger,
	metricsTrait *v1alpha1.MetricsTrait) {
	var gcCandidates []string
	copy(metricsTrait.Status.ServiceMonitorNames, gcCandidates)
	if metricsTrait.Spec.ScrapeService.Enabled != nil && !*metricsTrait.Spec.ScrapeService.Enabled {
		// initialize it to be an empty list, gc everything
		metricsTrait.Status.ServiceMonitorNames = []string{}
	} else {
		// re-initialize to the current service monitor
		metricsTrait.Status.ServiceMonitorNames = []string{metricsTrait.Name}
	}
	for _, smn := range gcCandidates {
		if smn != metricsTrait.Name {
			if err := r.Delete(ctx, &monitoring.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{
					Kind:       serviceMonitorKind,
					APIVersion: serviceMonitorAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      smn,
					Namespace: metricsTrait.GetNamespace(),
				},
			}, client.GracePeriodSeconds(10)); err != nil {
				mLog.Error(err, "Failed to delete serviceMonitor", "name", smn, "error", err)
				// add it back
				metricsTrait.Status.ServiceMonitorNames = append(metricsTrait.Status.ServiceMonitorNames, smn)
			}
		}
	}
}

// construct a serviceMonitor given a metrics trait along with a label selector pointing to the underlying service
func constructServiceMonitor(metricsTrait *v1alpha1.MetricsTrait,
	serviceLabels map[string]string) *monitoring.ServiceMonitor {
	return &monitoring.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceMonitorKind,
			APIVersion: serviceMonitorAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      metricsTrait.Name,
			Namespace: ServiceMonitorNSName,
			Labels:    oamServiceLabel,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         metricsTrait.GetObjectKind().GroupVersionKind().GroupVersion().String(),
					Kind:               metricsTrait.GetObjectKind().GroupVersionKind().Kind,
					UID:                metricsTrait.GetUID(),
					Name:               metricsTrait.GetName(),
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: monitoring.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: serviceLabels,
			},
			// we assumed that the service is in the same namespace as the trait
			NamespaceSelector: monitoring.NamespaceSelector{
				MatchNames: []string{metricsTrait.Namespace},
			},
			Endpoints: []monitoring.Endpoint{
				{
					TargetPort: &metricsTrait.Spec.ScrapeService.TargetPort,
					Path:       metricsTrait.Spec.ScrapeService.Path,
					Scheme:     metricsTrait.Spec.ScrapeService.Scheme,
				},
			},
		},
	}
}

func (r *MetricsTraitReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("MetricsTrait")).
		WithAnnotations("controller", "metricsTrait")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MetricsTrait{}).
		Owns(&monitoring.ServiceMonitor{}).
		Complete(r)
}

// Setup adds a controller that reconciles MetricsTrait.
func Setup(mgr ctrl.Manager) error {
	reconciler := MetricsTraitReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("MetricsTrait"),
		Scheme: mgr.GetScheme(),
	}
	return reconciler.SetupWithManager(mgr)
}
