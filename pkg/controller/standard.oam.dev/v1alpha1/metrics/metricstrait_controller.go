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
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

const (
	errApplyServiceMonitor = "failed to apply the service monitor"
	errFailDiscoveryLabels = "failed to discover labels from pod template, use workload labels directly"
	servicePort            = 4848
)

var (
	serviceMonitorKind       = reflect.TypeOf(monitoring.ServiceMonitor{}).Name()
	serviceMonitorAPIVersion = monitoring.SchemeGroupVersion.String()
)

var (
	// ServiceMonitorNSName is the name of the namespace in which the serviceMonitor resides
	// it must be the same that the prometheus operator is listening to
	ServiceMonitorNSName = "monitoring"
)

// GetOAMServiceLabel will return oamServiceLabel as the pre-defined labels for any serviceMonitor
// created by the MetricsTrait,  prometheus operator listens on this
func GetOAMServiceLabel() map[string]string {
	return map[string]string{
		"k8s-app":    "oam",
		"controller": "metricsTrait",
	}
}

// Reconciler reconciles a MetricsTrait object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	Log    logr.Logger
	Scheme *runtime.Scheme
	record event.Recorder
}

// Reconcile is the main logic for metric trait controller
// +kubebuilder:rbac:groups=standard.oam.dev,resources=metricstraits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=metricstraits/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=*/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=*/status,verbs=get;
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;create;update;patch
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
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

	ctx = oamutil.SetNnamespaceInCtx(ctx, metricsTrait.Namespace)

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
		r.record.Event(eventObj, event.Warning(common.ErrLocatingWorkload, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &metricsTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingWorkload)))
	}
	var targetPort = metricsTrait.Spec.ScrapeService.TargetPort
	// try to see if the workload already has services as child resources
	serviceLabel, err := r.fetchServicesLabel(ctx, mLog, workload, targetPort)
	if err != nil && !apierrors.IsNotFound(err) {
		r.record.Event(eventObj, event.Warning(common.ErrLocatingService, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &metricsTrait,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingService)))
	} else if serviceLabel == nil {
		// TODO: use podMonitor instead?
		// no service with the targetPort found, we will create a service that talks to the targetPort
		serviceLabel, targetPort, err = r.createService(ctx, mLog, workload, &metricsTrait)
		if err != nil {
			r.record.Event(eventObj, event.Warning(common.ErrCreatingService, err))
			return oamutil.ReconcileWaitResult,
				oamutil.PatchCondition(ctx, r, &metricsTrait,
					cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrCreatingService)))
		}
	}

	metricsTrait.Status.Port = targetPort
	metricsTrait.Status.SelectorLabels = serviceLabel

	// construct the serviceMonitor that hooks the service to the prometheus server
	serviceMonitor := constructServiceMonitor(&metricsTrait, targetPort)
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
	(&metricsTrait).SetConditions(cpv1alpha1.ReconcileSuccess())
	return ctrl.Result{}, errors.Wrap(r.UpdateStatus(ctx, &metricsTrait), common.ErrUpdateStatus)
}

// fetch the label of the service that is associated with the workload
func (r *Reconciler) fetchServicesLabel(ctx context.Context, mLog logr.Logger,
	workload *unstructured.Unstructured, targetPort intstr.IntOrString) (map[string]string, error) {
	// Fetch the child resources list from the corresponding workload
	resources, err := oamutil.FetchWorkloadChildResources(ctx, mLog, r, r.dm, workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			mLog.Error(err, "Error while fetching the workload child resources", "workload kind", workload.GetKind(),
				"workload name", workload.GetName())
		}
		return nil, err
	}
	// find the service that has the port
	for _, childRes := range resources {
		if childRes.GetAPIVersion() == common.ServiceAPIVersion && childRes.GetKind() == common.ServiceKind {
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
func (r *Reconciler) createService(ctx context.Context, mLog logr.Logger, workload *unstructured.Unstructured,
	metricsTrait *v1alpha1.MetricsTrait) (map[string]string, intstr.IntOrString, error) {
	oamService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       common.ServiceKind,
			APIVersion: common.ServiceAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "oam-" + workload.GetName(),
			Namespace: workload.GetNamespace(),
			Labels:    GetOAMServiceLabel(),
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
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	var targetPort = metricsTrait.Spec.ScrapeService.TargetPort
	// assign selector
	ports, labels, err := utils.DiscoveryFromPodTemplate(workload, "spec", "template")
	if err != nil {
		mLog.Info(errFailDiscoveryLabels, "err", err)
		if len(metricsTrait.Spec.ScrapeService.TargetSelector) == 0 {
			// we assumed that the pods have the same label as the workload if no discoverable
			oamService.Spec.Selector = workload.GetLabels()
		} else {
			oamService.Spec.Selector = metricsTrait.Spec.ScrapeService.TargetSelector
		}
	} else {
		oamService.Spec.Selector = labels
	}
	if targetPort.String() == "0" {
		if len(ports) == 0 {
			return nil, intstr.IntOrString{}, fmt.Errorf("no ports discovered or specified")
		}
		// choose the first one if no port specified
		targetPort = ports[0]
	}
	oamService.Spec.Ports = []corev1.ServicePort{
		{
			Port:       servicePort,
			TargetPort: targetPort,
			Protocol:   corev1.ProtocolTCP,
		},
	}
	// server side apply the service, only the fields we set are touched
	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner(metricsTrait.GetUID())}
	if err := r.Patch(ctx, oamService, client.Apply, applyOpts...); err != nil {
		mLog.Error(err, "Failed to apply to service")
		return nil, intstr.IntOrString{}, err
	}
	return oamService.Spec.Selector, targetPort, nil
}

// remove all service monitors that are no longer used
func (r *Reconciler) gcOrphanServiceMonitor(ctx context.Context, mLog logr.Logger,
	metricsTrait *v1alpha1.MetricsTrait) {
	var gcCandidate = metricsTrait.Status.ServiceMonitorName
	if metricsTrait.Spec.ScrapeService.Enabled != nil && !*metricsTrait.Spec.ScrapeService.Enabled {
		// initialize it to be an empty list, gc everything
		metricsTrait.Status.ServiceMonitorName = ""
	} else {
		// re-initialize to the current service monitor
		metricsTrait.Status.ServiceMonitorName = metricsTrait.Name
	}
	if gcCandidate == metricsTrait.Name {
		return
	}
	if err := r.Delete(ctx, &monitoring.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceMonitorKind,
			APIVersion: serviceMonitorAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gcCandidate,
			Namespace: metricsTrait.GetNamespace(),
		},
	}, client.GracePeriodSeconds(10)); err != nil {
		mLog.Error(err, "Failed to delete serviceMonitor", "name", gcCandidate, "error", err)
	}
}

// construct a serviceMonitor given a metrics trait along with a label selector pointing to the underlying service
func constructServiceMonitor(metricsTrait *v1alpha1.MetricsTrait, targetPort intstr.IntOrString) *monitoring.ServiceMonitor {
	return &monitoring.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       serviceMonitorKind,
			APIVersion: serviceMonitorAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      metricsTrait.Name,
			Namespace: ServiceMonitorNSName,
			Labels:    GetOAMServiceLabel(),
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
				MatchLabels: GetOAMServiceLabel(),
			},
			// we assumed that the service is in the same namespace as the trait
			NamespaceSelector: monitoring.NamespaceSelector{
				MatchNames: []string{metricsTrait.Namespace},
			},
			Endpoints: []monitoring.Endpoint{
				{
					TargetPort: &targetPort,
					Path:       metricsTrait.Spec.ScrapeService.Path,
					Scheme:     metricsTrait.Spec.ScrapeService.Scheme,
				},
			},
		},
	}
}

// SetupWithManager setup Reconciler with ctrl.Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("MetricsTrait")).
		WithAnnotations("controller", "metricsTrait")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MetricsTrait{}).
		Owns(&monitoring.ServiceMonitor{}).
		Complete(r)
}

// UpdateStatus updates v1alpha1.MetricsTrait's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, mt *v1alpha1.MetricsTrait, opts ...client.UpdateOption) error {
	status := mt.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, types.NamespacedName{Namespace: mt.Namespace, Name: mt.Name}, mt); err != nil {
			return
		}
		mt.Status = status
		return r.Status().Update(ctx, mt, opts...)
	})
}

// Setup adds a controller that reconciles MetricsTrait.
func Setup(mgr ctrl.Manager) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	reconciler := Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("MetricsTrait"),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return reconciler.SetupWithManager(mgr)
}
