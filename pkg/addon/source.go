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

// ClassifyItemByPattern will filter and classify addon data, data will be classified by pattern it meets
func ClassifyItemByPattern(meta *SourceMeta, r AsyncReader) map[string][]Item {
	var p = make(map[string][]Item)
	for _, it := range meta.Items {
		pt := GetPatternFromItem(it, r, meta.Name)
		if pt == "" {
			continue
		}
		items := p[pt]
		items = append(items, it)
		p[pt] = items
	}
	return p
}

// GetUIData get UIData of an addon from a registry
func GetUIData(r *Registry, meta *SourceMeta, opt ListOptions) (*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	addon, err := GetUIDataFromReader(reader, meta, opt)
	if err != nil {
		return nil, err
	}
	if len(addon.GlobalParameters) != 0 {
		addon.Parameters = addon.GlobalParameters
	}
	addon.RegistryName = r.Name
	return addon, nil
}

// ListUIData list UI data from addon registry
func ListUIData(r *Registry, registryAddonMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return ListAddonUIDataFromReader(reader, registryAddonMeta, r.Name, opt)
}

// GetInstallPackage get install package which is all needed to enable an addon from addon registry
func GetInstallPackage(r *Registry, meta *SourceMeta, uiData *UIData) (*InstallPackage, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return GetInstallPackageFromReader(reader, meta, uiData)
}

// ItemInfo contains summary information about an addon
type ItemInfo struct {
	Name              string
	Description       string
	AvailableVersions []string
}

type itemInfoMap map[string]ItemInfo

// ListAddonInfo lists addon info (name, versions, etc.) from a registry
func ListAddonInfo(r *Registry) (map[string]ItemInfo, error) {
	addonInfoMap := make(map[string]ItemInfo)

	// local registry doesn't support listing addons
	if IsLocalRegistry(*r) {
		return addonInfoMap, nil
	}
	if IsVersionRegistry(*r) {
		versionedRegistry, err := ToVersionedRegistry(*r)
		if err != nil {
			return nil, err
		}
		addonList, err := versionedRegistry.ListAddon()
		if err != nil {
			return nil, err
		}
		for _, a := range addonList {
			addonInfoMap[a.Name] = ItemInfo{
				Name:              a.Name,
				Description:       a.Description,
				AvailableVersions: a.AvailableVersions,
			}
		}
	} else {
		meta, err := r.ListAddonMeta()
		if err != nil {
			return nil, err
		}
		addonList, err := ListUIData(r, meta, ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, a := range addonList {
			addonInfoMap[a.Name] = ItemInfo{
				Name:              a.Name,
				Description:       a.Description,
				AvailableVersions: a.AvailableVersions,
			}
		}
	}

	return addonInfoMap, nil
}

// registryLister adapts a Registry to ItemInfoLister. The listing entry points
// became package functions when Registry moved to pkg/registry/component, so a
// registry reaches the interface through this wrapper rather than directly.
type registryLister struct{ r *Registry }

// ListAddonInfo satisfies ItemInfoLister.
func (l registryLister) ListAddonInfo() (map[string]ItemInfo, error) { return ListAddonInfo(l.r) }
