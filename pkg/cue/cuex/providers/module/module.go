/*
Copyright 2026 The KubeVela Authors.

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

// Package module provides the CueX `module.#Render` provider, which fetches a
// module and renders its owned Application via the injected render
// service.
package module

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
)

// ProviderName is the CUE #provider value.
const ProviderName = "module"

//go:embed module.cue
var template string

// RenderVars is the $params shape.
type RenderVars struct {
	Module    string `json:"module"`
	Registry  string `json:"registry"`
	Namespace string `json:"namespace"`
	// Version selects the module package version (the OCI/ECR tag); empty means
	// latest. Not the API line.
	Version string `json:"version"`
}

// ResultVars is the $returns shape.
type ResultVars struct {
	Application map[string]interface{} `json:"application"`
}

// RenderParams is the params for the render action.
type RenderParams providers.Params[RenderVars]

// RenderReturns is the returns for the render action.
type RenderReturns providers.Returns[ResultVars]

// Render fetches the module and renders its owned Application via the injected
// render service.
func Render(ctx context.Context, params *RenderParams) (*RenderReturns, error) {
	// This package is registered on the compilers unconditionally, because the module
	// ComponentDefinition imports vela/module and could not compile otherwise. The
	// gate is therefore checked here rather than at registration. When it is off no
	// renderer is wired up either, but the nil-renderer error below does not say what
	// to do about it, so report the gate explicitly.
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableModuleComponent) {
		return nil, fmt.Errorf("module-as-component is disabled; enable the EnableModuleComponent feature gate to use type: module components")
	}
	r := api.DefaultRenderer()
	if r == nil {
		return nil, fmt.Errorf("module renderer not initialized")
	}
	p := params.Params
	res, err := r.RenderModule(ctx, api.ModuleRequest{
		Module:    p.Module,
		Registry:  p.Registry,
		Namespace: p.Namespace,
		Version:   p.Version,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("module renderer returned no result")
	}
	return &RenderReturns{Returns: ResultVars{Application: res.Application}}, nil
}

// GetTemplate returns the CUE template.
func GetTemplate() string {
	return template
}

// GetProviders returns the CUE providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"render": cuexruntime.GenericProviderFn[RenderParams, RenderReturns](Render),
	}
}

// Package is the internal CueX package registered on the WorkloadCompiler.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, GetTemplate(), GetProviders()))
