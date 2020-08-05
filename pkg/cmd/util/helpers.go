package util

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"

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

type ApplicationStatusMeta struct {
	Status   string                          `json:"status,omitempty"`
	Workload corev1alpha2.WorkloadDefinition `json:"workload,omitempty"`
	Traits   []corev1alpha2.TraitDefinition  `json:"trait,omitempty"`
}

func GetComponent(ctx context.Context, c client.Client, componentName string, namespace string) (corev1alpha2.Component, error) {
	var component corev1alpha2.Component
	err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, &component)
	return component, err
}

func GetTraitNamesByApplicationConfiguration(app corev1alpha2.ApplicationConfiguration) []string {
	var traitNames []string

	traitDefinitionList := ListTraitDefinitionsByApplicationConfiguration(app)
	for _, t := range traitDefinitionList {
		traitNames = append(traitNames, t.Name)
	}
	return traitNames
}

func ListTraitDefinitionsByApplicationConfiguration(app corev1alpha2.ApplicationConfiguration) []corev1alpha2.TraitDefinition {
	var traitDefinitionList []corev1alpha2.TraitDefinition
	for _, t := range app.Spec.Components[0].Traits {
		var trait corev1alpha2.TraitDefinition
		if err := json.Unmarshal(t.Trait.Raw, &trait); err == nil {
			traitDefinitionList = append(traitDefinitionList, trait)
		}
	}
	return traitDefinitionList
}

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently rudrx only supports one component exists in one application
*/
func RetrieveApplicationsByName(ctx context.Context, c client.Client, applicationName string, namespace string) ([]ApplicationMeta, error) {
	var applicationMetaList []ApplicationMeta
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
		if err := json.Unmarshal(component.Spec.Workload.Raw, &workload); err == nil {
			workloadName := workload.TypeMeta.Kind

			traitNames := GetTraitNamesByApplicationConfiguration(a)

			applicationMetaList = append(applicationMetaList, ApplicationMeta{
				Name:        a.Name,
				Workload:    workloadName,
				Traits:      traitNames,
				Status:      string(a.Status.Conditions[0].Status),
				CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
			})
		}
	}

	return applicationMetaList, nil
}

func RetrieveApplicationStatusByName(ctx context.Context, c client.Client, applicationName string, namespace string) (ApplicationStatusMeta, error) {
	var applicationStatusMeta ApplicationStatusMeta
	var appConfig corev1alpha2.ApplicationConfiguration
	if err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &appConfig); err != nil {
		return applicationStatusMeta, err
	}

	component, err := GetComponent(ctx, c, applicationName, namespace)
	if err != nil {
		return applicationStatusMeta, err
	}

	var workload corev1alpha2.WorkloadDefinition
	err = json.Unmarshal(component.Spec.Workload.Raw, &workload)
	if err != nil {
		return applicationStatusMeta, err
	}

	traitDefinitionList := ListTraitDefinitionsByApplicationConfiguration(appConfig)
	applicationStatusMeta = ApplicationStatusMeta{
		Status:   string(appConfig.Status.Conditions[0].Status),
		Workload: workload,
		Traits:   traitDefinitionList,
	}
	return applicationStatusMeta, err
}

func GetTraitAliasByTraitDefinition(traitDefinition corev1alpha2.TraitDefinition) string {
	return traitDefinition.Annotations["short"]
}

func GetTraitDefinitionByName(ctx context.Context, c client.Client, namespace string, traitName string) (corev1alpha2.TraitDefinition, error) {
	var t corev1alpha2.TraitDefinition
	err := c.Get(ctx, client.ObjectKey{Name: traitName, Namespace: namespace}, &t)
	return t, err
}

func GetTraitAliasByName(ctx context.Context, c client.Client, namespace string, traitName string) string {
	var traitAlias string
	t, err := GetTraitDefinitionByName(ctx, c, namespace, traitName)
	if err == nil {
		traitAlias = GetTraitAliasByTraitDefinition(t)
	}
	return traitAlias
}

func GetTraitDefinitionByAlias(ctx context.Context, c client.Client, traitAlias string) (corev1alpha2.TraitDefinition, error) {
	var traitDefinitionList corev1alpha2.TraitDefinitionList
	var traitDefinition corev1alpha2.TraitDefinition
	err := c.List(ctx, &traitDefinitionList)
	if err == nil {
		for _, t := range traitDefinitionList.Items {
			template, err := types.ConvertTemplateJson2Object(t.Spec.Extension)
			if err == nil && strings.EqualFold(template.Alias, traitAlias) {
				traitDefinition = t
				break
			}
		}
	}
	return traitDefinition, err
}

// GetTraitNameAndAlias return the name and alias of a TraitDefinition by a string which might be
// the trait name, the trait alias, or invalid name
func GetTraitNameAliasKind(ctx context.Context, c client.Client, namespace string, name string) (string, string, string) {
	var tName, tAlias, tKind string

	t, err := GetTraitDefinitionByName(ctx, c, namespace, name)

	if err == nil {
		template, err := types.ConvertTemplateJson2Object(t.Spec.Extension)
		if err == nil {
			tName, tAlias = t.Spec.Reference.Name, template.Alias
			tKind = fmt.Sprintf("%v", template.Object["kind"])
		}
	} else {
		t, err := GetTraitDefinitionByAlias(ctx, c, name)
		if err == nil {
			template, err := types.ConvertTemplateJson2Object(t.Spec.Extension)
			if err == nil {
				tName, tAlias = t.Spec.Reference.Name, template.Alias
				tKind = fmt.Sprintf("%v", template.Object["kind"])
			}
		}
	}

	return tName, tAlias, tKind
}

func GetWorkloadNameAliasKind(ctx context.Context, c client.Client, namespace string, workloadName string) (string, string, string) {
	var name, alias, kind string

	w, err := GetWorkloadDefinitionByName(ctx, c, namespace, workloadName)

	if err == nil { // workloadName is complete name
		var workloadTemplate types.Template
		workloadTemplate, err := types.ConvertTemplateJson2Object(w.Spec.Extension)
		if err == nil {
			name, alias = w.Name, workloadTemplate.Alias
		}
	} else { // workloadName is alias or kind
		w, err := GetWorkloadDefinitionByAlias(ctx, c, name)
		if err == nil {
			workloadTemplate, err := types.ConvertTemplateJson2Object(w.Spec.Extension)
			if err == nil {
				name, alias, kind = w.Name, workloadTemplate.Alias, w.Kind
			}

		}
	}

	return name, alias, kind
}

func GetWorkloadDefinitionByName(ctx context.Context, c client.Client, namespace string, name string) (corev1alpha2.WorkloadDefinition, error) {
	var w corev1alpha2.WorkloadDefinition
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &w)
	return w, err
}

func GetWorkloadDefinitionByAlias(ctx context.Context, c client.Client, traitAlias string) (corev1alpha2.WorkloadDefinition, error) {
	var workloadDefinitionList corev1alpha2.WorkloadDefinitionList
	var workloadDefinition corev1alpha2.WorkloadDefinition
	if err := c.List(ctx, &workloadDefinitionList); err == nil {
		for _, t := range workloadDefinitionList.Items {
			if strings.EqualFold(t.ObjectMeta.Annotations["short"], traitAlias) {
				workloadDefinition = t
				break
			}
		}
	}

	return workloadDefinition, nil
}
