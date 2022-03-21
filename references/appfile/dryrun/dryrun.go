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

package dryrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/util/openapi"
	"k8s.io/kubectl/pkg/util/openapi/validation"
	kval "k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// DryRun executes dry-run on an application
type DryRun interface {
	ExecuteDryRun(ctx context.Context, app *v1beta1.Application) ([]*types.ComponentManifest, error)
}

// NewDryRunOption creates a dry-run option
func NewDryRunOption(c client.Client, cfg *rest.Config, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, as []oam.Object) *Option {
	return &Option{c, dm, pd, cfg, as}
}

// Option contains options to execute dry-run
type Option struct {
	Client          client.Client
	DiscoveryMapper discoverymapper.DiscoveryMapper
	PackageDiscover *packages.PackageDiscover

	cfg *rest.Config
	// Auxiliaries are capability definitions used to parse application.
	// DryRun will use capabilities in Auxiliaries as higher priority than
	// getting one from cluster.
	Auxiliaries []oam.Object
}

// validateObjectFromFile will read file into Unstructured object
func (d *Option) validateObjectFromFile(filename string) (*unstructured.Unstructured, error) {
	fileContent, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}

	fileType := filepath.Ext(filename)
	switch fileType {
	case ".yaml", ".yml":
		fileContent, err = yaml.YAMLToJSON(fileContent)
		if err != nil {
			return nil, err
		}
	}

	dc, err := discovery.NewDiscoveryClientForConfig(d.cfg)
	if err != nil {
		return nil, err
	}
	openAPIGetter := openapi.NewOpenAPIGetter(dc)
	resources, err := openapi.NewOpenAPIParser(openAPIGetter).Parse()
	if err != nil {
		return nil, err
	}

	valids := kval.ConjunctiveSchema{validation.NewSchemaValidation(resources), kval.NoDoubleKeySchema{}}
	if err = valids.ValidateBytes(fileContent); err != nil {
		return nil, err
	}

	app := new(unstructured.Unstructured)
	err = json.Unmarshal(fileContent, app)
	return app, err
}

// ValidateApp will validate app with client schema check and server side dry-run
func (d *Option) ValidateApp(ctx context.Context, filename string) error {
	app, err := d.validateObjectFromFile(filename)
	if err != nil {
		return err
	}

	app2 := app.DeepCopy()

	err = d.Client.Get(ctx, client.ObjectKey{Namespace: app.GetNamespace(), Name: app.GetName()}, app2)
	if err == nil {
		app.SetResourceVersion(app2.GetResourceVersion())
		return d.Client.Update(ctx, app, client.DryRunAll)
	}
	return d.Client.Create(ctx, app, client.DryRunAll)
}

// ExecuteDryRun simulates applying an application into cluster and returns rendered
// resources but not persist them into cluster.
func (d *Option) ExecuteDryRun(ctx context.Context, app *v1beta1.Application) ([]*types.ComponentManifest, error) {
	parser := appfile.NewDryRunApplicationParser(d.Client, d.DiscoveryMapper, d.PackageDiscover, d.Auxiliaries)
	if app.Namespace != "" {
		ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	}
	appFile, err := parser.GenerateAppFile(ctx, app)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot generate appFile from application")
	}
	if appFile.Namespace == "" {
		appFile.Namespace = corev1.NamespaceDefault
	}
	comps, err := appFile.GenerateComponentManifests()
	if err != nil {
		return nil, errors.WithMessage(err, "cannot generate AppConfig and Components")
	}

	return comps, nil
}
