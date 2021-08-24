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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
func NewDryRunOption(c client.Client, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, as []oam.Object) *Option {
	return &Option{c, dm, pd, as}
}

// Option contains options to execute dry-run
type Option struct {
	Client          client.Client
	DiscoveryMapper discoverymapper.DiscoveryMapper
	PackageDiscover *packages.PackageDiscover
	// Auxiliaries are capability definitions used to parse application.
	// DryRun will use capabilities in Auxiliaries as higher priority than
	// getting one from cluster.
	Auxiliaries []oam.Object
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

	for _, comp := range comps {
		if comp.StandardWorkload != nil {
			if comp.StandardWorkload.GetName() == "" {
				comp.StandardWorkload.SetName(comp.Name)
			}
			if comp.StandardWorkload.GetNamespace() == "" {
				comp.StandardWorkload.SetNamespace(appFile.Namespace)
			}
		}

		for _, trait := range comp.Traits {
			if trait.GetName() == "" {
				traitType := trait.GetLabels()[oam.TraitTypeLabel]
				traitName := oamutil.GenTraitNameCompatible(comp.Name, trait, traitType)
				trait.SetName(traitName)
			}
			if trait.GetNamespace() == "" {
				trait.SetNamespace(appFile.Namespace)
			}
		}
	}
	return comps, nil
}
