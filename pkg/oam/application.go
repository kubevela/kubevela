package oam

import (
	"context"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/server/apis"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ComponentMeta struct {
	Name        string                                `json:"name"`
	App         string                                `json:"app"`
	Workload    string                                `json:"workload,omitempty"`
	Traits      []string                              `json:"traits,omitempty"`
	Status      string                                `json:"status,omitempty"`
	CreatedTime string                                `json:"created,omitempty"`
	AppConfig   corev1alpha2.ApplicationConfiguration `json:"-"`
	Component   corev1alpha2.Component                `json:"-"`
}

type Option struct {
	// Optional filter, if specified, only components in such app will be listed
	AppName string

	Namespace string
}

/*
	Get component list
*/
func ListComponents(ctx context.Context, c client.Client, opt Option) ([]ComponentMeta, error) {
	var componentMetaList []ComponentMeta
	var applicationList corev1alpha2.ApplicationConfigurationList

	if opt.AppName != "" {
		var application corev1alpha2.ApplicationConfiguration
		if err := c.Get(ctx, client.ObjectKey{Name: opt.AppName, Namespace: opt.Namespace}, &application); err != nil {
			return nil, err
		}
		applicationList.Items = append(applicationList.Items, application)
	} else {
		err := c.List(ctx, &applicationList, &client.ListOptions{Namespace: opt.Namespace})
		if err != nil {
			return nil, err
		}
	}

	for _, a := range applicationList.Items {
		for _, com := range a.Spec.Components {
			component, err := cmdutil.GetComponent(ctx, c, com.ComponentName, opt.Namespace)
			if err != nil {
				return componentMetaList, err
			}
			traitAlias := GetTraitAliasByComponentTraitList(com.Traits)
			var workload string
			if component.Annotations != nil {
				workload = component.Annotations[types.AnnWorkloadDef]
			}
			componentMetaList = append(componentMetaList, ComponentMeta{
				Name:        com.ComponentName,
				App:         a.Name,
				Workload:    workload,
				Status:      types.StatusDeployed,
				Traits:      traitAlias,
				CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
				Component:   component,
				AppConfig:   a,
			})
		}
	}
	return componentMetaList, nil
}

func RetrieveApplicationStatusByName(ctx context.Context, c client.Client, applicationName string, namespace string) (apis.ApplicationStatusMeta, error) {
	var applicationStatusMeta apis.ApplicationStatusMeta
	var appConfig corev1alpha2.ApplicationConfiguration
	if err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &appConfig); err != nil {
		return applicationStatusMeta, err
	}
	for _, com := range appConfig.Spec.Components {
		// Just get the one component from appConfig
		if com.ComponentName != applicationName {
			continue
		}
		component, err := cmdutil.GetComponent(ctx, c, com.ComponentName, namespace)
		if err != nil {
			return applicationStatusMeta, err
		}
		var status = "UNKNOWN"
		if len(appConfig.Status.Conditions) != 0 {
			status = string(appConfig.Status.Conditions[0].Status)
		}
		applicationStatusMeta = apis.ApplicationStatusMeta{
			Status:   status,
			Workload: component.Spec,
			Traits:   com.Traits,
		}
	}
	return applicationStatusMeta, nil
}
