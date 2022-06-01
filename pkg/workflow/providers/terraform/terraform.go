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

package terraform

import (
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "terraform"
)

type provider struct {
	app      *v1beta1.Application
	renderer oamProvider.WorkloadRenderer
}

func (p *provider) LoadTerraformComponents(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	var components []common.ApplicationComponent
	for _, comp := range p.app.Spec.Components {
		wl, err := p.renderer(comp)
		if err != nil {
			return errors.Wrapf(err, "failed to render component into workload")
		}
		if wl.CapabilityCategory != types.TerraformCategory {
			continue
		}
		components = append(components, comp)
	}
	return v.FillObject(components, "outputs", "components")
}

func (p *provider) GetConnectionStatus(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	componentName, err := v.GetString("inputs", "componentName")
	if err != nil {
		return errors.Wrapf(err, "failed to get component name")
	}
	for _, svc := range p.app.Status.Services {
		if svc.Name == componentName {
			return v.FillObject(svc.Healthy, "outputs", "healthy")
		}
	}
	return v.FillObject(false, "outputs", "healthy")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, app *v1beta1.Application, renderer oamProvider.WorkloadRenderer) {
	prd := &provider{app: app, renderer: renderer}
	p.Register(ProviderName, map[string]providers.Handler{
		"load-terraform-components": prd.LoadTerraformComponents,
		"get-connection-status":     prd.GetConnectionStatus,
	})
}
