package application

import (
	"context"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/go-logr/logr"
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

type reter struct {
	c   client.Client
	app *v1alpha2.Application
	l   logr.Logger
}

func (ret *reter) Err(err error) (ctrl.Result, error) {
	nerr := ret.c.Status().Update(context.Background(), ret.app)
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

func (ret *reter) apply(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	// set ownerReference for ApplicationConfiguration and Components created by Application
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

func (ret *reter) healthCheck(appfile *appfile.Appfile) error {
	for _, wl := range appfile.Workloads {
		pCtx := process.NewContext(wl.Name)
		if err := wl.EvalContext(pCtx); err != nil {
			return err
		}
		for _, tr := range wl.Traits {
			if err := tr.EvalContext(pCtx); err != nil {
				return err
			}
		}
		if err := wl.EvalHealth(pCtx, ret.c, appfile.Name); err != nil {
			return err
		}
		for _, trait := range wl.Traits {
			if err := trait.EvalHealth(pCtx, ret.c, appfile.Name); err != nil {
				return err
			}
		}
	}
	return nil
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
func (ret *reter) Sync(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component) error {
	for _, comp := range comps {
		if err := CreateOrUpdateComponent(ctx, ret.c, comp.DeepCopy()); err != nil {
			return err
		}
	}

	if err := CreateOrUpdateAppConfig(ctx, ret.c, ac); err != nil {
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
		if err := ret.c.Delete(ctx, oldC); err != nil {
			return err
		}
	}
	return nil
}
