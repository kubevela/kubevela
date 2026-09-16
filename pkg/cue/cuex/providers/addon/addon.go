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
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/validation"
	"github.com/oam-dev/kubevela/pkg/features"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
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
	p := params.Params
	if validation.IsValidationOnly(ctx) {
		return placeholderReturns(p), nil
	}
	r := api.DefaultRenderer()
	if r == nil {
		return nil, fmt.Errorf("addon renderer not initialized")
	}
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

// placeholderReturns is what Render yields under a validation-only context: a
// structurally valid but empty Application standing in for the one the addon
// would have rendered. It keeps `output: _render.$returns.application` in the
// addon ComponentDefinition satisfiable, so the component's CUE and parameters
// are still typechecked, with no registry access. See
// validation.WithValidationOnly.
//
// ResolvedVersion echoes the requested version rather than resolving one. An
// unpinned component resolves to whatever the registry currently calls latest,
// which is not knowable without asking it, and reporting an invented version
// would be worse than reporting none.
//
// Known divergence from a real render, which also sets a namespace, the
// oam.LabelAddonRegistry label, an ApplyOnce policy, and a populated
// spec.components: a trait on a type: addon component whose CUE reads into any
// of those would evaluate against this placeholder during admission and could
// reject an Application that renders correctly. Nothing forbids traits on an
// addon component today. Widening the placeholder only moves the line, since it
// cannot carry the addon's real components without fetching them, which is what
// this exists to avoid; narrowing what admission evaluates is the real fix, the
// way ValidateCUESchematicAppfile already skips PostDispatch traits.
func placeholderReturns(p RenderVars) *RenderReturns {
	return &RenderReturns{Returns: ResultVars{
		ResolvedVersion: p.Version,
		Registry:        p.Registry,
		Application: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1beta1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name": addonutil.Addon2AppName(p.Addon),
			},
			"spec": map[string]interface{}{
				"components": []interface{}{},
			},
		},
	}}
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
