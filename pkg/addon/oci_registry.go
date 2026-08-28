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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/pkg/errors"
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

// ociRegistry resolves addons stored as OCI Helm charts (e.g. in ECR/GHCR).
// It satisfies VersionedRegistry: OCI tags are the addon versions. An empty
// version resolves to the highest semver tag (there is no reliance on a
// floating "latest" tag, which `helm push` does not create).
type ociRegistry struct {
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

// BuildOCIRegistry builds an OCI addon registry reader.
func BuildOCIRegistry(name, url, username, token string) VersionedRegistry {
	return &ociRegistry{
		name:           name,
		url:            url,
		username:       username,
		token:          token,
		pullFn:         pullOCIChart,
		tagsFn:         listOCITags,
		catalogFn:      listOCIRepositories,
		catalogIndexFn: listPortableOCICatalog,
	}
}

// ociRegistryLocation returns the registry host and repository prefix.
func ociRegistryLocation(rawURL string) (host, prefix string) {
	base := strings.Trim(strings.TrimPrefix(rawURL, "oci://"), "/")
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
func (i *ociRegistry) resolveVersion(ctx context.Context, repoRef, host, version string) (string, []string, error) {
	if version != "" {
		return version, nil, nil
	}
	list := i.tagsFn
	if list == nil {
		list = listOCITags
	}
	tags, err := list(ctx, repoRef, host, i.username, i.token)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to list tags for OCI addon %s", repoRef)
	}
	if len(tags) == 0 {
		return "", nil, errors.Wrapf(ErrNotExist, "no semver tags found for OCI addon %s; push a versioned tag or pin an explicit version", repoRef)
	}
	// helm's Tags returns semver-filtered, highest-first.
	return tags[0], tags, nil
}

func newOCIClientWithPlainHTTP(host, username, password string, plainHTTP bool) (*registry.Client, error) {
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
	return client, nil
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
			// Not every registry implements /v2/_catalog (ECR does not). Treat those
			// answers as "no catalog to enumerate" rather than a read failure, so a
			// push can still bootstrap a portable catalog there.
			switch resp.StatusCode {
			case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
				return nil, errors.Wrapf(ErrOCICatalogAbsent, "OCI catalog enumeration is unsupported at %s: server returned %s", host, resp.Status)
			}
			return nil, errors.Errorf("failed to list OCI catalog at %s: server returned %s", host, resp.Status)
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

// loadAddon pulls the addon's OCI chart and turns it into a WholeAddonPackage,
// reusing the shared archive -> InstallPackage pipeline.
func (i *ociRegistry) loadAddon(ctx context.Context, name, version string) (pkg *WholeAddonPackage, err error) {
	// Classify failures as registry-level so callers can tell "this registry
	// cannot provide this addon" from "stop everything". installDependency uses
	// isSkippableRegistryError to decide whether to try the next registry; without
	// this, any OCI error (missing tag, auth failure, unreachable host) aborts
	// dependency resolution instead of falling through. Mirrors the ErrFetch
	// wrapping already done in Installer.getAddonMeta.
	defer func() {
		if err != nil && !errors.Is(err, ErrNotExist) && !errors.Is(err, ErrFetch) {
			err = errors.Wrapf(ErrFetch, "OCI registry %s: %v", i.name, err)
		}
	}()

	repoRef, host := ociRepoRef(i.url, name)
	resolved, available, err := i.resolveVersion(ctx, repoRef, host, version)
	if err != nil {
		return nil, err
	}
	ref := fmt.Sprintf("%s:%s", repoRef, resolved)
	pull := i.pullFn
	if pull == nil {
		pull = pullOCIChart
	}
	archive, err := pull(ctx, ref, host, i.username, i.token)
	if err != nil {
		return nil, err
	}
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load addon chart archive %s", ref)
	}
	pkg, err = loadAddonPackage(name, files)
	if err != nil {
		return nil, err
	}
	pkg.RegistryName = i.name
	// loadAddonPackage builds the package from the chart archive, which knows
	// nothing about sibling tags, so AvailableVersions would otherwise stay empty
	// and the UI would show the addon as having a single version. versionedRegistry
	// attaches its version list the same way. A pinned request lists no tags and
	// so carries no list -- the caller asked about one version.
	pkg.AvailableVersions = available
	klog.V(5).Infof("Addon '%s' loaded from OCI registry '%s' (%s)", name, i.name, ref)
	return pkg, nil
}

// GetAddonInstallPackage returns the addon's install package from the OCI registry.
func (i *ociRegistry) GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error) {
	pkg, err := i.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return &pkg.InstallPackage, nil
}

// GetDetailedAddon returns the whole addon package from the OCI registry.
func (i *ociRegistry) GetDetailedAddon(ctx context.Context, addonName, version string) (*WholeAddonPackage, error) {
	return i.loadAddon(ctx, addonName, version)
}

// GetAddonUIData returns the addon's UI data from the OCI registry.
func (i *ociRegistry) GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error) {
	pkg, err := i.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	// Mirror versionedRegistry.GetAddonUIData: dropping these leaves UI and cache
	// consumers with metadata that is incomplete compared with an HTTP registry.
	return &UIData{
		Meta:              pkg.Meta,
		APISchema:         pkg.APISchema,
		Parameters:        pkg.Parameters,
		Detail:            pkg.Detail,
		Definitions:       pkg.Definitions,
		AvailableVersions: pkg.AvailableVersions,
		CUEDefinitions:    pkg.CUEDefinitions,
	}, nil
}

// ListAddon enumerates repositories below the configured OCI prefix and loads
// the latest semver-tagged addon metadata for each repository.
func (i *ociRegistry) ListAddon() ([]*UIData, error) {
	ctx := context.Background()
	var indexErr error
	if i.catalogIndexFn != nil {
		addons, err := i.catalogIndexFn(ctx, i.url, i.username, i.token)
		if err == nil {
			for _, addon := range addons {
				addon.RegistryName = i.name
			}
			return addons, nil
		}
		indexErr = err
		klog.V(4).Infof("Portable OCI addon catalog is unavailable for registry %q, falling back to registry catalog discovery: %v", i.name, err)
	}

	list := i.catalogFn
	if list == nil {
		list = listOCIRepositories
	}
	names, err := list(ctx, i.url, i.username, i.token)
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
		repoRef, host := ociRepoRef(i.url, name)
		tags := i.tagsFn
		if tags == nil {
			tags = listOCITags
		}
		versions, err := tags(ctx, repoRef, host, i.username, i.token)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list versions for OCI addon %s", name)
		}
		if len(versions) == 0 {
			continue
		}
		addon, err := i.GetAddonUIData(ctx, name, versions[0])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load metadata for OCI addon %s", name)
		}
		addon.RegistryName = i.name
		addon.AvailableVersions = versions
		addons = append(addons, addon)
	}
	return addons, nil
}

// GetAddonAvailableVersion lists semver tags for an OCI addon.
func (i *ociRegistry) GetAddonAvailableVersion(addonName string) ([]*repo.ChartVersion, error) {
	repoRef, host := ociRepoRef(i.url, addonName)
	list := i.tagsFn
	if list == nil {
		list = listOCITags
	}
	tags, err := list(context.Background(), repoRef, host, i.username, i.token)
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
