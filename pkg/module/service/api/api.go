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

// Package api holds the leaf types and injection seam shared between the module
// render service and the CueX module provider. It imports only the standard
// library so neither the provider nor the CueX compiler pulls in
// pkg/module/service (which would create an import cycle through the CueX compiler).
package api

import (
	"context"
	"sync"
)

// ModuleRequest is the resolved input for rendering one module. There is no
// per-line selector: enablement is the module author's, via each line's
// enabled flag.
type ModuleRequest struct {
	Module   string
	Registry string
	// Namespace is where the module's definitions install. Empty means the
	// default system namespace. It does not place the owned Application, which
	// always lives in the system namespace. The module is still installed once
	// cluster-wide.
	Namespace string
	// Version selects the module package version (the OCI/ECR tag vela module
	// publish writes from _module.cue's version field). A git source ignores
	// this silently.
	Version string
}

// ModuleResult is the rendered output: the module's owned Application as a
// generic map, ready to hand back to CUE.
type ModuleResult struct {
	Application map[string]interface{}
}

// Renderer fetches a module and renders its owned Application without
// dispatching anything to the cluster.
type Renderer interface {
	RenderModule(ctx context.Context, req ModuleRequest) (*ModuleResult, error)
}

var (
	mu       sync.RWMutex
	defaultR Renderer
)

// SetDefaultRenderer installs the process-wide renderer (called once at startup).
func SetDefaultRenderer(r Renderer) { mu.Lock(); defaultR = r; mu.Unlock() }

// DefaultRenderer returns the installed renderer, or nil if startup has not wired one.
func DefaultRenderer() Renderer { mu.RLock(); defer mu.RUnlock(); return defaultR }
