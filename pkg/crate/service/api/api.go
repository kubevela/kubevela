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

// Package api holds the leaf types and injection seam shared between the crate
// render service and the CueX crate provider. It does not import
// pkg/crate/service, which would cycle through the CueX compiler. It does
// import the core API types, because CrateRequest carries a Placement.
package api

import (
	"context"
	"sync"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
)

// CrateRequest is the resolved input for rendering one crate. There is no
// per-line selector: enablement is the crate author's, via each line's
// enabled flag.
type CrateRequest struct {
	Crate    string
	Registry string
	// Namespace is where the crate's definitions install. Empty means the
	// default system namespace. It does not place the owned Application, which
	// always lives in the system namespace. The crate is still installed once
	// cluster-wide.
	Namespace string
	// Version selects the crate package version (the OCI/ECR tag vela crate
	// publish writes from _crate.cue's version field). Empty resolves the
	// highest published semver tag.
	Version string
	// Placement is the installer's request for where this crate's runtime
	// objects land. Nil means the installer did not ask. A hub marker on the
	// crate can still partition the render on its own.
	Placement *v1alpha1.Placement
}

// CrateResult is the rendered output: the crate's owned Application as a
// generic map, ready to hand back to CUE.
type CrateResult struct {
	Application map[string]interface{}
}

// Renderer fetches a crate and renders its owned Application without
// dispatching anything to the cluster.
type Renderer interface {
	RenderCrate(ctx context.Context, req CrateRequest) (*CrateResult, error)
}

var (
	mu       sync.RWMutex
	defaultR Renderer
)

// SetDefaultRenderer installs the process-wide renderer (called once at startup).
func SetDefaultRenderer(r Renderer) { mu.Lock(); defaultR = r; mu.Unlock() }

// DefaultRenderer returns the installed renderer, or nil if startup has not wired one.
func DefaultRenderer() Renderer { mu.RLock(); defer mu.RUnlock(); return defaultR }
