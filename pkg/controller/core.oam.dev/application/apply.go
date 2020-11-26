package application

import (
	"context"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/go-logr/logr"
	v1alpha22 "github.com/oam-dev/kubevela/api/core.oam.dev/v1alpha2"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

const (
	serviceFinalizer = "services.finalizer.core.oam.dev"
)

func registerFinalizers(app *v1alpha22.Application) bool {
	newFinalizer := false
	if !meta.FinalizerExists(&app.ObjectMeta, serviceFinalizer) {
		meta.AddFinalizer(&app.ObjectMeta, serviceFinalizer)
		newFinalizer = true
	}
	return newFinalizer
}

func removeFinalizers(app *v1alpha22.Application) bool {
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
		Message:            err.Error(),
	}
}

func readyCondition(tpy string) runtimev1alpha1.Condition {
	return runtimev1alpha1.Condition{
		Type:               runtimev1alpha1.ConditionType(tpy),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

type reter struct {
	h   *ApplicationReconciler
	app *v1alpha22.Application
	l   logr.Logger
}

func (ret *reter) Err(err error) (ctrl.Result, error) {
	nerr := ret.h.Update(context.Background(), ret.app)
	if err == nil {
		return ctrl.Result{}, nerr
	}
	if nerr != nil {
		ret.h.Log.Error(nerr, "update application")
	}
	return ctrl.Result{}, err
}

type Object interface {
	metav1.Object
	runtime.Object
}

func (ret *reter) apply(ac *core.ApplicationConfiguration, comps ...*core.Component) error {
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
