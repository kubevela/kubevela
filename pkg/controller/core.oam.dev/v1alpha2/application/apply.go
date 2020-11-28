package application

import (
	"context"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	serviceFinalizer = "services.finalizer.core.oam.dev"
)

func registerFinalizers(app *v1alpha2.Application) bool {
	newFinalizer := false
	if !meta.FinalizerExists(&app.ObjectMeta, serviceFinalizer) {
		meta.AddFinalizer(&app.ObjectMeta, serviceFinalizer)
		newFinalizer = true
	}
	return newFinalizer
}

func removeFinalizers(app *v1alpha2.Application) bool {
	remove := false
	if meta.FinalizerExists(&app.ObjectMeta, serviceFinalizer) {
		meta.RemoveFinalizer(&app.ObjectMeta, serviceFinalizer)
		remove = true
	}
	return remove
}

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
	h   *applicationReconciler
	app *v1alpha2.Application
	l   logr.Logger
}

func (ret *reter) Err(err error) (ctrl.Result, error) {

	nerr := ret.h.Status().Update(context.Background(), ret.app)
	if err == nil && nerr == nil {
		return ctrl.Result{}, nil
	}
	if nerr != nil {
		ret.l.Error(nerr, "[Update] application")
	}
	if err != nil {
		ret.l.Error(err, "[Handle]")
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 10,
	}, nil
}

// Object interface
type Object interface {
	metav1.Object
	runtime.Object
}

func (ret *reter) apply(ac *v1alpha2.ApplicationConfiguration, comps ...*v1alpha2.Component) error {
	objs := []Object{}
	for _, c := range comps {
		objs = append(objs, c)
	}
	objs = append(objs, ac)
	return ret.Apply(objs...)
}

func (ret *reter) Apply(objs ...Object) error {
	isController := true
	owner := metav1.OwnerReference{
		APIVersion: ret.app.APIVersion,
		Kind:       ret.app.Kind,
		Name:       ret.app.Name,
		UID:        ret.app.UID,
		Controller: &isController,
	}

	ctx := context.Background()
	for _, obj := range objs {
		obj.SetOwnerReferences([]metav1.OwnerReference{owner})
		if err := ret.h.Create(ctx, obj); err != nil && !kerrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil

}
