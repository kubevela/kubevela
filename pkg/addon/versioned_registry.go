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
	"regexp"

	"sort"

	"github.com/Masterminds/semver/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/repo"
)

// VersionedRegistry is the interface of support version registry
type VersionedRegistry interface {
	ListAddon() ([]*UIData, error)
	GetAddonUIData(ctx context.Context, addonName, version string) (*UIData, error)
	GetAddonInstallPackage(ctx context.Context, addonName, version string) (*InstallPackage, error)
	GetDetailedAddon(ctx context.Context, addonName, version string) (*WholeAddonPackage, error)
	GetAddonAvailableVersion(addonName string) ([]*repo.ChartVersion, error)
}

// BuildVersionedRegistry is build versioned addon registry
func BuildVersionedRegistry(name, repoURL string, opts *common.HTTPOption) VersionedRegistry {
	return &versionedRegistry{
		name: name,
		url:  repoURL,
		h:    helm.NewHelperWithCache(),
		Opts: opts,
	}
}

type versionedRegistry struct {
	url  string
	name string
	h    *helm.Helper
	// username and password for registry needs basic auth
	Opts *common.HTTPOption
}

func (i *versionedRegistry) ListAddon() ([]*UIData, error) {
	chartIndex, err := i.h.GetIndexInfo(i.url, false, i.Opts)
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

func (i *versionedRegistry) GetDetailedAddon(ctx context.Context, addonName, version string) (*WholeAddonPackage, error) {
	wholePackage, err := i.loadAddon(ctx, addonName, version)
	if err != nil {
		return nil, err
	}
	return wholePackage, nil
}

func (i versionedRegistry) GetAddonAvailableVersion(addonName string) ([]*repo.ChartVersion, error) {
	return i.loadAddonVersions(addonName)
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
	versions, err := i.h.ListVersions(i.url, name, false, i.Opts)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, ErrNotExist
	}
	sort.Sort(sort.Reverse(versions))
	addonVersion, availableVersions := chooseVersion(version, versions)
	if addonVersion == nil {
		return nil, fmt.Errorf("specified version %s not exist", version)
	}
	for _, chartURL := range addonVersion.URLs {
		if !utils.IsValidURL(chartURL) {
			chartURL, err = utils.JoinURL(i.url, chartURL)
			if err != nil {
				return nil, fmt.Errorf("cannot join versionedRegistryURL %s and chartURL %s, %w", i.url, chartURL, err)
			}
		}
		archive, err := common.HTTPGetWithOption(ctx, chartURL, i.Opts)
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
		addonPkg.RegistryName = i.name
		addonPkg.Meta.SystemRequirements = LoadSystemRequirements(addonVersion.Annotations["system"])
		return addonPkg, nil
	}
	return nil, fmt.Errorf("cannot fetch addon package")
}

// loadAddonVersions Load all available versions of the addon
func (i versionedRegistry) loadAddonVersions(addonName string) ([]*repo.ChartVersion, error) {
	versions, err := i.h.ListVersions(i.url, addonName, false, i.Opts)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, ErrNotExist
	}
	sort.Sort(sort.Reverse(versions))
	return versions, nil
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

// chooseVersion will return the target version and all available versions
func chooseVersion(specifiedVersion string, versions []*repo.ChartVersion) (*repo.ChartVersion, []string) {
	var addonVersion *repo.ChartVersion
	var availableVersions []string
	for i, v := range versions {
		availableVersions = append(availableVersions, v.Version)
		if addonVersion != nil {
			// already find the latest not-prerelease version, skip the find
			continue
		}
		if len(specifiedVersion) != 0 {
			if v.Version == specifiedVersion {
				addonVersion = versions[i]
			}
		} else {
			vv, err := semver.NewVersion(v.Version)
			if err != nil {
				continue
			}
			if len(vv.Prerelease()) != 0 {
				continue
			}
			addonVersion = v
			log.Logger.Infof("Not specified any version, so use the latest version %s", v.Version)
		}
	}
	return addonVersion, availableVersions
}

// LoadSystemRequirements load the system version requirements from the addon's meta file
func LoadSystemRequirements(requirements string) *SystemRequirements {
	if len(requirements) == 0 {
		return nil
	}
	patternReq := `vela(.*\d+\.\d+\.\d+);\s?kubernetes(.*\d+\.\d+\.\d+)`
	regexReq := regexp.MustCompile(patternReq)
	matched := regexReq.FindStringSubmatch(requirements)
	if len(matched) < 2 {
		return nil
	}
	velaReq, k8sReq := matched[1], matched[2]
	req := &SystemRequirements{
		VelaVersion:       velaReq,
		KubernetesVersion: k8sReq,
	}
	return req
}
