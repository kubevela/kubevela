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
	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
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

func errorCondition(tpy string, err error) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               runtimev1alpha1.ConditionType(tpy),
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             runtimev1alpha1.ReasonReconcileError,
		Message:            err.Error(),
	}
}

func readyCondition(tpy string) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               runtimev1alpha1.ConditionType(tpy),
		Status:             corev1.ConditionTrue,
		Reason:             runtimev1alpha1.ReasonAvailable,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

type appHandler struct {
	r                    *Reconciler
	app                  *v1beta1.Application
	appfile              *appfile.Appfile
	previousRevisionName string
	isNewRevision        bool
	revisionHash         string
	autodetect           bool
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

func (h *appHandler) apply(ctx context.Context, appRev *v1beta1.ApplicationRevision, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component, policies []*unstructured.Unstructured) error {

	if h.app.Spec.Workflow != nil || ac.Annotations[oam.AnnotationAppRevisionOnly] == "true" {
		h.FinalizeAppRevision(appRev, ac, comps)
		err := h.createOrUpdateAppRevision(ctx, appRev)
		if err != nil {
			return err
		}
		return h.createResourcesConfigMap(ctx, appRev, ac, comps, policies)
	}

	owners := []metav1.OwnerReference{{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       v1beta1.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(true),
	}}
	ac.SetOwnerReferences(owners)
	var err error
	for _, comp := range comps {
		comp.SetOwnerReferences(owners)

		if h.isNewRevision && h.checkAutoDetect(comp) {
			if err = h.applyHelmModuleResources(ctx, comp, owners); err != nil {
				return errors.Wrap(err, "cannot apply Helm module resources")
			}
			continue
		}

		newComp := comp.DeepCopy()
		// newComp will be updated and return the revision name instead of the component name
		revisionName, err := h.createOrUpdateComponent(ctx, newComp)
		if err != nil {
			return err
		}
		for i := 0; i < len(ac.Spec.Components); i++ {
			// update the AC using the component revision instead of component name
			// we have to make AC immutable including the component it's pointing to
			if ac.Spec.Components[i].ComponentName == newComp.Name {
				ac.Spec.Components[i].RevisionName = revisionName
				ac.Spec.Components[i].ComponentName = ""
			}
		}
		// isNewRevision indicates app's newly created or spec has changed
		// skip applying helm resources if no spec change
		if h.isNewRevision && comp.Spec.Helm != nil {
			if err = h.applyHelmModuleResources(ctx, comp, owners); err != nil {
				return errors.Wrap(err, "cannot apply Helm module resources")
			}
		}
	}
	h.FinalizeAppRevision(appRev, ac, comps)

	if h.autodetect {
		// TODO(yangsoon) autodetect is temporarily not implemented
		return fmt.Errorf("helm mode component doesn't specify workload, the traits attached to the helm mode component will fail to work")
	}

	if err := h.createOrUpdateAppRevision(ctx, appRev); err != nil {
		return err
	}

	if !appWillReleaseByRollout(h.app) && !h.autodetect {
		a := assemble.NewAppManifests(appRev).WithWorkloadOption(assemble.DiscoveryHelmBasedWorkload(ctx, h.r.Client))
		manifests, err := a.AssembledManifests()
		if err != nil {
			return errors.WithMessage(err, "cannot assemble resources' manifests")
		}
		d := dispatch.NewAppManifestsDispatcher(h.r.Client, appRev)
		if len(h.previousRevisionName) != 0 {
			latestTracker := &v1beta1.ResourceTracker{}
			latestTracker.SetName(dispatch.ConstructResourceTrackerName(h.previousRevisionName, h.app.Namespace))
			d = d.EnableUpgradeAndGC(latestTracker)
		}
		if _, err := d.Dispatch(ctx, manifests); err != nil {
			return errors.WithMessage(err, "cannot dispatch resources' manifests")
		}
	}
	return nil
}

func (h *appHandler) createOrUpdateAppRevision(ctx context.Context, appRev *v1beta1.ApplicationRevision) error {
	if appRev.Labels == nil {
		appRev.Labels = make(map[string]string)
	}
	appRev.SetLabels(oamutil.MergeMapOverrideWithDst(appRev.Labels, map[string]string{oam.LabelAppName: h.app.Name}))

	if h.isNewRevision {
		var revisionNum int64
		appRev.Name, revisionNum = utils.GetAppNextRevision(h.app)
		// only new revision update the status
		if err := h.UpdateRevisionStatus(ctx, appRev.Name, h.revisionHash, revisionNum); err != nil {
			return err
		}
		return h.r.Create(ctx, appRev)
	}

	return h.r.Update(ctx, appRev)
}

func (h *appHandler) statusAggregate(appFile *appfile.Appfile) ([]common.ApplicationComponentStatus, bool, error) {
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

// createOrUpdateComponent creates a component if not exist and update if exists.
// it returns the corresponding component revisionName and if a new component revision is created
func (h *appHandler) createOrUpdateComponent(ctx context.Context, comp *v1alpha2.Component) (string, error) {
	curComp := v1alpha2.Component{}
	var preRevisionName, curRevisionName string
	compName := comp.GetName()
	compNameSpace := comp.GetNamespace()
	compKey := ctypes.NamespacedName{Name: compName, Namespace: compNameSpace}

	err := h.r.Get(ctx, compKey, &curComp)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return "", err
		}
		if err = h.r.Create(ctx, comp); err != nil {
			return "", err
		}
		klog.InfoS("Created a new component", "component", klog.KObj(comp))
	} else {
		// remember the revision if there is a previous component
		if curComp.Status.LatestRevision != nil {
			preRevisionName = curComp.Status.LatestRevision.Name
		}
		comp.ResourceVersion = curComp.ResourceVersion
		if err := h.r.Update(ctx, comp); err != nil {
			return "", err
		}
		klog.InfoS("Updated a component", "component", klog.KObj(comp))
	}
	// remove the object from the raw extension before we can compare with the existing componentRevision whose
	// object is persisted as Raw data after going through api server
	updatedComp := comp.DeepCopy()
	updatedComp.Spec.Workload.Object = nil
	if updatedComp.Spec.Helm != nil {
		updatedComp.Spec.Helm.Release.Object = nil
		updatedComp.Spec.Helm.Repository.Object = nil
	}
	if len(preRevisionName) != 0 {
		needNewRevision, err := utils.CompareWithRevision(ctx, h.r, compName, compNameSpace, preRevisionName, &updatedComp.Spec)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("compare with existing controllerRevision %s failed",
				preRevisionName))
		}
		if !needNewRevision {
			klog.InfoS("No need to wait for a new component revision", "component", klog.KObj(updatedComp),
				"revision", preRevisionName)
			return preRevisionName, nil
		}
	}
	klog.InfoS("Wait for a new component revision", "component name", compName,
		"previous revision", preRevisionName)
	// get the new component revision that contains the component with retry
	checkForRevision := func() (bool, error) {
		if err := h.r.Get(ctx, compKey, &curComp); err != nil {
			// retry no matter what
			// nolint:nilerr
			return false, nil
		}
		if curComp.Status.LatestRevision == nil || curComp.Status.LatestRevision.Name == preRevisionName {
			return false, nil
		}
		needNewRevision, err := utils.CompareWithRevision(ctx, h.r, compName,
			compNameSpace, curComp.Status.LatestRevision.Name, &updatedComp.Spec)
		if err != nil {
			// retry no matter what
			// nolint:nilerr
			return false, nil
		}
		// end the loop if we find the revision
		if !needNewRevision {
			curRevisionName = curComp.Status.LatestRevision.Name
			klog.InfoS("Get a matching component revision", "component name", compName,
				"current revision", curRevisionName)
		}
		return !needNewRevision, nil
	}
	if err := wait.ExponentialBackoff(utils.DefaultBackoff, checkForRevision); err != nil {
		return "", err
	}
	return curRevisionName, nil
}

func (h *appHandler) applyHelmModuleResources(ctx context.Context, comp *v1alpha2.Component, owners []metav1.OwnerReference) error {
	klog.Info("Process a Helm module component")
	repo, err := oamutil.RawExtension2Unstructured(&comp.Spec.Helm.Repository)
	if err != nil {
		return err
	}
	release, err := oamutil.RawExtension2Unstructured(&comp.Spec.Helm.Release)
	if err != nil {
		return err
	}

	release.SetOwnerReferences(owners)
	repo.SetOwnerReferences(owners)

	if err := h.r.applicator.Apply(ctx, repo); err != nil {
		return err
	}
	klog.InfoS("Apply a HelmRepository", "namespace", repo.GetNamespace(), "name", repo.GetName())
	if err := h.r.applicator.Apply(ctx, release); err != nil {
		return err
	}
	klog.InfoS("Apply a HelmRelease", "namespace", release.GetNamespace(), "name", release.GetName())
	return nil
}

// checkAutoDetect judge whether the workload type of a helm mode component is not clear, an autodetect type workload
// will be specified by default In this case, the traits attached to the helm mode component will fail to generate, so
// we only call applyHelmModuleResources to create the helm resource, don't generate other K8s resources.
func (h *appHandler) checkAutoDetect(component *v1alpha2.Component) bool {
	if len(component.Spec.Workload.Raw) == 0 && component.Spec.Workload.Object == nil && component.Spec.Helm != nil {
		h.autodetect = true
		return true
	}
	return false
}

type garbageCollectFunc func(ctx context.Context, h *appHandler) error

// execute garbage collection functions, including:
// - clean up legacy app revisions
func garbageCollection(ctx context.Context, h *appHandler) error {
	collectFuncs := []garbageCollectFunc{
		garbageCollectFunc(cleanUpApplicationRevision),
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
