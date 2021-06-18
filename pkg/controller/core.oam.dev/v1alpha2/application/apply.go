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

package application

import (
	"context"
	"fmt"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationrollout"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

type appHandler struct {
	r              *Reconciler
	app            *v1beta1.Application
	currentAppRev  *v1beta1.ApplicationRevision
	latestAppRev   *v1beta1.ApplicationRevision
	isNewRevision  bool
	currentRevHash string
}

func (h *appHandler) handleErr(err error) (ctrl.Result, error) {
	nerr := h.r.UpdateStatus(context.Background(), h.app)
	if err == nil && nerr == nil {
		return ctrl.Result{}, nil
	}
	if nerr != nil {
		klog.InfoS("Failed to update application status", "err", nerr)
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 10,
	}, nil
}

func (h *appHandler) applyAppManifests(ctx context.Context, comps []*types.ComponentManifest, policies []*unstructured.Unstructured) error {
	appRev := h.currentAppRev
	if h.app.Spec.Workflow != nil || h.app.Annotations[oam.AnnotationAppRevisionOnly] == "true" {
		return h.createResourcesConfigMap(ctx, appRev, comps, policies)
	}
	if appWillRollout(h.app) {
		return nil
	}

	var latestTracker *v1beta1.ResourceTracker
	if h.app.Status.LatestRevision != nil {
		latestTracker = &v1beta1.ResourceTracker{}
		latestTracker.SetName(dispatch.ConstructResourceTrackerName(h.app.Status.LatestRevision.Name, h.app.Namespace))
	}
	// only do GC when ALL resources are dispatched successfully
	// so skip GC while dispatching addon resources
	d := dispatch.NewAppManifestsDispatcher(h.r.Client, appRev).StartAndSkipGC(latestTracker)
	// dispatch packaged workload resources before dispatching assembled manifests
	for _, comp := range comps {
		if len(comp.PackagedWorkloadResources) != 0 {
			if _, err := d.Dispatch(ctx, comp.PackagedWorkloadResources); err != nil {
				return errors.WithMessage(err, "cannot dispatch packaged workload resources")
			}
		}
		if checkAutoDetectComponent(comp.StandardWorkload) {
			return fmt.Errorf("helm mode component doesn't specify workload, the traits attached to the helm mode component will fail to work")
		}
	}
	a := assemble.NewAppManifests(appRev).WithWorkloadOption(assemble.DiscoveryHelmBasedWorkload(ctx, h.r.Client))
	manifests, err := a.AssembledManifests()
	if err != nil {
		return errors.WithMessage(err, "cannot assemble application manifests")
	}
	if _, err := d.EndAndGC(latestTracker).Dispatch(ctx, manifests); err != nil {
		return errors.WithMessage(err, "cannot dispatch application manifests")
	}
	return nil
}

// checkAutoDetectComponent will check if the standardWorkload is empty,
// currently only Helm-based component is possible to be auto-detected
// TODO implement auto-detect mechanism
func checkAutoDetectComponent(wl *unstructured.Unstructured) bool {
	return wl == nil || (len(wl.GetAPIVersion()) == 0 && len(wl.GetKind()) == 0)
}

func (h *appHandler) aggregateHealthStatus(appFile *appfile.Appfile) ([]common.ApplicationComponentStatus, bool, error) {
	var appStatus []common.ApplicationComponentStatus
	var healthy = true
	for _, wl := range appFile.Workloads {
		var status = common.ApplicationComponentStatus{
			Name:               wl.Name,
			WorkloadDefinition: wl.FullTemplate.Reference.Definition,
			Healthy:            true,
		}

		var (
			outputSecretName string
			err              error
			pCtx             process.Context
		)

		if wl.IsCloudResourceProducer() {
			outputSecretName, err = appfile.GetOutputSecretNames(wl)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, setting outputSecretName error", appFile.Name, wl.Name)
			}
			pCtx.InsertSecrets(outputSecretName, wl.RequiredSecrets)
		}

		switch wl.CapabilityCategory {
		case types.TerraformCategory:
			pCtx = appfile.NewBasicContext(wl, appFile.Name, appFile.RevisionName, appFile.Namespace)
			ctx := context.Background()
			var configuration terraformapi.Configuration
			if err := h.r.Client.Get(ctx, client.ObjectKey{Name: wl.Name, Namespace: h.app.Namespace}, &configuration); err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appFile.Name, wl.Name)
			}
			if configuration.Status.State != terraformtypes.Available {
				healthy = false
				status.Healthy = false
			} else {
				status.Healthy = true
			}
			status.Message = configuration.Status.Message
		default:
			pCtx = process.NewContext(h.app.Namespace, wl.Name, appFile.Name, appFile.RevisionName)
			if err := wl.EvalContext(pCtx); err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate context error", appFile.Name, wl.Name)
			}
			workloadHealth, err := wl.EvalHealth(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appFile.Name, wl.Name)
			}
			if !workloadHealth {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				status.Healthy = false
				healthy = false
			}

			status.Message, err = wl.EvalStatus(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate workload status message error", appFile.Name, wl.Name)
			}
		}

		var traitStatusList []common.ApplicationTraitStatus
		for _, tr := range wl.Traits {
			if err := tr.EvalContext(pCtx); err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate context error", appFile.Name, wl.Name, tr.Name)
			}

			var traitStatus = common.ApplicationTraitStatus{
				Type:    tr.Name,
				Healthy: true,
			}
			traitHealth, err := tr.EvalHealth(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, check health error", appFile.Name, wl.Name, tr.Name)
			}
			if !traitHealth {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				traitStatus.Healthy = false
				healthy = false
			}
			traitStatus.Message, err = tr.EvalStatus(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate status message error", appFile.Name, wl.Name, tr.Name)
			}
			traitStatusList = append(traitStatusList, traitStatus)
		}

		status.Traits = traitStatusList
		status.Scopes = generateScopeReference(wl.Scopes)
		appStatus = append(appStatus, status)
	}
	return appStatus, healthy, nil
}

func generateScopeReference(scopes []appfile.Scope) []runtimev1alpha1.TypedReference {
	var references []runtimev1alpha1.TypedReference
	for _, scope := range scopes {
		references = append(references, runtimev1alpha1.TypedReference{
			APIVersion: scope.GVK.GroupVersion().String(),
			Kind:       scope.GVK.Kind,
			Name:       scope.Name,
		})
	}
	return references
}

type garbageCollectFunc func(ctx context.Context, h *appHandler) error

// execute garbage collection functions, including:
// - clean up legacy app revisions
// - clean up legacy component revisions
func garbageCollection(ctx context.Context, h *appHandler) error {
	collectFuncs := []garbageCollectFunc{
		garbageCollectFunc(cleanUpApplicationRevision),
		garbageCollectFunc(cleanUpComponentRevision),
	}
	for _, collectFunc := range collectFuncs {
		if err := collectFunc(ctx, h); err != nil {
			return err
		}
	}
	return nil
}

func (h *appHandler) handleRollout(ctx context.Context) (reconcile.Result, error) {
	var comps []string
	for _, component := range h.app.Spec.Components {
		comps = append(comps, component.Name)
	}

	// targetRevision should always points to LatestRevison
	targetRevision := h.app.Status.LatestRevision.Name
	var srcRevision string
	target, _ := oamutil.ExtractRevisionNum(targetRevision, "-")
	// if target == 1 this is a initial scale operation, sourceRevision should be empty
	// otherwise source revision always is targetRevision - 1
	if target > 1 {
		srcRevision = utils.ConstructRevisionName(h.app.Name, int64(target-1))
	}

	appRollout := v1beta1.AppRollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.app.Name,
			Namespace: h.app.Namespace,
			UID:       h.app.UID,
		},
		Spec: v1beta1.AppRolloutSpec{
			SourceAppRevisionName: srcRevision,
			TargetAppRevisionName: targetRevision,
			ComponentList:         comps,
			RolloutPlan:           *h.app.Spec.RolloutPlan,
		},
		Status: h.app.Status.Rollout,
	}

	// construct a fake rollout object and call rollout.DoReconcile
	r := applicationrollout.NewReconciler(h.r.Client, h.r.dm, h.r.Recorder, h.r.Scheme)
	res, err := r.DoReconcile(ctx, &appRollout)
	if err != nil {
		return reconcile.Result{}, err
	}

	// write back rollout status to application
	h.app.Status.Rollout = appRollout.Status
	return res, nil
}
