package util

import (
	"fmt"
	"os"
	"strings"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"context"
	"encoding/json"

	"k8s.io/klog"
)

const (
	DefaultErrorExitCode = 1
)

func Print(msg string) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
}

func fatal(msg string, code int) {
	if klog.V(2) {
		klog.FatalDepth(2, msg)
	}
	if len(msg) > 0 {
		// add newline if needed
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		fmt.Fprint(os.Stderr, msg)
	}
	os.Exit(code)
}

func CheckErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "error: ") {
		msg = fmt.Sprintf("error: %s", msg)
	}
	fatal(msg, DefaultErrorExitCode)
}

type ApplicationMeta struct {
	Name        string   `json:"name"`
	Workload    string   `json:"workload,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedTime string   `json:"created,omitempty"`
}

func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

func GetTraitNames(app corev1alpha2.ApplicationConfiguration) []string {
	var traitNames []string
	for _, t := range app.Spec.Components[0].Traits {
		var trait corev1alpha2.TraitDefinition
		json.Unmarshal(t.Trait.Raw, &trait)
		traitNames = append(traitNames, trait.Kind)
	}
	return traitNames
}

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently rudrx only supports one component exists in one application
*/
func RetrieveApplicationsByApplicationName(ctx context.Context, c client.Client, applicationName string, namespace string) ([]ApplicationMeta, error) {
	var applicationMetaList []ApplicationMeta

	if namespace == "" {
		namespace = "default"
	}

	var applicationList corev1alpha2.ApplicationConfigurationList

	if applicationName != "" {
		var application corev1alpha2.ApplicationConfiguration
		err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &application)

		if err != nil {
			return applicationMetaList, err
		}

		applicationList.Items = append(applicationList.Items, application)
	} else {
		err := c.List(ctx, &applicationList)
		if err != nil {
			return applicationMetaList, err
		}
	}

	for _, a := range applicationList.Items {
		componentName := a.Spec.Components[0].ComponentName

		component, err := GetComponent(ctx, c, componentName, namespace)
		if err != nil {
			return applicationMetaList, err
		}

		var workload corev1alpha2.WorkloadDefinition
		json.Unmarshal(component.Spec.Workload.Raw, &workload)
		workloadName := workload.TypeMeta.Kind

		traitNames := GetTraitNames(a)

		applicationMetaList = append(applicationMetaList, ApplicationMeta{
			Name:        a.Name,
			Workload:    workloadName,
			Traits:      traitNames,
			Status:      string(a.Status.Conditions[0].Status),
			CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
		})
	}

	return applicationMetaList, nil
}
