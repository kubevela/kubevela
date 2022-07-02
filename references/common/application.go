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
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

const (
	resourceTrackerFinalizer = "app.oam.dev/resource-tracker-finalizer"
	// legacyOnlyRevisionFinalizer is to delete all resource trackers of app revisions which may be used
	// out of the domain of app controller, e.g., AppRollout controller.
	legacyOnlyRevisionFinalizer = "app.oam.dev/only-revision-finalizer"
)

// AppfileOptions is some configuration that modify options for an Appfile
type AppfileOptions struct {
	Kubecli   client.Client
	IO        cmdutil.IOStreams
	Namespace string
}

// BuildResult is the export struct from AppFile yaml or AppFile object
type BuildResult struct {
	appFile     *api.AppFile
	application *corev1beta1.Application
	scopes      []oam.Object
}

// Option is option work with dashboard api server
type Option struct {
	// Optional filter, if specified, only components in such app will be listed
	AppName string

	Namespace string
}

// DeleteOptions is options for delete
type DeleteOptions struct {
	Namespace string
	AppName   string
	CompName  string
	Client    client.Client
	C         common.Args

	Wait        bool
	ForceDelete bool
}

// DeleteApp will delete app including server side
func (o *DeleteOptions) DeleteApp(io cmdutil.IOStreams) error {
	if o.ForceDelete {
		return o.ForceDeleteApp(io)
	}
	if o.Wait {
		return o.WaitUntilDeleteApp(io)
	}
	return o.DeleteAppWithoutDoubleCheck(io)
}

// ForceDeleteApp force delete the application
func (o *DeleteOptions) ForceDeleteApp(io cmdutil.IOStreams) error {
	ctx := context.Background()
	err := o.DeleteAppWithoutDoubleCheck(io)
	if err != nil {
		return err
	}
	app := new(corev1beta1.Application)
	err = o.Client.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Namespace}, app)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	io.Info("force deleted the resources created by application")
	err = wait.PollImmediate(1*time.Second, 1*time.Minute, func() (done bool, err error) {
		err = o.Client.Get(ctx, client.ObjectKeyFromObject(app), app)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		rk, err := resourcekeeper.NewResourceKeeper(ctx, o.Client, app)
		if err != nil {
			return false, errors.Wrapf(err, "failed to create resource keeper to run garbage collection")
		}
		if done, _, err = rk.GarbageCollect(ctx); err != nil && !apierrors.IsConflict(err) {
			return false, errors.Wrapf(err, "failed to run garbage collect")
		}
		if done {
			meta.RemoveFinalizer(app, resourceTrackerFinalizer)
			meta.RemoveFinalizer(app, legacyOnlyRevisionFinalizer)
			if err = o.Client.Update(ctx, app); err != nil && !apierrors.IsConflict(err) && !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to update app finalizer")
			}
		}
		return false, nil
	})
	if err != nil {
		io.Info("successfully cleanup the resources created by application, but fail to delete the application")
		return err
	}
	return nil
}

// WaitUntilDeleteApp will wait until the application is completely deleted
func (o *DeleteOptions) WaitUntilDeleteApp(io cmdutil.IOStreams) error {
	tryCnt, startTime := 0, time.Now()
	writer := uilive.New()
	writer.Start()
	defer writer.Stop()

	io.Infof(color.New(color.FgYellow).Sprintf("waiting for delete the application \"%s\"...\n", o.AppName))
	err := wait.PollImmediate(2*time.Second, 5*time.Minute, func() (done bool, err error) {
		tryCnt++
		fmt.Fprintf(writer, "try to delete the application for the %d time, wait a total of %f s\n", tryCnt, time.Since(startTime).Seconds())
		err = o.DeleteAppWithoutDoubleCheck(io)
		if err != nil {
			fmt.Printf("Failed delete Application \"%s\": %s\n", o.AppName, err.Error())
			return false, nil
		}
		app := new(corev1beta1.Application)
		err = o.Client.Get(context.Background(), client.ObjectKey{Name: o.AppName, Namespace: o.Namespace}, app)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		io.Info("waiting for the application to be deleted timed out, please try again")
		return err
	}
	return nil
}

// DeleteAppWithoutDoubleCheck delete application without double check
func (o *DeleteOptions) DeleteAppWithoutDoubleCheck(io cmdutil.IOStreams) error {
	ctx := context.Background()

	if o.ForceDelete {
		if err := prepareToForceDeleteTerraformComponents(ctx, o.Client, o.Namespace, o.AppName); err != nil {
			return err
		}
	}

	var app = new(corev1beta1.Application)
	err := o.Client.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Namespace}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("app %s already deleted or not exist", o.AppName)
		}
		return fmt.Errorf("delete application err: %w", err)
	}

	err = o.Client.Delete(ctx, app)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete application err: %w", err)
	}

	for _, cmp := range app.Spec.Components {
		healthScopeName, ok := cmp.Scopes[api.DefaultHealthScopeKey]
		if ok {
			var healthScope corev1alpha2.HealthScope
			if err := o.Client.Get(ctx, client.ObjectKey{Namespace: o.Namespace, Name: healthScopeName}, &healthScope); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("delete health scope %s err: %w", healthScopeName, err)
			}
			if err = o.Client.Delete(ctx, &healthScope); err != nil {
				return fmt.Errorf("delete health scope %s err: %w", healthScopeName, err)
			}
		}
	}
	return nil
}

// prepareToForceDeleteTerraformComponents sets Terraform typed Component to force-delete mode
func prepareToForceDeleteTerraformComponents(ctx context.Context, k8sClient client.Client, namespace, name string) error {
	var (
		app         = new(corev1beta1.Application)
		forceDelete = true
	)
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("app %s already deleted or not exist", name)
		}
		return fmt.Errorf("delete application err: %w", err)
	}
	for _, c := range app.Spec.Components {
		var def corev1beta1.ComponentDefinition
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: c.Type, Namespace: types.DefaultKubeVelaNS}, &def); err != nil {
			return err
		}
		if def.Spec.Schematic != nil && def.Spec.Schematic.Terraform != nil {
			var conf terraformapi.Configuration
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: c.Name, Namespace: namespace}, &conf); err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
			}
			conf.Spec.ForceDelete = &forceDelete
			if err := k8sClient.Update(ctx, &conf); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteComponent will delete one component including server side.
func (o *DeleteOptions) DeleteComponent(io cmdutil.IOStreams) error {
	var err error
	if o.AppName == "" {
		return errors.New("app name is required")
	}
	app, err := appfile.LoadApplication(o.Namespace, o.AppName, o.C)
	if err != nil {
		return err
	}

	if len(appfile.GetComponents(app)) <= 1 {
		return o.DeleteApp(io)
	}

	// Remove component from local appfile
	if err := appfile.RemoveComponent(app, o.CompName); err != nil {
		return err
	}

	// Remove component from appConfig in k8s cluster
	ctx := context.Background()

	if err := o.Client.Update(ctx, app); err != nil {
		return err
	}

	// It's the server responsibility to GC component
	return nil
}

// LoadAppFile will load vela appfile from remote URL or local file system.
func LoadAppFile(pathOrURL string) (*api.AppFile, error) {
	body, err := ReadRemoteOrLocalPath(pathOrURL)
	if err != nil {
		return nil, err
	}
	return api.LoadFromBytes(body)
}

// ReadRemoteOrLocalPath will read a path remote or locally
func ReadRemoteOrLocalPath(pathOrURL string) ([]byte, error) {
	if pathOrURL == "-" {
		return ioutil.ReadAll(os.Stdin)
	}
	var body []byte
	var err error
	if utils.IsValidURL(pathOrURL) {
		body, err = common.HTTPGetWithOption(context.Background(), pathOrURL, nil)
		if err != nil {
			return nil, err
		}
		if err = localSave(pathOrURL, body); err != nil {
			return nil, err
		}
	} else {
		body, err = os.ReadFile(filepath.Clean(pathOrURL))
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

// IsAppfile check if a file is Appfile format or application format, return true if it's appfile, false means application object
func IsAppfile(body []byte) bool {
	if j.Valid(body) {
		// we only support json format for appfile
		return true
	}
	res := map[string]interface{}{}
	err := yaml.Unmarshal(body, &res)
	if err != nil {
		return false
	}
	// appfile didn't have apiVersion
	if _, ok := res["apiVersion"]; ok {
		return false
	}
	return true
}

func localSave(url string, body []byte) error {
	var name string
	ext := filepath.Ext(url)
	switch ext {
	case ".json":
		name = "vela.json"
	case ".yaml", ".yml":
		name = "vela.yaml"
	default:
		if j.Valid(body) {
			name = "vela.json"
		} else {
			name = "vela.yaml"
		}
	}
	//nolint:gosec
	return os.WriteFile(name, body, 0644)
}

// ExportFromAppFile exports Application from appfile object
func (o *AppfileOptions) ExportFromAppFile(app *api.AppFile, namespace string, quiet bool, c common.Args) (*BuildResult, []byte, error) {
	tm, err := template.Load(namespace, c)
	if err != nil {
		return nil, nil, err
	}

	appHandler := appfile.NewApplication(app, tm)

	// new
	retApplication, err := appHandler.ConvertToApplication(o.Namespace, o.IO, appHandler.Tm, quiet)
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

	result := &BuildResult{
		appFile:     app,
		application: retApplication,
	}
	return result, w.Bytes(), nil
}

// Export export Application object from the path of Appfile
func (o *AppfileOptions) Export(filePath, namespace string, quiet bool, c common.Args) (*BuildResult, []byte, error) {
	var app *api.AppFile
	var err error
	if !quiet {
		o.IO.Info("Parsing vela application file ...")
	}
	if filePath != "" {
		app, err = LoadAppFile(filePath)
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
	result, _, err := o.Export(filePath, namespace, false, c)
	if err != nil {
		return err
	}
	return o.BaseAppFileRun(result, c)
}

// BaseAppFileRun starts an application according to Appfile
func (o *AppfileOptions) BaseAppFileRun(result *BuildResult, args common.Args) error {

	kubernetesComponent, err := appfile.ApplyTerraform(result.application, o.Kubecli, o.IO, o.Namespace, args)
	if err != nil {
		return err
	}
	result.application.Spec.Components = kubernetesComponent

	o.IO.Infof("\nApplying application ...\n")
	return o.ApplyApp(result.application, result.scopes)
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
	o.IO.Infof(Info(app))
	return nil
}

func (o *AppfileOptions) apply(app *corev1beta1.Application, scopes []oam.Object) error {
	if err := appfile.Run(context.TODO(), o.Kubecli, app, scopes); err != nil {
		return err
	}
	return nil
}

// Info shows the status of each service in the Appfile
func Info(app *corev1beta1.Application) string {
	yellow := color.New(color.FgYellow)
	appName := app.Name
	var appUpMessage = "✅ App has been deployed 🚀🚀🚀\n" +
		"    Port forward: " + yellow.Sprintf("vela port-forward %s\n", appName) +
		"             SSH: " + yellow.Sprintf("vela exec %s\n", appName) +
		"         Logging: " + yellow.Sprintf("vela logs %s\n", appName) +
		"      App status: " + yellow.Sprintf("vela status %s\n", appName) +
		"        Endpoint: " + yellow.Sprintf("vela status %s --endpoint\n", appName)
	return appUpMessage
}

// ApplyApplication will apply an application file in K8s GVK format
func ApplyApplication(app corev1beta1.Application, ioStream cmdutil.IOStreams, clt client.Client) error {
	if app.Namespace == "" {
		app.Namespace = types.DefaultAppNamespace
	}
	_, err := ioStream.Out.Write([]byte("Applying an application in vela K8s object format...\n"))
	if err != nil {
		return err
	}
	applicator := apply.NewAPIApplicator(clt)
	err = applicator.Apply(context.Background(), &app)
	if err != nil {
		return err
	}
	ioStream.Infof(Info(&app))
	return nil
}
