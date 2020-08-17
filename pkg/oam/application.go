package oam

import (
	"context"
	"encoding/json"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ApplicationMeta struct {
	Name        string   `json:"name"`
	Workload    string   `json:"workload,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Status      string   `json:"status,omitempty"`
	CreatedTime string   `json:"created,omitempty"`
}

/*
	Get application list by optional filter `applicationName`
	Application name is equal to Component name as currently vela only supports one component exists in one application
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
		err := c.List(ctx, &applicationList, &client.ListOptions{Namespace: namespace})
		if err != nil {
			return applicationMetaList, err
		}
	}

	for _, a := range applicationList.Items {
		for _, com := range a.Spec.Components {
			componentName := com.ComponentName
			component, err := cmdutil.GetComponent(ctx, c, componentName, namespace)
			if err != nil {
				return applicationMetaList, err
			}
			var workload corev1alpha2.WorkloadDefinition
			if err := json.Unmarshal(component.Spec.Workload.Raw, &workload); err == nil {
				workloadName := workload.TypeMeta.Kind
				traitAlias := GetTraitAliasByComponentTraitList(ctx, c, com.Traits)
				var status = "UNKNOWN"
				if len(a.Status.Conditions) != 0 {
					status = string(a.Status.Conditions[0].Status)
				}
				applicationMetaList = append(applicationMetaList, ApplicationMeta{
					Name:        a.Name,
					Workload:    workloadName,
					Traits:      traitAlias,
					Status:      status,
					CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
				})
			}
		}
	}
	return applicationMetaList, nil
}
