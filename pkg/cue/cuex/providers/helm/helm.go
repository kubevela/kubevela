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

package helm

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

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

// ValuesFromParams represents a values source
type ValuesFromParams struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key,omitempty"`
	URL       string `json:"url,omitempty"`
	Tag       string `json:"tag,omitempty"`
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

// RenderParams represents the parameters for rendering a Helm chart
type RenderParams struct {
	Chart      ChartSourceParams    `json:"chart"`
	Release    *ReleaseParams       `json:"release,omitempty"`
	Values     interface{}          `json:"values,omitempty"`
	ValuesFrom []ValuesFromParams   `json:"valuesFrom,omitempty"`
	Options    *RenderOptionsParams `json:"options,omitempty"`
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

// DefaultCacheTTLConfig returns the default cache TTL configuration
func DefaultCacheTTLConfig() *CacheTTLConfig {
	return &CacheTTLConfig{
		ImmutableVersionTTL: 24 * time.Hour,  // 24 hours for fixed versions
		MutableVersionTTL:   5 * time.Minute, // 5 minutes for mutable tags
	}
}

// Provider is the Helm chart provider
type Provider struct {
	cache      *utils.MemoryCacheStore
	helmClient *cli.EnvSettings
	cacheTTL   *CacheTTLConfig
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
			cache:      utils.NewMemoryCacheStore(context.Background()),
			helmClient: cli.New(),
			cacheTTL:   DefaultCacheTTLConfig(),
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
		cache:      utils.NewMemoryCacheStore(context.Background()),
		helmClient: cli.New(),
		cacheTTL:   ttlConfig,
	}
}

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
func (p *Provider) fetchChart(ctx context.Context, params *ChartSourceParams, options *RenderOptionsParams) (*chart.Chart, error) {
	sourceType := detectChartSourceType(params.Source)

	// Build cache key: <cache_key_prefix>/<source_type>/<source>/<version>
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

	// Check if caching is disabled
	if options != nil && options.Cache != nil && options.Cache.TTL == "0" {
		klog.V(4).Info("Cache disabled for this chart")
		return p.fetchChartWithoutCache(ctx, params, sourceType)
	}

	// Check if we have a cached chart
	if cached := p.cache.Get(cacheKey); cached != nil {
		if ch, ok := cached.(*chart.Chart); ok {
			klog.V(3).Infof("Using cached chart with key: %s", cacheKey)
			return ch, nil
		}
	}

	klog.V(4).Infof("Cache miss for key: %s, fetching chart", cacheKey)

	ch, err := p.fetchChartWithoutCache(ctx, params, sourceType)
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
func (p *Provider) fetchChartWithoutCache(ctx context.Context, params *ChartSourceParams, sourceType string) (*chart.Chart, error) {
	switch sourceType {
	case "oci":
		return p.fetchOCIChart(ctx, params)
	case "url":
		return p.fetchURLChart(ctx, params)
	case "repo":
		return p.fetchRepoChart(ctx, params)
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

// fetchOCIChart fetches a chart from an OCI registry
func (p *Provider) fetchOCIChart(ctx context.Context, params *ChartSourceParams) (*chart.Chart, error) {
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OCI registry client")
	}

	// Remove oci:// prefix
	ref := strings.TrimPrefix(params.Source, "oci://")
	if params.Version != "" {
		ref = fmt.Sprintf("%s:%s", ref, params.Version)
	}

	// Pull the chart
	result, err := registryClient.Pull(ref)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull OCI chart %s", ref)
	}

	ch, err := loader.LoadArchive(bytes.NewReader(result.Chart.Data))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load OCI chart")
	}

	return ch, nil
}

// fetchURLChart fetches a chart from a direct URL
func (p *Provider) fetchURLChart(ctx context.Context, params *ChartSourceParams) (*chart.Chart, error) {
	// Create HTTP client with options
	opts := &common.HTTPOption{}
	// TODO: Add authentication support from params.Auth

	chartBytes, err := common.HTTPGetWithOption(ctx, params.Source, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download chart from %s", params.Source)
	}

	ch, err := loader.LoadArchive(bytes.NewReader(chartBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart archive")
	}

	return ch, nil
}

// fetchRepoChart fetches a chart from a Helm repository
func (p *Provider) fetchRepoChart(ctx context.Context, params *ChartSourceParams) (*chart.Chart, error) {
	if params.RepoURL == "" {
		return nil, fmt.Errorf("repoURL is required for repository-based charts")
	}

	// Create temporary directory for operations
	tmpDir, err := os.MkdirTemp("", "helm-chart-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory")
	}
	defer os.RemoveAll(tmpDir)

	// First, fetch the repository index to find the chart
	indexURL := fmt.Sprintf("%s/index.yaml", params.RepoURL)

	// Use HTTP client to fetch index
	indexBytes, err := common.HTTPGetWithOption(ctx, indexURL, &common.HTTPOption{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch repository index from %s", indexURL)
	}

	// Parse the index to find the chart URL
	var index repo.IndexFile
	if err := yaml.Unmarshal(indexBytes, &index); err != nil {
		return nil, errors.Wrap(err, "failed to parse repository index")
	}

	// Find the chart in the index
	chartVersions, ok := index.Entries[params.Source]
	if !ok {
		return nil, fmt.Errorf("chart %s not found in repository %s", params.Source, params.RepoURL)
	}

	// Find the requested version
	var chartVersion *repo.ChartVersion
	for _, cv := range chartVersions {
		if params.Version == "" || cv.Version == params.Version {
			chartVersion = cv
			break
		}
	}

	if chartVersion == nil {
		return nil, fmt.Errorf("version %s of chart %s not found", params.Version, params.Source)
	}

	// Get the download URL
	var downloadURL string
	if len(chartVersion.URLs) > 0 {
		downloadURL = chartVersion.URLs[0]
		// Make URL absolute if it's relative
		if !strings.HasPrefix(downloadURL, "http://") && !strings.HasPrefix(downloadURL, "https://") {
			downloadURL = fmt.Sprintf("%s/%s", params.RepoURL, downloadURL)
		}
	} else {
		return nil, fmt.Errorf("no download URL found for chart %s", params.Source)
	}

	// Download the chart
	chartBytes, err := common.HTTPGetWithOption(ctx, downloadURL, &common.HTTPOption{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download chart from %s", downloadURL)
	}

	// Load the chart from bytes
	ch, err := loader.LoadArchive(bytes.NewReader(chartBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart archive")
	}

	return ch, nil
}

// mergeValues merges values from multiple sources
func (p *Provider) mergeValues(ctx context.Context, baseValues interface{}, valuesFrom []ValuesFromParams) (map[string]interface{}, error) {
	// Start with base values
	result := make(map[string]interface{})

	if baseValues != nil {
		if m, ok := baseValues.(map[string]interface{}); ok {
			result = m
		}
	}

	// Merge values from each source
	for _, source := range valuesFrom {
		values, err := p.loadValuesFromSource(ctx, source)
		if err != nil {
			if source.Optional {
				klog.V(4).Infof("Skipping optional values source %s/%s: %v", source.Kind, source.Name, err)
				continue
			}
			return nil, errors.Wrapf(err, "failed to load values from %s/%s", source.Kind, source.Name)
		}

		// Merge values
		result = chartutil.CoalesceTables(result, values)
	}

	return result, nil
}

// loadValuesFromSource loads values from a specific source
func (p *Provider) loadValuesFromSource(ctx context.Context, source ValuesFromParams) (map[string]interface{}, error) {
	switch source.Kind {
	case "ConfigMap":
		// TODO: Implement ConfigMap loading
		return nil, fmt.Errorf("configmap values source not yet implemented")
	case "Secret":
		// TODO: Implement Secret loading
		return nil, fmt.Errorf("secret values source not yet implemented")
	case "OCIRepository":
		// TODO: Implement OCI repository loading
		return nil, fmt.Errorf("ocirepository values source not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported values source kind: %s", source.Kind)
	}
}

// renderChart renders a Helm chart to Kubernetes resources
func (p *Provider) renderChart(_ context.Context, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams) ([]map[string]interface{}, string, error) {
	// Set default options
	if options == nil {
		options = &RenderOptionsParams{}
	}

	// Set defaults
	includeCRDs := true
	if options.IncludeCRDs != nil {
		includeCRDs = *options.IncludeCRDs
	}

	skipTests := true
	if options.SkipTests != nil {
		skipTests = *options.SkipTests
	}

	// Configure action
	actionConfig := &action.Configuration{}

	// Create install action for rendering
	install := action.NewInstall(actionConfig)
	install.DryRun = true
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.IncludeCRDs = includeCRDs
	install.ClientOnly = true
	if options.SkipHooks != nil {
		install.DisableHooks = *options.SkipHooks
	} else {
		install.DisableHooks = false
	}

	// Use the actual cluster version from the control plane
	clusterVersion := types.ControlPlaneClusterVersion
	if clusterVersion.Major != "" && clusterVersion.Minor != "" {
		// Build version string from cluster info
		versionString := fmt.Sprintf("v%s.%s", clusterVersion.Major, clusterVersion.Minor)
		if clusterVersion.GitVersion != "" {
			versionString = clusterVersion.GitVersion
		}

		install.KubeVersion = &chartutil.KubeVersion{
			Version: versionString,
			Major:   clusterVersion.Major,
			Minor:   clusterVersion.Minor,
		}
		klog.V(2).Infof("Helm provider: Using Kubernetes version %s for chart compatibility", versionString)
	} else {
		klog.Warning("Helm provider: Cluster version not available, chart version requirements may not be evaluated correctly")
	}

	// Render the chart
	release, err := install.Run(ch, values)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to render chart")
	}

	// Parse manifests into unstructured objects
	resources := []map[string]interface{}{}
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(release.Manifest), 4096)

	for {
		resource := &unstructured.Unstructured{}
		if err := decoder.Decode(&resource); err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", errors.Wrap(err, "failed to decode manifest")
		}

		// Skip empty resources
		if resource.Object == nil {
			continue
		}

		// Skip test resources if requested
		if skipTests && isTestResource(resource) {
			continue
		}

		// Clean and add the resource
		cleanedResource := cleanResource(resource.Object)
		resources = append(resources, cleanedResource)
	}

	// Order resources: CRDs first, then namespaces, then other resources
	resources = orderResources(resources)

	klog.Infof("Helm provider: Checking namespace creation for release namespace: %s", releaseNamespace)

	// Check if we need to create the namespace
	// Default to creating namespace if resources need it and it's not "default"
	createNamespace := true
	if options != nil && options.CreateNamespace != nil {
		createNamespace = *options.CreateNamespace
	}

	klog.Infof("Helm provider: createNamespace option is %v", createNamespace)

	if createNamespace && releaseNamespace != "" && releaseNamespace != "default" {
		// Check if namespace already exists in resources
		namespaceExists := false
		for _, res := range resources {
			if kind, _, _ := unstructured.NestedString(res, "kind"); kind == "Namespace" {
				if name, _, _ := unstructured.NestedString(res, "metadata", "name"); name == releaseNamespace {
					namespaceExists = true
					break
				}
			}
		}

		// If namespace doesn't exist, add it
		if !namespaceExists {
			klog.Infof("Helm provider: Namespace %s not found in resources, checking if needed", releaseNamespace)

			// Check if any resource needs this namespace
			needsNamespace := false
			resourceCount := 0
			for _, res := range resources {
				if ns, _, _ := unstructured.NestedString(res, "metadata", "namespace"); ns == releaseNamespace {
					needsNamespace = true
					resourceCount++
				}
			}

			if needsNamespace {
				klog.Infof("Helm provider: Creating namespace %s as %d resources need it", releaseNamespace, resourceCount)
				// Create namespace resource
				namespaceResource := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": releaseNamespace,
						"labels": map[string]interface{}{
							"app.kubernetes.io/managed-by": "Helm",
							"app.kubernetes.io/instance":   releaseName,
							"app.oam.dev/created-by":       "kubevela-helm-provider",
							"app.oam.dev/render-type":      "helm",
							"helm.sh/release-name":         releaseName,
							"helm.sh/release-namespace":    releaseNamespace,
						},
						"annotations": map[string]interface{}{
							"meta.helm.sh/release-name":      releaseName,
							"meta.helm.sh/release-namespace": releaseNamespace,
							"app.oam.dev/kubevela-version":   "latest", // Could be injected from build
						},
					},
				}
				// Prepend namespace to resources (it will be ordered properly later)
				resources = append([]map[string]interface{}{namespaceResource}, resources...)
				// Re-order to ensure namespace is in the right place
				resources = orderResources(resources)
			}
		}
	}

	return resources, release.Info.Notes, nil
}

// isTestResource checks if a resource is a test resource
func isTestResource(resource *unstructured.Unstructured) bool {
	annotations := resource.GetAnnotations()
	if annotations != nil {
		if hookType, exists := annotations["helm.sh/hook"]; exists {
			return strings.Contains(hookType, "test")
		}
	}
	return false
}

// cleanResource removes any nil values from a resource map
func cleanResource(resource map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for k, v := range resource {
		if v != nil {
			switch val := v.(type) {
			case map[string]interface{}:
				// Recursively clean nested maps
				cleanedMap := cleanResource(val)
				if len(cleanedMap) > 0 {
					cleaned[k] = cleanedMap
				}
			case []interface{}:
				// Clean arrays
				cleanedArray := make([]interface{}, 0)
				for _, item := range val {
					if item != nil {
						if m, ok := item.(map[string]interface{}); ok {
							cleanedArray = append(cleanedArray, cleanResource(m))
						} else {
							cleanedArray = append(cleanedArray, item)
						}
					}
				}
				if len(cleanedArray) > 0 {
					cleaned[k] = cleanedArray
				}
			default:
				// Keep non-nil values
				cleaned[k] = v
			}
		}
	}
	return cleaned
}

// orderResources orders resources with CRDs first, then namespaces, then others
func orderResources(resources []map[string]interface{}) []map[string]interface{} {
	var crds, namespaces, others []map[string]interface{}

	for _, r := range resources {
		kind, _, _ := unstructured.NestedString(r, "kind")
		switch kind {
		case "CustomResourceDefinition":
			crds = append(crds, r)
		case "Namespace":
			namespaces = append(namespaces, r)
		default:
			others = append(others, r)
		}
	}

	// Combine in order
	result := make([]map[string]interface{}, 0, len(resources))
	result = append(result, crds...)
	result = append(result, namespaces...)
	result = append(result, others...)

	return result
}

// Render is the main provider function for rendering Helm charts
func Render(ctx context.Context, params *providers.Params[RenderParams]) (*providers.Returns[RenderReturns], error) {
	p := NewProvider()

	renderParams := params.Params

	klog.V(2).Infof("Helm provider: Starting render for chart %s from %s", renderParams.Chart.Source, renderParams.Chart.RepoURL)

	// Set default release name and namespace
	releaseName := "release"
	releaseNamespace := "default"

	if renderParams.Release != nil {
		if renderParams.Release.Name != "" {
			releaseName = renderParams.Release.Name
		}
		if renderParams.Release.Namespace != "" {
			releaseNamespace = renderParams.Release.Namespace
		}
	}

	klog.V(3).Infof("Helm provider: Release name=%s, namespace=%s", releaseName, releaseNamespace)

	// Fetch the chart
	chart, err := p.fetchChart(ctx, &renderParams.Chart, renderParams.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch chart")
	}
	klog.V(2).Infof("Helm provider: Successfully fetched chart %s", chart.Name())

	// Merge values from all sources
	values, err := p.mergeValues(ctx, renderParams.Values, renderParams.ValuesFrom)
	if err != nil {
		return nil, errors.Wrap(err, "failed to merge values")
	}
	klog.V(3).Infof("Helm provider: Merged values: %v", values)

	// Render the chart
	resources, notes, err := p.renderChart(ctx, chart, releaseName, releaseNamespace, values, renderParams.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to render chart")
	}

	klog.Infof("Helm provider: Rendered %d resources for chart %s", len(resources), renderParams.Chart.Source)

	// Log first resource for debugging
	if len(resources) > 0 {
		if kind, found, _ := unstructured.NestedString(resources[0], "kind"); found {
			if name, found, _ := unstructured.NestedString(resources[0], "metadata", "name"); found {
				klog.Infof("Helm provider: First resource is %s/%s", kind, name)
			}
		}

		// Log raw JSON of first resource for debugging
		if jsonBytes, err := json.MarshalIndent(resources[0], "", "  "); err == nil {
			klog.Infof("Helm provider: First resource JSON:\n%s", string(jsonBytes))

			// Check for any nil values in metadata
			if metadata, found, _ := unstructured.NestedMap(resources[0], "metadata"); found {
				if annotations, found, _ := unstructured.NestedMap(metadata, "annotations"); found {
					klog.Infof("Helm provider: First resource has %d annotations", len(annotations))
					for key, val := range annotations {
						if val == nil {
							klog.Warningf("Helm provider: Annotation %s has nil value", key)
						}
					}
				}
			}
		} else {
			klog.Warningf("Helm provider: Failed to marshal first resource to JSON: %v", err)
		}

		// Also log a summary of all resources
		klog.Infof("Helm provider: All resources summary:")
		for i, res := range resources {
			if kind, found, _ := unstructured.NestedString(res, "kind"); found {
				if name, found, _ := unstructured.NestedString(res, "metadata", "name"); found {
					klog.Infof("  [%d] %s/%s", i, kind, name)
				}
			}
		}
	}

	result := &providers.Returns[RenderReturns]{
		Returns: RenderReturns{
			Resources: resources,
			Notes:     notes,
		},
	}

	klog.V(3).Infof("Helm provider: Returning result with %d resources", len(result.Returns.Resources))

	return result, nil
}

// ProviderName is the name of this provider
const ProviderName = "helm"

//go:embed helm.cue
var template string

// Template exports the CUE template for use by workflow providers
var Template = template

// Package exports the provider package for registration
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render": cuexruntime.GenericProviderFn[providers.Params[RenderParams], providers.Returns[RenderReturns]](Render),
}))
