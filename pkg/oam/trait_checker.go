package oam

import (
	"context"
	"encoding/json"
	"fmt"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	v12 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/application"
	autoscalers "github.com/oam-dev/kubevela/pkg/controller/v1alpha1/autoscaler"
)

// CheckStatus defines the type of checking status
type CheckStatus string

const (
	// StatusChecking means in checking loop
	StatusChecking = "checking"
	// StatusDone means check has done
	StatusDone = "done"
)

// GetChecker will get Trait checker for 'vela status'
func GetChecker(traitType string, c client.Client) Checker {
	switch traitType {
	case "route":
		return &RouteChecker{c: c}
	case "metrics":
		return &MetricChecker{c: c}
	case "autoscale":
		return &AutoscalerChecker{c: c}
	}

	return &DefaultChecker{c: c}
}

// Checker defines the interface of checker
type Checker interface {
	Check(ctx context.Context, reference runtimev1alpha1.TypedReference, compName string, appConfig *v1alpha2.ApplicationConfiguration, app *application.Application) (CheckStatus, string, error)
}

// DefaultChecker defines the default checker
type DefaultChecker struct {
	c client.Client
}

// Check default check object if exist and print the configs
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
		message += fmt.Sprintf("%v=%v\n\t\t", k, v)
	}
	return StatusDone, message, err
}

// MetricChecker check for 'metrics' core trait
type MetricChecker struct {
	c client.Client
}

// Check metrics
func (d *MetricChecker) Check(ctx context.Context, reference runtimev1alpha1.TypedReference, _ string, appConfig *v1alpha2.ApplicationConfiguration, _ *application.Application) (CheckStatus, string, error) {
	metric := v1alpha1.MetricsTrait{}
	if err := d.c.Get(ctx, client.ObjectKey{Namespace: appConfig.Namespace, Name: reference.Name}, &metric); err != nil {
		return StatusChecking, "", err
	}
	condition := metric.Status.Conditions
	if len(condition) < 1 {
		return StatusChecking, "", nil
	}
	if condition[0].Status != v1.ConditionTrue {
		return StatusChecking, condition[0].Message, nil
	}
	if metric.Spec.ScrapeService.Enabled != nil && !*metric.Spec.ScrapeService.Enabled {
		return StatusDone, "Monitoring disabled", nil
	}
	var message = fmt.Sprintf("Monitoring port: %s, path: %s, format: %s, schema: %s.",
		metric.Status.Port.String(), metric.Spec.ScrapeService.Path,
		metric.Spec.ScrapeService.Format, metric.Spec.ScrapeService.Scheme)
	return StatusDone, message, nil
}

// RouteChecker check for 'route' core trait
type RouteChecker struct {
	c client.Client
}

// Check understand route status
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
		addr := value[0].IP
		if value[0].Hostname != "" {
			addr = value[0].Hostname
		}
		message += fmt.Sprintf("\tVisiting URL: %s\tIP: %s\n", url, addr)
	}
	if len(route.Status.Ingresses) == 0 {
		message += fmt.Sprintf("Visiting by using 'vela port-forward %s --route'\n", appConfig.Name)
	}
	return StatusDone, message, nil
}

// AutoscalerChecker checks 'autoscale' trait
type AutoscalerChecker struct {
	c client.Client
}

// Check should understand autoscale trait status
func (d *AutoscalerChecker) Check(ctx context.Context, ref runtimev1alpha1.TypedReference, _ string, appConfig *v1alpha2.ApplicationConfiguration, _ *application.Application) (CheckStatus, string, error) {
	traitName := ref.Name
	var scaler v1alpha1.Autoscaler
	if err := d.c.Get(ctx, client.ObjectKey{Namespace: appConfig.Namespace, Name: traitName}, &scaler); err != nil {
		return StatusChecking, "", err
	}
	var scalerType string
	triggers := scaler.Spec.Triggers
	if len(triggers) >= 1 {
		scalerType = string(triggers[0].Type)
	}

	hpaName := "keda-hpa-" + traitName
	var hpa v12.HorizontalPodAutoscaler
	if err := d.c.Get(ctx, client.ObjectKey{Namespace: appConfig.Namespace, Name: hpaName}, &hpa); err != nil {
		return StatusChecking, "", err
	}
	message := fmt.Sprintf("type: %-8s", scalerType)
	if scalerType == string(autoscalers.CPUType) {
		// When attaching trait, and before the scaler trait works, `CurrentCPUUtilizationPercentage` is nil
		currentCPUUtilizationPercentage := hpa.Status.CurrentCPUUtilizationPercentage
		var zeroPercentage int32 = 0
		if currentCPUUtilizationPercentage == nil {
			currentCPUUtilizationPercentage = &zeroPercentage
		}
		message += fmt.Sprintf("cpu-utilization(target/current): %v%%/%v%%\t",
			*hpa.Spec.TargetCPUUtilizationPercentage, *currentCPUUtilizationPercentage)
	}
	message += fmt.Sprintf("replicas(min/max/current): %v/%v/%v", *hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas,
		hpa.Status.CurrentReplicas)
	return StatusDone, message, nil
}

// GetUnstructured get object by GVK.
func GetUnstructured(ctx context.Context, c client.Client, ns string, resourceRef runtimev1alpha1.TypedReference) (*unstructured.Unstructured, error) {
	resource := unstructured.Unstructured{}
	resource.SetGroupVersionKind(resourceRef.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: resourceRef.Name}, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}

// GetStatusFromObject get Unstructured object status
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
