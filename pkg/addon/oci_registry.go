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
func (i *ociRegistry) resolveVersion(ctx context.Context, repoRef, host, version string) (string, error) {
	if version != "" {
		return version, nil
	}
	list := i.tagsFn
	if list == nil {
		list = listOCITags
	}
	tags, err := list(ctx, repoRef, host, i.username, i.token)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list tags for OCI addon %s", repoRef)
	}
	if len(tags) == 0 {
		return "", errors.Errorf("no semver tags found for OCI addon %s; push a versioned tag or pin an explicit version", repoRef)
	}
	// helm's Tags returns semver-filtered, highest-first.
	return tags[0], nil
}

// newOCIClient builds an authenticated Helm OCI registry client.
func newOCIClient(host, username, password string) (*registry.Client, error) {
	client, err := registry.NewClient()
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
	client, err := newOCIClient(host, username, password)
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
	client, err := newOCIClient(host, username, password)
	if err != nil {
		return nil, err
	}
	return client.Tags(repoRef)
}

// listOCIRepositories enumerates the OCI distribution catalog and returns
// repository names relative to the configured registry prefix. The catalog API
// is paginated through RFC 5988 Link headers.
func listOCIRepositories(ctx context.Context, registryURL, username, password string) ([]string, error) {
	host, prefix := ociRegistryLocation(registryURL)
	next := &url.URL{
		Scheme:   "https",
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
			next, err = req.URL.Parse(link[start+1 : end])
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse OCI catalog pagination link")
			}
		}
	}

	sort.Strings(addons)
	return addons, nil
}

// loadAddon pulls the addon's OCI chart and turns it into a WholeAddonPackage,
// reusing the shared archive -> InstallPackage pipeline.
func (i *ociRegistry) loadAddon(ctx context.Context, name, version string) (*WholeAddonPackage, error) {
	repoRef, host := ociRepoRef(i.url, name)
	resolved, err := i.resolveVersion(ctx, repoRef, host, version)
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
	pkg, err := loadAddonPackage(name, files)
	if err != nil {
		return nil, err
	}
	pkg.RegistryName = i.name
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
	return &UIData{
		Meta:      pkg.Meta,
		APISchema: pkg.APISchema,
		Detail:    pkg.Detail,
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
