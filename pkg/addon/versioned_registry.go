/*
Copyright 2021 The KubeVela Authors.

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
	"fmt"
	"sort"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

// VersionedRegistry is the interface of support version registry
type VersionedRegistry interface {
	ListAddon() ([]*UIData, error)
	GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error)
	GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error)
}

// BuildVersionedRegistry is build versioned addon registry
func BuildVersionedRegistry(name, repoURL string) VersionedRegistry {
	return &versionedRegistry{
		name: name,
		url:  repoURL,
		h:    helm.NewHelperWithCache(),
	}
}

type versionedRegistry struct {
	url  string
	name string
	h    *helm.Helper
}

func (i *versionedRegistry) ListAddon() ([]*UIData, error) {
	chartIndex, err := i.h.GetIndexInfo(i.url, false, nil)
	if err != nil {
		return nil, err
	}
	return i.resolveAddonListFromIndex(i.name, chartIndex), nil
}

func (i *versionedRegistry) GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error) {
	wholePackage, err := i.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return &UIData{
		Meta:              wholePackage.Meta,
		APISchema:         wholePackage.APISchema,
		Parameters:        wholePackage.Parameters,
		Detail:            wholePackage.Detail,
		Definitions:       wholePackage.Definitions,
		AvailableVersions: wholePackage.AvailableVersions,
	}, nil
}

func (i *versionedRegistry) GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error) {
	wholePackage, err := i.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return &wholePackage.InstallPackage, nil
}

func (i *versionedRegistry) resolveAddonListFromIndex(repoName string, index *repo.IndexFile) []*UIData {
	var res []*UIData
	for addonName, versions := range index.Entries {
		if len(versions) == 0 {
			continue
		}
		sort.Sort(sort.Reverse(versions))
		latestVersion := versions[0]
		var availableVersions []string
		for _, version := range versions {
			availableVersions = append(availableVersions, version.Version)
		}
		o := UIData{Meta: Meta{
			Name:        addonName,
			Icon:        latestVersion.Icon,
			Tags:        latestVersion.Keywords,
			Description: latestVersion.Description,
			Version:     latestVersion.Version,
		}, RegistryName: repoName, AvailableVersions: availableVersions}
		res = append(res, &o)
	}
	return res
}

func (i versionedRegistry) loadAddon(ctx context.Context, name, version string) (*WholeAddonPackage, error) {
	versions, err := i.h.ListVersions(i.url, name, false, nil)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, ErrNotExist
	}
	var addonVersion *repo.ChartVersion
	sort.Sort(sort.Reverse(versions))
	if len(version) == 0 {
		// if not specify version will always use the latest version
		addonVersion = versions[0]
	}
	var availableVersions []string
	for i, v := range versions {
		availableVersions = append(availableVersions, v.Version)
		if v.Version == version {
			addonVersion = versions[i]
		}
	}
	if addonVersion == nil {
		return nil, fmt.Errorf("specified version %s not exist", version)
	}
	for _, chartURL := range addonVersion.URLs {
		archive, err := common.HTTPGetWithOption(ctx, chartURL, nil)
		if err != nil {
			continue
		}
		bufferedFile, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
		if err != nil {
			continue
		}
		addonPkg, err := loadAddonPackage(name, bufferedFile)
		if err != nil {
			return nil, err
		}
		addonPkg.AvailableVersions = availableVersions
		return addonPkg, nil
	}
	return nil, fmt.Errorf("cannot fetch addon package")
}

func loadAddonPackage(addonName string, files []*loader.BufferedFile) (*WholeAddonPackage, error) {
	mr := MemoryReader{Name: addonName, Files: files}
	metas, err := mr.ListAddonMeta()
	if err != nil {
		return nil, err
	}
	meta := metas[addonName]
	addonUIData, err := GetUIDataFromReader(&mr, &meta, UIMetaOptions)
	if err != nil {
		return nil, err
	}
	installPackage, err := GetInstallPackageFromReader(&mr, &meta, addonUIData)
	if err != nil {
		return nil, err
	}
	return &WholeAddonPackage{
		InstallPackage: *installPackage,
		Detail:         addonUIData.Detail,
		APISchema:      addonUIData.APISchema,
	}, nil
}
