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
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
func (p *Provider) fetchChart(ctx context.Context, params *ChartSourceParams, options *RenderOptionsParams, releaseNamespace string) (*chart.Chart, error) {
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
		return p.fetchChartWithoutCache(ctx, params, sourceType, releaseNamespace)
	}

	// Check if we have a cached chart
	if cached := p.cache.Get(cacheKey); cached != nil {
		if ch, ok := cached.(*chart.Chart); ok {
			klog.V(3).Infof("Using cached chart with key: %s", cacheKey)
			return ch, nil
		}
	}

	klog.V(4).Infof("Cache miss for key: %s, fetching chart", cacheKey)

	ch, err := p.fetchChartWithoutCache(ctx, params, sourceType, releaseNamespace)
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
func (p *Provider) fetchChartWithoutCache(ctx context.Context, params *ChartSourceParams, sourceType, releaseNamespace string) (*chart.Chart, error) {
	switch sourceType {
	case "oci":
		return p.fetchOCIChart(ctx, params, releaseNamespace)
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

// createTempCredentialsFile writes a Docker-style config.json with basic-auth
// credentials for the given registry host and returns the file path along with
// a cleanup function that removes the temp directory. Using a per-call temp file
// (via ClientOptCredentialsFile) avoids writing to the shared Helm credentials
// file and avoids the live network round-trip that registry.Client.Login() makes.
func createTempCredentialsFile(host, username, password string) (string, func(), error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	config := map[string]interface{}{
		"auths": map[string]interface{}{
			host: map[string]string{"auth": encoded},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		return "", func() {}, errors.Wrap(err, "failed to marshal OCI credentials")
	}
	dir, err := os.MkdirTemp("", "kubevela-helm-oci-*")
	if err != nil {
		return "", func() {}, errors.Wrap(err, "failed to create temp dir for OCI credentials")
	}
	cleanup := func() { os.RemoveAll(dir) }
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		cleanup()
		return "", func() {}, errors.Wrap(err, "failed to write OCI credentials file")
	}
	return path, cleanup, nil
}

// fetchOCIChart fetches a chart from an OCI registry.
// If params.Auth is set, credentials are resolved from the named Kubernetes Secret
// and injected into the registry client via a per-call temp credentials file
// (ClientOptCredentialsFile). This avoids writing to the shared Helm credentials
// file and avoids the live network round-trip that registry.Client.Login() makes.
// This is a package-level function (not a Provider method) because it uses no
// Provider state — only the cluster client via singleton.KubeClient.
func (p *Provider) fetchOCIChart(ctx context.Context, params *ChartSourceParams, releaseNamespace string) (*chart.Chart, error) {
	var clientOpts []registry.ClientOption

	// Extract host before appending the version tag so the split is clean even
	// for bare registries (e.g. "myregistry:5000" with no repository path).
	ref := strings.TrimPrefix(params.Source, "oci://")
	host := strings.SplitN(ref, "/", 2)[0]

	if params.Version != "" {
		ref = fmt.Sprintf("%s:%s", ref, params.Version)
	}

	if params.Auth != nil && params.Auth.SecretRef != nil {
		username, password, err := resolveOCICredentials(ctx, params.Auth, releaseNamespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve OCI registry credentials")
		}
		credFile, cleanup, err := createTempCredentialsFile(host, username, password)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create OCI credentials file")
		}
		defer cleanup()
		clientOpts = append(clientOpts, registry.ClientOptCredentialsFile(credFile))
	}

	registryClient, err := registry.NewClient(clientOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OCI registry client")
	}

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

	// Sort entries so that Get() returns the highest matching version
	index.SortEntries()

	// Find the requested chart version. Get() supports exact versions (e.g., "1.2.3"),
	// semver constraints (e.g., "^1.2.0", ">=1.0.0 <2.0.0"), and empty string (latest stable).
	chartVersion, err := index.Get(params.Source, params.Version)
	if err != nil {
		return nil, fmt.Errorf("version %q of chart %s not found in repository %s: %w", params.Version, params.Source, params.RepoURL, err)
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

// defaultValuesKey is the key looked up in a ConfigMap/Secret when the user
// does not specify one explicitly. Matches the FluxCD and Helm CLI convention.
const defaultValuesKey = "values.yaml"

// valueSourceMissingError is returned by loaders when a ConfigMap/Secret or the
// requested key inside it does not exist. mergeValues uses this sentinel type to
// decide whether source.Optional allows the source to be skipped. Parse errors
// and other failures produce different error types, so Optional never swallows
// them — a common source of silent misconfiguration bugs.
type valueSourceMissingError struct {
	kind, name, namespace, key string
	cause                      error
}

func (e *valueSourceMissingError) Error() string {
	if e.key != "" {
		return fmt.Sprintf("%s %s/%s key %q not found: %v", e.kind, e.namespace, e.name, e.key, e.cause)
	}
	return fmt.Sprintf("%s %s/%s not found: %v", e.kind, e.namespace, e.name, e.cause)
}

func (e *valueSourceMissingError) Unwrap() error { return e.cause }

func isValueSourceMissing(err error) bool {
	var target *valueSourceMissingError
	return stderrors.As(err, &target)
}

// errCrossNamespaceValuesFrom is returned when a valuesFrom source references a
// namespace other than the Application's own namespace. The controller has
// cluster-scoped read on ConfigMaps/Secrets, so without this guard a tenant could
// read Secrets from any namespace by submitting a crafted Application.
var errCrossNamespaceValuesFrom = stderrors.New("cross-namespace valuesFrom sources are not permitted")

// mergeValues merges inline `values` and any `valuesFrom` sources into a single
// map. Priority (highest wins): inline > valuesFrom[N] > valuesFrom[N-1] > ... >
// valuesFrom[0]. Later entries override earlier ones. The merge is a deep-merge
// of map keys via chartutil.CoalesceTables; arrays are replaced wholesale (not
// concatenated), and `null` values are preserved (not treated as delete), so
// semantics diverge slightly from `helm CLI --values a.yaml --values b.yaml`
// which uses chartutil.CoalesceValues.
//
// A valuesFrom entry that omits `namespace` resolves to releaseNamespace (the
// natural co-location with the chart's deployed resources). An entry that sets
// an explicit Namespace is only allowed if it matches either releaseNamespace
// or appNamespace; any other namespace is rejected to block cross-tenant reads
// via the controller's cluster-wide RBAC.
func (p *Provider) mergeValues(ctx context.Context, baseValues interface{}, valuesFrom []ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	accumulated := map[string]interface{}{}

	for _, source := range valuesFrom {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		values, err := p.loadValuesFromSource(ctx, source, appNamespace, releaseNamespace)
		if err != nil {
			if source.Optional && isValueSourceMissing(err) {
				klog.V(2).Infof("Helm provider: skipping optional values source %s %q: %v", source.Kind, source.Name, err)
				continue
			}
			return nil, errors.Wrapf(err, "failed to load values from %s %q", source.Kind, source.Name)
		}
		// CoalesceTables(dst, src) treats dst as authoritative. `values` is the
		// newer source, so it's passed as dst to override `accumulated` (older).
		accumulated = chartutil.CoalesceTables(values, accumulated)
	}

	// Inline values override everything from valuesFrom. Clone before merging
	// because CoalesceTables mutates dst in place, and dst here is the caller's
	// map (renderParams.Values).
	if inline, ok := baseValues.(map[string]interface{}); ok {
		clone := make(map[string]interface{}, len(inline))
		for k, v := range inline {
			clone[k] = v
		}
		accumulated = chartutil.CoalesceTables(clone, accumulated)
	}

	return accumulated, nil
}

// loadValuesFromSource dispatches to the appropriate loader based on source.Kind.
func (p *Provider) loadValuesFromSource(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	switch source.Kind {
	case "ConfigMap":
		return p.loadConfigMapValues(ctx, source, appNamespace, releaseNamespace)
	case "Secret":
		return p.loadSecretValues(ctx, source, appNamespace, releaseNamespace)
	default:
		return nil, fmt.Errorf("unsupported values source kind: %s", source.Kind)
	}
}

// resolveValuesFromNamespace returns the effective namespace for a valuesFrom
// entry. The default (empty) resolves to releaseNamespace — the natural place
// to co-locate chart values with the chart's resources. An explicit namespace
// is accepted only if it matches releaseNamespace or appNamespace; any other
// value is rejected so a tenant cannot coerce the controller's cluster-wide
// RBAC into reading Secrets from unrelated namespaces.
func resolveValuesFromNamespace(source ValuesFromParams, appNamespace, releaseNamespace string) (string, error) {
	if source.Namespace == "" {
		return releaseNamespace, nil
	}
	if source.Namespace == releaseNamespace || source.Namespace == appNamespace {
		return source.Namespace, nil
	}
	return "", fmt.Errorf("%w: %s %q requested namespace %q but Application is in %q and release is in %q",
		errCrossNamespaceValuesFrom, source.Kind, source.Name, source.Namespace, appNamespace, releaseNamespace)
}

// loadConfigMapValues reads a ConfigMap in the Application namespace and parses
// the requested key as YAML. When source.Key is empty it falls back to
// "values.yaml" (Helm/FluxCD convention). Not-found errors (missing ConfigMap
// or missing key) are returned as valueSourceMissingError so optional sources
// can skip them; parse errors are surfaced as-is and are never swallowed by
// optional.
//
// singleton.KubeClient.Get() here is the kubevela-pkg default client built via
// controller-runtime's client.New — this is an UNCACHED client that reads
// directly from the API server. Do NOT switch this to manager.GetClient() or
// a cached reader: that would register a cluster-wide ConfigMap/Secret
// informer on first use and load every CM/Secret cluster-wide into the
// controller's memory. Direct API reads per valuesFrom entry are the intended
// trade-off.
func (p *Provider) loadConfigMapValues(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	ns, err := resolveValuesFromNamespace(source, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}
	key := source.Key
	if key == "" {
		key = defaultValuesKey
	}

	k8s := singleton.KubeClient.Get()
	cm := &corev1.ConfigMap{}
	if err := k8s.Get(ctx, client.ObjectKey{Name: source.Name, Namespace: ns}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &valueSourceMissingError{kind: "ConfigMap", name: source.Name, namespace: ns, cause: err}
		}
		return nil, errors.Wrapf(err, "failed to read ConfigMap %s/%s", ns, source.Name)
	}

	raw, ok := cm.Data[key]
	if !ok {
		// If the key lives in binaryData (kubectl create cm --from-file of
		// non-UTF-8 content), reject explicitly. Helm values are textual; a
		// binary blob is unparseable. The clear message saves operators from
		// chasing a mismatch when `kubectl get cm` shows the key under
		// binaryData and the loader reports "not found".
		if _, isBinary := cm.BinaryData[key]; isBinary {
			return nil, errors.Errorf("ConfigMap %s/%s key %q is in binaryData; valuesFrom requires a textual YAML value in .data",
				ns, source.Name, key)
		}
		return nil, &valueSourceMissingError{
			kind: "ConfigMap", name: source.Name, namespace: ns, key: key,
			cause: fmt.Errorf("key not found in .data"),
		}
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
		return nil, errors.Wrapf(err, "ConfigMap %s/%s key %q: invalid YAML", ns, source.Name, key)
	}
	return values, nil
}

// loadSecretValues reads a Secret in the Application namespace and parses the
// requested key as YAML. Kubernetes already base64-decodes Secret.Data on read,
// so the bytes are consumed as-is. Error messages intentionally never include
// raw secret bytes.
func (p *Provider) loadSecretValues(ctx context.Context, source ValuesFromParams, appNamespace, releaseNamespace string) (map[string]interface{}, error) {
	ns, err := resolveValuesFromNamespace(source, appNamespace, releaseNamespace)
	if err != nil {
		return nil, err
	}
	key := source.Key
	if key == "" {
		key = defaultValuesKey
	}

	k8s := singleton.KubeClient.Get()
	secret := &corev1.Secret{}
	if err := k8s.Get(ctx, client.ObjectKey{Name: source.Name, Namespace: ns}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &valueSourceMissingError{kind: "Secret", name: source.Name, namespace: ns, cause: err}
		}
		return nil, errors.Wrapf(err, "failed to read Secret %s/%s", ns, source.Name)
	}

	raw, ok := secret.Data[key]
	if !ok {
		return nil, &valueSourceMissingError{
			kind: "Secret", name: source.Name, namespace: ns, key: key,
			cause: fmt.Errorf("key not found in .data"),
		}
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return nil, errors.Wrapf(err, "Secret %s/%s key %q: invalid YAML", ns, source.Name, key)
	}
	return values, nil
}

// resolveOCICredentials resolves OCI registry credentials from a Kubernetes Secret.
// Returns empty strings when auth is nil or has no SecretRef — callers should
// proceed unauthenticated in that case.
// When SecretRef.Namespace is empty, releaseNamespace is used as the default,
// consistent with how valuesFrom Secret sources resolve in this provider.
//
// The Secret must contain "username" and "password" keys in .Data.
// Credentials are returned as plain strings; Kubernetes already base64-decodes
// Secret.Data on read, so no further decoding is needed.
//
// This is a package-level function (not a Provider method) because it uses no
// Provider state — only the cluster client via singleton.KubeClient.
func resolveOCICredentials(ctx context.Context, authParams *AuthParams, releaseNamespace string) (username, password string, err error) {
	if authParams == nil || authParams.SecretRef == nil {
		return "", "", nil
	}

	// TODO(GWCP-98771): Implement a cross-namespace guard here (matching resolveValuesFromNamespace)
	// before enabling auth in production. fetchOCIChart is now wired into the system via Render().
	// Requires threading appNamespace through fetchChart -> fetchChartWithoutCache -> fetchOCIChart
	// -> resolveOCICredentials alongside releaseNamespace. Until then, an explicit
	// auth.secretRef.namespace can reference any namespace the controller can read.
	ns := authParams.SecretRef.Namespace
	if ns == "" {
		ns = releaseNamespace
	}

	k8s := singleton.KubeClient.Get()
	secret := &corev1.Secret{}
	if getErr := k8s.Get(ctx, client.ObjectKey{Name: authParams.SecretRef.Name, Namespace: ns}, secret); getErr != nil {
		if apierrors.IsNotFound(getErr) {
			return "", "", fmt.Errorf("auth secret %s/%s not found: %w", ns, authParams.SecretRef.Name, getErr)
		}
		return "", "", errors.Wrapf(getErr, "failed to read auth secret %s/%s", ns, authParams.SecretRef.Name)
	}

	usernameBytes, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("auth secret %s/%s missing required key %q", ns, authParams.SecretRef.Name, "username")
	}

	passwordBytes, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("auth secret %s/%s missing required key %q", ns, authParams.SecretRef.Name, "password")
	}

	return string(usernameBytes), string(passwordBytes), nil
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
			if stderrors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrap(err, "post-renderer: failed to decode manifest")
		}

		if len(obj.Object) == 0 {
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
	labels := map[string]string{
		"app.oam.dev/name":      velaCtx.AppName,
		"app.oam.dev/namespace": velaCtx.AppNamespace,
		"app.oam.dev/component": velaCtx.Name,
	}
	// Embed the publishVersion pin in the release labels so subsequent
	// reconciles can short-circuit when the App is at a stable pin and the
	// release was already installed at that pin.
	if velaCtx.PublishVersion != "" {
		labels["app.oam.dev/publishVersion"] = velaCtx.PublishVersion
	}
	return labels
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
//
// Empty-values inputs are normalised to an empty map before hashing. Helm
// stores release.Config as nil when no values were supplied, but mergeValues
// returns map[string]interface{}{} for the same logical input — without this
// guard the two would hash to sha256("null") and sha256("{}") respectively,
// causing the dedup check below to mis-fire and trigger spurious helm upgrades
// on every reconcile for any release that was installed with empty/optional
// values.
func computeReleaseFingerprint(ch *chart.Chart, values map[string]interface{}) string {
	version := ""
	if ch != nil && ch.Metadata != nil {
		version = ch.Metadata.Version
	}
	if values == nil {
		values = map[string]interface{}{}
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
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)

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
		if cached, ok := p.releaseFingerprints[cacheKey]; ok {
			klog.Infof("Helm provider [%s]: Release %s not found in cluster but cached (fingerprint=%s), clearing stale cache", velaContextStr(velaCtx), releaseName, cached[:16])
			delete(p.releaseFingerprints, cacheKey)
			delete(p.releaseManifests, cacheKey)
			delete(p.releaseVersions, cacheKey)
		}
	}

	if getErr == nil && existingRelease != nil {
		// Check if this release was installed by KubeVela (has our ownership labels
		// on the release Secret). If not, it's an external release that we need to
		// adopt by forcing an upgrade — even if the fingerprint matches — so the
		// post-renderer injects KubeVela ownership labels onto every resource.
		needsAdoption := velaCtx != nil && !isOwnedByVela(existingRelease, velaCtx)
		if needsAdoption {
			klog.Infof("Helm provider [%s]: Release %s exists but was not installed by KubeVela (missing ownership labels), forcing upgrade to adopt", velaContextStr(velaCtx), releaseName)
			// Label all existing release secrets with KubeVela ownership so they
			// can be tracked by the ResourceTracker and cleaned up on App deletion.
			p.labelReleaseSecrets(releaseNamespace, releaseName, velaCtx)
		}

		// publishVersion pin short-circuit: when the App is at a stable
		// publishVersion pin AND the deployed release was installed at the
		// same pin AND the chart version is unchanged, return the deployed
		// manifest unchanged regardless of any apparent values drift.
		//
		// Without this, a render path that bypasses the workflow gate
		// (state-keep / drift detection / post-dispatch traits / periodic
		// CUE evaluation) re-merges valuesFrom sources and the cluster-side
		// fingerprint compare below would mis-fire whenever a referenced
		// CM/Secret was edited. The user's explicit pin is the contract:
		// nothing changes until they bump the pin.
		//
		// Initial install has no existingRelease so this branch is skipped,
		// and the initial mergeValues runs normally — picking up the
		// referenced CM/Secret content and stamping it into the release.
		if !needsAdoption && velaCtx != nil && velaCtx.PublishVersion != "" &&
			existingRelease.Info != nil && existingRelease.Info.Status == release.StatusDeployed &&
			existingRelease.Chart != nil && existingRelease.Chart.Metadata != nil &&
			existingRelease.Chart.Metadata.Version == ch.Metadata.Version &&
			existingRelease.Labels["app.oam.dev/publishVersion"] == velaCtx.PublishVersion {
			klog.V(2).Infof("Helm provider [%s]: Release %s held by publishVersion pin %q, skipping upgrade",
				velaContextStr(velaCtx), releaseName, velaCtx.PublishVersion)
			p.releaseFingerprints[cacheKey] = fingerprint
			p.releaseManifests[cacheKey] = existingRelease.Manifest
			p.releaseVersions[cacheKey] = existingRelease.Version
			return existingRelease.Manifest, existingRelease.Info.Notes, existingRelease.Version, nil
		}

		// Release exists — check if it is already deployed with the same fingerprint
		if !needsAdoption && existingRelease.Info.Status == release.StatusDeployed {
			clusterFingerprint := computeReleaseFingerprint(existingRelease.Chart, existingRelease.Config)
			if clusterFingerprint == fingerprint {
				klog.V(3).Infof("Helm provider [%s]: Release %s already deployed and unchanged (cluster fingerprint match), skipping upgrade", velaContextStr(velaCtx), releaseName)
				p.releaseFingerprints[cacheKey] = fingerprint
				p.releaseManifests[cacheKey] = existingRelease.Manifest
				p.releaseVersions[cacheKey] = existingRelease.Version
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

		klog.Infof("Helm provider [%s]: Upgrading release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
		rel, err := upgrade.RunWithContext(ctx, releaseName, ch, values)
		if err != nil {
			return "", "", 0, errors.Wrapf(err, "failed to upgrade helm release %s", releaseName)
		}
		klog.Infof("Helm provider [%s]: Successfully upgraded release %s", velaContextStr(velaCtx), releaseName)
		p.releaseFingerprints[cacheKey] = fingerprint
		p.releaseManifests[cacheKey] = rel.Manifest
		p.releaseVersions[cacheKey] = rel.Version
		return rel.Manifest, rel.Info.Notes, rel.Version, nil
	}

	// No existing release — perform a fresh install
	rel, err := p.freshInstall(ctx, actionConfig, ch, releaseName, releaseNamespace, values, options, postRenderer, releaseLabels, velaCtx)
	if err != nil {
		return "", "", 0, err
	}
	klog.Infof("Helm provider [%s]: Successfully installed release %s", velaContextStr(velaCtx), releaseName)
	p.releaseFingerprints[cacheKey] = fingerprint
	p.releaseManifests[cacheKey] = rel.Manifest
	p.releaseVersions[cacheKey] = rel.Version
	return rel.Manifest, rel.Info.Notes, rel.Version, nil
}

// freshInstall performs a helm install with retry logic for orphaned/corrupted state.
func (p *Provider) freshInstall(ctx context.Context, actionConfig *action.Configuration, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string, velaCtx *ContextParams) (*release.Release, error) {
	install := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)

	klog.Infof("Helm provider [%s]: Installing release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
	rel, err := install.RunWithContext(ctx, ch, values)
	if err == nil {
		return rel, nil
	}

	// If install fails due to corrupted/orphaned release secrets or ownership
	// conflicts, clean up the broken state and retry once.
	if !isRetryableInstallError(err) {
		return nil, errors.Wrapf(err, "failed to install helm release %s", releaseName)
	}

	klog.Warningf("Helm provider [%s]: Install failed for %s due to orphaned state (%v), cleaning up and retrying", velaContextStr(velaCtx), releaseName, err)
	if cleanErr := p.cleanOrphanedReleaseSecrets(actionConfig, releaseName, releaseNamespace, velaCtx); cleanErr != nil {
		klog.Warningf("Helm provider [%s]: Failed to clean orphaned secrets for %s: %v", velaContextStr(velaCtx), releaseName, cleanErr)
		return nil, errors.Wrapf(err, "failed to install helm release %s (cleanup also failed: %v)", releaseName, cleanErr)
	}

	retry := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)
	klog.Infof("Helm provider [%s]: Retrying install for release %s after cleanup", velaContextStr(velaCtx), releaseName)
	rel, err = retry.RunWithContext(ctx, ch, values)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install helm release %s after cleanup retry", releaseName)
	}
	return rel, nil
}

// newInstallAction creates a configured helm install action.
func (p *Provider) newInstallAction(actionConfig *action.Configuration, releaseName, releaseNamespace string, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string) *action.Install {
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
	return install
}

// isRetryableInstallError returns true if the error indicates orphaned state
// that can be fixed by cleaning up and retrying.
func isRetryableInstallError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "cannot be imported") ||
		strings.Contains(msg, "invalid ownership metadata") ||
		strings.Contains(msg, "no revision for release") ||
		strings.Contains(msg, "release: already exists")
}

// dryRunRender performs a client-only Helm template render without touching the
// cluster. Used during webhook validation to verify the chart can be fetched,
// values are valid, and templates render without errors — without blocking on
// real resource creation, hooks, or waiting.
func (p *Provider) dryRunRender(ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, error) {
	install := action.NewInstall(&action.Configuration{})
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = true
	install.ClientOnly = true

	// Set Kubernetes version capabilities so charts with kubeVersion constraints
	// don't fail against Helm's default v1.20.0. We query the real cluster version
	// via the REST config. If unreachable, the kubeVersion check is skipped —
	// the real install during reconciliation will validate it.
	if kv := p.getKubeVersion(); kv != nil {
		install.KubeVersion = kv
	}

	install.PostRenderer = &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}

	if options != nil {
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}

	rel, err := install.Run(ch, values)
	if err != nil {
		return "", "", errors.Wrapf(err, "dry-run render failed for chart %s", ch.Name())
	}

	return rel.Manifest, rel.Info.Notes, nil
}

// getKubeVersion queries the cluster's Kubernetes version for use in dry-run
// rendering. Returns nil if the cluster is unreachable — Helm will then skip
// the kubeVersion constraint check, deferring it to the real install.
func (p *Provider) getKubeVersion() *chartutil.KubeVersion {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil
	}
	return &chartutil.KubeVersion{
		Version: fmt.Sprintf("v%s.%s", info.Major, info.Minor),
		Major:   info.Major,
		Minor:   info.Minor,
	}
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
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
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
		delete(p.releaseFingerprints, cacheKey)
		delete(p.releaseManifests, cacheKey)
		delete(p.releaseVersions, cacheKey)
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
		delete(p.releaseFingerprints, cacheKey)
		delete(p.releaseManifests, cacheKey)
		delete(p.releaseVersions, cacheKey)
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
func (p *Provider) cleanOrphanedReleaseSecrets(_ *action.Configuration, releaseName, releaseNamespace string, velaCtx *ContextParams) error {
	// Primary approach: delete secrets directly via Kubernetes API.
	// This is the most reliable method for corrupted secrets.
	klog.Infof("Helm provider [%s]: Cleaning up release secrets for %s in namespace %s via direct deletion", velaContextStr(velaCtx), releaseName, releaseNamespace)
	if err := p.deleteReleaseSecretsDirect(releaseNamespace, releaseName, velaCtx); err != nil {
		return fmt.Errorf("failed to clean release secrets for %s: %w", releaseName, err)
	}
	return nil
}

// listReleaseSecretNames returns the names of all Helm release secrets for the
// given release. Used to track all revision secrets in the ResourceTracker so
// GC cleans them all up on Application deletion.
func (p *Provider) listReleaseSecretNames(namespace, releaseName string) []string {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
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
		// Only include secrets that have KubeVela ownership labels.
		// Secrets from vanilla helm installs (before KubeVela adoption) won't
		// have these labels, and including them would fail the MustBeControlledByApp
		// check during pre-dispatch dryrun.
		if s.Labels["app.oam.dev/name"] != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

// labelReleaseSecrets adds KubeVela ownership labels to all existing Helm
// release secrets that don't already have them. Called during adoption of
// external releases so that listReleaseSecretNames picks them up for GC tracking.
func (p *Provider) labelReleaseSecrets(namespace, releaseName string, velaCtx *ContextParams) {
	if velaCtx == nil {
		return
	}
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return
	}

	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		return
	}

	for _, s := range secretList.Items {
		if s.Labels["app.oam.dev/name"] != "" {
			continue // already labeled
		}
		patch := fmt.Sprintf(`{"metadata":{"labels":{"app.oam.dev/name":%q,"app.oam.dev/namespace":%q,"app.oam.dev/component":%q}}}`,
			velaCtx.AppName, velaCtx.AppNamespace, velaCtx.Name)
		_, patchErr := clientset.CoreV1().Secrets(namespace).Patch(
			context.Background(), s.Name, "application/strategic-merge-patch+json",
			[]byte(patch), metav1.PatchOptions{},
		)
		if patchErr != nil {
			klog.Warningf("Helm provider [%s]: Failed to label release secret %s/%s: %v", velaContextStr(velaCtx), namespace, s.Name, patchErr)
		} else {
			klog.Infof("Helm provider [%s]: Labeled release secret %s/%s for adoption", velaContextStr(velaCtx), namespace, s.Name)
		}
	}
}

// deleteReleaseSecretsDirect uses the Kubernetes API directly to delete Helm
// release secrets. This is the last-resort cleanup for secrets that are too
// corrupted for Helm's own storage driver or uninstall action to handle.
func (p *Provider) deleteReleaseSecretsDirect(namespace, releaseName string, velaCtx *ContextParams) error {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
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
		klog.Infof("Helm provider [%s]: Directly deleting corrupted release secret %s/%s", velaContextStr(velaCtx), namespace, secret.Name)
		if err := clientset.CoreV1().Secrets(namespace).Delete(context.Background(), secret.Name, metav1.DeleteOptions{}); err != nil {
			klog.Warningf("Helm provider: Failed to delete secret %s/%s: %v", namespace, secret.Name, err)
		}
	}

	klog.Infof("Helm provider [%s]: Deleted %d orphaned release secrets for %s in namespace %s", velaContextStr(velaCtx), len(secretList.Items), releaseName, namespace)
	return nil
}

// InvalidateRelease clears the in-memory cache for a specific release. This
// can be called by external components (e.g., ResourceTracker GC) when they
// detect that a Helm release secret has been deleted or is missing.
func (p *Provider) InvalidateRelease(releaseName, releaseNamespace string) {
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()
	delete(p.releaseFingerprints, cacheKey)
	delete(p.releaseManifests, cacheKey)
	delete(p.releaseVersions, cacheKey)
	klog.Infof("Helm provider: Invalidated cache for release %s/%s", releaseNamespace, releaseName)
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
			if stderrors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrap(err, "failed to decode manifest")
		}

		// Skip empty resources
		if resource == nil || len(resource.Object) == 0 {
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

	klog.V(2).Infof("Helm provider [%s]: Starting render for chart %s from %s", velaContextStr(renderParams.Context), renderParams.Chart.Source, renderParams.Chart.RepoURL)

	// Application namespace is the tenant boundary. When the Application has no
	// explicit context, fall back to the release namespace below so the same
	// Application can be rendered outside a ComponentDefinition path.
	appNamespace := ""
	if renderParams.Context != nil {
		appNamespace = renderParams.Context.AppNamespace
	}

	releaseName := "release"
	releaseNamespace := appNamespace
	if renderParams.Release != nil {
		if renderParams.Release.Name != "" {
			releaseName = renderParams.Release.Name
		}
		if renderParams.Release.Namespace != "" {
			releaseNamespace = renderParams.Release.Namespace
		}
	}
	// Guarantee a non-empty release namespace. Under the normal KubeVela
	// code path the controller always sets Context.AppNamespace before
	// calling Render, but callers that invoke the provider directly (tests,
	// CLI tooling) may leave both context and Release.Namespace empty.
	// Falling back to "default" preserves the pre-refactor behavior and
	// keeps Helm's namespace resolution from depending on the caller's
	// kubeconfig default.
	if releaseNamespace == "" {
		releaseNamespace = "default"
	}
	if appNamespace == "" {
		appNamespace = releaseNamespace
	}

	// Resolve the App's publishVersion annotation, if any. We pass it through
	// ContextParams.PublishVersion so installOrUpgradeChart can short-circuit
	// when the deployed release is already at the current pin and so
	// velaOwnerLabels can stamp the pin onto the release at install time.
	// Skipped in dry-run: admission validation must not depend on cluster
	// state, and the user-visible behaviour (CUE shape OK / not OK) is
	// independent of the pin.
	//
	// IsNotFound is treated as "App is being deleted" and falls through with
	// an empty pin — the subsequent uninstall path handles cleanup. Any other
	// error (RBAC change, transient API failure, network blip) is surfaced
	// rather than silently swallowed: a swallowed error would leave the pin
	// empty for this reconcile and bypass the pin short-circuit downstream,
	// allowing an unintended helm upgrade to fire even though the user's
	// publishVersion annotation is still in place.
	if !isDryRun(ctx) && renderParams.Context != nil && renderParams.Context.AppName != "" && appNamespace != "" {
		var app v1beta1.Application
		switch getErr := singleton.KubeClient.Get().Get(ctx, client.ObjectKey{Name: renderParams.Context.AppName, Namespace: appNamespace}, &app); {
		case getErr == nil:
			if pin := app.GetAnnotations()[oam.AnnotationPublishVersion]; pin != "" {
				renderParams.Context.PublishVersion = pin
			}
		case apierrors.IsNotFound(getErr):
			// App is gone (deletion in flight). Proceed without a pin.
		default:
			return nil, errors.Wrapf(getErr,
				"failed to read Application %s/%s for publishVersion lookup; refusing to proceed without pin context",
				appNamespace, renderParams.Context.AppName)
		}
	}

	klog.V(3).Infof("Helm provider: Release name=%s, namespace=%s", releaseName, releaseNamespace)

	// Fetch the chart
	ch, err := p.fetchChart(ctx, &renderParams.Chart, renderParams.Options, releaseNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch chart")
	}
	klog.V(2).Infof("Helm provider: Successfully fetched chart %s", ch.Name())

	// Skip valuesFrom resolution in dry-run (webhook admission): the webhook
	// validates CUE shape and renders the chart, not the final merged values,
	// and running loadValuesFromSource during admission adds N cluster reads
	// per Application create/update plus ordering hazards when the referenced
	// CM/Secret is applied in the same kubectl batch.
	var values map[string]interface{}
	if isDryRun(ctx) {
		if inline, ok := renderParams.Values.(map[string]interface{}); ok {
			values = inline
		} else {
			values = map[string]interface{}{}
		}
	} else {
		values, err = p.mergeValues(ctx, renderParams.Values, renderParams.ValuesFrom, appNamespace, releaseNamespace)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: failed to merge values", velaContextStr(renderParams.Context))
		}
	}

	// In dry-run mode (webhook validation), render client-side only — no cluster
	// interaction, no real install, no hooks. This prevents the webhook from
	// blocking for 30-60s on large charts.
	var manifest string
	var notes string
	if isDryRun(ctx) {
		klog.V(2).Infof("Helm provider: Dry-run mode — rendering chart %s client-side only", ch.Name())
		manifest, notes, err = p.dryRunRender(ch, releaseName, releaseNamespace, values, renderParams.Options, renderParams.Context)
		if err != nil {
			return nil, errors.Wrap(err, "failed to dry-run render chart")
		}
	} else {
		// Install or upgrade the chart via the Helm SDK
		manifest, notes, _, err = p.installOrUpgradeChart(ctx, ch, releaseName, releaseNamespace, values, renderParams.Options, renderParams.Context)
		if err != nil {
			return nil, errors.Wrap(err, "failed to install/upgrade chart")
		}
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
	// Include ALL Helm release Secrets as skeleton resources so KubeVela's
	// ResourceTracker records them and GC deletes them on Application deletion.
	// The skeleton intentionally omits the data field — KubeVela's merge-patch
	// strategy preserves unspecified fields, so Helm's data.release blob is
	// untouched. No special dispatcher changes needed.
	if renderParams.Context != nil {
		releaseSecretNames := p.listReleaseSecretNames(releaseNamespace, releaseName)
		for _, secName := range releaseSecretNames {
			secretMeta := map[string]interface{}{
				"name":      secName,
				"namespace": releaseNamespace,
			}
			// Add KubeVela ownership labels so MustBeControlledByApp passes
			// during pre-dispatch dryrun (especially for adoption of vanilla releases)
			if renderParams.Context != nil {
				secretMeta["labels"] = map[string]interface{}{
					"app.oam.dev/name":      renderParams.Context.AppName,
					"app.oam.dev/namespace": renderParams.Context.AppNamespace,
					"app.oam.dev/component": renderParams.Context.Name,
				}
			}
			releaseSecret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   secretMeta,
				"type":       "helm.sh/release.v1",
			}
			resources = append(resources, releaseSecret)
		}
		if len(releaseSecretNames) > 0 {
			klog.V(3).Infof("Helm provider: Tracking %d release secrets for %s", len(releaseSecretNames), releaseName)
		}
	}

	klog.Infof("Helm provider [%s]: Deployed %d resources for chart %s", velaContextStr(renderParams.Context), len(resources), renderParams.Chart.Source)

	// Log resource summary for debugging
	if len(resources) > 0 {
		if kind, found, _ := unstructured.NestedString(resources[0], "kind"); found {
			if name, found, _ := unstructured.NestedString(resources[0], "metadata", "name"); found {
				klog.Infof("Helm provider [%s]: First resource is %s/%s", velaContextStr(renderParams.Context), kind, name)
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
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
	p.releaseMu.Lock()
	delete(p.releaseFingerprints, cacheKey)
	delete(p.releaseManifests, cacheKey)
	delete(p.releaseVersions, cacheKey)
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
