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

// Package addon provides the CueX `addon.#Render` provider, which resolves and
// renders an addon via the injected render-only service.
package addon

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/features"
)

// ProviderName is the CUE #provider value.
const ProviderName = "addon"

//go:embed addon.cue
var template string

// RenderVars is the $params shape.
type RenderVars struct {
	Addon               string                 `json:"addon"`
	Version             string                 `json:"version"`
	Registry            string                 `json:"registry"`
	Properties          map[string]interface{} `json:"properties"`
	SkipVersionValidate bool                   `json:"skipVersionValidate"`
}

// ResultVars is the $returns shape.
type ResultVars struct {
	ResolvedVersion string                 `json:"resolvedVersion"`
	Registry        string                 `json:"registry"`
	Application     map[string]interface{} `json:"application"`
}

// RenderParams is the params for the render action.
type RenderParams providers.Params[RenderVars]

// RenderReturns is the returns for the render action.
type RenderReturns providers.Returns[ResultVars]

// Render resolves and renders an addon via the injected render-only service.
func Render(ctx context.Context, params *RenderParams) (*RenderReturns, error) {
	// This package is registered on the compilers unconditionally, because the addon
	// ComponentDefinition imports vela/addon and could not compile otherwise. The
	// gate is therefore checked here rather than at registration. When it is off no
	// renderer is wired up either, but the nil-renderer error below does not say what
	// to do about it, so report the gate explicitly.
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableAddonComponent) {
		return nil, fmt.Errorf("addon-as-component is disabled; enable the EnableAddonComponent feature gate to use type: addon components")
	}
	r := api.DefaultRenderer()
	if r == nil {
		return nil, fmt.Errorf("addon renderer not initialized")
	}
	p := params.Params
	res, err := r.RenderAddon(ctx, api.AddonRequest{
		Name:                p.Addon,
		Version:             p.Version,
		Registry:            p.Registry,
		Properties:          p.Properties,
		SkipVersionValidate: p.SkipVersionValidate,
	})
	if err != nil {
		return nil, err
	}
	return &RenderReturns{Returns: ResultVars{
		ResolvedVersion: res.ResolvedVersion,
		Registry:        res.Registry,
		Application:     res.Application,
	}}, nil
}

// Package is the internal CueX package registered on the WorkloadCompiler.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render": cuexruntime.GenericProviderFn[RenderParams, RenderReturns](Render),
}))
