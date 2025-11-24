/*
Copyright 2025 The KubeVela Authors.

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

package addon

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"k8s.io/klog/v2"
)

// AddonRenderer is the interface for rendering addons without heavy imports
type AddonRenderer interface {
	RenderAddon(ctx context.Context, req *AddonRequest) (*AddonResult, error)
}

// AddonRequest contains all parameters for addon rendering
type AddonRequest struct {
	Addon      string
	Version    string
	Registry   string
	Properties map[string]interface{}
	Include    *IncludeOptions
}

// IncludeOptions specifies which addon components to include
type IncludeOptions struct {
	Definitions     bool `json:"definitions"`
	ConfigTemplates bool `json:"configTemplates"`
	Views           bool `json:"views"`
	Resources       bool `json:"resources"`
}

// AddonResult contains the rendered addon output
type AddonResult struct {
	ResolvedVersion string
	Registry        string
	Application     map[string]interface{}
	Resources       []map[string]interface{}
}

type Params struct {
	Addon      string                 `json:"addon"`
	Version    string                 `json:"version"`
	Registry   string                 `json:"registry"`
	Properties map[string]interface{} `json:"properties"`
	Include    *IncludeOptions        `json:"include"`
}

type Returns struct {
	ResolvedVersion string                   `json:"resolvedVersion"`
	Registry        string                   `json:"registry"`
	Application     map[string]interface{}   `json:"application"`
	Resources       []map[string]interface{} `json:"resources"`
}

// AddonParams is the params for addon rendering  
type AddonParams providers.Params[Params]

// AddonReturns is the returns for addon rendering
type AddonReturns providers.Returns[Returns]

// Global renderer instance - will be injected at startup
var renderer AddonRenderer

// SetRenderer injects the addon renderer implementation
func SetRenderer(r AddonRenderer) {
	renderer = r
}

func Render(ctx context.Context, params *AddonParams) (*AddonReturns, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		// Always log at INFO level
		klog.Infof("Addon CueX render took %v for addon: %s (registry: %s, version: %s)", 
			duration, params.Params.Addon, params.Params.Registry, params.Params.Version)
	}()

	if renderer == nil {
		return nil, fmt.Errorf("addon renderer not initialized")
	}

	p := params.Params

	// Create request
	req := &AddonRequest{
		Addon:      p.Addon,
		Version:    p.Version,
		Registry:   p.Registry,
		Properties: p.Properties,
		Include:    p.Include,
	}

	// Delegate to injected renderer
	result, err := renderer.RenderAddon(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AddonReturns{
		Returns: Returns{
			ResolvedVersion: result.ResolvedVersion,
			Registry:        result.Registry,
			Application:     result.Application,
			Resources:       result.Resources,
		},
	}, nil
}

const ProviderName = "addon"

//go:embed addon.cue
var template string

var Template = template


var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render": cuexruntime.GenericProviderFn[AddonParams, AddonReturns](Render),
}))
