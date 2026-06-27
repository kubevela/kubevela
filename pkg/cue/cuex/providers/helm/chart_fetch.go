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

// isMutableVersion determines if a version string represents a mutable tag
func isMutableVersion(version string) bool {
	// OCI digest references (e.g. sha256:a1b2c3d4...) are content-addressed
	// and fully immutable. They MUST get the long (24-hour) cache TTL.
	if strings.HasPrefix(version, "sha256:") {
		return false
	}

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
	var cacheKey string
	if options != nil && options.Cache != nil && options.Cache.Key != "" {
		// User provided cache key
		cacheKey = fmt.Sprintf("%s/%s/%s/%s",
			options.Cache.Key,
			sourceType,
			strings.ReplaceAll(strings.ReplaceAll(params.Source, "://", "-"), "/", "-"),
			params.Version)
	} else {
		// No cache key provided - use source-based key
		cacheKey = fmt.Sprintf("%s/%s/%s",
			sourceType,
			strings.ReplaceAll(strings.ReplaceAll(params.Source, "://", "-"), "/", "-"),
			params.Version)
	}
	if authTag != "" {
		cacheKey = cacheKey + "/auth-" + authTag
	}

	// Check if caching is disabled
	if options != nil && options.Cache != nil && options.Cache.TTL == "0" {
		klog.V(4).Info("Cache disabled for this chart")
		return p.fetchChartWithoutCache(ctx, params, sourceType, appNamespace, releaseNamespace)
	}

	// Check if we have a cached chart. The auth-bound cache key above is
	// the primary guard against stale credentials. The explicit resolver
	// re-check below remains as a belt-and-suspenders measure: it catches
	// a missing or malformed Secret immediately, with the same RFC-cited
	// errors the cache-miss path would surface, instead of returning a
	// confusing cache-hit chart for a misconfigured request.
	if cached := p.cache.Get(cacheKey); cached != nil {
		if ch, ok := cached.(*chart.Chart); ok {
			if params.Auth != nil && params.Auth.SecretRef != nil {
				if _, _, err := resolveHTTPOptions(ctx, params, appNamespace, releaseNamespace, sourceType); err != nil {
					return nil, err
				}
			}
			klog.V(3).Infof("Using cached chart with key: %s", cacheKey)
			return ch, nil
		}
	}

	klog.V(4).Infof("Cache miss for key: %s, fetching chart", cacheKey)

	ch, err := p.fetchChartWithoutCache(ctx, params, sourceType, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}

	// Determine cache TTL
	cacheTTL := p.determineCacheTTL(params.Version, options)

	// Cache the chart with appropriate TTL
	if cacheTTL > 0 {
		p.cache.Put(cacheKey, ch, cacheTTL)
		klog.V(3).Infof("Cached chart with key: %s (TTL: %v)", cacheKey, cacheTTL)
	}

	return ch, nil
}

// fetchChartWithoutCache fetches a chart without using cache
func (p *Provider) fetchChartWithoutCache(ctx context.Context, params *ChartSourceParams, sourceType string, appNamespace, releaseNamespace string) (*chart.Chart, error) {
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
func (p *Provider) fetchOCIChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) (*chart.Chart, error) {
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
	return loader.LoadArchive(bytes.NewReader(result.Chart.Data))
}

// fetchURLChart fetches a chart from a direct URL.
func (p *Provider) fetchURLChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) (*chart.Chart, error) {
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
	ch, err := loader.LoadArchive(bytes.NewReader(chartBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart archive")
	}
	return ch, nil
}

// fetchRepoChart fetches a chart from a Helm repository.
func (p *Provider) fetchRepoChart(ctx context.Context, params *ChartSourceParams, appNamespace, releaseNamespace string) (*chart.Chart, error) {
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
	ch, err := loader.LoadArchive(bytes.NewReader(chartBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart archive")
	}
	return ch, nil
}
