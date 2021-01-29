package serverlib

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/api"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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

// AppfileOptions is some configuration that modify options for an Appfile
type AppfileOptions struct {
	Kubecli client.Client
	IO      cmdutil.IOStreams
	Env     *types.EnvMeta
}

// BuildResult is the export struct from AppFile yaml or AppFile object
type BuildResult struct {
	appFile     *api.AppFile
	application *v1alpha2.Application
	scopes      []oam.Object
}

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
	if err := appfile.Delete(o.Env.Name, o.AppName); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	ctx := context.Background()
	var app = new(corev1alpha2.Application)
	err := o.Client.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Env.Namespace}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("app \"%s\" already deleted", o.AppName), nil
		}
		return "", fmt.Errorf("delete appconfig err: %w", err)
	}

	err = o.Client.Delete(ctx, app)
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("delete application err: %w", err)
	}

	// TODO(wonderflow): delete the default health scope here
	return fmt.Sprintf("app \"%s\" deleted from env \"%s\"", o.AppName, o.Env.Name), nil
}

// DeleteComponent will delete one component including server side.
func (o *DeleteOptions) DeleteComponent(io cmdutil.IOStreams) (string, error) {
	var app *api.Application
	var err error
	if o.AppName != "" {
		app, err = appfile.LoadApplication(o.Env.Name, o.AppName)
	} else {
		app, err = appfile.MatchAppByComp(o.Env.Name, o.CompName)
	}
	if err != nil {
		return "", err
	}

	if len(appfile.GetComponents(app)) <= 1 {
		return o.DeleteApp()
	}

	// Remove component from local appfile
	if err := appfile.RemoveComponent(app, o.CompName); err != nil {
		return "", err
	}
	if err := appfile.Save(app, o.Env.Name); err != nil {
		return "", err
	}

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()
	if err := appfile.BuildRun(ctx, app, o.Client, o.Env, io); err != nil {
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

	return fmt.Sprintf("component \"%s\" deleted from \"%s\"", o.CompName, o.AppName), nil
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
func GetServicesWhenDescribingApplication(cmd *cobra.Command, app *api.Application) ([]string, error) {
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

func saveRemoteAppfile(url string) (string, error) {
	body, err := common.HTTPGet(context.Background(), url)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(url)
	dest := "Appfile"
	if ext == ".json" {
		dest = "vela.json"
	} else if ext == ".yaml" || ext == ".yml" {
		dest = "vela.yaml"
	}
	//nolint:gosec
	return dest, ioutil.WriteFile(dest, body, 0644)
}

// ExportFromAppFile exports Application from appfile object
func (o *AppfileOptions) ExportFromAppFile(app *api.AppFile, quiet bool) (*BuildResult, []byte, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, nil, err
	}

	appHandler := appfile.NewApplication(app, tm)

	// new
	retApplication, scopes, err := appHandler.BuildOAMApplication(o.Env, o.IO, appHandler.Tm, quiet)
	if err != nil {
		return nil, nil, err
	}

	var w bytes.Buffer

	options := json.SerializerOptions{Yaml: true, Pretty: false, Strict: false}
	enc := json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, nil, options)
	err = enc.Encode(retApplication, &w)
	if err != nil {
		return nil, nil, fmt.Errorf("yaml encode application failed: %w", err)
	}
	w.WriteByte('\n')

	for _, scope := range scopes {
		w.WriteString("---\n")
		err = enc.Encode(scope, &w)
		if err != nil {
			return nil, nil, fmt.Errorf("yaml encode scope (%s) failed: %w", scope.GetName(), err)
		}
		w.WriteByte('\n')
	}

	result := &BuildResult{
		appFile:     app,
		application: retApplication,
		scopes:      scopes,
	}
	return result, w.Bytes(), nil
}

// Export export Application object from the path of Appfile
func (o *AppfileOptions) Export(filePath string, quiet bool) (*BuildResult, []byte, error) {
	var app *api.AppFile
	var err error
	if !quiet {
		o.IO.Info("Parsing vela appfile ...")
	}
	if filePath != "" {
		if strings.HasPrefix(filePath, "https://") || strings.HasPrefix(filePath, "http://") {
			filePath, err = saveRemoteAppfile(filePath)
			if err != nil {
				return nil, nil, err
			}
		}
		app, err = api.LoadFromFile(filePath)
	} else {
		app, err = api.Load()
	}
	if err != nil {
		return nil, nil, err
	}

	if !quiet {
		o.IO.Info("Load Template ...")
	}
	return o.ExportFromAppFile(app, quiet)
}

// Run starts an application according to Appfile
func (o *AppfileOptions) Run(filePath string, config *rest.Config) error {
	result, data, err := o.Export(filePath, false)
	if err != nil {
		return err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return err
	}
	return o.BaseAppFileRun(result, data, dm)
}

// BaseAppFileRun starts an application according to Appfile
func (o *AppfileOptions) BaseAppFileRun(result *BuildResult, data []byte, dm discoverymapper.DiscoveryMapper) error {
	deployFilePath := ".vela/deploy.yaml"
	o.IO.Infof("Writing deploy config to (%s)\n", deployFilePath)
	if err := os.MkdirAll(filepath.Dir(deployFilePath), 0700); err != nil {
		return err
	}

	if err := ioutil.WriteFile(deployFilePath, data, 0600); err != nil {
		return errors.Wrap(err, "write deploy config manifests failed")
	}

	if err := o.saveToAppDir(result.appFile); err != nil {
		return errors.Wrap(err, "save to app dir failed")
	}

	kubernetesComponent, err := appfile.ApplyTerraform(result.application, o.Kubecli, o.IO, o.Env.Namespace, dm)
	if err != nil {
		return err
	}
	result.application.Spec.Components = kubernetesComponent

	o.IO.Infof("\nApplying application ...\n")
	return o.ApplyApp(result.application, result.scopes)
}

func (o *AppfileOptions) saveToAppDir(f *api.AppFile) error {
	app := &api.Application{AppFile: f}
	return appfile.Save(app, o.Env.Name)
}

// ApplyApp applys config resources for the app.
// It differs by create and update:
// - for create, it displays app status along with information of url, metrics, ssh, logging.
// - for update, it rolls out a canary deployment and prints its information. User can verify the canary deployment.
//   This will wait for user approval. If approved, it continues upgrading the whole; otherwise, it would rollback.
func (o *AppfileOptions) ApplyApp(app *v1alpha2.Application, scopes []oam.Object) error {
	key := apitypes.NamespacedName{
		Namespace: app.Namespace,
		Name:      app.Name,
	}
	o.IO.Infof("Checking if app has been deployed...\n")
	var tmpApp v1alpha2.Application
	err := o.Kubecli.Get(context.TODO(), key, &tmpApp)
	switch {
	case apierrors.IsNotFound(err):
		o.IO.Infof("App has not been deployed, creating a new deployment...\n")
	case err == nil:
		o.IO.Infof("App exists, updating existing deployment...\n")
	default:
		return err
	}
	if err := o.apply(app, scopes); err != nil {
		return err
	}
	o.IO.Infof(o.Info(app))
	return nil
}

func (o *AppfileOptions) apply(app *v1alpha2.Application, scopes []oam.Object) error {
	if err := appfile.Run(context.TODO(), o.Kubecli, app, scopes); err != nil {
		return err
	}
	return nil
}

// Info shows the status of each service in the Appfile
func (o *AppfileOptions) Info(app *v1alpha2.Application) string {
	appName := app.Name
	var appUpMessage = "✅ App has been deployed 🚀🚀🚀\n" +
		fmt.Sprintf("    Port forward: vela port-forward %s\n", appName) +
		fmt.Sprintf("             SSH: vela exec %s\n", appName) +
		fmt.Sprintf("         Logging: vela logs %s\n", appName) +
		fmt.Sprintf("      App status: vela status %s\n", appName)
	for _, comp := range app.Spec.Components {
		appUpMessage += fmt.Sprintf("  Service status: vela status %s --svc %s\n", appName, comp.Name)
	}
	return appUpMessage
}
