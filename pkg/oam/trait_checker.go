package oam

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/api/networking/v1beta1"

	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/api/v1alpha1"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/application"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CheckStatus string

const (
	StatusChecking = "checking"
	StatusDone     = "done"
)

func GetChecker(traitType string, c client.Client) Checker {
	switch traitType {
	case "route":
		return &RouteChecker{c: c}
	}
	return &DefaultChecker{c: c}
}

type Checker interface {
	Check(ctx context.Context, reference runtimev1alpha1.TypedReference, compName string, appConfig *v1alpha2.ApplicationConfiguration, app *application.Application) (CheckStatus, string, error)
}

type DefaultChecker struct {
	c client.Client
}

func (d *DefaultChecker) Check(ctx context.Context, reference runtimev1alpha1.TypedReference, compName string, appConfig *v1alpha2.ApplicationConfiguration, app *application.Application) (CheckStatus, string, error) {
	tr, err := GetUnstructured(ctx, d.c, appConfig.Namespace, reference)
	if err != nil {
		return StatusChecking, "", err
	}
	traitType, ok := tr.GetLabels()[oam.TraitTypeLabel]
	if !ok {
		message, err := GetStatusFromObject(tr)
		return StatusDone, message, err
	}
	traitData, err := app.GetTraitsByType(compName, traitType)
	if err != nil {
		return StatusDone, err.Error(), err
	}
	var message string
	for k, v := range traitData {
		message += fmt.Sprintf("%v=%v\n", k, v)
	}
	return StatusDone, message, err
}

type RouteChecker struct {
	c client.Client
}

func (d *RouteChecker) Check(ctx context.Context, reference runtimev1alpha1.TypedReference, _ string, appConfig *v1alpha2.ApplicationConfiguration, _ *application.Application) (CheckStatus, string, error) {
	route := v1alpha1.Route{}
	if err := d.c.Get(ctx, client.ObjectKey{Namespace: appConfig.Namespace, Name: reference.Name}, &route); err != nil {
		return StatusChecking, "", err
	}
	condition := route.Status.Conditions
	if len(condition) < 1 {
		return StatusChecking, "", nil
	}
	if condition[0].Status != v1.ConditionTrue {
		return StatusChecking, condition[0].Message, nil
	}
	var message string
	for _, ingress := range route.Status.Ingresses {
		var in v1beta1.Ingress
		if err := d.c.Get(ctx, client.ObjectKey{Namespace: appConfig.Namespace, Name: ingress.Name}, &in); err != nil {
			return StatusChecking, "", err
		}
		value := in.Status.LoadBalancer.Ingress
		if len(value) < 1 {
			return StatusChecking, "", fmt.Errorf("%s IP not assigned yet", in.Name)
		}
		var url string
		if len(in.Spec.TLS) >= 1 {
			url = "https://" + in.Spec.Rules[0].Host
		} else {
			url = "http://" + in.Spec.Rules[0].Host
		}
		message += fmt.Sprintf("\tVisiting URL: %s\tIP: %s\n", url, value[0].IP)
	}
	return StatusDone, message, nil
}

func GetUnstructured(ctx context.Context, c client.Client, ns string, resourceRef runtimev1alpha1.TypedReference) (*unstructured.Unstructured, error) {
	resource := unstructured.Unstructured{}
	resource.SetGroupVersionKind(resourceRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: resourceRef.Name}, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}

func GetStatusFromObject(resource *unstructured.Unstructured) (string, error) {
	var message string
	statusData, foundStatus, _ := unstructured.NestedMap(resource.Object, "status")
	if foundStatus {
		statusJSON, err := json.Marshal(statusData)
		if err != nil {
			return "", err
		}
		message = string(statusJSON)
	} else {
		message = "status not found"
	}
	return fmt.Sprintf("%s status: %s", resource.GetName(), message), nil
}
