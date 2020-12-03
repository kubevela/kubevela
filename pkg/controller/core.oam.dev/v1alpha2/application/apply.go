package application

import (
	"context"
	"time"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/apply"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/builder"
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
	h   *Reconciler
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

func (ret *reter) apply(ac *v1alpha2.ApplicationConfiguration, comps ...*v1alpha2.Component) error {
	objs := []apply.Object{}
	for _, c := range comps {
		objs = append(objs, c)
	}
	objs = append(objs, ac)

	listOption := listObjs(client.MatchingLabels{
		builder.OamApplicationLabel: ret.app.Name,
	}, client.InNamespace(ret.app.Namespace))

	isController := true

	owner := metav1.OwnerReference{
		APIVersion: v1alpha2.Group + "/" + v1alpha2.Version,
		Kind:       "Application",
		Name:       ret.app.Name,
		UID:        ret.app.UID,
		Controller: &isController,
	}

	return apply.New(ret.h.Client).Apply(objs,
		apply.DanglingPolicy("delete"),
		apply.List(listOption),
		apply.SetOwnerReferences([]metav1.OwnerReference{owner}),
	)
}

func listObjs(listOpts ...client.ListOption) apply.Lister {
	return func(cli client.Client) ([]apply.Object, error) {
		ctx := context.Background()
		acs := new(v1alpha2.ApplicationConfigurationList)
		if err := cli.List(ctx, acs, listOpts...); err != nil {
			return nil, err
		}
		comps := new(v1alpha2.ComponentList)
		if err := cli.List(ctx, comps, listOpts...); err != nil {
			return nil, err
		}
		objs := []apply.Object{}
		for index := range acs.Items {
			ac := acs.Items[index]
			if ac.DeletionTimestamp != nil {
				continue
			}
			objs = append(objs, &ac)
		}
		for index := range comps.Items {
			comp := comps.Items[index]
			if comp.DeletionTimestamp != nil {
				continue
			}
			objs = append(objs, &comp)
		}
		return objs, nil
	}

}
