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

// Detects chart source type (OCI / URL / repo) and fetches charts with a TTL-bounded in-memory cache.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// detectChartSourceType detects the type of chart source based on the source string
func detectChartSourceType(source string) string {
	// OCI registry detection
	if strings.HasPrefix(source, "oci://") {
		return "oci"
	}

	// Direct URL detection
	if strings.HasSuffix(source, ".tgz") || strings.HasSuffix(source, ".tar.gz") {
		return "url"
	}

	// HTTP/HTTPS URL detection
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return "url"
	}

	// Default to repository-based chart
	return "repo"
}

// repoCacheTag returns a short, stable discriminator for the chart repository
// a repo-type source resolves against, or "" when the source string already
// identifies its own origin (oci:// and direct .tgz URLs).
func repoCacheTag(sourceType, repoURL string) string {
	if sourceType != sourceTypeRepo || repoURL == "" {
		return ""
	}
	// Trailing slashes are insignificant to the index.yaml fetch, so
	// normalise them away to avoid two entries for one repository.
	sum := sha256.Sum256([]byte(strings.TrimRight(repoURL, "/")))
	return hex.EncodeToString(sum[:])[:16]
}

// isMutableVersion determines if a version string represents a mutable tag
func isMutableVersion(version string) bool {
	// Common mutable tags
	mutableTags := []string{"latest", "dev", "develop", "main", "master", "edge", "canary", "nightly"}

	// Check for exact matches (case-insensitive)
	lowerVersion := strings.ToLower(version)
	for _, tag := range mutableTags {
		if lowerVersion == tag {
			return true
		}
	}

	// Check for branch-like patterns (e.g., "feature-*", "release-*")
	if strings.Contains(version, "-SNAPSHOT") ||
		strings.Contains(version, "-dev") ||
		strings.Contains(version, "-alpha") ||
		strings.Contains(version, "-beta") ||
		strings.Contains(version, "-rc") {
		return true
	}

	// Semantic versions are typically immutable
	// Simple check: if it starts with 'v' followed by a digit, or just digits
	if strings.HasPrefix(version, "v") && len(version) > 1 {
		if version[1] >= '0' && version[1] <= '9' {
			return false // Likely a semantic version like v1.2.3
		}
	}
	if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
		return false // Likely a semantic version like 1.2.3
	}

	// Default to mutable for safety (shorter cache)
	return true
}

// Miss-reason label values for HelmChartCacheMissesTotal.
const (
	missReasonAbsent  string = "absent"  // key not present in cache (first request)
	missReasonExpired string = "expired" // TTL expired (detected via OnEvict callback)
	missReasonEvicted string = "evicted" // evicted by LRU capacity pressure
	missReasonCorrupt string = "corrupt" // entry existed but failed to load as a valid chart archive
)

// chartFetchResult carries the outcome of a chart fetch back to every caller
// of the singleflight so hit/miss accounting happens per caller rather than
// only once per deduplicated key.
type chartFetchResult struct {
	chart      *chart.Chart
	cacheHit   bool
	missReason string // populated only when cacheHit is false
}

// fetchChart fetches a Helm chart from the specified source
func (p *Provider) fetchChart(ctx context.Context, params *ChartSourceParams, options *RenderOptionsParams, appNamespace, releaseNamespace string) (*chart.Chart, error) {
	sourceType := detectChartSourceType(params.Source)

	// When the source declares auth.secretRef, the cache key is bound to a
	// hash of the resolved Secret data. Rotating the Secret (or creating a
	// new Application that points at a different Secret) invalidates the
	// cache automatically and forces a fresh registry call that exercises
	// the new credentials at the wire. Without this binding, cached chart
	// bytes pulled by an earlier authorized request would be served to a
	// subsequent request whose Secret no longer authenticates against the
	// registry — a real auth bypass for the cache TTL window.
	authTag, err := computeAuthCacheTag(ctx, params, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}

	// Build cache key: <cache_key_prefix>/<source_type>/<source>/<version>[/auth-<tag>]
	// For repo sources, Source is a bare chart name, so a hash-derived tag of
	// RepoURL is folded into the key to prevent charts of the same name and
	// version from colliding across different repositories. OCI (oci://) and
	// direct URL (.tgz/http) sources already carry their origin inside Source
	// and thus need no extra discriminator.
	sourceID := strings.ReplaceAll(strings.ReplaceAll(params.Source, "://", "-"), "/", "-")
	if repoTag := repoCacheTag(sourceType, params.RepoURL); repoTag != "" {
		sourceID = sourceID + "/repo-" + repoTag
	}

	var cacheKey string
	if options != nil && options.Cache != nil && options.Cache.Key != "" {
		// User provided cache key
		cacheKey = fmt.Sprintf("%s/%s/%s/%s",
			options.Cache.Key,
			sourceType,
			sourceID,
			params.Version)
	} else {
		// No cache key provided - use source-based key
		cacheKey = fmt.Sprintf("%s/%s/%s",
			sourceType,
			sourceID,
			params.Version)
	}
	if authTag != "" {
		cacheKey = cacheKey + "/auth-" + authTag
	}

	// Check if caching is disabled
	if options != nil && options.Cache != nil && options.Cache.TTL == "0" {
		klog.V(4).Info("Cache disabled for this chart")
		chartBytes, err := p.fetchChartWithoutCache(ctx, params, sourceType, appNamespace, releaseNamespace)
		if err != nil {
			return nil, err
		}
		chart, err := loader.LoadArchive(bytes.NewReader(chartBytes))
		if err != nil {
			return nil, errors.Wrap(err, "failed to load chart archive")
		}

		return chart, nil
	}
	v, err, _ := p.chartFlight.Do(cacheKey, func() (interface{}, error) {
		missReason := missReasonAbsent

		// Check if we have a cached chart. The auth-bound cache key above is
		// the primary guard against stale credentials. The explicit resolver
		// re-check below remains as a belt-and-suspenders measure: it catches
		// a missing or malformed Secret immediately, with the same RFC-cited
		// errors the cache-miss path would surface, instead of returning a
		// confusing cache-hit chart for a misconfigured request.
		if cached, found := p.cache.Get(cacheKey); found && cached != nil {
			if ch, err := loader.LoadArchive(bytes.NewReader(cached)); err == nil {
				if params.Auth != nil && params.Auth.SecretRef != nil {
					if _, _, err := resolveHTTPOptions(ctx, params, appNamespace, releaseNamespace, sourceType); err != nil {
						return nil, err
					}
				}
				klog.V(3).Infof("Using cached chart with key: %s", cacheKey)
				return &chartFetchResult{chart: ch, cacheHit: true}, nil
			}
			klog.V(2).Infof("Cached chart with key %s failed to load, evicting and refetching", cacheKey)
			p.cache.Delete(cacheKey)
			missReason = missReasonCorrupt
		} else {
			// Set reason for cache miss
			if reason, ok := p.cacheRecentEvictions.LoadAndDelete(cacheKey); ok {
				missReason = reason.(string)
			}
		}

		klog.V(4).Infof("Cache miss for key: %s, fetching chart", cacheKey)

		ch, err := p.fetchChartWithoutCache(ctx, params, sourceType, appNamespace, releaseNamespace)
		if err != nil {
			return nil, err
		}

		chart, err := loader.LoadArchive(bytes.NewReader(ch))
		if err != nil {
			p.cache.Delete(cacheKey)
			return nil, errors.Wrap(err, "failed to load chart archive")
		}
		// Determine cache TTL
		cacheTTL := p.determineCacheTTL(params.Version, options)

		// Cache the chart with appropriate TTL
		if cacheTTL > 0 {
			p.cache.Put(cacheKey, ch, cacheTTL)
			HelmChartCacheBytes.Set(float64(p.cache.CurrentBytes()))
			klog.V(3).Infof("Cached chart with key: %s (TTL: %v)", cacheKey, cacheTTL)
		}
		return &chartFetchResult{chart: chart, cacheHit: false, missReason: missReason}, nil
	})
	if err != nil {
		return nil, err
	}
	res, ok := v.(*chartFetchResult)
	if !ok {
		return nil, fmt.Errorf("unexpected type in cache for key %s: %T", cacheKey, v)
	}

	// Update metrics based on cache hit or miss
	if res.cacheHit {
		HelmChartCacheHitsTotal.Inc()
	} else {
		HelmChartCacheMissesTotal.WithLabelValues(res.missReason).Inc()
	}
	return res.chart, nil
}

// fetchChartWithoutCache fetches a chart without using cache
func (p *Provider) fetchChartWithoutCache(ctx context.Context, params *ChartSourceParams, sourceType string, appNamespace, releaseNamespace string) ([]byte, error) {
	switch sourceType {
	case "oci":
		return p.fetchOCIChart(ctx, params, appNamespace, releaseNamespace)
	case "url":
		return p.fetchURLChart(ctx, params, appNamespace, releaseNamespace)
	case "repo":
		return p.fetchRepoChart(ctx, params, appNamespace, releaseNamespace)
	default:
		return nil, fmt.Errorf("unsupported chart source type: %s", sourceType)
	}
}

// determineCacheTTL determines the cache TTL based on configuration and version type
func (p *Provider) determineCacheTTL(version string, options *RenderOptionsParams) time.Duration {
	// Check for explicit cache configuration in options
	if options != nil && options.Cache != nil {
		// If single TTL is specified, use it for all versions
		if options.Cache.TTL != "" && options.Cache.TTL != "0" {
			if duration, err := time.ParseDuration(options.Cache.TTL); err == nil {
				klog.V(4).Infof("Using explicit TTL from options: %v", duration)
				return duration
			} else {
				klog.Warningf("Invalid cache TTL %q, using defaults: %v", options.Cache.TTL, err)
			}
		}

		// Check for version-specific TTLs
		if isMutableVersion(version) {
			if options.Cache.MutableTTL != "" {
				if duration, err := time.ParseDuration(options.Cache.MutableTTL); err == nil {
					klog.V(4).Infof("Using mutable TTL from options: %v", duration)
					return duration
				} else {
					klog.Warningf("Invalid mutable cache TTL %q: %v", options.Cache.MutableTTL, err)
				}
			}
		} else {
			if options.Cache.ImmutableTTL != "" {
				if duration, err := time.ParseDuration(options.Cache.ImmutableTTL); err == nil {
					klog.V(4).Infof("Using immutable TTL from options: %v", duration)
					return duration
				} else {
					klog.Warningf("Invalid immutable cache TTL %q: %v", options.Cache.ImmutableTTL, err)
				}
			}
		}
	}

	// Fall back to provider defaults
	if isMutableVersion(version) {
		klog.V(4).Infof("Version %q detected as mutable, using default TTL of %v", version, p.cacheTTL.MutableVersionTTL)
		return p.cacheTTL.MutableVersionTTL
	}

	klog.V(4).Infof("Version %q detected as immutable, using default TTL of %v", version, p.cacheTTL.ImmutableVersionTTL)
	return p.cacheTTL.ImmutableVersionTTL
}

// fetchOCIChart fetches a chart from an OCI registry.
func (p *Provider) fetchOCIChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) ([]byte, error) {
	httpOpts, rawDockerCfg, err := resolveHTTPOptions(ctx, params, appNamespace, releaseNamespace, sourceTypeOCI)
	if err != nil {
		return nil, errors.Wrap(err, "auth resolution failed")
	}

	clientOpts := []registry.ClientOption{}
	if httpOpts != nil || rawDockerCfg != nil {
		host := extractRegistryHost(params.Source, params.RepoURL)
		credFile, cleanup, werr := writeOCIRegistryConfigFile(httpOpts, rawDockerCfg, host)
		if werr != nil {
			return nil, errors.Wrap(werr, "failed to materialize OCI credentials file")
		}
		defer cleanup()
		clientOpts = append(clientOpts, registry.ClientOptCredentialsFile(credFile))
	}
	if httpOpts != nil && httpOpts.PlainHTTP {
		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}

	registryClient, err := registry.NewClient(clientOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OCI registry client")
	}

	ref := strings.TrimPrefix(params.Source, "oci://")
	if params.Version != "" {
		ref = fmt.Sprintf("%s:%s", ref, params.Version)
	}
	result, err := registryClient.Pull(ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull OCI chart %s", ref)
	}
	// return loader.LoadArchive(bytes.NewReader(result.Chart.Data))
	return result.Chart.Data, nil
}

// fetchURLChart fetches a chart from a direct URL.
func (p *Provider) fetchURLChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) ([]byte, error) {
	httpOpts, _, err := resolveHTTPOptions(ctx, params, appNamespace, releaseNamespace, sourceTypeURL)
	if err != nil {
		return nil, errors.Wrap(err, "auth resolution failed")
	}
	if httpOpts == nil {
		httpOpts = &common.HTTPOption{}
	}

	chartBytes, err := common.HTTPGetWithOption(ctx, params.Source, httpOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download chart from %s", params.Source)
	}
	return chartBytes, nil
}

// fetchRepoChart fetches a chart from a Helm repository.
func (p *Provider) fetchRepoChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) ([]byte, error) {
	if params.RepoURL == "" {
		return nil, fmt.Errorf("repoURL is required for repository-based charts")
	}

	httpOpts, _, err := resolveHTTPOptions(ctx, params, appNamespace, releaseNamespace, sourceTypeRepo)
	if err != nil {
		return nil, errors.Wrap(err, "auth resolution failed")
	}
	if httpOpts == nil {
		httpOpts = &common.HTTPOption{}
	}

	indexURL := fmt.Sprintf("%s/index.yaml", params.RepoURL)
	indexBytes, err := common.HTTPGetWithOption(ctx, indexURL, httpOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch repository index from %s", indexURL)
	}

	var index repo.IndexFile
	if err := yaml.Unmarshal(indexBytes, &index); err != nil {
		return nil, errors.Wrap(err, "failed to parse repository index")
	}
	index.SortEntries()

	chartVersion, err := index.Get(params.Source, params.Version)
	if err != nil {
		return nil, fmt.Errorf("version %q of chart %s not found in repository %s: %w", params.Version, params.Source, params.RepoURL, err)
	}

	var downloadURL string
	if len(chartVersion.URLs) > 0 {
		downloadURL = chartVersion.URLs[0]
		if !strings.HasPrefix(downloadURL, "http://") && !strings.HasPrefix(downloadURL, "https://") {
			downloadURL = fmt.Sprintf("%s/%s", params.RepoURL, downloadURL)
		}
	} else {
		return nil, fmt.Errorf("no download URL found for chart %s", params.Source)
	}

	chartBytes, err := common.HTTPGetWithOption(ctx, downloadURL, httpOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download chart from %s", downloadURL)
	}
	return chartBytes, nil
}
