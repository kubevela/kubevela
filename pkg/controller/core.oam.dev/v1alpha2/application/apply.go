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
	"strconv"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

func errorCondition(tpy string, err error) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               runtimev1alpha1.ConditionType(tpy),
		Status:             v1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             runtimev1alpha1.ReasonReconcileError,
		Message:            err.Error(),
	}
}

func readyCondition(tpy string) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               runtimev1alpha1.ConditionType(tpy),
		Status:             v1.ConditionTrue,
		Reason:             runtimev1alpha1.ReasonAvailable,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

type appHandler struct {
	r             *Reconciler
	app           *v1beta1.Application
	appfile       *appfile.Appfile
	logger        logr.Logger
	inplace       bool
	isNewRevision bool
	revisionHash  string
}

// setInplace will mark if the application should upgrade the workload within the same instance(name never changed)
func (h *appHandler) setInplace(isInplace bool) {
	h.inplace = isInplace
}

func (h *appHandler) handleErr(err error) (ctrl.Result, error) {
	nerr := h.r.UpdateStatus(context.Background(), h.app)
	if err == nil && nerr == nil {
		return ctrl.Result{}, nil
	}
	if nerr != nil {
		h.logger.Error(nerr, "[Update] application status")
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 10,
	}, nil
}

// apply will
// 1. set ownerReference for ApplicationConfiguration and Components
// 2. update AC's components using the component revision name
// 3. update or create the AC with new revision and remember it in the application status
// 4. garbage collect unused components
func (h *appHandler) apply(ctx context.Context, appRev *v1beta1.ApplicationRevision, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	owners := []metav1.OwnerReference{{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       v1beta1.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(true),
	}}

	// don't create components and AC if revision-only annotation is set
	if ac.Annotations[oam.AnnotationAppRevisionOnly] == "true" {
		h.FinalizeAppRevision(appRev, ac, comps)
		return h.createOrUpdateAppRevision(ctx, appRev)
	}

	for _, comp := range comps {
		comp.SetOwnerReferences(owners)
		newComp := comp.DeepCopy()
		// newComp will be updated and return the revision name instead of the component name
		revisionName, err := h.createOrUpdateComponent(ctx, newComp)
		if err != nil {
			return err
		}
		// find the ACC that contains this component
		for i := 0; i < len(ac.Spec.Components); i++ {
			// update the AC using the component revision instead of component name
			// we have to make AC immutable including the component it's pointing to
			if ac.Spec.Components[i].ComponentName == newComp.Name {
				ac.Spec.Components[i].RevisionName = revisionName
				ac.Spec.Components[i].ComponentName = ""
			}
		}
		if comp.Spec.Helm != nil {
			// TODO(wonderflow): do we still need to apply helm resource if the spec has no difference?
			if err = h.applyHelmModuleResources(ctx, comp, owners); err != nil {
				return errors.Wrap(err, "cannot apply Helm module resources")
			}
		}
	}
	ac.SetOwnerReferences(owners)
	h.FinalizeAppRevision(appRev, ac, comps)

	if err := h.createOrUpdateAppRevision(ctx, appRev); err != nil {
		return err
	}

	// the rollout will create AppContext which will launch the real K8s resources.
	// Otherwise, we should create/update the appContext here when there if no rollout controller to take care of new versions
	// In this case, the workload should update with the annotation `app.oam.dev/inplace-upgrade=true`
	if _, exist := h.app.GetAnnotations()[oam.AnnotationAppRollout]; !exist && h.app.Spec.RolloutPlan == nil {
		h.setInplace(true)
		return h.createOrUpdateAppContext(ctx, owners)
	}
	h.setInplace(false)

	return nil
}

func (h *appHandler) createOrUpdateAppRevision(ctx context.Context, appRev *v1beta1.ApplicationRevision) error {
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
			Name:    wl.Name,
			Healthy: true,
		}

		var (
			outputSecretName string
			err              error
		)
		pCtx := process.NewContext(h.app.Namespace, wl.Name, appFile.Name, appFile.RevisionName)
		if wl.IsCloudResourceProducer() {
			outputSecretName, err = appfile.GetOutputSecretNames(wl)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, setting outputSecretName error", appFile.Name, wl.Name)
			}
			pCtx.InsertSecrets(outputSecretName, wl.RequiredSecrets)
		}
		if err := wl.EvalContext(pCtx); err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate context error", appFile.Name, wl.Name)
		}
		for _, tr := range wl.Traits {
			if err := tr.EvalContext(pCtx); err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate context error", appFile.Name, wl.Name, tr.Name)
			}
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
		var traitStatusList []common.ApplicationTraitStatus
		for _, trait := range wl.Traits {
			var traitStatus = common.ApplicationTraitStatus{
				Type:    trait.Name,
				Healthy: true,
			}
			traitHealth, err := trait.EvalHealth(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, check health error", appFile.Name, wl.Name, trait.Name)
			}
			if !traitHealth {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				traitStatus.Healthy = false
				healthy = false
			}
			traitStatus.Message, err = trait.EvalStatus(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate status message error", appFile.Name, wl.Name, trait.Name)
			}
			traitStatusList = append(traitStatusList, traitStatus)
		}
		status.Traits = traitStatusList
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
		h.logger.Info("Created a new component", "component name", comp.GetName())
	} else {
		// remember the revision if there is a previous component
		if curComp.Status.LatestRevision != nil {
			preRevisionName = curComp.Status.LatestRevision.Name
		}
		comp.ResourceVersion = curComp.ResourceVersion
		if err := h.r.Update(ctx, comp); err != nil {
			return "", err
		}
		h.logger.Info("Updated a component", "component name", comp.GetName())
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
		needNewRevision, err := utils.CompareWithRevision(ctx, h.r,
			logging.NewLogrLogger(h.logger), compName, compNameSpace, preRevisionName, &updatedComp.Spec)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("compare with existing controllerRevision %s failed",
				preRevisionName))
		}
		if !needNewRevision {
			h.logger.Info("no need to wait for a new component revision", "component name", updatedComp.GetName(),
				"revision", preRevisionName)
			return preRevisionName, nil
		}
	}
	h.logger.Info("wait for a new component revision", "component name", compName,
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
		needNewRevision, err := utils.CompareWithRevision(ctx, h.r, logging.NewLogrLogger(h.logger), compName,
			compNameSpace, curComp.Status.LatestRevision.Name, &updatedComp.Spec)
		if err != nil {
			// retry no matter what
			// nolint:nilerr
			return false, nil
		}
		// end the loop if we find the revision
		if !needNewRevision {
			curRevisionName = curComp.Status.LatestRevision.Name
			h.logger.Info("get a matching component revision", "component name", compName,
				"current revision", curRevisionName)
		}
		return !needNewRevision, nil
	}
	if err := wait.ExponentialBackoff(utils.DefaultBackoff, checkForRevision); err != nil {
		return "", err
	}
	return curRevisionName, nil
}

// createOrUpdateAppContext will make sure the appContext points to the latest application revision
// this will only be called in the case of no rollout,
func (h *appHandler) createOrUpdateAppContext(ctx context.Context, owners []metav1.OwnerReference) error {
	var curAppContext v1alpha2.ApplicationContext
	// AC name is the same as the app name if there is no rollout
	appContext := v1alpha2.ApplicationContext{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.app.Name,
			Namespace: h.app.Namespace,
		},
		Spec: v1alpha2.ApplicationContextSpec{
			// new AC always point to the latest app revision
			ApplicationRevisionName: h.app.Status.LatestRevision.Name,
		},
	}
	appContext.SetOwnerReferences(owners)
	// set the AC label and annotation
	appLabel := h.app.GetLabels()
	if appLabel == nil {
		appLabel = make(map[string]string)
	}
	appLabel[oam.LabelAppRevisionHash] = h.app.Status.LatestRevision.RevisionHash
	appContext.SetLabels(appLabel)

	appAnnotation := h.app.GetAnnotations()
	if appAnnotation == nil {
		appAnnotation = make(map[string]string)
	}
	appAnnotation[oam.AnnotationInplaceUpgrade] = strconv.FormatBool(h.inplace)
	appContext.SetAnnotations(appAnnotation)

	key := ctypes.NamespacedName{Name: appContext.Name, Namespace: appContext.Namespace}

	if err := h.r.Get(ctx, key, &curAppContext); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		klog.InfoS("create a new appContext", "application name",
			appContext.GetName(), "revision it points to", appContext.Spec.ApplicationRevisionName)
		return h.r.Create(ctx, &appContext)
	}

	// we don't need to create another appConfig
	klog.InfoS("replace the existing appContext", "application name", appContext.GetName(),
		"revision it points to", appContext.Spec.ApplicationRevisionName)
	appContext.ResourceVersion = curAppContext.ResourceVersion
	return h.r.Update(ctx, &appContext)
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

// handleResourceTracker check the namesapce of  all components and traits, if the namespace is different with application, the tracker will own them
func (h *appHandler) handleResourceTracker(ctx context.Context, components []*v1alpha2.Component, ac *v1alpha2.ApplicationConfiguration) error {
	ref := new(metav1.OwnerReference)
	// resourceTracker is cache for resourceTracker, avoid get from k8s every time
	resourceTracker := new(v1beta1.ResourceTracker)
	needTracker := false
	for i, c := range components {
		u, err := oamutil.RawExtension2Unstructured(&c.Spec.Workload)
		if err != nil {
			return err
		}
		if checkResourceDiffWithApp(u, h.app.Namespace) {
			needTracker = true
			if len(resourceTracker.Name) == 0 {
				resourceTracker, ref, err = h.getResourceTrackerAndOwnReference(ctx)
				if err != nil {
					return err
				}
			}
			u.SetOwnerReferences([]metav1.OwnerReference{*ref})
			raw := oamutil.Object2RawExtension(u)
			components[i].Spec.Workload = raw
		}
	}
	for _, acComponent := range ac.Spec.Components {
		for i, t := range acComponent.Traits {
			u, err := oamutil.RawExtension2Unstructured(&t.Trait)
			if err != nil {
				return err
			}
			if checkResourceDiffWithApp(u, h.app.Namespace) {
				needTracker = true
				if len(resourceTracker.Name) == 0 {
					resourceTracker, ref, err = h.getResourceTrackerAndOwnReference(ctx)
					if err != nil {
						return err
					}
				}
				u.SetOwnerReferences([]metav1.OwnerReference{*ref})
				raw := oamutil.Object2RawExtension(u)
				acComponent.Traits[i].Trait = raw
			}
		}
	}
	if !needTracker {
		h.app.Status.ResourceTracker = nil
		// check weather related resourceTracker is existed, if yes delete it
		err := h.r.Get(ctx, ctypes.NamespacedName{Name: h.generateResourceTrackerName()}, resourceTracker)
		if err == nil {
			return h.r.Delete(ctx, resourceTracker)
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	h.app.Status.ResourceTracker = &runtimev1alpha1.TypedReference{
		Name:       resourceTracker.Name,
		Kind:       v1beta1.ResourceTrackerGroupKind,
		APIVersion: v1beta1.ResourceTrackerKindAPIVersion,
		UID:        resourceTracker.UID}

	return nil
}

func (h *appHandler) getResourceTrackerAndOwnReference(ctx context.Context) (*v1beta1.ResourceTracker, *metav1.OwnerReference, error) {
	resourceTracker := new(v1beta1.ResourceTracker)
	key := ctypes.NamespacedName{Name: h.generateResourceTrackerName()}
	err := h.r.Get(ctx, key, resourceTracker)
	if err != nil {
		if apierrors.IsNotFound(err) {
			resourceTracker = &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{
					Name: h.generateResourceTrackerName(),
				},
			}
			if err = h.r.Client.Create(ctx, resourceTracker); err != nil {
				return nil, nil, err
			}
			return resourceTracker, metav1.NewControllerRef(resourceTracker, v1beta1.ResourceTrackerKindVersionKind), nil
		}
		return nil, nil, err
	}
	return resourceTracker, metav1.NewControllerRef(resourceTracker, v1beta1.ResourceTrackerKindVersionKind), nil
}

func (h *appHandler) generateResourceTrackerName() string {
	return fmt.Sprintf("%s-%s", h.app.Namespace, h.app.Name)
}

func checkResourceDiffWithApp(u *unstructured.Unstructured, appNs string) bool {
	return len(u.GetNamespace()) != 0 && u.GetNamespace() != appNs
}

// finalizeResourceTracker func return whether need to update application
func (h *appHandler) removeResourceTracker(ctx context.Context) (bool, error) {
	client := h.r.Client
	rt := new(v1beta1.ResourceTracker)
	trackerName := h.generateResourceTrackerName()
	key := ctypes.NamespacedName{Name: trackerName}
	err := client.Get(ctx, key, rt)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// for some cases the resourceTracker have been deleted but finalizer still exist
			if meta.FinalizerExists(h.app, resourceTrackerFinalizer) {
				meta.RemoveFinalizer(h.app, resourceTrackerFinalizer)
				return true, nil
			}
			// for some cases: informer cache haven't sync resourceTracker from k8s, return error trigger reconcile again
			if h.app.Status.ResourceTracker != nil {
				return false, fmt.Errorf("application status has resouceTracker but cannot get from k8s ")
			}
			return false, nil
		}
		return false, err
	}
	rt = &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name: trackerName,
		},
	}
	err = h.r.Client.Delete(ctx, rt)
	if err != nil {
		return false, err
	}
	h.logger.Info("delete application resourceTracker")
	meta.RemoveFinalizer(h.app, resourceTrackerFinalizer)
	h.app.Status.ResourceTracker = nil
	return true, nil
}
