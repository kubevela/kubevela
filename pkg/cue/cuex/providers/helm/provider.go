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

import (
	"context"
	"fmt"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/cli"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// dryRunContextKey is used to signal the helm provider that it should perform
// a client-only dry-run (helm template) instead of a real install/upgrade.
// This is set by the webhook validation path to avoid blocking on real Helm
// operations during Application admission.
type contextKey string

const dryRunContextKey contextKey = "helm.dryRun"

// WithDryRun returns a context with the dry-run flag set. When the helm
// provider receives this context, it renders the chart client-side without
// creating any resources in the cluster.
func WithDryRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, dryRunContextKey, true)
}

// isDryRun checks if the context has the dry-run flag set.
func isDryRun(ctx context.Context) bool {
	v, _ := ctx.Value(dryRunContextKey).(bool)
	return v
}

// velaContextStr returns a human-readable prefix like "app=myapp/default component=web"
// for use in log messages. Returns empty string when velaCtx is nil.
func velaContextStr(velaCtx *ContextParams) string {
	if velaCtx == nil {
		return ""
	}
	return fmt.Sprintf("app=%s/%s component=%s", velaCtx.AppName, velaCtx.AppNamespace, velaCtx.Name)
}

// releaseCacheKey returns a namespace-scoped cache key to avoid collisions
// when different namespaces have releases with the same name.
func releaseCacheKey(namespace, name string) string {
	return namespace + "/" + name
}

// DefaultCacheTTLConfig returns the default cache TTL configuration
func DefaultCacheTTLConfig() *CacheTTLConfig {
	return &CacheTTLConfig{
		ImmutableVersionTTL: 24 * time.Hour,  // 24 hours for fixed versions
		MutableVersionTTL:   5 * time.Minute, // 5 minutes for mutable tags
	}
}

// Provider is the Helm chart provider
type Provider struct {
	cache               *utils.MemoryCacheStore
	helmClient          *cli.EnvSettings
	cacheTTL            *CacheTTLConfig
	releaseMu           sync.Mutex        // serializes install/upgrade/uninstall calls
	releaseFingerprints map[string]string // namespace/releaseName → fingerprint (chartVersion|valuesHash)
	releaseManifests    map[string]string // namespace/releaseName → last successful manifest
	releaseVersions     map[string]int    // namespace/releaseName → current release version number
}

var (
	// globalProvider is a singleton instance of the Helm provider
	globalProvider *Provider
	// providerOnce ensures the provider is initialized only once
	providerOnce sync.Once
)

// NewProvider creates a new Helm provider (returns singleton)
func NewProvider() *Provider {
	providerOnce.Do(func() {
		globalProvider = &Provider{
			cache:               utils.NewMemoryCacheStore(context.Background()),
			helmClient:          cli.New(),
			cacheTTL:            DefaultCacheTTLConfig(),
			releaseFingerprints: make(map[string]string),
			releaseManifests:    make(map[string]string),
			releaseVersions:     make(map[string]int),
		}
	})
	return globalProvider
}

// NewProviderWithConfig creates a new Helm provider with custom cache configuration
func NewProviderWithConfig(ttlConfig *CacheTTLConfig) *Provider {
	if ttlConfig == nil {
		ttlConfig = DefaultCacheTTLConfig()
	}
	return &Provider{
		cache:               utils.NewMemoryCacheStore(context.Background()),
		helmClient:          cli.New(),
		cacheTTL:            ttlConfig,
		releaseFingerprints: make(map[string]string),
		releaseManifests:    make(map[string]string),
		releaseVersions:     make(map[string]int),
	}
}
