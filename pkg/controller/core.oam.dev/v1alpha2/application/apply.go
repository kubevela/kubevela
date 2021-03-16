package application

import (
	"context"
	"fmt"
	"strconv"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
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
	r      *Reconciler
	app    *v1alpha2.Application
	logger logr.Logger
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
func (h *appHandler) apply(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	owners := []metav1.OwnerReference{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(true),
	}}
	ac.SetOwnerReferences(owners)
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
			if err := h.applyHelmModuleResources(ctx, comp, owners); err != nil {
				return errors.Wrap(err, "cannot apply Helm module resources")
			}
		}
	}

	if err := h.createOrUpdateAppConfig(ctx, ac); err != nil {
		return err
	}

	return nil
}

func (h *appHandler) statusAggregate(appfile *appfile.Appfile) ([]v1alpha2.ApplicationComponentStatus, bool, error) {
	var appStatus []v1alpha2.ApplicationComponentStatus
	var healthy = true
	for _, wl := range appfile.Workloads {
		var status = v1alpha2.ApplicationComponentStatus{
			Name:    wl.Name,
			Healthy: true,
		}
		pCtx := process.NewContext(wl.Name, appfile.Name, appfile.RevisionName)
		if err := wl.EvalContext(pCtx); err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate context error", appfile.Name, wl.Name)
		}
		for _, tr := range wl.Traits {
			if err := tr.EvalContext(pCtx); err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate context error", appfile.Name, wl.Name, tr.Name)
			}
		}

		workloadHealth, err := wl.EvalHealth(pCtx, h.r, h.app.Namespace)
		if err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appfile.Name, wl.Name)
		}
		if !workloadHealth {
			// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
			status.Healthy = false
			healthy = false
		}

		status.Message, err = wl.EvalStatus(pCtx, h.r, h.app.Namespace)
		if err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate workload status message error", appfile.Name, wl.Name)
		}
		var traitStatusList []v1alpha2.ApplicationTraitStatus
		for _, trait := range wl.Traits {
			var traitStatus = v1alpha2.ApplicationTraitStatus{
				Type:    trait.Name,
				Healthy: true,
			}
			traitHealth, err := trait.EvalHealth(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, check health error", appfile.Name, wl.Name, trait.Name)
			}
			if !traitHealth {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				traitStatus.Healthy = false
				healthy = false
			}
			traitStatus.Message, err = trait.EvalStatus(pCtx, h.r, h.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate status message error", appfile.Name, wl.Name, trait.Name)
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

// createOrUpdateAppConfig will find the latest revision of the AC according
// it will create a new revision if the appConfig is different from the existing one
func (h *appHandler) createOrUpdateAppConfig(ctx context.Context, appConfig *v1alpha2.ApplicationConfiguration) error {
	var curAppConfig v1alpha2.ApplicationConfiguration
	specHashLabel, err := utils.ComputeSpecHash(appConfig.Spec)
	if err != nil {
		return err
	}
	appConfig.SetLabels(oamutil.MergeMapOverrideWithDst(appConfig.GetLabels(),
		map[string]string{
			oam.LabelAppConfigHash: specHashLabel,
		}))
	// first time ever
	if h.app.Status.LatestRevision == nil {
		h.logger.Info("create the first appConfig", "application name", h.app.GetName())
		return h.createNewAppConfig(ctx, appConfig)
	}
	// get the AC with the last revision name stored in the application
	key := ctypes.NamespacedName{Name: h.app.Status.LatestRevision.Name, Namespace: h.app.Namespace}
	if err := h.r.Get(ctx, key, &curAppConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		h.logger.Info("create a new appConfig that the last creation failed to create", "application name",
			h.app.GetName(), "latest revision that does not exist", h.app.Status.LatestRevision.Name)
		return h.createNewAppConfig(ctx, appConfig)
	}
	// check if the old AC has the same HASH value first, just replace lable/annotation if that's the case
	if curAppConfig.GetLabels()[oam.LabelAppConfigHash] == appConfig.GetLabels()[oam.LabelAppConfigHash] {
		// Just to be safe that it's not because of a random Hash collision
		if apiequality.Semantic.DeepEqual(&curAppConfig.Spec, &appConfig.Spec) {
			// same spec, no need to create another AC, still need to update the AC to apply label/annotation
			h.logger.Info("update latest application config", "application name",
				h.app.GetName(), "latest revision to be updated", h.app.Status.LatestRevision.Name)
			oamutil.PassLabelAndAnnotation(appConfig, &curAppConfig)
			return h.r.Update(ctx, &curAppConfig)
		}
		h.logger.Info("encountered a different app spec with same hash", "current spec",
			curAppConfig.Spec, "new appConfig spec", appConfig.Spec)
	}
	nextRevisionName, _ := utils.GetAppNextRevision(h.app)
	if nextRevisionName == h.app.Status.LatestRevision.Name {
		// we don't need to create another appConfig
		h.logger.Info("replace the existing application config", "application name",
			h.app.GetName(), "latest revision to be replaced", h.app.Status.LatestRevision.Name, "new hash value", specHashLabel)
		appConfig.ResourceVersion = curAppConfig.ResourceVersion
		appConfig.Name = nextRevisionName
		h.app.Status.LatestRevision.RevisionHash = specHashLabel

		// record that last appConfig we created first in the app's status
		// make sure that we persist the latest revision first
		if err := h.r.UpdateStatus(ctx, h.app); err != nil {
			return err
		}
		// it ok if the update fails, we will update again in the next loop
		return h.r.Update(ctx, appConfig)
	}

	// create the next version
	h.logger.Info("create a new appConfig", "application name", h.app.GetName(),
		"latest revision that does not match the appConfig", h.app.Status.LatestRevision.Name)
	return h.createNewAppConfig(ctx, appConfig)
}

// create a new appConfig given the latest revision in the application
func (h *appHandler) createNewAppConfig(ctx context.Context, appConfig *v1alpha2.ApplicationConfiguration) error {
	revisionName, nextRevision := utils.GetAppNextRevision(h.app)
	// update the next revision in the application's status
	h.app.Status.LatestRevision = &v1alpha2.Revision{
		Name:         revisionName,
		Revision:     nextRevision,
		RevisionHash: appConfig.GetLabels()[oam.LabelAppConfigHash],
	}
	appConfig.Name = revisionName
	// indicate that the applicationConfig is created if we are doing rolling out
	if _, exist := h.app.GetAnnotations()[oam.AnnotationAppRollout]; exist || h.app.Spec.RolloutPlan != nil {
		h.logger.Info(fmt.Sprintf("The application %s rolling out is controlled by a rollout plan", h.app.Name))
		appConfig.SetAnnotations(oamutil.MergeMapOverrideWithDst(appConfig.GetAnnotations(), map[string]string{
			oam.AnnotationAppRollout: strconv.FormatBool(true),
		}))
	}

	// record that last appConfig we created first in the app's status
	// make sure that we persist the latest revision first
	if err := h.r.UpdateStatus(ctx, h.app); err != nil {
		return err
	}
	h.logger.Info("recorded the latest appConfig revision", "application name", h.app.GetName(),
		"latest revision", revisionName)
	// it ok if the create failed, we will create again in  the next loop
	return h.r.Create(ctx, appConfig)
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
