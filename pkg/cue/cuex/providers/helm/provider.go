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

// Provider singleton, constructors, dry-run context flag, and small ownership/cache-key utilities.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/client-go/kubernetes"

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
	// actionConfigFactory builds a helm action.Configuration for a given
	// namespace. Defaults to getActionConfig (a real cluster client). Tests
	// override this to inject a fake KubeClient + memory storage driver so
	// they can exercise the install/upgrade dispatcher without a cluster.
	actionConfigFactory func(namespace string) (*action.Configuration, error)
	// kubeClientFactory builds a kubernetes.Interface client used by the
	// release-secret helpers (list, label, delete, validate health) which
	// talk to the API server directly rather than via the helm SDK. Tests
	// override this to inject a fake clientset so the dispatcher's adoption
	// path and background health checks do not leak to the active cluster.
	kubeClientFactory func() (kubernetes.Interface, error)
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
		globalProvider.actionConfigFactory = globalProvider.getActionConfig
		globalProvider.kubeClientFactory = globalProvider.getKubeClientset
	})
	return globalProvider
}

// NewProviderWithConfig creates a new Helm provider with custom cache configuration
func NewProviderWithConfig(ttlConfig *CacheTTLConfig) *Provider {
	if ttlConfig == nil {
		ttlConfig = DefaultCacheTTLConfig()
	}
	p := &Provider{
		cache:               utils.NewMemoryCacheStore(context.Background()),
		helmClient:          cli.New(),
		cacheTTL:            ttlConfig,
		releaseFingerprints: make(map[string]string),
		releaseManifests:    make(map[string]string),
		releaseVersions:     make(map[string]int),
	}
	p.actionConfigFactory = p.getActionConfig
	p.kubeClientFactory = p.getKubeClientset
	return p
}

// Close stops the provider's background goroutines (cache sweeper). Callers
// that create a provider via NewProviderWithConfig SHOULD call Close when
// the provider is no longer needed, particularly in tests, to prevent
// goroutine leaks. Safe to call multiple times.
func (p *Provider) Close() {
	if p.cache != nil {
		p.cache.Stop()
	}
}

// getKubeClientset is the default kubeClientFactory: builds a typed Kubernetes
// client from the helm CLI environment's REST config. Tests inject a fake
// clientset instead so the release-secret helpers never reach the active
// cluster.
func (p *Provider) getKubeClientset() (kubernetes.Interface, error) {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get REST config")
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kubernetes client")
	}
	return cs, nil
}
