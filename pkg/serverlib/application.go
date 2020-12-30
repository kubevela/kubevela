package serverlib

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/server/apis"
)

// nolint:golint
const (
	DefaultChosenAllSvc = "ALL SERVICES"
	FlagNotSet          = "FlagNotSet"
	FlagIsInvalid       = "FlagIsInvalid"
	FlagIsValid         = "FlagIsValid"
)

type componentMetaList []apis.ComponentMeta
type applicationMetaList []apis.ApplicationMeta

func (comps componentMetaList) Len() int {
	return len(comps)
}
func (comps componentMetaList) Swap(i, j int) {
	comps[i], comps[j] = comps[j], comps[i]
}
func (comps componentMetaList) Less(i, j int) bool {
	return comps[i].CreatedTime > comps[j].CreatedTime
}

func (a applicationMetaList) Len() int {
	return len(a)
}
func (a applicationMetaList) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a applicationMetaList) Less(i, j int) bool {
	return a[i].CreatedTime > a[j].CreatedTime
}

// Option is option work with dashboard api server
type Option struct {
	// Optional filter, if specified, only components in such app will be listed
	AppName string

	Namespace string
}

// DeleteOptions is options for delete
type DeleteOptions struct {
	AppName  string
	CompName string
	Client   client.Client
	Env      *types.EnvMeta
}

// ListApplications lists all applications
func ListApplications(ctx context.Context, c client.Client, opt Option) ([]apis.ApplicationMeta, error) {
	var applicationMetaList applicationMetaList
	appConfigList, err := ListApplicationConfigurations(ctx, c, opt)
	if err != nil {
		return nil, err
	}

	for _, a := range appConfigList.Items {
		// ignore the deleted resource
		if a.GetDeletionGracePeriodSeconds() != nil {
			continue
		}
		applicationMeta, err := RetrieveApplicationStatusByName(ctx, c, a.Name, a.Namespace)
		if err != nil {
			return applicationMetaList, nil
		}
		applicationMeta.Components = nil
		applicationMetaList = append(applicationMetaList, applicationMeta)
	}
	sort.Stable(applicationMetaList)
	return applicationMetaList, nil
}

// ListApplicationConfigurations lists all OAM ApplicationConfiguration
func ListApplicationConfigurations(ctx context.Context, c client.Reader, opt Option) (corev1alpha2.ApplicationConfigurationList, error) {
	var appConfigList corev1alpha2.ApplicationConfigurationList

	if opt.AppName != "" {
		var appConfig corev1alpha2.ApplicationConfiguration
		if err := c.Get(ctx, client.ObjectKey{Name: opt.AppName, Namespace: opt.Namespace}, &appConfig); err != nil {
			return appConfigList, err
		}
		appConfigList.Items = append(appConfigList.Items, appConfig)
	} else {
		err := c.List(ctx, &appConfigList, &client.ListOptions{Namespace: opt.Namespace})
		if err != nil {
			return appConfigList, err
		}
	}
	return appConfigList, nil
}

// ListComponents will list all components for dashboard
func ListComponents(ctx context.Context, c client.Client, opt Option) ([]apis.ComponentMeta, error) {
	var componentMetaList componentMetaList
	var appConfigList corev1alpha2.ApplicationConfigurationList
	var err error
	if appConfigList, err = ListApplicationConfigurations(ctx, c, opt); err != nil {
		return nil, err
	}

	for _, a := range appConfigList.Items {
		for _, com := range a.Spec.Components {
			component, err := cmdutil.GetComponent(ctx, c, com.ComponentName, opt.Namespace)
			if err != nil {
				return componentMetaList, err
			}
			componentMetaList = append(componentMetaList, apis.ComponentMeta{
				Name:        com.ComponentName,
				Status:      types.StatusDeployed,
				CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
				Component:   component,
				AppConfig:   a,
				App:         a.Name,
			})
		}
	}
	sort.Stable(componentMetaList)
	return componentMetaList, nil
}

// RetrieveApplicationStatusByName will get app status
func RetrieveApplicationStatusByName(ctx context.Context, c client.Client, applicationName string, namespace string) (apis.ApplicationMeta, error) {
	var applicationMeta apis.ApplicationMeta
	var appConfig corev1alpha2.ApplicationConfiguration
	if err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &appConfig); err != nil {
		return applicationMeta, err
	}

	var status = "Unknown"
	if len(appConfig.Status.Conditions) != 0 {
		status = string(appConfig.Status.Conditions[0].Status)
	}
	applicationMeta.Name = appConfig.Name
	applicationMeta.Status = status
	applicationMeta.CreatedTime = appConfig.CreationTimestamp.Format(time.RFC3339)

	for _, com := range appConfig.Spec.Components {
		componentName := com.ComponentName
		component, err := cmdutil.GetComponent(ctx, c, componentName, namespace)
		if err != nil {
			return applicationMeta, err
		}

		applicationMeta.Components = append(applicationMeta.Components, apis.ComponentMeta{
			Name:     componentName,
			Status:   status,
			Workload: component.Spec.Workload,
			Traits:   com.Traits,
		})
		applicationMeta.Status = status

	}
	return applicationMeta, nil
}

// DeleteApp will delete app including server side
func (o *DeleteOptions) DeleteApp() (string, error) {
	if err := application.Delete(o.Env.Name, o.AppName); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	ctx := context.Background()
	var app=new(corev1alpha2.Application)
	err := o.Client.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Env.Namespace}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("app %s already deleted", o.AppName), nil
		}
		return "", fmt.Errorf("delete appconfig err %w", err)
	}

	err = o.Client.Delete(ctx, app)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete application err %w", err)
	}

	return fmt.Sprintf("delete apps succeed %s from %s", o.AppName, o.Env.Name), nil
}

// DeleteComponent will delete one component including server side.
func (o *DeleteOptions) DeleteComponent(io cmdutil.IOStreams) (string, error) {
	var app *driver.Application
	var err error
	if o.AppName != "" {
		app, err = application.Load(o.Env.Name, o.AppName)
	} else {
		app, err = application.MatchAppByComp(o.Env.Name, o.CompName)
	}
	if err != nil {
		return "", err
	}

	if len(application.GetComponents(app)) <= 1 {
		return o.DeleteApp()
	}

	// Remove component from local appfile
	if err := application.RemoveComponent(app, o.CompName); err != nil {
		return "", err
	}
	if err := application.Save(app, o.Env.Name); err != nil {
		return "", err
	}

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()
	if err := application.BuildRun(ctx, app, o.Client, o.Env, io); err != nil {
		return "", err
	}

	// Remove component in k8s cluster
	var c corev1alpha2.Component
	c.Name = o.CompName
	c.Namespace = o.Env.Namespace
	err = o.Client.Delete(context.Background(), &c)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete component err: %w", err)
	}

	return fmt.Sprintf("delete component succeed %s from %s", o.CompName, o.AppName), nil
}

func chooseSvc(services []string) (string, error) {
	var svcName string
	services = append(services, DefaultChosenAllSvc)
	prompt := &survey.Select{
		Message: "Please choose one service: ",
		Options: services,
		Default: DefaultChosenAllSvc,
	}
	err := survey.AskOne(prompt, &svcName)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve services of the application, err %w", err)
	}
	return svcName, nil
}

// GetServicesWhenDescribingApplication gets the target services list either from cli `--svc` flag or from survey
func GetServicesWhenDescribingApplication(cmd *cobra.Command, app *driver.Application) ([]string, error) {
	var svcFlag string
	var svcFlagStatus string
	// to store the value of flag `--svc` set in Cli, or selected value in survey
	var targetServices []string
	if svcFlag = cmd.Flag("svc").Value.String(); svcFlag == "" {
		svcFlagStatus = FlagNotSet
	} else {
		svcFlagStatus = FlagIsInvalid
	}
	// all services name of the application `appName`
	var services []string
	for svcName := range app.Services {
		services = append(services, svcName)
		if svcFlag == svcName {
			svcFlagStatus = FlagIsValid
			targetServices = append(targetServices, svcName)
		}
	}
	totalServices := len(services)
	if svcFlagStatus == FlagNotSet && totalServices == 1 {
		targetServices = services
	}
	if svcFlagStatus == FlagIsInvalid || (svcFlagStatus == FlagNotSet && totalServices > 1) {
		if svcFlagStatus == FlagIsInvalid {
			cmd.Printf("The service name '%s' is not valid\n", svcFlag)
		}
		chosenSvc, err := chooseSvc(services)
		if err != nil {
			return []string{}, err
		}

		if chosenSvc == DefaultChosenAllSvc {
			targetServices = services
		} else {
			targetServices = targetServices[:0]
			targetServices = append(targetServices, chosenSvc)
		}
	}
	return targetServices, nil
}
