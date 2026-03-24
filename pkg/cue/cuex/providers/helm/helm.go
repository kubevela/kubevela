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
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"

	"github.com/oam-dev/kubevela/pkg/oam"
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

// ContextParams holds KubeVela ownership information to be injected as labels
type ContextParams struct {
	AppName      string `json:"appName"`
	AppNamespace string `json:"appNamespace"`
	Name         string `json:"name"`      // component name
	Namespace    string `json:"namespace"` // component namespace
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
	releaseFingerprints map[string]string // releaseName → fingerprint (chartVersion|valuesHash)
	releaseManifests    map[string]string // releaseName → last successful manifest
	releaseVersions     map[string]int    // releaseName → current release version number
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

// getActionConfig initializes a Helm action.Configuration with a real Kubernetes
// REST client and a secrets-based storage driver so that releases persist in-cluster.
func (p *Provider) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := &action.Configuration{}
	if err := actionConfig.Init(
		p.helmClient.RESTClientGetter(),
		namespace,
		"secret",
		klog.Infof,
	); err != nil {
		return nil, errors.Wrap(err, "failed to initialize helm action configuration")
	}
	return actionConfig, nil
}

// velaLabelPostRenderer implements postrender.PostRenderer.
// It injects KubeVela ownership labels and annotations into every resource
// before Helm deploys them, enabling KubeVela to adopt the resources.
// It also injects Helm ownership annotations (meta.helm.sh/release-name and
// meta.helm.sh/release-namespace) so that resources re-applied from
// KubeVela's ResourceTracker can be adopted by a subsequent helm install.
type velaLabelPostRenderer struct {
	context          *ContextParams
	releaseName      string
	releaseNamespace string
}

// Run implements postrender.PostRenderer. It parses each YAML document in the
// rendered manifests, injects KubeVela ownership labels/annotations, and returns
// the modified manifests.
func (r *velaLabelPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	if r.context == nil {
		return renderedManifests, nil
	}

	out := &bytes.Buffer{}
	decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(renderedManifests.Bytes()), 4096)

	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err, "post-renderer: failed to decode manifest")
		}

		if obj.Object == nil || len(obj.Object) == 0 {
			continue
		}

		// Inject KubeVela ownership labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["app.oam.dev/name"] = r.context.AppName
		labels["app.oam.dev/namespace"] = r.context.AppNamespace
		labels["app.oam.dev/component"] = r.context.Name
		obj.SetLabels(labels)

		// Inject ownership annotations (both KubeVela and Helm)
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["app.oam.dev/owner"] = "helm-provider"
		// Inject Helm ownership annotations so that resources re-applied from
		// KubeVela's ResourceTracker retain Helm adoption metadata. Without
		// these, a subsequent helm install would fail with:
		//   "cannot be imported into the current release: invalid ownership metadata"
		if r.releaseName != "" {
			annotations["meta.helm.sh/release-name"] = r.releaseName
		}
		if r.releaseNamespace != "" {
			annotations["meta.helm.sh/release-namespace"] = r.releaseNamespace
		}
		obj.SetAnnotations(annotations)

		// Serialize back to YAML
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, errors.Wrap(err, "post-renderer: failed to marshal resource")
		}

		out.WriteString("---\n")
		out.Write(data)
	}

	return out, nil
}

// velaOwnerLabels returns KubeVela ownership labels suitable for embedding in Helm
// action Labels (which are written onto the Kubernetes release Secret). Returns nil
// when velaCtx is nil so callers can skip the label map safely.
func velaOwnerLabels(velaCtx *ContextParams) map[string]string {
	if velaCtx == nil {
		return nil
	}
	return map[string]string{
		"app.oam.dev/name":      velaCtx.AppName,
		"app.oam.dev/namespace": velaCtx.AppNamespace,
		"app.oam.dev/component": velaCtx.Name,
	}
}

// isOwnedByVela checks whether a Helm release was installed/managed by KubeVela
// by looking for the app.oam.dev/name label on the release's metadata (which is
// stored on the Kubernetes release Secret via install.Labels / upgrade.Labels).
// An external release (installed via `helm install` on the CLI) won't have this label.
func isOwnedByVela(rel *release.Release, velaCtx *ContextParams) bool {
	if rel == nil || velaCtx == nil {
		return false
	}
	if rel.Labels == nil {
		return false
	}
	return rel.Labels["app.oam.dev/name"] != ""
}

// computeReleaseFingerprint builds a deterministic string from chart version and a
// SHA-256 hash of the values so repeated reconciles with no real changes can be
// detected cheaply without calling the Kubernetes API.
func computeReleaseFingerprint(ch *chart.Chart, values map[string]interface{}) string {
	version := ""
	if ch != nil && ch.Metadata != nil {
		version = ch.Metadata.Version
	}
	valuesJSON, _ := json.Marshal(values)
	h := sha256.Sum256(valuesJSON)
	return version + "|" + hex.EncodeToString(h[:])
}

// installOrUpgradeChart performs a real Helm install or upgrade against the cluster.
// It uses a post-renderer to inject KubeVela ownership labels so the deployed
// resources are immediately owned by the application.
//
// Dedup: a SHA-256 fingerprint of (chartVersion, values) is checked against an
// in-memory cache and the live release in the cluster. If the release is already
// deployed with an identical fingerprint the call is a no-op and the cached
// manifest is returned, preventing spurious revision bumps on every reconcile.
//
// KubeVela labels are also set on the Helm action (install.Labels / upgrade.Labels)
// so they are embedded in the Kubernetes release Secret by the Helm SDK. This allows
// KubeVela to track the release Secret in its ResourceTracker and delete it when the
// Application is deleted — which removes the release from `helm list`.
//
// Returns the release manifest string, notes, release version, and any error.
func (p *Provider) installOrUpgradeChart(ctx context.Context, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, int, error) {
	fingerprint := computeReleaseFingerprint(ch, values)

	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()

	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		return "", "", 0, err
	}

	postRenderer := &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}

	// Build labels to embed in the release Secret so KubeVela can track and
	// delete it via the ResourceTracker when the Application is deleted.
	releaseLabels := velaOwnerLabels(velaCtx)

	// Always check the live release in the cluster before using cached data.
	// This prevents stale cache entries from masking externally-deleted releases
	// (e.g., helm uninstall, deleted secrets, namespace deletion).
	getAction := action.NewGet(actionConfig)
	existingRelease, getErr := getAction.Run(releaseName)

	if getErr != nil {
		// Release not found in cluster — clear any stale cache entry so we
		// fall through to a fresh install below.
		if cached, ok := p.releaseFingerprints[releaseName]; ok {
			klog.Infof("Helm provider: Release %s not found in cluster but cached (fingerprint=%s), clearing stale cache", releaseName, cached[:16])
			delete(p.releaseFingerprints, releaseName)
			delete(p.releaseManifests, releaseName)
			delete(p.releaseVersions, releaseName)
		}
	}

	if getErr == nil && existingRelease != nil {
		// Check if this release was installed by KubeVela (has our ownership labels
		// on the release Secret). If not, it's an external release that we need to
		// adopt by forcing an upgrade — even if the fingerprint matches — so the
		// post-renderer injects KubeVela ownership labels onto every resource.
		needsAdoption := velaCtx != nil && !isOwnedByVela(existingRelease, velaCtx)
		if needsAdoption {
			klog.Infof("Helm provider: Release %s exists but was not installed by KubeVela (missing ownership labels), forcing upgrade to adopt", releaseName)
		}

		// Release exists — check if it is already deployed with the same fingerprint
		if !needsAdoption && existingRelease.Info.Status == release.StatusDeployed {
			clusterFingerprint := computeReleaseFingerprint(existingRelease.Chart, existingRelease.Config)
			if clusterFingerprint == fingerprint {
				klog.V(3).Infof("Helm provider: Release %s already deployed and unchanged (cluster fingerprint match), skipping upgrade", releaseName)
				p.releaseFingerprints[releaseName] = fingerprint
				p.releaseManifests[releaseName] = existingRelease.Manifest
				p.releaseVersions[releaseName] = existingRelease.Version
				// Run health validation in the background to detect corrupted
				// or missing release secrets early, without blocking this call.
				go p.validateReleaseHealth(releaseName, releaseNamespace)
				return existingRelease.Manifest, existingRelease.Info.Notes, existingRelease.Version, nil
			}
		}

		// Fingerprint differs, needs adoption, or release is not in a clean deployed state — upgrade
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = releaseNamespace
		upgrade.PostRenderer = postRenderer
		upgrade.Labels = releaseLabels

		if options != nil {
			if options.Atomic {
				upgrade.Atomic = true
			}
			if options.Wait || options.Atomic {
				upgrade.Wait = true
			}
			if options.Timeout != "" {
				if d, err := time.ParseDuration(options.Timeout); err == nil {
					upgrade.Timeout = d
				}
			}
			if options.Force {
				upgrade.Force = true
			}
			if options.CleanupOnFail {
				upgrade.CleanupOnFail = true
			}
			if options.RecreatePods {
				upgrade.Recreate = true
			}
			if options.MaxHistory > 0 {
				upgrade.MaxHistory = options.MaxHistory
			}
			if options.SkipHooks != nil {
				upgrade.DisableHooks = *options.SkipHooks
			}
		}

		klog.Infof("Helm provider: Upgrading release %s in namespace %s", releaseName, releaseNamespace)
		rel, err := upgrade.RunWithContext(ctx, releaseName, ch, values)
		if err != nil {
			return "", "", 0, errors.Wrapf(err, "failed to upgrade helm release %s", releaseName)
		}
		klog.Infof("Helm provider: Successfully upgraded release %s", releaseName)
		p.releaseFingerprints[releaseName] = fingerprint
		p.releaseManifests[releaseName] = rel.Manifest
		p.releaseVersions[releaseName] = rel.Version
		return rel.Manifest, rel.Info.Notes, rel.Version, nil
	}

	// No existing release — perform a fresh install
	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = false
	install.ClientOnly = false
	install.PostRenderer = postRenderer
	install.CreateNamespace = true
	install.Labels = releaseLabels

	if options != nil {
		if options.Atomic {
			install.Atomic = true
		}
		if options.Wait || options.Atomic {
			install.Wait = true
		}
		if options.Timeout != "" {
			if d, err := time.ParseDuration(options.Timeout); err == nil {
				install.Timeout = d
			}
		}
		if options.CreateNamespace != nil {
			install.CreateNamespace = *options.CreateNamespace
		}
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}

	klog.Infof("Helm provider: Installing release %s in namespace %s", releaseName, releaseNamespace)
	rel, err := install.RunWithContext(ctx, ch, values)
	if err != nil {
		// If install fails due to corrupted/orphaned release secrets or ownership
		// conflicts, clean up the broken state and retry once.
		errMsg := err.Error()
		if strings.Contains(errMsg, "cannot be imported") ||
			strings.Contains(errMsg, "invalid ownership metadata") ||
			strings.Contains(errMsg, "no revision for release") ||
			strings.Contains(errMsg, "release: already exists") {
			klog.Warningf("Helm provider: Install failed for %s due to orphaned state (%v), cleaning up and retrying", releaseName, err)
			if cleanErr := p.cleanOrphanedReleaseSecrets(actionConfig, releaseName, releaseNamespace); cleanErr != nil {
				klog.Warningf("Helm provider: Failed to clean orphaned secrets for %s: %v", releaseName, cleanErr)
				return "", "", 0, errors.Wrapf(err, "failed to install helm release %s (cleanup also failed: %v)", releaseName, cleanErr)
			}
			// Retry install after cleanup
			install2 := action.NewInstall(actionConfig)
			install2.ReleaseName = releaseName
			install2.Namespace = releaseNamespace
			install2.DryRun = false
			install2.ClientOnly = false
			install2.PostRenderer = postRenderer
			install2.CreateNamespace = true
			install2.Labels = releaseLabels
			klog.Infof("Helm provider: Retrying install for release %s after cleanup", releaseName)
			rel, err = install2.RunWithContext(ctx, ch, values)
			if err != nil {
				return "", "", 0, errors.Wrapf(err, "failed to install helm release %s after cleanup retry", releaseName)
			}
		} else {
			return "", "", 0, errors.Wrapf(err, "failed to install helm release %s", releaseName)
		}
	}
	klog.Infof("Helm provider: Successfully installed release %s", releaseName)
	p.releaseFingerprints[releaseName] = fingerprint
	p.releaseManifests[releaseName] = rel.Manifest
	p.releaseVersions[releaseName] = rel.Version
	return rel.Manifest, rel.Info.Notes, rel.Version, nil
}

// validateReleaseHealth checks that the Helm release secret exists and is
// readable in the cluster. If the release is missing or corrupted, the
// in-memory cache is invalidated so the next reconciliation performs a fresh
// install/upgrade instead of returning stale cached data.
//
// This method is designed to be called asynchronously (in a goroutine) after
// a successful cache-hit reconciliation, so it does not block the main render
// path. It acquires the release mutex only when it needs to clear the cache.
func (p *Provider) validateReleaseHealth(releaseName, releaseNamespace string) {
	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		klog.Warningf("Helm provider health check: failed to get action config for release %s: %v", releaseName, err)
		return
	}

	getAction := action.NewGet(actionConfig)
	rel, err := getAction.Run(releaseName)
	if err != nil {
		// Release not found or unreadable (corrupted secret) — invalidate cache
		klog.Warningf("Helm provider health check: release %s not found or unreadable in cluster, invalidating cache: %v", releaseName, err)
		p.releaseMu.Lock()
		delete(p.releaseFingerprints, releaseName)
		delete(p.releaseManifests, releaseName)
		delete(p.releaseVersions, releaseName)
		p.releaseMu.Unlock()
		return
	}

	// Release exists — verify it's in a healthy deployed state
	if rel.Info == nil || rel.Info.Status != release.StatusDeployed {
		status := "unknown"
		if rel.Info != nil {
			status = string(rel.Info.Status)
		}
		klog.Warningf("Helm provider health check: release %s is in state %q (expected deployed), invalidating cache", releaseName, status)
		p.releaseMu.Lock()
		delete(p.releaseFingerprints, releaseName)
		delete(p.releaseManifests, releaseName)
		delete(p.releaseVersions, releaseName)
		p.releaseMu.Unlock()
		return
	}

	klog.V(4).Infof("Helm provider health check: release %s is healthy (deployed, revision %d)", releaseName, rel.Version)
}

// cleanOrphanedReleaseSecrets removes corrupted or orphaned Helm release
// secrets for a release. This is called when helm install fails because it
// finds existing secrets it cannot parse or adopt.
//
// Strategy: always delete the secrets directly via the Kubernetes API first,
// since corrupted secrets cannot be reliably handled by Helm's own storage
// driver or uninstall action.
func (p *Provider) cleanOrphanedReleaseSecrets(actionConfig *action.Configuration, releaseName, releaseNamespace string) error {
	// Primary approach: delete secrets directly via Kubernetes API.
	// This is the most reliable method for corrupted secrets.
	klog.Infof("Helm provider: Cleaning up release secrets for %s in namespace %s via direct deletion", releaseName, releaseNamespace)
	if err := p.deleteReleaseSecretsDirect(releaseNamespace, releaseName); err != nil {
		return fmt.Errorf("failed to clean release secrets for %s: %v", releaseName, err)
	}
	return nil
}

// listReleaseSecretNames returns the names of all Helm release secrets for the
// given release. Used to track all revision secrets in the ResourceTracker so
// GC cleans them all up on Application deletion.
func (p *Provider) listReleaseSecretNames(namespace, releaseName string) []string {
	restClient := p.helmClient.RESTClientGetter()
	if restClient == nil {
		return nil
	}
	cfg, err := restClient.ToRESTConfig()
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to get REST config for listing secrets: %v", err)
		return nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to create clientset for listing secrets: %v", err)
		return nil
	}

	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to list release secrets for %s: %v", releaseName, err)
		return nil
	}

	names := make([]string, 0, len(secretList.Items))
	for _, s := range secretList.Items {
		names = append(names, s.Name)
	}
	return names
}

// deleteReleaseSecretsDirect uses the Kubernetes API directly to delete Helm
// release secrets. This is the last-resort cleanup for secrets that are too
// corrupted for Helm's own storage driver or uninstall action to handle.
func (p *Provider) deleteReleaseSecretsDirect(namespace, releaseName string) error {
	restClient := p.helmClient.RESTClientGetter()
	if restClient == nil {
		return fmt.Errorf("no REST client available")
	}
	cfg, err := restClient.ToRESTConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get REST config")
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	// List secrets with Helm's labels for this release
	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to list helm secrets for release %s", releaseName)
	}

	for _, secret := range secretList.Items {
		klog.Infof("Helm provider: Directly deleting corrupted release secret %s/%s", namespace, secret.Name)
		if err := clientset.CoreV1().Secrets(namespace).Delete(context.Background(), secret.Name, metav1.DeleteOptions{}); err != nil {
			klog.Warningf("Helm provider: Failed to delete secret %s/%s: %v", namespace, secret.Name, err)
		}
	}

	klog.Infof("Helm provider: Deleted %d orphaned release secrets for %s in namespace %s", len(secretList.Items), releaseName, namespace)
	return nil
}

// InvalidateRelease clears the in-memory cache for a specific release. This
// can be called by external components (e.g., ResourceTracker GC) when they
// detect that a Helm release secret has been deleted or is missing.
func (p *Provider) InvalidateRelease(releaseName string) {
	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()
	delete(p.releaseFingerprints, releaseName)
	delete(p.releaseManifests, releaseName)
	delete(p.releaseVersions, releaseName)
	klog.Infof("Helm provider: Invalidated cache for release %s", releaseName)
}

// parseManifestResources parses a Helm release manifest string into a slice of
// resource maps, skipping test hooks when requested and ordering CRDs first.
func (p *Provider) parseManifestResources(manifestStr string, options *RenderOptionsParams) ([]map[string]interface{}, error) {
	skipTests := true
	if options != nil && options.SkipTests != nil {
		skipTests = *options.SkipTests
	}

	resources := []map[string]interface{}{}
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestStr), 4096)

	for {
		resource := &unstructured.Unstructured{}
		if err := decoder.Decode(&resource); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err, "failed to decode manifest")
		}

		// Skip empty resources
		if resource == nil || resource.Object == nil || len(resource.Object) == 0 {
			continue
		}

		// Skip test resources if requested
		if skipTests && isTestResource(resource) {
			continue
		}

		cleanedResource := cleanResource(resource.Object)
		resources = append(resources, cleanedResource)
	}

	// Order resources: CRDs first, then namespaces, then other resources
	return orderResources(resources), nil
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

// Render is the main provider function: it performs a real Helm install/upgrade
// against the cluster and returns the deployed resources for KubeVela to adopt.
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
	ch, err := p.fetchChart(ctx, &renderParams.Chart, renderParams.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch chart")
	}
	klog.V(2).Infof("Helm provider: Successfully fetched chart %s", ch.Name())

	// Merge values from all sources
	values, err := p.mergeValues(ctx, renderParams.Values, renderParams.ValuesFrom)
	if err != nil {
		return nil, errors.Wrap(err, "failed to merge values")
	}
	klog.V(3).Infof("Helm provider: Merged values: %v", values)

	// Install or upgrade the chart via the Helm SDK
	manifest, notes, _, err := p.installOrUpgradeChart(ctx, ch, releaseName, releaseNamespace, values, renderParams.Options, renderParams.Context)
	if err != nil {
		return nil, errors.Wrap(err, "failed to install/upgrade chart")
	}

	// Parse the release manifest into KubeVela resource maps
	resources, err := p.parseManifestResources(manifest, renderParams.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse release manifest")
	}

	// Include ALL Helm release Secrets as tracked resources so KubeVela's
	// ResourceTracker records them and GC deletes them when the Application
	// is deleted. We query the cluster for every secret belonging to this
	// release (v1, v2, v3, …) so that:
	// - On Application deletion: all secrets are cleaned up, no orphans
	// - During upgrades: all existing secrets remain tracked, preventing
	//   accidental GC. Helm's own maxHistory handles old revision pruning.
	if renderParams.Context != nil {
		releaseSecretNames := p.listReleaseSecretNames(releaseNamespace, releaseName)
		for _, secName := range releaseSecretNames {
			releaseSecret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      secName,
					"namespace": releaseNamespace,
					"annotations": map[string]interface{}{
						oam.AnnotationTrackOnly: "true",
					},
				},
				"type": "helm.sh/release.v1",
			}
			resources = append(resources, releaseSecret)
		}
		if len(releaseSecretNames) > 0 {
			klog.V(3).Infof("Helm provider: Tracking %d release secrets for %s", len(releaseSecretNames), releaseName)
		}
	}

	klog.Infof("Helm provider: Deployed %d resources for chart %s", len(resources), renderParams.Chart.Source)

	// Log resource summary for debugging
	if len(resources) > 0 {
		if kind, found, _ := unstructured.NestedString(resources[0], "kind"); found {
			if name, found, _ := unstructured.NestedString(resources[0], "metadata", "name"); found {
				klog.Infof("Helm provider: First resource is %s/%s", kind, name)
			}
		}

		if jsonBytes, err := json.MarshalIndent(resources[0], "", "  "); err == nil {
			klog.V(4).Infof("Helm provider: First resource JSON:\n%s", string(jsonBytes))
		}

		klog.V(3).Infof("Helm provider: All resources summary:")
		for i, res := range resources {
			if kind, found, _ := unstructured.NestedString(res, "kind"); found {
				if name, found, _ := unstructured.NestedString(res, "metadata", "name"); found {
					klog.V(3).Infof("  [%d] %s/%s", i, kind, name)
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

// UninstallParams are the parameters for uninstalling a Helm release
type UninstallParams struct {
	Release     ReleaseParams `json:"release"`
	KeepHistory bool          `json:"keepHistory,omitempty"`
}

// UninstallReturns are the return values from uninstalling a Helm release
type UninstallReturns struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// Uninstall runs `helm uninstall` for the named release and clears the provider's
// in-memory fingerprint cache so a subsequent Render triggers a fresh install.
func Uninstall(ctx context.Context, params *providers.Params[UninstallParams]) (*providers.Returns[UninstallReturns], error) {
	p := NewProvider()
	up := params.Params

	releaseName := up.Release.Name
	releaseNamespace := up.Release.Namespace

	klog.Infof("Helm provider: Uninstalling release %s in namespace %s", releaseName, releaseNamespace)

	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize helm action config for uninstall")
	}

	uninstallAction := action.NewUninstall(actionConfig)
	uninstallAction.KeepHistory = up.KeepHistory

	_, err = uninstallAction.Run(releaseName)
	if err != nil {
		// Treat "not found" as a success — the release is already gone
		if strings.Contains(err.Error(), "not found") {
			klog.Infof("Helm provider: Release %s not found, treating as already uninstalled", releaseName)
		} else {
			return &providers.Returns[UninstallReturns]{
				Returns: UninstallReturns{Success: false, Message: err.Error()},
			}, err
		}
	} else {
		klog.Infof("Helm provider: Successfully uninstalled release %s", releaseName)
	}

	// Clear in-memory state so the next Render performs a fresh install
	p.releaseMu.Lock()
	delete(p.releaseFingerprints, releaseName)
	delete(p.releaseManifests, releaseName)
	delete(p.releaseVersions, releaseName)
	p.releaseMu.Unlock()

	return &providers.Returns[UninstallReturns]{
		Returns: UninstallReturns{Success: true},
	}, nil
}

// ProviderName is the name of this provider
const ProviderName = "helm"

//go:embed helm.cue
var template string

// Template exports the CUE template for use by workflow providers
var Template = template

// Package exports the provider package for registration
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render":    cuexruntime.GenericProviderFn[providers.Params[RenderParams], providers.Returns[RenderReturns]](Render),
	"uninstall": cuexruntime.GenericProviderFn[providers.Params[UninstallParams], providers.Returns[UninstallReturns]](Uninstall),
}))
