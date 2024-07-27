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
	"context"
	_ "embed"
	"fmt"

	"github.com/pkg/errors"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"

	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/types"
)

// Outputs is the output parameters for Terraform components.
type Outputs[T any] struct {
	Outputs T `json:"outputs"`
}

// Inputs is the input parameters for Terraform components.
type Inputs[T any] struct {
	Inputs T `json:"inputs"`
}

// ComponentVars is the input parameters for LoadTerraformComponents.
type ComponentVars struct {
	Components []common.ApplicationComponent `json:"components"`
}

// ComponentReturns is the return value for LoadTerraformComponents.
type ComponentReturns = Outputs[ComponentVars]

// LoadTerraformComponents loads Terraform components.
func LoadTerraformComponents(ctx context.Context, params *oamprovidertypes.OAMParams[any]) (*ComponentReturns, error) {
	res := &ComponentReturns{
		Outputs: ComponentVars{
			Components: make([]common.ApplicationComponent, 0),
		},
	}
	for _, comp := range params.App.Spec.Components {
		wl, err := params.WorkloadRender(ctx, comp)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to render component into workload")
		}
		if wl.CapabilityCategory != types.TerraformCategory {
			continue
		}
		res.Outputs.Components = append(res.Outputs.Components, comp)
	}
	return res, nil
}

// ComponentNameVars is the input parameters for GetConnectionStatus.
type ComponentNameVars struct {
	ComponentName string `json:"componentName"`
}

// ConnectionParams is the input parameters for GetConnectionStatus.
type ConnectionParams = oamprovidertypes.OAMParams[Inputs[ComponentNameVars]]

// ConnectionResult is the result for connection status.
type ConnectionResult struct {
	Healthy bool `json:"healthy"`
}

// ConnectionReturns is the return value for connection status.
type ConnectionReturns = Outputs[ConnectionResult]

// GetConnectionStatus returns the connection status of a component.
func GetConnectionStatus(_ context.Context, params *ConnectionParams) (*ConnectionReturns, error) {
	app := params.RuntimeParams.App
	componentName := params.Params.Inputs.ComponentName
	if componentName == "" {
		return nil, fmt.Errorf("componentName is required")
	}
	for _, svc := range app.Status.Services {
		if svc.Name == componentName {
			return &ConnectionReturns{
				Outputs: ConnectionResult{
					Healthy: svc.Healthy,
				},
			}, nil
		}
	}
	return &ConnectionReturns{
		Outputs: ConnectionResult{
			Healthy: false,
		},
	}, nil
}

//go:embed terraform.cue
var template string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"load-terraform-components": oamprovidertypes.OAMGenericProviderFn[any, ComponentReturns](LoadTerraformComponents),
		"get-connection-status":     oamprovidertypes.OAMGenericProviderFn[Inputs[ComponentNameVars], ConnectionReturns](GetConnectionStatus),
	}
}
