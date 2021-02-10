package application

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/common"
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
	r   *Reconciler
	app *v1alpha2.Application
	l   logr.Logger
}

func (h *appHandler) Err(err error) (ctrl.Result, error) {
	nerr := h.r.UpdateStatus(context.Background(), h.app)
	if err == nil && nerr == nil {
		return ctrl.Result{}, nil
	}
	if nerr != nil {
		h.l.Error(nerr, "[Update] application")
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 10,
	}, nil
}

// apply will set ownerReference for ApplicationConfiguration and Components created by Application
func (h *appHandler) apply(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	owners := []metav1.OwnerReference{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.BoolPtr(true),
	}}
	ac.SetOwnerReferences(owners)
	for _, c := range comps {
		c.SetOwnerReferences(owners)
	}

	return h.Sync(ctx, ac, comps)
}

func (h *appHandler) statusAggregate(appfile *appfile.Appfile) ([]v1alpha2.ApplicationComponentStatus, bool, error) {
	var appStatus []v1alpha2.ApplicationComponentStatus
	var healthy = true
	for _, wl := range appfile.Workloads {
		var status = v1alpha2.ApplicationComponentStatus{
			Name:    wl.Name,
			Healthy: true,
		}
		pCtx := process.NewContext(wl.Name, appfile.Name)
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

// createOrUpdateComponent will create if not exist and update if exists.
func createOrUpdateComponent(ctx context.Context, client client.Client, comp *v1alpha2.Component) error {
	var getc v1alpha2.Component
	key := ctypes.NamespacedName{Name: comp.Name, Namespace: comp.Namespace}
	if err := client.Get(ctx, key, &getc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return client.Create(ctx, comp)
	}
	comp.ResourceVersion = getc.ResourceVersion
	return client.Update(ctx, comp)
}

// CreateOrUpdateAppConfig will create if not exist and update if exists.
func (h *appHandler) CreateOrUpdateAppConfig(ctx context.Context, appConfig *v1alpha2.ApplicationConfiguration) error {
	var curAppConfig v1alpha2.ApplicationConfiguration
	// initialized
	if h.app.Status.LatestRevision == nil {
		revisionName := common.ConstructRevisionName(appConfig.Name, 0)
		h.app.Status.LatestRevision = &v1alpha2.Revision{
			Name:     revisionName,
			Revision: 0,
		}
	}
	// get the AC with the last revision name stored in the application
	key := ctypes.NamespacedName{Name: h.app.Status.LatestRevision.Name, Namespace: appConfig.Namespace}
	var exist = true
	if err := h.r.Get(ctx, key, &curAppConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		exist = false
	}
	if !exist {
		return h.createNewAppConfig(ctx, appConfig)
	}
	// compute a hash value of the appConfig spec
	specHash, err := hashstructure.Hash(appConfig.Spec, hashstructure.FormatV2, nil)
	if err != nil {
		return err
	}
	acLabels := map[string]string{
		oam.LabelAppConfigHash: strconv.FormatUint(specHash, 16),
	}
	appConfig.SetLabels(oamutil.MergeMapOverrideWithDst(appConfig.GetLabels(), acLabels))

	// check if the old AC has the same HASH value
	if curAppConfig.GetLabels()[oam.LabelAppConfigHash] == appConfig.GetLabels()[oam.LabelAppConfigHash] {
		// Just to be safe that it's not because of a random Hash collision
		if reflect.DeepEqual(curAppConfig.Spec, appConfig.Spec) {
			// same spec, no need to create another AC
			appConfig.ResourceVersion = curAppConfig.ResourceVersion
			return h.r.Update(ctx, appConfig)
		}
	}
	// create the next version
	return h.createNewAppConfig(ctx, appConfig)
}

// create a new appConfig revision
func (h *appHandler) createNewAppConfig(ctx context.Context, appConfig *v1alpha2.ApplicationConfiguration) error {
	nextRevision := h.app.Status.LatestRevision.Revision + 1
	revisionName := common.ConstructRevisionName(appConfig.Name, nextRevision)
	// update the next revision in the application's status
	h.app.Status.LatestRevision = &v1alpha2.Revision{
		Name:     revisionName,
		Revision: nextRevision,
	}
	acLabels := map[string]string{
		oam.AnnotationNewAppConfig: "true",
	}
	appConfig.SetLabels(oamutil.MergeMapOverrideWithDst(appConfig.GetLabels(), acLabels))
	appConfig.Name = revisionName
	if err := h.r.Create(ctx, appConfig); err != nil {
		return err
	}
	// record that last appConfig we created
	return h.r.UpdateStatus(ctx, h.app)
}

// Sync perform synchronization operations
func (h *appHandler) Sync(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	for _, comp := range comps {
		newComp := comp.DeepCopy()
		// newComp will be updated
		if err := createOrUpdateComponent(ctx, h.r, newComp); err != nil {
			return err
		}
		// update the AC using the component revision instead of component name
		for i := 0; i < len(ac.Spec.Components); i++ {
			if ac.Spec.Components[i].ComponentName == newComp.Name {
				if newComp.Status.LatestRevision == nil {
					return fmt.Errorf("can not find the component revision for component %s", comp.Name)
				}
				// set the revision
				ac.Spec.Components[i].RevisionName = newComp.Status.LatestRevision.Name
			}
		}
	}

	if err := h.CreateOrUpdateAppConfig(ctx, ac); err != nil {
		return err
	}

	// Garbage Collection for no used Components.
	// There's no need to ApplicationConfiguration Garbage Collection, it has the same name with Application.
	for _, comp := range h.app.Status.Components {
		var exist = false
		for _, cc := range comps {
			if comp.Name == cc.Name {
				exist = true
				break
			}
		}
		if exist {
			continue
		}
		// Component not exits in current Application, should be deleted
		var oldC = &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: comp.Name, Namespace: ac.Namespace}}
		if err := h.r.Delete(ctx, oldC); err != nil {
			return err
		}
	}
	return nil
}
