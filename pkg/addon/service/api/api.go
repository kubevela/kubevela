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

// Package api holds the leaf types and injection seam shared between the
// render-only addon service and the CueX addon provider. It imports only the
// standard library so neither the provider nor the CueX compiler pulls in
// pkg/addon (which would create an import cycle).
package api

import (
	"context"
	"sync"
)

// AddonRequest is the resolved input for rendering one addon.
type AddonRequest struct {
	Name       string
	Version    string
	Registry   string
	Properties map[string]interface{}
	// SkipVersionValidate skips the addon SystemRequirements (vela/kubernetes
	// version) compatibility check before rendering.
	SkipVersionValidate bool
}

// AddonResult is the rendered output: the addon Application (with its
// auxiliaries folded in as components) as a generic map, ready to hand back
// to CUE.
type AddonResult struct {
	ResolvedVersion string
	Registry        string
	Application     map[string]interface{}
}

// Renderer resolves and renders an addon without dispatching it to the cluster.
type Renderer interface {
	RenderAddon(ctx context.Context, req AddonRequest) (*AddonResult, error)
}

var (
	mu       sync.RWMutex
	defaultR Renderer
)

// SetDefaultRenderer installs the process-wide renderer (called once at startup).
func SetDefaultRenderer(r Renderer) { mu.Lock(); defaultR = r; mu.Unlock() }

// DefaultRenderer returns the installed renderer, or nil if startup has not wired one.
func DefaultRenderer() Renderer { mu.RLock(); defer mu.RUnlock(); return defaultR }
