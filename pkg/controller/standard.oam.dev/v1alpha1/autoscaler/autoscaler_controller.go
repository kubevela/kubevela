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

package autoscalers

import (
	"context"
	"fmt"
	"time"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
)

// nolint:golint
const (
	SpecWarningTargetWorkloadNotSet                = "Spec.targetWorkload is not set"
	SpecWarningStartAtTimeFormat                   = "startAt is not in the right format, which should be like `12:01`"
	SpecWarningStartAtTimeRequired                 = "spec.triggers.condition.startAt: Required value"
	SpecWarningDurationTimeRequired                = "spec.triggers.condition.duration: Required value"
	SpecWarningReplicasRequired                    = "spec.triggers.condition.replicas: Required value"
	SpecWarningDurationTimeNotInRightFormat        = "spec.triggers.condition.duration: not in the right format"
	SpecWarningSumOfStartAndDurationMoreThan24Hour = "the sum of the start hour and the duration hour has to be less than 24 hours."
)

// ReconcileWaitResult is the time to wait between reconciliation.
var ReconcileWaitResult = reconcile.Result{RequeueAfter: 30 * time.Second}

// AutoscalerReconciler reconciles a Autoscaler object
type AutoscalerReconciler struct {
	client.Client

	dm     discoverymapper.DiscoveryMapper
	Log    logr.Logger
	Scheme *runtime.Scheme
	record event.Recorder
}

// Reconcile is the main logic for autoscaler controller
// +kubebuilder:rbac:groups=standard.oam.dev,resources=autoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=autoscalers/status,verbs=get;update;patch
func (r *AutoscalerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("autoscaler", req.NamespacedName)
	log.Info("Reconciling Autoscaler...")
	ctx := context.Background()
	var scaler v1alpha1.Autoscaler
	if err := r.Get(ctx, req.NamespacedName, &scaler); err != nil {
		log.Error(err, "Failed to get trait", "traitName", scaler.Name)
		return ReconcileWaitResult, client.IgnoreNotFound(err)
	}
	log.Info("Retrieved trait Autoscaler", "APIVersion", scaler.APIVersion, "Kind", scaler.Kind)

	// find the resource object to record the event to, default is the parent appConfig.
	eventObj, err := util.LocateParentAppConfig(ctx, r.Client, &scaler)
	if err != nil {
		log.Error(err, "Failed to find the parent resource", "Autoscaler", scaler.Name)
		return util.ReconcileWaitResult, util.PatchCondition(ctx, r, &scaler,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrLocateAppConfig)))
	}
	if eventObj == nil {
		// fallback to workload itself
		log.Info("There is no parent resource", "Autoscaler", scaler.Name)
		eventObj = &scaler
	}

	// Fetch the instance to which the trait refers to
	workload, err := oamutil.FetchWorkload(ctx, r, log, &scaler)
	if err != nil {
		log.Error(err, "Error while fetching the workload", "workload reference",
			scaler.GetWorkloadReference())
		r.record.Event(&scaler, event.Warning(common.ErrLocatingWorkload, err))
		return oamutil.ReconcileWaitResult,
			oamutil.PatchCondition(ctx, r, &scaler,
				cpv1alpha1.ReconcileError(errors.Wrap(err, common.ErrLocatingWorkload)))
	}

	// Fetch the child resources list from the corresponding workload
	resources, err := util.FetchWorkloadChildResources(ctx, log, r, r.dm, workload)
	if err != nil {
		log.Error(err, "Error while fetching the workload child resources", "workload", workload.UnstructuredContent())
		r.record.Event(eventObj, event.Warning(util.ErrFetchChildResources, err))
		return util.ReconcileWaitResult, util.PatchCondition(ctx, r, &scaler,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrFetchChildResources)))
	}
	resources = append(resources, workload)

	targetWorkloadSetFlag := false
	for _, res := range resources {
		// Keda only support these four built-in workload now.
		if res.GetKind() == "Deployment" || res.GetKind() == "StatefulSet" || res.GetKind() == "DaemonSet" || res.GetKind() == "ReplicaSet" {
			scaler.Spec.TargetWorkload = v1alpha1.TargetWorkload{
				APIVersion: res.GetAPIVersion(),
				Kind:       res.GetKind(),
				Name:       res.GetName(),
			}
			targetWorkloadSetFlag = true
			break
		}
	}

	// if no child resource found, set the workload as target workload
	if !targetWorkloadSetFlag {
		scaler.Spec.TargetWorkload = v1alpha1.TargetWorkload{
			APIVersion: workload.GetAPIVersion(),
			Kind:       workload.GetKind(),
			Name:       workload.GetName(),
		}
	}

	namespace := req.NamespacedName.Namespace
	if err := r.scaleByKEDA(scaler, namespace, log); err != nil {
		return ReconcileWaitResult, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager will setup with event recorder
func (r *AutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Autoscaler")).
		WithAnnotations("controller", "Autoscaler")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Autoscaler{}).
		Complete(r)
}

// Setup adds a controller that reconciles MetricsTrait.
func Setup(mgr ctrl.Manager) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	r := AutoscalerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("Autoscaler"),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return r.SetupWithManager(mgr)
}
