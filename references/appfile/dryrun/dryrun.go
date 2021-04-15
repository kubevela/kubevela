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

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// DryRun executes dry-run on an application
type DryRun interface {
	ExecuteDryRun(ctx context.Context, app *v1beta1.Application) (*v1alpha2.ApplicationConfiguration, []*v1alpha2.Component, error)
}

// NewDryRunOption creates a dry-run option
func NewDryRunOption(c client.Client, dm discoverymapper.DiscoveryMapper, pd *definition.PackageDiscover, as []oam.Object) *Option {
	return &Option{c, dm, pd, as}
}

// Option contains options to execute dry-run
type Option struct {
	Client          client.Client
	DiscoveryMapper discoverymapper.DiscoveryMapper
	PackageDiscover *definition.PackageDiscover
	// Auxiliaries are capability definitions used to parse application.
	// DryRun will use capabilities in Auxiliaries as higher priority than
	// getting one from cluster.
	Auxiliaries []oam.Object
}

// ExecuteDryRun simulates applying an application into cluster and returns rendered
// resoures but not persist them into cluster.
func (d *Option) ExecuteDryRun(ctx context.Context, app *v1beta1.Application) (*v1alpha2.ApplicationConfiguration, []*v1alpha2.Component, error) {
	parser := appfile.NewDryRunApplicationParser(d.Client, d.DiscoveryMapper, d.PackageDiscover, d.Auxiliaries)
	if app.Namespace != "" {
		ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	}
	appFile, err := parser.GenerateAppFile(ctx, app)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate appFile from application")
	}
	ac, comps, err := appFile.GenerateApplicationConfiguration()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot generate AppConfig and Components")
	}
	return ac, comps, nil
}
