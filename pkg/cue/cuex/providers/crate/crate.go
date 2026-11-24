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

// Package crate provides the CueX `crate.#Render` provider, which fetches a
// crate and renders its owned Application via the injected render
// service.
package crate

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/crate/service/api"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/validation"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
)

// ProviderName is the CUE #provider value.
const ProviderName = "crate"

//go:embed crate.cue
var template string

// RenderVars is the $params shape.
type RenderVars struct {
	Crate     string `json:"crate"`
	Registry  string `json:"registry"`
	Namespace string `json:"namespace"`
	// Version selects the crate package version (the OCI/ECR tag); empty means
	// latest. Not the API line.
	Version string `json:"version"`
	// Placement is the installer's cluster request. Nil means they did not ask.
	// Objects marked runtime follow it. Unmarked objects stay on the hub.
	Placement *v1alpha1.Placement `json:"placement,omitempty"`
}

// ResultVars is the $returns shape.
type ResultVars struct {
	Application map[string]interface{} `json:"application"`
}

// RenderParams is the params for the render action.
type RenderParams providers.Params[RenderVars]

// RenderReturns is the returns for the render action.
type RenderReturns providers.Returns[ResultVars]

// Render fetches the crate and renders its owned Application via the injected
// render service.
func Render(ctx context.Context, params *RenderParams) (*RenderReturns, error) {
	// This package is registered on the compilers unconditionally, because the crate
	// ComponentDefinition imports vela/crate and could not compile otherwise. The
	// gate is therefore checked here rather than at registration. When it is off no
	// renderer is wired up either, but the nil-renderer error below does not say what
	// to do about it, so report the gate explicitly.
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCrateComponent) {
		return nil, fmt.Errorf("crate-as-component is disabled; enable the EnableCrateComponent feature gate to use type: crate components")
	}
	p := params.Params
	// Checked before the validation-only return, so admission rejects a bad
	// placement with the same message a render would give.
	if err := placement.Validate(p.Placement); err != nil {
		return nil, err
	}
	if validation.IsValidationOnly(ctx) {
		return placeholderReturns(p), nil
	}
	r := api.DefaultRenderer()
	if r == nil {
		return nil, fmt.Errorf("crate renderer not initialized")
	}
	res, err := r.RenderCrate(ctx, api.CrateRequest{
		Crate:     p.Crate,
		Registry:  p.Registry,
		Namespace: p.Namespace,
		Version:   p.Version,
		Placement: p.Placement,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("crate renderer returned no result")
	}
	return &RenderReturns{Returns: ResultVars{Application: res.Application}}, nil
}

// placeholderReturns is what Render yields under a validation-only context: an
// Application shaped like the one the crate would have rendered, but with no
// components, because filling those in is exactly the remote fetch that must
// not happen during admission. It keeps `output: _render.$returns.application`
// in the crate ComponentDefinition satisfiable, so the component's CUE and
// parameters are still typechecked. See validation.WithValidationOnly.
//
// The name, namespace and crate label match RenderApplication in
// pkg/crate/service, so anything reading those during admission sees the real
// values. Two fields deliberately diverge: spec.components is empty, and the
// crate-version annotation is absent because the real value is read from the
// fetched crate's own _crate.cue and cannot be known without fetching it.
// Traits on a type: crate component would evaluate against this placeholder,
// so ValidateCUESchematicAppfile skips them during admission.
func placeholderReturns(p RenderVars) *RenderReturns {
	return &RenderReturns{Returns: ResultVars{
		Application: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1beta1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "crate-" + p.Crate,
				"namespace": types.DefaultKubeVelaNS,
				"labels": map[string]interface{}{
					types.LabelDefinitionCrate: p.Crate,
				},
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
