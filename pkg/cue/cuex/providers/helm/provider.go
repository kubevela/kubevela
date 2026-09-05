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

	"github.com/kubevela/pkg/cache"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
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

// InitCacheTTL configures the cluster-wide chart cache TTL defaults before any
// provider is constructed. These apply only when a component does not pin an
// explicit TTL through options.cache (the helmchart.cue template leaves those
// fields optional so per-component values still win when set). Non-positive
// values fall back to the defaults so a partial flag set cannot wipe the cache.
func InitCacheTTL(immutableTTL, mutableTTL time.Duration) {
	if immutableTTL <= 0 {
		klog.Warningf("helm cache immutable TTL %v is invalid (must be > 0); using default %v", immutableTTL, DefaultImmutableVersionTTL)
		immutableTTL = DefaultImmutableVersionTTL
	}
	if mutableTTL <= 0 {
		klog.Warningf("helm cache mutable TTL %v is invalid (must be > 0); using default %v", mutableTTL, DefaultMutableVersionTTL)
		mutableTTL = DefaultMutableVersionTTL
	}
	cacheTTLImmutableVersion = immutableTTL
	cacheTTLMutableVersion = mutableTTL
	klog.Infof("Helm cache TTL configured: immutable=%v mutable=%v", immutableTTL, mutableTTL)
}

// DefaultCacheTTLConfig returns the effective cache TTL configuration, honoring
// the cluster-wide defaults configured via InitCacheTTL.
func DefaultCacheTTLConfig() *CacheTTLConfig {
	return &CacheTTLConfig{
		ImmutableVersionTTL: cacheTTLImmutableVersion,
		MutableVersionTTL:   cacheTTLMutableVersion,
	}
}

// missReasonLabel maps a cache eviction reason to the corresponding
// miss-reason label. Returns ok=false for eviction reasons that should not
// be recorded (delete, replace, purge) because they don't represent a
// cache miss on the next Get.
func missReasonLabel(reason cache.EvictionReason) (string, bool) {
	switch reason {
	case cache.EvictTTL:
		return missReasonExpired, true
	case cache.EvictCapacity:
		return missReasonEvicted, true
	default:
		return "", false
	}
}

// evictionReasonLabel normalizes the cache eviction reason into a stable,
// lowercase Prometheus label value.
func evictionReasonLabel(reason cache.EvictionReason) string {
	switch reason {
	case cache.EvictCapacity:
		return "capacity"
	case cache.EvictTTL:
		return "ttl"
	case cache.EvictDelete:
		return "delete"
	case cache.EvictReplace:
		return "replace"
	case cache.EvictPurge:
		return "purge"
	default:
		return "unknown"
	}
}

// Provider is the Helm chart provider
type Provider struct {
	cache                *cache.LRUStore[string, []byte]
	chartFlight          singleflight.Group
	helmClient           *cli.EnvSettings
	cacheTTL             *CacheTTLConfig
	cacheRecentEvictions *sync.Map         // cache key → miss-reason label (populated by OnEvict)
	releaseMu            sync.Mutex        // serializes install/upgrade/uninstall calls
	releaseFingerprints  map[string]string // namespace/releaseName → fingerprint (chartVersion|valuesHash)
	releaseManifests     map[string]string // namespace/releaseName → last successful manifest
	releaseVersions      map[string]int    // namespace/releaseName → current release version number
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

const (
	// DefaultChartCacheMaxBytes is the default byte budget for the chart
	// cache. Keep it well below the container memory limit: cached archives
	// are only part of the footprint, since each render also decompresses a
	// copy into chart objects.
	DefaultChartCacheMaxBytes int64 = 256 << 20 // 256 MB
	// DefaultChartCacheSweepInterval is how often expired entries are removed.
	// The sweep takes the same lock as Get and Put and scans every key, so
	// this trades reclaim latency against lock contention.
	DefaultChartCacheSweepInterval = 60 * time.Second
	// DefaultImmutableVersionTTL is the default cache TTL for immutable
	// (semver) chart versions. The helmchart.cue template deliberately leaves
	// options.cache.immutableTTL a plain optional field, so this default is
	// what actually applies for components that omit an explicit TTL.
	DefaultImmutableVersionTTL = 24 * time.Hour
	// DefaultMutableVersionTTL is the default cache TTL for mutable chart tags
	// (latest, dev, main, etc.).
	DefaultMutableVersionTTL = 5 * time.Minute
)

var (
	// chartCacheMaxBytes and chartCacheSweepInterval hold the effective cache
	// tuning. InitChartCache overwrites them from the controller flags before
	// any provider is constructed.
	chartCacheMaxBytes      = DefaultChartCacheMaxBytes
	chartCacheSweepInterval = DefaultChartCacheSweepInterval
	// chartCacheCtx bounds the lifetime of the singleton cache's sweeper.
	chartCacheCtx context.Context = context.Background()
	// cacheTTLImmutableVersion and cacheTTLMutableVersion hold the effective
	// cluster-wide TTL defaults. InitCacheTTL overwrites them from the
	// controller flags before any provider is constructed. They are only used
	// when a component does not pin an explicit TTL via options.cache.
	cacheTTLImmutableVersion = DefaultImmutableVersionTTL
	cacheTTLMutableVersion   = DefaultMutableVersionTTL
)

// InitChartCache configures the chart cache before the provider singleton is
// built. ctx SHOULD be the controller's root context so the sweeper goroutine
// is torn down with the manager. Non-positive values fall back to the defaults,
// so a partial flag set cannot produce an unbounded or hot-spinning cache.
func InitChartCache(ctx context.Context, maxBytes int64, sweepInterval time.Duration) {
	if ctx != nil {
		chartCacheCtx = ctx
	}
	if maxBytes <= 0 {
		klog.Warningf("helm chart cache max bytes %d is invalid (must be > 0); using default %d", maxBytes, DefaultChartCacheMaxBytes)
		maxBytes = DefaultChartCacheMaxBytes
	}
	if sweepInterval <= 0 {
		klog.Warningf("helm chart cache sweep interval %v is invalid (must be > 0); using default %v", sweepInterval, DefaultChartCacheSweepInterval)
		sweepInterval = DefaultChartCacheSweepInterval
	}
	chartCacheMaxBytes = maxBytes
	chartCacheSweepInterval = sweepInterval
	klog.Infof("Helm chart cache configured: maxBytes=%d sweepInterval=%v", maxBytes, sweepInterval)
}

// chartCacheOptions returns the shared configuration for the byte-bounded LRU
// chart cache used by every provider constructor.
func chartCacheOptions() cache.Options[string, []byte] {
	return cache.Options[string, []byte]{
		MaxSize:  0,
		MaxBytes: chartCacheMaxBytes,
		SizeOf: func(key string, value []byte) int64 {
			return int64(len(value))
		},
		SweepInterval: chartCacheSweepInterval,
	}
}

// NewProvider creates a new Helm provider (returns singleton)
func NewProvider() *Provider {
	providerOnce.Do(func() {
		cacheRecentEvictions := &sync.Map{}
		lruCache, err := cache.NewLRUStore[string, []byte](chartCacheCtx, chartCacheOptions())
		if err != nil {
			klog.Fatalf("Failed to create chart LRU cache: %v", err)
		}
		lruCache.OnEvict = func(key string, value []byte, reason cache.EvictionReason) {
			HelmChartCacheEvictionsTotal.WithLabelValues(evictionReasonLabel(reason)).Inc()
			HelmChartCacheBytes.Set(float64(lruCache.CurrentBytes()))
			if mr, ok := missReasonLabel(reason); ok {
				cacheRecentEvictions.Store(key, mr)
			}
		}
		globalProvider = &Provider{
			cache:                lruCache,
			helmClient:           cli.New(),
			cacheTTL:             DefaultCacheTTLConfig(),
			cacheRecentEvictions: cacheRecentEvictions,
			releaseFingerprints:  make(map[string]string),
			releaseManifests:     make(map[string]string),
			releaseVersions:      make(map[string]int),
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
	cacheRecentEvictions := &sync.Map{}
	lruCache, err := cache.NewLRUStore[string, []byte](chartCacheCtx, chartCacheOptions())
	if err != nil {
		klog.Fatalf("Failed to create chart LRU cache: %v", err)
	}
	lruCache.OnEvict = func(key string, value []byte, reason cache.EvictionReason) {
		HelmChartCacheEvictionsTotal.WithLabelValues(evictionReasonLabel(reason)).Inc()
		HelmChartCacheBytes.Set(float64(lruCache.CurrentBytes()))
		if mr, ok := missReasonLabel(reason); ok {
			cacheRecentEvictions.Store(key, mr)
		}
	}
	p := &Provider{
		cache:                lruCache,
		helmClient:           cli.New(),
		cacheTTL:             ttlConfig,
		cacheRecentEvictions: cacheRecentEvictions,
		releaseFingerprints:  make(map[string]string),
		releaseManifests:     make(map[string]string),
		releaseVersions:      make(map[string]int),
	}
	p.actionConfigFactory = p.getActionConfig
	p.kubeClientFactory = p.getKubeClientset
	return p
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
