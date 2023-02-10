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

	"github.com/fatih/color"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

// AppfileOptions is some configuration that modify options for an Appfile
type AppfileOptions struct {
	Kubecli   client.Client
	IO        cmdutil.IOStreams
	Namespace string
	Name      string
}

// BuildResult is the export struct from AppFile yaml or AppFile object
type BuildResult struct {
	appFile     *api.AppFile
	application *corev1beta1.Application
	scopes      []oam.Object
}

// PrepareToForceDeleteTerraformComponents sets Terraform typed Component to force-delete mode
func PrepareToForceDeleteTerraformComponents(ctx context.Context, k8sClient client.Client, namespace, name string) error {
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
			if !apierrors.IsNotFound(err) {
				return err
			}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: c.Type, Namespace: namespace}, &def); err != nil {
				return err
			}
		}
		if def.Spec.Schematic != nil && def.Spec.Schematic.Terraform != nil {
			var conf terraformapi.Configuration
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: c.Name, Namespace: namespace}, &conf); err != nil {
				return err
			}
			conf.Spec.ForceDelete = &forceDelete
			if err := k8sClient.Update(ctx, &conf); err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadAppFile will load vela appfile from remote URL or local file system.
func LoadAppFile(pathOrURL string) (*api.AppFile, error) {
	body, err := utils.ReadRemoteOrLocalPath(pathOrURL, false)
	if err != nil {
		return nil, err
	}
	return api.LoadFromBytes(body)
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
	o.Name = result.application.Name
	o.IO.Infof("\nApplying application ...\n")
	return o.ApplyApp(result.application, result.scopes)
}

// ApplyApp applys config resources for the app.
// It differs by create and update:
//   - for create, it displays app status along with information of url, metrics, ssh, logging.
//   - for update, it rolls out a canary deployment and prints its information. User can verify the canary deployment.
//     This will wait for user approval. If approved, it continues upgrading the whole; otherwise, it would rollback.
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
	if app.Namespace != "" && app.Namespace != "default" {
		appName += " -n " + app.Namespace
	}
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

// CollectApplicationResource collects all resources of an application
func CollectApplicationResource(ctx context.Context, c client.Client, opt query.Option) ([]unstructured.Unstructured, error) {
	app := new(corev1beta1.Application)
	appKey := client.ObjectKey{Name: opt.Name, Namespace: opt.Namespace}
	if err := c.Get(context.Background(), appKey, app); err != nil {
		return nil, err
	}
	collector := query.NewAppCollector(c, opt)
	appResList, err := collector.ListApplicationResources(context.Background(), app)
	if err != nil {
		return nil, err
	}
	var resources = make([]unstructured.Unstructured, 0)
	for _, res := range appResList {
		if res.ResourceTree != nil {
			resources = append(resources, sonLeafResource(*res, res.ResourceTree, opt.Filter.Kind, opt.Filter.APIVersion)...)
		}
		if (opt.Filter.Kind == "" && opt.Filter.APIVersion == "") || (res.Kind == opt.Filter.Kind && res.APIVersion == opt.Filter.APIVersion) {
			var object unstructured.Unstructured
			object.SetAPIVersion(opt.Filter.APIVersion)
			object.SetKind(opt.Filter.Kind)
			if err := c.Get(ctx, apitypes.NamespacedName{Namespace: res.Namespace, Name: res.Name}, &object); err == nil {
				resources = append(resources, object)
			}
		}
	}
	return resources, nil
}

func sonLeafResource(res querytypes.AppliedResource, node *querytypes.ResourceTreeNode, kind string, apiVersion string) []unstructured.Unstructured {
	objects := make([]unstructured.Unstructured, 0)
	if node.LeafNodes != nil {
		for i := 0; i < len(node.LeafNodes); i++ {
			objects = append(objects, sonLeafResource(res, node.LeafNodes[i], kind, apiVersion)...)
		}
	}
	if (kind == "" && apiVersion == "") || (node.Kind == kind && node.APIVersion == apiVersion) {
		objects = append(objects, node.Object)
	}
	return objects
}
