/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"bytes"
	"context"
	j "encoding/json"
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
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
	application *corev1beta1.Application
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
	C        common.Args
}

// ListApplications lists all applications
func ListApplications(ctx context.Context, c client.Reader, opt Option) ([]apis.ApplicationMeta, error) {
	var applicationMetaList applicationMetaList
	var appList corev1beta1.ApplicationList
	if opt.AppName != "" {
		var app corev1beta1.Application
		if err := c.Get(ctx, client.ObjectKey{Name: opt.AppName, Namespace: opt.Namespace}, &app); err != nil {
			return applicationMetaList, err
		}
		appList.Items = append(appList.Items, app)
	} else {
		err := c.List(ctx, &appList, &client.ListOptions{Namespace: opt.Namespace})
		if err != nil {
			return applicationMetaList, err
		}
	}
	for _, a := range appList.Items {
		// ignore the deleted resource
		if a.GetDeletionGracePeriodSeconds() != nil {
			continue
		}
		applicationMeta, err := RetrieveApplicationStatusByName(ctx, c, a.Name, a.Namespace)
		if err != nil {
			return applicationMetaList, err
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
func ListComponents(ctx context.Context, c client.Reader, opt Option) ([]apis.ComponentMeta, error) {
	var componentMetaList componentMetaList
	var appConfigList corev1alpha2.ApplicationConfigurationList
	var err error
	if appConfigList, err = ListApplicationConfigurations(ctx, c, opt); err != nil {
		return nil, err
	}

	for _, a := range appConfigList.Items {
		for _, com := range a.Spec.Components {
			component, _, err := oamutil.GetComponent(ctx, c, com, opt.Namespace)
			if err != nil {
				return componentMetaList, err
			}
			componentMetaList = append(componentMetaList, apis.ComponentMeta{
				Name:        com.ComponentName,
				Status:      types.StatusDeployed,
				CreatedTime: a.ObjectMeta.CreationTimestamp.String(),
				Component:   *component,
				AppConfig:   a,
				App:         a.Name,
			})
		}
	}
	sort.Stable(componentMetaList)
	return componentMetaList, nil
}

// RetrieveApplicationStatusByName will get app status
func RetrieveApplicationStatusByName(ctx context.Context, c client.Reader, applicationName string,
	namespace string) (apis.ApplicationMeta, error) {
	var applicationMeta apis.ApplicationMeta
	var app corev1beta1.Application
	if err := c.Get(ctx, client.ObjectKey{Name: applicationName, Namespace: namespace}, &app); err != nil {
		return applicationMeta, err
	}
	applicationMeta.Name = app.Name
	applicationMeta.Status = string(app.Status.Phase)
	applicationMeta.CreatedTime = app.CreationTimestamp.Format(time.RFC3339)

	return applicationMeta, nil
}

// DeleteApp will delete app including server side
func (o *DeleteOptions) DeleteApp() (string, error) {
	if err := appfile.Delete(o.Env.Name, o.AppName); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	ctx := context.Background()
	var app = new(corev1beta1.Application)
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

	for _, cmp := range app.Spec.Components {
		healthScopeName, ok := cmp.Scopes[api.DefaultHealthScopeKey]
		if ok {
			var healthScope corev1alpha2.HealthScope
			if err := o.Client.Get(ctx, client.ObjectKey{Namespace: o.Env.Namespace, Name: healthScopeName}, &healthScope); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return "", fmt.Errorf("delete health scope %s err: %w", healthScopeName, err)
			}
			if err = o.Client.Delete(ctx, &healthScope); err != nil {
				return "", fmt.Errorf("delete health scope %s err: %w", healthScopeName, err)
			}
		}
	}
	return fmt.Sprintf("app \"%s\" deleted from env \"%s\"", o.AppName, o.Env.Name), nil
}

// DeleteComponent will delete one component including server side.
func (o *DeleteOptions) DeleteComponent(io cmdutil.IOStreams) (string, error) {
	var err error
	if o.AppName == "" {
		return "", errors.New("app name is required")
	}
	app, err := appfile.LoadApplication(o.Env.Namespace, o.AppName, o.C)
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

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()

	if err := o.Client.Update(ctx, app); err != nil {
		return "", err
	}

	// It's the server responsibility to GC component

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

func saveAndLoadRemoteAppfile(url string) (*api.AppFile, error) {
	body, err := common.HTTPGet(context.Background(), url)
	if err != nil {
		return nil, err
	}
	af := api.NewAppFile()
	ext := filepath.Ext(url)
	dest := "Appfile"
	switch ext {
	case ".json":
		dest = "vela.json"
		af, err = api.JSONToYaml(body, af)
	case ".yaml", ".yml":
		dest = "vela.yaml"
		err = yaml.Unmarshal(body, af)
	default:
		if j.Valid(body) {
			af, err = api.JSONToYaml(body, af)
		} else {
			err = yaml.Unmarshal(body, af)
		}
	}
	if err != nil {
		return nil, err
	}
	//nolint:gosec
	return af, os.WriteFile(dest, body, 0644)
}

// ExportFromAppFile exports Application from appfile object
func (o *AppfileOptions) ExportFromAppFile(app *api.AppFile, namespace string, quiet bool, c common.Args) (*BuildResult, []byte, error) {
	tm, err := template.Load(namespace, c)
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
func (o *AppfileOptions) Export(filePath, namespace string, quiet bool, c common.Args) (*BuildResult, []byte, error) {
	var app *api.AppFile
	var err error
	if !quiet {
		o.IO.Info("Parsing vela appfile ...")
	}
	if filePath != "" {
		if strings.HasPrefix(filePath, "https://") || strings.HasPrefix(filePath, "http://") {
			app, err = saveAndLoadRemoteAppfile(filePath)
		} else {
			app, err = api.LoadFromFile(filePath)
		}
	} else {
		app, err = api.Load()
	}
	if err != nil {
		return nil, nil, err
	}

	if !quiet {
		o.IO.Info("Load Template ...")
	}
	return o.ExportFromAppFile(app, namespace, quiet, c)
}

// Run starts an application according to Appfile
func (o *AppfileOptions) Run(filePath, namespace string, c common.Args) error {
	result, data, err := o.Export(filePath, namespace, false, c)
	if err != nil {
		return err
	}
	return o.BaseAppFileRun(result, data, c)
}

// BaseAppFileRun starts an application according to Appfile
func (o *AppfileOptions) BaseAppFileRun(result *BuildResult, data []byte, args common.Args) error {
	deployFilePath := ".vela/deploy.yaml"
	o.IO.Infof("Writing deploy config to (%s)\n", deployFilePath)
	if err := os.MkdirAll(filepath.Dir(deployFilePath), 0700); err != nil {
		return err
	}

	if err := os.WriteFile(deployFilePath, data, 0600); err != nil {
		return errors.Wrap(err, "write deploy config manifests failed")
	}

	if err := o.saveToAppDir(result.appFile); err != nil {
		return errors.Wrap(err, "save to app dir failed")
	}

	kubernetesComponent, err := appfile.ApplyTerraform(result.application, o.Kubecli, o.IO, o.Env.Namespace, args)
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
func (o *AppfileOptions) ApplyApp(app *corev1beta1.Application, scopes []oam.Object) error {
	key := apitypes.NamespacedName{
		Namespace: app.Namespace,
		Name:      app.Name,
	}
	o.IO.Infof("Checking if app has been deployed...\n")
	var tmpApp corev1beta1.Application
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

func (o *AppfileOptions) apply(app *corev1beta1.Application, scopes []oam.Object) error {
	if err := appfile.Run(context.TODO(), o.Kubecli, app, scopes); err != nil {
		return err
	}
	return nil
}

// Info shows the status of each service in the Appfile
func (o *AppfileOptions) Info(app *corev1beta1.Application) string {
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

// ApplyApplication will apply an application file in K8s GVK format
func ApplyApplication(app corev1beta1.Application, ioStream cmdutil.IOStreams, clt client.Client) error {
	if app.Namespace == "" {
		app.Namespace = types.DefaultAppNamespace
	}
	_, err := ioStream.Out.Write([]byte("Applying an application in K8S format...\n"))
	if err != nil {
		return err
	}
	applicator := apply.NewAPIApplicator(clt)
	err = applicator.Apply(context.Background(), &app)
	if err != nil {
		return err
	}
	_, err = ioStream.Out.Write([]byte("Successfully apply application"))
	if err != nil {
		return err
	}
	return nil
}
