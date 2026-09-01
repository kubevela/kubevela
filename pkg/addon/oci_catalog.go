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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/registry"
)

const (
	ociCatalogAPIVersion = "addons.kubevela.io/v1alpha1"
	ociCatalogChartName  = "kubevela-addon-catalog"
	ociCatalogFileName   = "catalog.json"
)

// OCIAddonCatalog is the portable addon index stored as a Helm chart at the
// well-known <registry-prefix>/kubevela-addon-catalog repository.
type OCIAddonCatalog struct {
	APIVersion string                 `json:"apiVersion"`
	Addons     []OCIAddonCatalogEntry `json:"addons"`
}

// OCIAddonCatalogEntry describes one addon in a portable OCI catalog.
type OCIAddonCatalogEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Versions    []string `json:"versions"`
}

// ociCatalogIndexLister reads the portable KubeVela catalog artifact.
type ociCatalogIndexLister func(ctx context.Context, registryURL, username, password string) ([]*UIData, error)

// listPortableOCICatalog pulls and decodes the newest catalog artifact. Because
// its repository name is fixed, it only uses portable OCI operations: list tags
// for a known repository and pull a known manifest.
func listPortableOCICatalog(_ context.Context, registryURL, username, password string) ([]*UIData, error) {
	return listPortableOCICatalogWithTransport(registryURL, username, password, false)
}

func listPortableOCICatalogWithPlainHTTP(_ context.Context, registryURL, username, password string) ([]*UIData, error) {
	return listPortableOCICatalogWithTransport(registryURL, username, password, true)
}

func listPortableOCICatalogWithTransport(registryURL, username, password string, plainHTTP bool) ([]*UIData, error) {
	repoRef, host := ociRepoRef(registryURL, ociCatalogChartName)
	tags, err := listOCITagsWithTransport(repoRef, host, username, password, plainHTTP)
	if err != nil {
		// A registry that has never had a catalog pushed answers "repository does
		// not exist". That is an absence, not a read failure, so the first push to
		// such a registry can still bootstrap the catalog.
		if isOCIRepositoryAbsentError(err) {
			return nil, errors.Wrapf(ErrOCICatalogAbsent, "portable OCI addon catalog repository %s does not exist: %v", repoRef, err)
		}
		return nil, errors.Wrap(err, "portable OCI addon catalog is unavailable")
	}
	if len(tags) == 0 {
		return nil, errors.Wrap(ErrOCICatalogAbsent, "portable OCI addon catalog has no semver tags")
	}
	archive, err := pullOCIChartWithTransport(repoRef+":"+tags[0], host, username, password, plainHTTP)
	if err != nil {
		return nil, errors.Wrap(err, "failed to pull portable OCI addon catalog")
	}
	return decodeOCIAddonCatalog(archive)
}

func decodeOCIAddonCatalog(archive []byte) ([]*UIData, error) {
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load portable OCI addon catalog archive")
	}

	var data []byte
	for _, file := range files {
		if filepath.Base(file.Name) == ociCatalogFileName {
			data = file.Data
			break
		}
	}
	if len(data) == 0 {
		return nil, errors.Errorf("portable OCI addon catalog does not contain %s", ociCatalogFileName)
	}

	var catalog OCIAddonCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, errors.Wrap(err, "failed to decode portable OCI addon catalog")
	}
	if catalog.APIVersion != ociCatalogAPIVersion {
		return nil, errors.Errorf("unsupported OCI addon catalog API version %q", catalog.APIVersion)
	}

	addons := make([]*UIData, 0, len(catalog.Addons))
	for _, entry := range catalog.Addons {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, errors.New("portable OCI addon catalog contains an addon without a name")
		}
		// Version has to carry the newest version, not stay empty: the shared addon
		// cache keys versioned UIData by it (Cache.listVersionRegistryUIDataAndCache),
		// so an empty value writes a dead "<name>-" entry and leaves `vela addon
		// list` showing a blank version for cached OCI registries.
		addons = append(addons, &UIData{
			Meta: Meta{
				Name:        entry.Name,
				Description: entry.Description,
				Version:     newestOCICatalogVersion(entry.Versions),
			},
			AvailableVersions: entry.Versions,
		})
	}
	sort.Slice(addons, func(a, b int) bool {
		return addons[a].Name < addons[b].Name
	})
	return addons, nil
}

// newestOCICatalogVersion returns the highest semver in versions, falling back to
// the first entry when none of them parse. Catalog entries are written sorted by
// listOCITags, but a hand-edited catalog need not be.
func newestOCICatalogVersion(versions []string) string {
	var newest *semver.Version
	var newestRaw string
	for _, v := range versions {
		parsed, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		if newest == nil || parsed.GreaterThan(newest) {
			newest, newestRaw = parsed, v
		}
	}
	if newestRaw != "" {
		return newestRaw
	}
	if len(versions) > 0 {
		return versions[0]
	}
	return ""
}

// confirmPortableCatalogAbsent re-probes the catalog repository to confirm that
// there is genuinely no catalog to preserve, and returns an error describing why
// it could not be confirmed otherwise.
func confirmPortableCatalogAbsent(source *HelmSource, plainHTTP bool) error {
	repoRef, host := ociRepoRef(source.URL, ociCatalogChartName)
	tags, err := listOCITagsWithTransport(repoRef, host, source.Username, source.Token, plainHTTP)
	return classifyCatalogAbsenceProbe(repoRef, tags, err)
}

// classifyCatalogAbsenceProbe decides whether a tag-list probe of the catalog
// repository confirms that no catalog exists. It returns nil only for a
// confirmed absence.
//
// The asymmetry is deliberate. Wrongly concluding "absent" republishes the
// catalog with a single addon and silently drops every other entry, with no
// signal to the operator. Wrongly concluding "present" refuses a push and says
// why, which the operator can act on. So anything short of the registry stating
// that the repository does not exist is a refusal.
func classifyCatalogAbsenceProbe(repoRef string, tags []string, probeErr error) error {
	switch {
	case probeErr != nil && isOCIRepositoryAbsentError(probeErr):
		// The registry states the repository does not exist. Nothing to lose.
		return nil
	case probeErr != nil:
		return errors.Wrapf(probeErr, "refusing to rewrite the OCI addon catalog: cannot confirm whether %s already holds a catalog", repoRef)
	case len(tags) > 0:
		return errors.Errorf("refusing to rewrite the OCI addon catalog: %s holds catalog tag %q, which could not be read; publishing now would drop every addon already listed there", repoRef, tags[0])
	default:
		// The repository answered without stating that it does not exist, yet
		// exposes no semver tag. helm's tag listing drops anything that is not
		// strict semver ("latest", "v0.0.1", "1.0"), so a catalog may well be
		// published here under a tag this code cannot see.
		return errors.Errorf("refusing to rewrite the OCI addon catalog: %s did not report a missing repository but exposes no semver-tagged catalog, so its contents cannot be confirmed; delete the repository to start a fresh catalog", repoRef)
	}
}

// maxCatalogPublishAttempts bounds the read-merge-publish retry loop in
// updateOCIAddonCatalog. The OCI distribution API has no conditional-put or
// ETag primitive to serialize catalog publication on, so concurrent `vela
// addon push` runs detect and back off from each other by re-reading and
// retrying rather than being prevented from racing in the first place. This
// narrows the collision window (the gap between the last tag-listing check
// and the push itself) but, without a true conditional write, cannot close it
// completely: two publishers can still both pass their final check and push
// the same computed version moments apart.
//
// A tag listing failure is treated the same as a detected conflict (see
// publishCatalogEntry), so a registry that structurally cannot list tags at
// all (permission, unsupported endpoint) now exhausts every attempt here on
// every push, rather than the prior behavior of silently publishing a
// possibly-wrong catalog version on the first try. push.go treats the whole
// catalog update as non-fatal to the addon push itself, so this only costs a
// warning and the accumulated backoff, never a failed `vela addon push`.
const maxCatalogPublishAttempts = 5

// catalogPublishBackoff returns the delay before retrying a catalog publish
// that lost a race, growing with each attempt so repeatedly-colliding
// publishers spread out rather than immediately re-racing.
func catalogPublishBackoff(attempt int) time.Duration {
	return time.Duration(50*(attempt+1)) * time.Millisecond
}

// updateOCIAddonCatalogOnceFn is a seam so tests can exercise the retry loop
// in updateOCIAddonCatalog without a real OCI registry. Production always
// uses updateOCIAddonCatalogOnce.
var updateOCIAddonCatalogOnceFn = updateOCIAddonCatalogOnce

// catalogRepoTagsFn lists the portable catalog repository's own tags. It is a
// seam so tests can drive updateOCIAddonCatalogOnce's conflict check directly
// (returning a different tag list on the pre-push re-check than on the
// initial read) without a real OCI registry. Production always uses
// listOCITagsWithTransport.
var catalogRepoTagsFn = listOCITagsWithTransport

// addonVersionsTagsFn lists the addon-under-publish's own repository tags,
// used only to populate the catalog entry's Versions field. A separate seam
// from catalogRepoTagsFn so tests can stub it out without affecting the
// catalog-tag call count the conflict check depends on.
var addonVersionsTagsFn = listOCITagsWithTransport

// updateOCIAddonCatalog upserts an addon after it has been pushed and publishes
// a new catalog chart version. The fixed catalog repository makes discovery
// portable across OCI registries.
func updateOCIAddonCatalog(client *registry.Client, source *HelmSource, addonMeta *chart.Metadata, plainHTTP bool) error {
	var lastErr error
	for attempt := range maxCatalogPublishAttempts {
		conflict, err := updateOCIAddonCatalogOnceFn(client, source, addonMeta, plainHTTP)
		if err == nil {
			return nil
		}
		if !conflict {
			return err
		}
		lastErr = err
		if attempt == maxCatalogPublishAttempts-1 {
			break
		}
		time.Sleep(catalogPublishBackoff(attempt))
	}
	return errors.Wrapf(lastErr, "failed to publish the portable OCI addon catalog after %d attempts: either a concurrent publisher kept winning the race for the catalog tag, or the registry's tag listing is intermittently unavailable", maxCatalogPublishAttempts)
}

// updateOCIAddonCatalogOnce runs one read-merge-publish attempt. conflict is
// true when either a concurrent publisher's tag appeared between this
// attempt's read of the catalog tags and its publish, or a tag listing failed
// outright and this attempt cannot confirm the catalog's state at all; the
// caller retries from a fresh read rather than surfacing either as a failure,
// since publishing here would risk colliding on the same version tag,
// dropping whichever addon another publisher just added, or overwriting a
// catalog this attempt never actually got to read.
func updateOCIAddonCatalogOnce(client *registry.Client, source *HelmSource, addonMeta *chart.Metadata, plainHTTP bool) (conflict bool, err error) {
	pullFn := pullOCIChart
	tagsFn := listOCITags
	catalogFn := listOCIRepositories
	catalogIndexFn := listPortableOCICatalog
	if plainHTTP {
		pullFn = pullOCIChartWithPlainHTTP
		tagsFn = listOCITagsWithPlainHTTP
		catalogFn = listOCIRepositoriesWithPlainHTTP
		catalogIndexFn = listPortableOCICatalogWithPlainHTTP
	}
	reader := &ociHelmBackend{
		name:           source.URL,
		url:            source.URL,
		username:       source.Username,
		token:          source.Token,
		pullFn:         pullFn,
		tagsFn:         tagsFn,
		catalogFn:      catalogFn,
		catalogIndexFn: catalogIndexFn,
	}
	existing, err := reader.listUIData(context.Background())
	if err != nil {
		// A registry with no portable catalog and no repository enumeration can
		// still bootstrap a catalog with the addon currently being pushed. Any
		// other failure means a catalog may well exist and simply could not be
		// read; rebuilding from an empty list would publish a catalog containing
		// only this addon and silently drop every other entry.
		if !errors.Is(err, ErrOCICatalogAbsent) {
			return false, errors.Wrap(err, "refusing to rewrite the OCI addon catalog: cannot read the existing catalog")
		}
		// ErrOCICatalogAbsent is the right answer for readers -- there is nothing
		// to list -- but it is too weak to authorise an overwrite. Several
		// non-absences reach it: a tag list that survives helm's strict-semver
		// filter empty, a 404 from a proxy or gateway, and a registry that does
		// not serve /v2/_catalog. Confirm the absence against the catalog
		// repository itself before replacing what is published there.
		if err := confirmPortableCatalogAbsent(source, plainHTTP); err != nil {
			return false, err
		}
		existing = nil
	}

	return publishCatalogEntry(client, source, addonMeta, existing, plainHTTP)
}

// publishCatalogEntry merges addonMeta into existing (the catalog's current
// addons, already resolved by the caller) and publishes the result. Split out
// from updateOCIAddonCatalogOnce so tests can drive the version-computation
// and conflict-detection logic directly, with a controlled existing list,
// without needing a real registry to satisfy the existing-catalog read that
// precedes it.
func publishCatalogEntry(client *registry.Client, source *HelmSource, addonMeta *chart.Metadata, existing []*UIData, plainHTTP bool) (conflict bool, err error) {
	addonRepo, host := ociRepoRef(source.URL, addonMeta.Name)
	versions, err := addonVersionsTagsFn(addonRepo, host, source.Username, source.Token, plainHTTP)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list versions for OCI addon %s", addonMeta.Name)
	}

	entries := make(map[string]OCIAddonCatalogEntry, len(existing)+1)
	for _, addon := range existing {
		entries[addon.Name] = OCIAddonCatalogEntry{
			Name:        addon.Name,
			Description: addon.Description,
			Versions:    addon.AvailableVersions,
		}
	}
	entries[addonMeta.Name] = OCIAddonCatalogEntry{
		Name:        addonMeta.Name,
		Description: addonMeta.Description,
		Versions:    versions,
	}

	catalog := OCIAddonCatalog{APIVersion: ociCatalogAPIVersion}
	for _, entry := range entries {
		catalog.Addons = append(catalog.Addons, entry)
	}
	sort.Slice(catalog.Addons, func(a, b int) bool {
		return catalog.Addons[a].Name < catalog.Addons[b].Name
	})
	catalogData, err := json.Marshal(catalog)
	if err != nil {
		return false, errors.Wrap(err, "failed to encode portable OCI addon catalog")
	}

	catalogRepo, _ := ociRepoRef(source.URL, ociCatalogChartName)
	catalogTags, tagErr := catalogRepoTagsFn(catalogRepo, host, source.Username, source.Token, plainHTTP)
	if tagErr != nil && !isOCIRepositoryAbsentError(tagErr) {
		// A failed listing is not a confirmed-empty catalog. NAME_UNKNOWN is the
		// one exception: it is the registry stating the catalog repository does
		// not exist yet, which is the normal, expected state on the very first
		// push to a registry -- confirmPortableCatalogAbsent already reached the
		// same conclusion for `existing` above, so treating it as anything but
		// "no tags" here would retry every attempt and never bootstrap the
		// catalog. Any other failure genuinely cannot confirm the current tag;
		// defaulting to "0.0.1" for those would either collide with a version
		// that already exists or publish one older than what is already there,
		// so those retry instead of guessing.
		return true, errors.Wrap(tagErr, "cannot confirm the portable OCI addon catalog's current tag")
	}
	catalogVersion := "0.0.1"
	if len(catalogTags) > 0 {
		current, parseErr := semver.NewVersion(catalogTags[0])
		if parseErr != nil {
			return false, errors.Wrap(parseErr, "failed to parse portable OCI catalog version")
		}
		next := current.IncPatch()
		catalogVersion = next.String()
	}

	catalogChart := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion:  chart.APIVersionV2,
			Name:        ociCatalogChartName,
			Version:     catalogVersion,
			Description: "KubeVela portable OCI addon catalog",
			Type:        "library",
		},
		Files: []*chart.File{{
			Name: ociCatalogFileName,
			Data: catalogData,
		}},
	}
	tmp, err := os.MkdirTemp("", "kubevela-oci-catalog-")
	if err != nil {
		return false, err
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	archivePath, err := chartutil.Save(catalogChart, tmp)
	if err != nil {
		return false, errors.Wrap(err, "failed to package portable OCI addon catalog")
	}
	archive, err := os.ReadFile(filepath.Clean(archivePath))
	if err != nil {
		return false, errors.Wrap(err, "failed to read portable OCI addon catalog package")
	}

	// Re-check the catalog tag right before publishing. A failed listing here
	// is treated the same as a changed tag: neither lets this attempt confirm
	// it is safe to publish, so both retry from a fresh read rather than risk
	// overwriting a catalog this attempt can no longer vouch for.
	latestTags, latestErr := catalogRepoTagsFn(catalogRepo, host, source.Username, source.Token, plainHTTP)
	if latestErr != nil && !isOCIRepositoryAbsentError(latestErr) {
		return true, errors.Wrap(latestErr, "cannot confirm the portable OCI addon catalog tag is still unchanged before publishing")
	}
	if catalogTagHead(catalogTags) != catalogTagHead(latestTags) {
		return true, errors.Errorf("a concurrent publisher updated the portable OCI addon catalog while this attempt was preparing %s", catalogVersion)
	}

	if _, err := client.Push(archive, catalogRepo+":"+catalogVersion); err != nil {
		return false, errors.Wrap(err, "failed to push portable OCI addon catalog")
	}
	return false, nil
}

// catalogTagHead returns the highest catalog tag seen, or "" if none. A
// listing error is the caller's responsibility to handle before reaching
// here -- conflating "failed to list" with "confirmed empty" would let an
// unreadable catalog look identical to an unchanged one and publish over it.
func catalogTagHead(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}
