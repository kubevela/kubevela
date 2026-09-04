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

package addon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/klog/v2"
)

// ociPuller pulls a Helm-chart artifact from an OCI registry and returns the raw
// chart archive bytes. It is a seam so unit tests can avoid the network.
type ociPuller func(ctx context.Context, ref, host, username, password string) ([]byte, error)

// ociTagLister lists the semver tags of an OCI repository, highest first. It is
// a seam so unit tests can avoid the network.
type ociTagLister func(ctx context.Context, repoRef, host, username, password string) ([]string, error)

// ociCatalogLister lists addon repository names below an OCI registry prefix.
// It is a seam so unit tests can avoid the network.
type ociCatalogLister func(ctx context.Context, registryURL, username, password string) ([]string, error)

// ociHelmBackend resolves addons stored as OCI Helm charts (e.g. in ECR/GHCR).
// OCI tags are the addon versions. An empty version resolves to the highest
// semver tag; there is no reliance on a floating "latest" tag, which `helm push`
// does not create.
//
// An OCI registry has no index.yaml, so cross-addon discovery needs a catalog of
// its own and version enumeration is a tag listing. That is what this backend
// supplies; everything downstream of the chart archive is shared.
type ociHelmBackend struct {
	name     string
	url      string
	username string
	token    string
	// pullFn/tagsFn/catalogFn/catalogIndexFn default to the production
	// implementations;
	// overridden in tests.
	pullFn         ociPuller
	tagsFn         ociTagLister
	catalogFn      ociCatalogLister
	catalogIndexFn ociCatalogIndexLister
}

// ociScheme is the URL scheme prefix this file strips before building a
// registry host and repository reference. IsOCIURL classifies the scheme
// case-insensitively, so parsing here has to match, or a registry stored as
// "OCI://..." would classify as OCI but build a malformed host such as "OCI:".
const ociScheme = "oci://"

// ociRegistryLocation returns the registry host and repository prefix.
func ociRegistryLocation(rawURL string) (host, prefix string) {
	trimmed := rawURL
	if len(rawURL) >= len(ociScheme) && strings.EqualFold(rawURL[:len(ociScheme)], ociScheme) {
		trimmed = rawURL[len(ociScheme):]
	}
	base := strings.Trim(trimmed, "/")
	host = base
	if i := strings.Index(base, "/"); i >= 0 {
		host = base[:i]
		prefix = strings.Trim(base[i+1:], "/")
	}
	return host, prefix
}

// ociRepoRef builds the OCI repository reference (no tag) and host from a
// registry URL and addon name. The URL may carry an "oci://" scheme and/or a
// trailing slash. The host is the registry authority (everything before the
// first path separator), used for login.
func ociRepoRef(url, addon string) (repoRef, host string) {
	host, prefix := ociRegistryLocation(url)
	repoRef = host
	if prefix != "" {
		repoRef += "/" + prefix
	}
	repoRef += "/" + strings.TrimPrefix(addon, "/")
	return repoRef, host
}

// resolveVersion returns the tag to pull. A pinned version is used as-is; an
// empty version is resolved to the highest semver tag published in the repo.
// resolveVersion picks the tag to pull and also reports the tags it saw getting
// there, so callers can fill in AvailableVersions without a second round trip.
// A pinned version needs no listing and returns no tag list.
func (b *ociHelmBackend) resolveVersion(ctx context.Context, repoRef, host, version string) (string, []string, error) {
	if version != "" {
		return version, nil, nil
	}
	list := b.tagsFn
	if list == nil {
		list = listOCITags
	}
	tags, err := list(ctx, repoRef, host, b.username, b.token)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to list tags for OCI addon %s", repoRef)
	}
	if len(tags) == 0 {
		return "", nil, errors.Wrapf(ErrNotExist, "no semver tags found for OCI addon %s; push a versioned tag or pin an explicit version", repoRef)
	}
	// helm's Tags returns semver-filtered, highest-first.
	return tags[0], tags, nil
}

// ociClientCache reuses a logged-in registry client across calls.
//
// Every OCI operation funnels through newOCIClientWithPlainHTTP, so without
// this a single listing costs one TLS handshake and one login per addon: the
// catalog is enumerated, then each entry is resolved and its versions listed.
// Against a real registry that fails once the catalog holds more than a couple
// of addons, with connection resets and TLS handshake timeouts, and it gets
// worse as more addons are published.
//
// The key includes the credentials, so a rotated password (an ECR login token
// lasts 12 hours) yields a new client rather than reusing a stale one.
var ociClientCache = struct {
	sync.Mutex
	clients map[string]*registry.Client
}{clients: map[string]*registry.Client{}}

// ociClientCacheLimit bounds the cache. Entries are keyed by credentials, so the
// live set is small; the cap only stops unbounded growth as tokens rotate.
const ociClientCacheLimit = 16

// ociClientCreation coordinates concurrent creation of the same cache entry so
// two goroutines resolving the same registry at once do not both dial and log
// in. Keying it by cache key, rather than using ociClientCache's own lock for
// this, keeps unrelated keys from blocking on each other's network I/O.
var ociClientCreation singleflight.Group

func ociClientCacheKey(host, username, password string, plainHTTP bool) string {
	sum := sha256.Sum256([]byte(username + "\x00" + password))
	return fmt.Sprintf("%s|%t|%x", host, plainHTTP, sum[:8])
}

func cachedOCIClient(key string) (*registry.Client, bool) {
	ociClientCache.Lock()
	defer ociClientCache.Unlock()
	client, ok := ociClientCache.clients[key]
	return client, ok
}

func storeOCIClient(key string, client *registry.Client) {
	ociClientCache.Lock()
	defer ociClientCache.Unlock()
	if len(ociClientCache.clients) >= ociClientCacheLimit {
		// Cheap eviction: the entries are interchangeable, and a dropped one is
		// only re-logged-in on next use.
		for k := range ociClientCache.clients {
			delete(ociClientCache.clients, k)
			break
		}
	}
	ociClientCache.clients[key] = client
}

func newOCIClientWithPlainHTTP(host, username, password string, plainHTTP bool) (*registry.Client, error) {
	key := ociClientCacheKey(host, username, password, plainHTTP)

	if client, ok := cachedOCIClient(key); ok {
		return client, nil
	}

	// The dial and login below run outside ociClientCache's lock: only the map
	// reads and writes hold it, so a slow or unreachable registry blocks
	// nothing but callers asking for this same key.
	result, err, _ := ociClientCreation.Do(key, func() (interface{}, error) {
		if client, ok := cachedOCIClient(key); ok {
			return client, nil
		}

		var opts []registry.ClientOption
		if plainHTTP {
			opts = append(opts, registry.ClientOptPlainHTTP())
		}
		client, err := registry.NewClient(opts...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create OCI registry client")
		}
		if username != "" || password != "" {
			if err := client.Login(host, registry.LoginOptBasicAuth(username, password)); err != nil {
				return nil, errors.Wrapf(err, "failed to login to OCI registry %s", host)
			}
		}

		storeOCIClient(key, client)
		return client, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*registry.Client), nil
}

// pullOCIChart is the production puller: it logs in (when credentials are set)
// and pulls the chart layer from the OCI registry via the Helm registry client.
func pullOCIChart(_ context.Context, ref, host, username, password string) ([]byte, error) {
	return pullOCIChartWithTransport(ref, host, username, password, false)
}

func pullOCIChartWithPlainHTTP(_ context.Context, ref, host, username, password string) ([]byte, error) {
	return pullOCIChartWithTransport(ref, host, username, password, true)
}

func pullOCIChartWithTransport(ref, host, username, password string, plainHTTP bool) ([]byte, error) {
	client, err := newOCIClientWithPlainHTTP(host, username, password, plainHTTP)
	if err != nil {
		return nil, err
	}
	result, err := client.Pull(ref, registry.PullOptWithChart(true))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull addon chart %s", ref)
	}
	if result == nil || result.Chart == nil || len(result.Chart.Data) == 0 {
		return nil, errors.Errorf("addon chart %s has no chart layer", ref)
	}
	return result.Chart.Data, nil
}

// listOCITags lists the repository's semver tags (highest first) via the Helm
// registry client, which filters non-semver tags and sorts descending.
func listOCITags(_ context.Context, repoRef, host, username, password string) ([]string, error) {
	return listOCITagsWithTransport(repoRef, host, username, password, false)
}

func listOCITagsWithPlainHTTP(_ context.Context, repoRef, host, username, password string) ([]string, error) {
	return listOCITagsWithTransport(repoRef, host, username, password, true)
}

func listOCITagsWithTransport(repoRef, host, username, password string, plainHTTP bool) ([]string, error) {
	client, err := newOCIClientWithPlainHTTP(host, username, password, plainHTTP)
	if err != nil {
		return nil, err
	}
	return client.Tags(repoRef)
}

// ociErrCodeNameUnknown is how the OCI distribution spec reports a repository
// that does not exist. oras-go renders the code by lowercasing it and turning
// underscores into spaces (NAME_UNKNOWN -> "name unknown").
const ociErrCodeNameUnknown = "name unknown"

// isOCIRepositoryAbsentError reports whether err is a registry answer confirming
// "this repository does not exist", as opposed to "this repository could not be
// read". Only the first lets the caller publish a catalog, because there is
// nothing to preserve; misreading the second rebuilds the catalog from an empty
// list and drops every addon already published.
//
// The test is the NAME_UNKNOWN error code, not the 404 status. A bare 404 is
// ambiguous -- a proxy, a gateway, or a registry that does not serve the
// tag-list route answers the same way for a repository that does exist -- so it
// stays on the conservative branch.
//
// The code has to be read out of the message: oras-go v1.2.5 builds these errors
// with fmt.Errorf and keeps its error types in the unexported
// pkg/registry/remote/internal/errutil. A miss is safe in the same direction --
// callers refuse to rewrite the catalog.
func isOCIRepositoryAbsentError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ociErrCodeNameUnknown)
}

// isDockerHubHost reports whether host is a known Docker Hub registry alias.
// Mirrors the alias list in pkg/cue/cuex/providers/helm/auth.go's
// normalizeDockerHubAliases; kept local rather than imported because that
// package is a different, heavier dependency (CUE #Helm chart-fetch auth)
// that pkg/addon does not otherwise need.
func isDockerHubHost(host string) bool {
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	switch h {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return true
	}
	return false
}

// classifyCatalogListStatus turns a /v2/_catalog response status into the
// enumeration result. Split out from listOCIRepositoriesWithScheme's HTTP
// loop so the Docker-Hub-specific 401 handling is unit-testable without a
// real HTTP round trip.
//
// Docker Hub's registry front door never grants the registry:catalog:* scope
// to anyone, including the account owner, so it deterministically answers
// /v2/_catalog with 401 regardless of credentials. For every other registry a
// 401 here usually means a real permission problem worth surfacing, so the
// relaxation is scoped to Docker Hub's known hosts rather than applied
// globally. This is safe even for a Docker Hub user with wrong credentials:
// listOCIRepositoriesWithScheme is only ever called alongside a portable
// catalog read that performs its own real login, and a genuine auth failure
// there still surfaces as a hard error (see ociHelmBackend.listUIData).
func classifyCatalogListStatus(host string, status int, statusText string) error {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return errors.Wrapf(ErrOCICatalogAbsent, "OCI catalog enumeration is unsupported at %s: server returned %s", host, statusText)
	case http.StatusUnauthorized:
		if isDockerHubHost(host) {
			return errors.Wrapf(ErrOCICatalogAbsent, "OCI catalog enumeration is unsupported at %s: Docker Hub does not grant catalog listing to any credential: server returned %s", host, statusText)
		}
	}
	return errors.Errorf("failed to list OCI catalog at %s: server returned %s", host, statusText)
}

// listOCIRepositories enumerates the OCI distribution catalog and returns
// repository names relative to the configured registry prefix. The catalog API
// is paginated through RFC 5988 Link headers.
func listOCIRepositories(ctx context.Context, registryURL, username, password string) ([]string, error) {
	return listOCIRepositoriesWithScheme(ctx, registryURL, username, password, "https")
}

func listOCIRepositoriesWithPlainHTTP(ctx context.Context, registryURL, username, password string) ([]string, error) {
	return listOCIRepositoriesWithScheme(ctx, registryURL, username, password, "http")
}

func listOCIRepositoriesWithScheme(ctx context.Context, registryURL, username, password, scheme string) ([]string, error) {
	host, prefix := ociRegistryLocation(registryURL)
	next := &url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     "/v2/_catalog",
		RawQuery: "n=1000",
	}
	seen := map[string]bool{}
	var addons []string

	for next != nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next.String(), nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build OCI catalog request")
		}
		if username != "" || password != "" {
			req.SetBasicAuth(username, password)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list OCI catalog at %s", host)
		}

		var page struct {
			Repositories []string `json:"repositories"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&page)
		closeErr := resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			// Not every registry implements /v2/_catalog. Treat a refusal that
			// names the route as unsupported (or, for Docker Hub specifically, its
			// deterministic 401) as "no catalog to enumerate" rather than a read
			// failure, so a push can still bootstrap a portable catalog there.
			// Anything else stays a read failure, because rebuilding the catalog
			// from a misread empty list would drop every entry already published.
			return nil, classifyCatalogListStatus(host, resp.StatusCode, resp.Status)
		}
		if decodeErr != nil {
			return nil, errors.Wrap(decodeErr, "failed to decode OCI catalog response")
		}
		if closeErr != nil {
			return nil, errors.Wrap(closeErr, "failed to close OCI catalog response")
		}

		for _, repository := range page.Repositories {
			addonName := strings.Trim(repository, "/")
			if prefix != "" {
				prefixWithSlash := prefix + "/"
				if !strings.HasPrefix(addonName, prefixWithSlash) {
					continue
				}
				addonName = strings.TrimPrefix(addonName, prefixWithSlash)
			}
			if addonName != "" && addonName != ociCatalogChartName && !seen[addonName] {
				seen[addonName] = true
				addons = append(addons, addonName)
			}
		}

		next = nil
		link := resp.Header.Get("Link")
		if start, end := strings.Index(link, "<"), strings.Index(link, ">"); start >= 0 && end > start && strings.Contains(link[end:], `rel="next"`) {
			candidate, parseErr := req.URL.Parse(link[start+1 : end])
			if parseErr != nil {
				return nil, errors.Wrap(parseErr, "failed to parse OCI catalog pagination link")
			}
			// The Link header is registry-supplied and url.Parse resolves an
			// absolute URL by replacing scheme and host outright. Every request in
			// this loop attaches the registry's BasicAuth credentials, so following
			// such a link would hand them to a host we were never configured to
			// talk to. Accept only links that stay on the original scheme and host.
			if candidate.Scheme != scheme || candidate.Host != host {
				return nil, errors.Errorf("refusing OCI catalog pagination link %q: expected an %s link on host %s", candidate.Redacted(), scheme, host)
			}
			next = candidate
		}
	}

	sort.Strings(addons)
	return addons, nil
}

// classify translates a load failure into the shared error vocabulary.
// installDependency uses isSkippableRegistryError to decide whether to try the
// next registry; without this, any OCI error (a missing tag, an auth failure, an
// unreachable host) would abort dependency resolution instead of falling
// through. The HTTP backend deliberately does not do this, which is why it is an
// optional capability rather than part of chartBackend.
func (b *ociHelmBackend) classify(err error) error {
	if err == nil || errors.Is(err, ErrNotExist) || errors.Is(err, ErrFetch) {
		return err
	}
	return errors.Wrapf(ErrFetch, "OCI registry %s: %v", b.name, err)
}

// supportsVersionRequirements reports false. Versions here are synthesized from
// repository tags and carry no annotations, so LoadSystemRequirements would read
// nil for every one of them and report the newest tag as meeting any
// requirement. Answering "no opinion" is the honest result.
func (b *ociHelmBackend) supportsVersionRequirements() bool { return false }

// Resolve pulls the addon's OCI chart and decodes the archive.
func (b *ociHelmBackend) resolve(ctx context.Context, addonName, version string) (*resolvedChart, error) {
	repoRef, host := ociRepoRef(b.url, addonName)
	resolved, available, err := b.resolveVersion(ctx, repoRef, host, version)
	if err != nil {
		return nil, err
	}
	ref := fmt.Sprintf("%s:%s", repoRef, resolved)
	pull := b.pullFn
	if pull == nil {
		pull = pullOCIChart
	}
	archive, err := pull(ctx, ref, host, b.username, b.token)
	if err != nil {
		return nil, err
	}
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load addon chart archive %s", ref)
	}
	klog.V(5).Infof("Addon '%s' loaded from OCI registry '%s' (%s)", addonName, b.name, ref)
	return &resolvedChart{
		files:   files,
		version: resolved,
		// A pinned request lists no tags and so carries no list: the caller asked
		// about one version.
		availableVersions: available,
		// The chart's own metadata.yaml is the only source of requirements here.
		// Unlike an index entry, a tag carries no annotations to override it with.
		requirementsSet: false,
	}, nil
}

// Versions lists the semver tags of an OCI addon, highest first.
func (b *ociHelmBackend) versions(ctx context.Context, addonName string) ([]*repo.ChartVersion, error) {
	repoRef, host := ociRepoRef(b.url, addonName)
	list := b.tagsFn
	if list == nil {
		list = listOCITags
	}
	tags, err := list(ctx, repoRef, host, b.username, b.token)
	if err != nil {
		return nil, err
	}
	versions := make([]*repo.ChartVersion, 0, len(tags))
	for _, tag := range tags {
		versions = append(versions, &repo.ChartVersion{
			Metadata: &chart.Metadata{Name: addonName, Version: tag},
		})
	}
	return versions, nil
}

// loadUIData builds one listing entry from a real chart, for the discovery path
// that has only repository names to work with.
func (b *ociHelmBackend) loadUIData(ctx context.Context, addonName, version string) (data *UIData, err error) {
	defer func() {
		if err != nil {
			err = b.classify(err)
		}
	}()
	resolved, err := b.resolve(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	pkg, err := loadAddonPackage(addonName, resolved.files)
	if err != nil {
		return nil, err
	}
	pkg.AvailableVersions = resolved.availableVersions
	return uiDataFromPackage(pkg), nil
}

// ListUIData enumerates repositories below the configured OCI prefix and loads
// the latest semver-tagged addon metadata for each repository.
func (b *ociHelmBackend) listUIData(ctx context.Context) ([]*UIData, error) {
	var indexErr error
	if b.catalogIndexFn != nil {
		addons, err := b.catalogIndexFn(ctx, b.url, b.username, b.token)
		if err == nil {
			return addons, nil
		}
		indexErr = err
		klog.V(4).Infof("Portable OCI addon catalog is unavailable for registry %q, falling back to registry catalog discovery: %v", b.name, err)
	}

	list := b.catalogFn
	if list == nil {
		list = listOCIRepositories
	}
	names, err := list(ctx, b.url, b.username, b.token)
	if err != nil {
		if indexErr != nil {
			// Only a genuine absence on BOTH sides means there is no catalog. If
			// either failure was a read error, callers must not treat the result as
			// an empty catalog.
			if errors.Is(indexErr, ErrOCICatalogAbsent) && errors.Is(err, ErrOCICatalogAbsent) {
				return nil, errors.Wrapf(ErrOCICatalogAbsent, "no OCI addon catalog at portable location (%v) or registry catalog (%v)", indexErr, err)
			}
			return nil, errors.Errorf("failed to list OCI addons from portable catalog (%v) and registry catalog (%v)", indexErr, err)
		}
		return nil, err
	}

	var addons []*UIData
	for _, name := range names {
		repoRef, host := ociRepoRef(b.url, name)
		tags := b.tagsFn
		if tags == nil {
			tags = listOCITags
		}
		versions, err := tags(ctx, repoRef, host, b.username, b.token)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list versions for OCI addon %s", name)
		}
		if len(versions) == 0 {
			continue
		}
		addon, err := b.loadUIData(ctx, name, versions[0])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load metadata for OCI addon %s", name)
		}
		addon.AvailableVersions = versions
		addons = append(addons, addon)
	}
	return addons, nil
}
