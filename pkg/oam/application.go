package oam

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/server/apis"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type componentMetaList []ComponentMeta

func (comps componentMetaList) Len() int {
	return len(comps)
}
func (comps componentMetaList) Swap(i, j int) {
	comps[i], comps[j] = comps[j], comps[i]
}
func (comps componentMetaList) Less(i, j int) bool {
	return comps[i].CreatedTime > comps[j].CreatedTime
}

type Option struct {
	// Optional filter, if specified, only components in such app will be listed
	AppName string

	Namespace string
}

type DeleteOptions struct {
	AppName  string
	CompName string
	Client   client.Client
	Env      *types.EnvMeta
}

/*
	Get component list
*/
func ListComponents(ctx context.Context, c client.Client, opt Option) ([]ComponentMeta, error) {
	var componentMetaList componentMetaList
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
	sort.Stable(componentMetaList)
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

func (o *DeleteOptions) DeleteApp() (string, error) {
	if err := application.Delete(o.Env.Name, o.AppName); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	ctx := context.Background()
	var appConfig corev1alpha2.ApplicationConfiguration
	err := o.Client.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Env.Namespace}, &appConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", err
		}
		return "", fmt.Errorf("delete appconfig err %s", err)
	}
	for _, comp := range appConfig.Spec.Components {
		var c corev1alpha2.Component
		//TODO(wonderflow): what if we use componentRevision here?
		c.Name = comp.ComponentName
		c.Namespace = o.Env.Namespace
		err = o.Client.Delete(ctx, &c)
		if err != nil && !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("delete component err: %s", err)
		}
	}
	err = o.Client.Delete(ctx, &appConfig)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete appconfig err %s", err)
	}

	var healthscope corev1alpha2.HealthScope
	healthscope.Name = application.FormatDefaultHealthScopeName(o.AppName)
	healthscope.Namespace = o.Env.Namespace
	err = o.Client.Delete(ctx, &healthscope)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete health scope %s err %v", healthscope.Name, err)
	}

	return fmt.Sprintf("delete apps succeed %s from %s", o.AppName, o.Env.Name), nil
}

func (o *DeleteOptions) DeleteComponent() (string, error) {
	var app *application.Application
	var err error
	if o.AppName != "" {
		app, err = application.Load(o.Env.Name, o.AppName)
	} else {
		app, err = application.MatchAppByComp(o.Env.Name, o.CompName)
	}
	if err != nil {
		return "", err
	}

	if len(app.GetComponents()) <= 1 {
		return o.DeleteApp()
	}

	// Remove component from local appfile
	if err := app.RemoveComponent(o.CompName); err != nil {
		return "", err
	}
	if err := app.Save(o.Env.Name); err != nil {
		return "", err
	}

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()
	if err := app.Run(ctx, o.Client, o.Env); err != nil {
		return "", err
	}

	// Remove component in k8s cluster
	var c corev1alpha2.Component
	c.Name = o.CompName
	c.Namespace = o.Env.Namespace
	err = o.Client.Delete(context.Background(), &c)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete component err: %s", err)
	}

	return fmt.Sprintf("delete component succeed %s from %s", o.CompName, o.AppName), nil
}
