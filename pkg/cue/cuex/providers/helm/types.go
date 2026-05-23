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

package helm

import "time"

// ChartSourceParams represents the chart source configuration
type ChartSourceParams struct {
	Source  string      `json:"source"`
	RepoURL string      `json:"repoURL,omitempty"`
	Version string      `json:"version,omitempty"`
	Auth    *AuthParams `json:"auth,omitempty"`
}

// AuthParams represents authentication configuration
type AuthParams struct {
	SecretRef *SecretRefParams `json:"secretRef,omitempty"`
}

// SecretRefParams represents a reference to a secret
type SecretRefParams struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// ReleaseParams represents the release configuration
type ReleaseParams struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// ValuesFromParams represents a values source.
type ValuesFromParams struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
}

// CacheParams represents cache configuration from the template
type CacheParams struct {
	Key          string `json:"key,omitempty"`          // Cache key prefix
	TTL          string `json:"ttl,omitempty"`          // Single TTL for all versions
	ImmutableTTL string `json:"immutableTTL,omitempty"` // TTL for immutable versions
	MutableTTL   string `json:"mutableTTL,omitempty"`   // TTL for mutable versions
}

// RenderOptionsParams represents rendering options
type RenderOptionsParams struct {
	IncludeCRDs     *bool             `json:"includeCRDs,omitempty"`
	SkipTests       *bool             `json:"skipTests,omitempty"`
	SkipHooks       *bool             `json:"skipHooks,omitempty"`
	CreateNamespace *bool             `json:"createNamespace,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	MaxHistory      int               `json:"maxHistory,omitempty"`
	Atomic          bool              `json:"atomic,omitempty"`
	Wait            bool              `json:"wait,omitempty"`
	WaitTimeout     string            `json:"waitTimeout,omitempty"`
	Force           bool              `json:"force,omitempty"`
	RecreatePods    bool              `json:"recreatePods,omitempty"`
	CleanupOnFail   bool              `json:"cleanupOnFail,omitempty"`
	PostRender      *PostRenderParams `json:"postRender,omitempty"`
	Cache           *CacheParams      `json:"cache,omitempty"`
}

// PostRenderParams represents post-rendering configuration
type PostRenderParams struct {
	Kustomize *KustomizeParams `json:"kustomize,omitempty"`
	Exec      *ExecParams      `json:"exec,omitempty"`
}

// KustomizeParams represents Kustomize post-rendering options
type KustomizeParams struct {
	Patches               []interface{} `json:"patches,omitempty"`
	PatchesJson6902       []interface{} `json:"patchesJson6902,omitempty"`
	PatchesStrategicMerge []interface{} `json:"patchesStrategicMerge,omitempty"`
	Images                []interface{} `json:"images,omitempty"`
	Replicas              []interface{} `json:"replicas,omitempty"`
}

// ExecParams represents external binary post-rendering
type ExecParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// ContextParams holds KubeVela ownership information to be injected as labels
type ContextParams struct {
	AppName      string `json:"appName"`
	AppNamespace string `json:"appNamespace"`
	Name         string `json:"name"`      // component name
	Namespace    string `json:"namespace"` // component namespace
	// PublishVersion is the value of the Application's app.oam.dev/publishVersion
	// annotation, if any. When set, the provider records it as a label on the
	// helm release so subsequent reconciles can short-circuit when the pin is
	// stable. Populated by Render() via an Application lookup; not part of the
	// CUE-passed context shape.
	PublishVersion string `json:"-"`
}

// RenderParams represents the parameters for rendering a Helm chart
type RenderParams struct {
	Chart      ChartSourceParams    `json:"chart"`
	Release    *ReleaseParams       `json:"release,omitempty"`
	Values     interface{}          `json:"values,omitempty"`
	ValuesFrom []ValuesFromParams   `json:"valuesFrom,omitempty"`
	Options    *RenderOptionsParams `json:"options,omitempty"`
	Context    *ContextParams       `json:"context,omitempty"` // KubeVela ownership context
}

// RenderReturns represents the return value from rendering
type RenderReturns struct {
	Resources []map[string]interface{} `json:"resources"`
	Notes     string                   `json:"notes,omitempty"`
}

// CacheTTLConfig defines cache TTL settings for different version types
type CacheTTLConfig struct {
	// TTL for immutable versions (e.g., "1.2.3", "v2.0.0")
	ImmutableVersionTTL time.Duration
	// TTL for mutable tags (e.g., "latest", "dev", "main")
	MutableVersionTTL time.Duration
}

