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
func listPortableOCICatalog(ctx context.Context, registryURL, username, password string) ([]*UIData, error) {
	repoRef, host := ociRepoRef(registryURL, ociCatalogChartName)
	tags, err := listOCITags(ctx, repoRef, host, username, password)
	if err != nil {
		return nil, errors.Wrap(err, "portable OCI addon catalog is unavailable")
	}
	if len(tags) == 0 {
		return nil, errors.New("portable OCI addon catalog has no semver tags")
	}
	archive, err := pullOCIChart(ctx, repoRef+":"+tags[0], host, username, password)
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
		addons = append(addons, &UIData{
			Meta: Meta{
				Name:        entry.Name,
				Description: entry.Description,
			},
			AvailableVersions: entry.Versions,
		})
	}
	sort.Slice(addons, func(a, b int) bool {
		return addons[a].Name < addons[b].Name
	})
	return addons, nil
}

// updateOCIAddonCatalog upserts an addon after it has been pushed and publishes
// a new catalog chart version. The fixed catalog repository makes discovery
// portable across OCI registries.
func updateOCIAddonCatalog(ctx context.Context, client *registry.Client, source *OCIAddonSource, addonMeta *chart.Metadata) error {
	reader := &ociRegistry{
		url:            source.URL,
		username:       source.Username,
		token:          source.Token,
		pullFn:         pullOCIChart,
		tagsFn:         listOCITags,
		catalogFn:      listOCIRepositories,
		catalogIndexFn: listPortableOCICatalog,
	}
	existing, err := reader.ListAddon()
	if err != nil {
		// A registry with no portable catalog and no repository enumeration can
		// still bootstrap a catalog with the addon currently being pushed.
		existing = nil
	}

	addonRepo, host := ociRepoRef(source.URL, addonMeta.Name)
	versions, err := listOCITags(ctx, addonRepo, host, source.Username, source.Token)
	if err != nil {
		return errors.Wrapf(err, "failed to list versions for OCI addon %s", addonMeta.Name)
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
		return errors.Wrap(err, "failed to encode portable OCI addon catalog")
	}

	catalogRepo, _ := ociRepoRef(source.URL, ociCatalogChartName)
	catalogVersion := "0.0.1"
	catalogTags, tagErr := listOCITags(ctx, catalogRepo, host, source.Username, source.Token)
	if tagErr == nil && len(catalogTags) > 0 {
		current, parseErr := semver.NewVersion(catalogTags[0])
		if parseErr != nil {
			return errors.Wrap(parseErr, "failed to parse portable OCI catalog version")
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
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()
	archivePath, err := chartutil.Save(catalogChart, tmp)
	if err != nil {
		return errors.Wrap(err, "failed to package portable OCI addon catalog")
	}
	archive, err := os.ReadFile(filepath.Clean(archivePath))
	if err != nil {
		return errors.Wrap(err, "failed to read portable OCI addon catalog package")
	}
	if _, err := client.Push(archive, catalogRepo+":"+catalogVersion); err != nil {
		return errors.Wrap(err, "failed to push portable OCI addon catalog")
	}
	return nil
}
