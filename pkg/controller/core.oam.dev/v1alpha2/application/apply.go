package application

import (
	"context"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/go-logr/logr"
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
	"github.com/oam-dev/kubevela/pkg/dsl/process"
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

func (ret *appHandler) Err(err error) (ctrl.Result, error) {
	nerr := ret.r.UpdateStatus(context.Background(), ret.app)
	if err == nil && nerr == nil {
		return ctrl.Result{}, nil
	}
	if nerr != nil {
		ret.l.Error(nerr, "[Update] application")
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 10,
	}, nil
}

// apply will set ownerReference for ApplicationConfiguration and Components created by Application
func (ret *appHandler) apply(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	owners := []metav1.OwnerReference{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.ApplicationKind,
		Name:       ret.app.Name,
		UID:        ret.app.UID,
		Controller: pointer.BoolPtr(true),
	}}
	ac.SetOwnerReferences(owners)
	for _, c := range comps {
		c.SetOwnerReferences(owners)
	}
	return ret.Sync(ctx, ac, comps)
}

func (ret *appHandler) statusAggregate(appfile *appfile.Appfile) ([]v1alpha2.ApplicationComponentStatus, bool, error) {
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

		workloadHealth, err := wl.EvalHealth(pCtx, ret.r, ret.app.Namespace)
		if err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appfile.Name, wl.Name)
		}
		if !workloadHealth {
			// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
			status.Healthy = false
			healthy = false
		}
		status.Message, err = wl.EvalStatus(pCtx, ret.r, ret.app.Namespace)
		if err != nil {
			return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, evaluate workload status message error", appfile.Name, wl.Name)
		}
		var traitStatusList []v1alpha2.ApplicationTraitStatus
		for _, trait := range wl.Traits {
			var traitStatus = v1alpha2.ApplicationTraitStatus{
				Type:    trait.Name,
				Healthy: true,
			}
			traitHealth, err := trait.EvalHealth(pCtx, ret.r, ret.app.Namespace)
			if err != nil {
				return nil, false, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, check health error", appfile.Name, wl.Name, trait.Name)
			}
			if !traitHealth {
				// TODO(wonderflow): we should add a custom way to let the template say why it's unhealthy, only a bool flag is not enough
				traitStatus.Healthy = false
				healthy = false
			}
			traitStatus.Message, err = trait.EvalStatus(pCtx, ret.r, ret.app.Namespace)
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

// CreateOrUpdateComponent will create if not exist and update if exists.
func CreateOrUpdateComponent(ctx context.Context, client client.Client, comp *v1alpha2.Component) error {
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
func CreateOrUpdateAppConfig(ctx context.Context, client client.Client, appConfig *v1alpha2.ApplicationConfiguration) error {
	var geta v1alpha2.ApplicationConfiguration
	key := ctypes.NamespacedName{Name: appConfig.Name, Namespace: appConfig.Namespace}
	var exist = true
	if err := client.Get(ctx, key, &geta); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		exist = false
	}
	if !exist {
		return client.Create(ctx, appConfig)
	}
	appConfig.ResourceVersion = geta.ResourceVersion
	return client.Update(ctx, appConfig)
}

// Sync perform synchronization operations
func (ret *appHandler) Sync(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	for _, comp := range comps {
		if err := CreateOrUpdateComponent(ctx, ret.r, comp.DeepCopy()); err != nil {
			return err
		}
	}

	if err := CreateOrUpdateAppConfig(ctx, ret.r, ac); err != nil {
		return err
	}

	// Garbage Collection for no used Components.
	// There's no need to ApplicationConfiguration Garbage Collection, it has the same name with Application.
	for _, comp := range ret.app.Status.Components {
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
		if err := ret.r.Delete(ctx, oldC); err != nil {
			return err
		}
	}
	return nil
}
