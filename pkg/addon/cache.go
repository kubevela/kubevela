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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
)

// We have three addon layer here
// 1. List metadata for all file path of one registry
// 2. UIData: Read file content that including README.md and other necessary things being used in UI apiserver
// 3. InstallPackage: All file content that used to be installation

// Cache package only cache for 1 and 2, we don't cache InstallPackage, and it only read for real installation
type Cache struct {

	// uiData caches all the decoded UIData addons
	// the key in the map is the registry name
	uiData map[string][]*UIData

	// registryMeta caches the addon metadata of every registry
	// the key in the map is the registry name
	registryMeta map[string]map[string]SourceMeta

	registry map[string]Registry

	mutex *sync.RWMutex

	ds RegistryDataStore
}

// NewCache will build a new cache instance
func NewCache(ds RegistryDataStore) *Cache {
	return &Cache{
		uiData:       make(map[string][]*UIData),
		registryMeta: make(map[string]map[string]SourceMeta),
		registry:     make(map[string]Registry),
		mutex:        new(sync.RWMutex),
		ds:           ds,
	}
}

// DiscoverAndRefreshLoop will run a loop to automatically discovery and refresh addons from registry
func (u *Cache) DiscoverAndRefreshLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// This is infinite loop, we can receive a channel for close
	for ; true; <-ticker.C {
		u.discoverAndRefreshRegistry()
	}
}

// ListAddonMeta will list metadata from registry, if cache not found, it will find from source
func (u *Cache) ListAddonMeta(r *Registry) (map[string]SourceMeta, error) {
	registryMeta := u.getCachedRegistryMeta(r.Name)
	if registryMeta == nil {
		return r.ListAddonMeta()
	}
	return registryMeta, nil
}

// GetAddonUIData get addon data for UI display from cache, if cache not found, it will find from source
func (u *Cache) GetAddonUIData(r Registry, registry, addonName string) (*UIData, error) {
	addon := u.getAddonFromCache(registry, addonName)
	if addon != nil {
		return addon, nil
	}
	var err error
	registryMeta, err := u.ListAddonMeta(&r)
	if err != nil {
		return nil, err
	}
	meta, ok := registryMeta[addonName]
	if !ok {
		return nil, ErrNotExist
	}
	return r.GetUIData(&meta, UIMetaOptions)
}

func (u *Cache) getAddonFromCache(registry, addonName string) *UIData {
	addons := u.getCachedUIDataFromRegistry(registry)
	for _, a := range addons {
		if a.Name == addonName {
			return a
		}
	}
	return nil
}

// GetAddonsFromRegistry will always get addons from cache first, if not exist, read from source.
func (u *Cache) GetAddonsFromRegistry(r Registry) ([]*UIData, error) {
	var err error
	listAddons := u.getCachedUIDataFromRegistry(r.Name)
	if listAddons != nil {
		return listAddons, nil
	}
	registryMeta, err := u.ListAddonMeta(&r)
	if err != nil {
		return nil, err
	}
	listAddons, err = r.ListUIData(registryMeta, UIMetaOptions)
	if err != nil {
		return nil, fmt.Errorf("fail to get addons from registry %s, %w", r.Name, err)
	}
	u.putAddonUIMeta2Cache(r.Name, listAddons)
	return listAddons, nil
}

// getCachedUIDataFromRegistry will get cached addons from specified registry in cache
func (u *Cache) getCachedUIDataFromRegistry(name string) []*UIData {
	if u == nil {
		return nil
	}
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	d, ok := u.uiData[name]
	if !ok {
		return nil
	}
	return d
}

// getCachedRegistryMeta will get cached registry meta from specified registry in cache
func (u *Cache) getCachedRegistryMeta(name string) map[string]SourceMeta {
	if u == nil {
		return nil
	}
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	d, ok := u.registryMeta[name]
	if !ok {
		return nil
	}
	return d
}

func (u *Cache) putAddonUIMeta2Cache(name string, addons []*UIData) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.uiData[name] = addons
}

func (u *Cache) putAddonRegistryMeta2Cache(name string, addonMeta map[string]SourceMeta) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.registryMeta[name] = addonMeta
}

func (u *Cache) putAddonRegistry2Cache(registry []Registry) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()
	for k := range u.registry {
		var found = false
		for _, r := range registry {
			if r.Name == k {
				found = true
				break
			}
		}
		// clean deleted registry
		if !found {
			delete(u.registry, k)
			delete(u.registryMeta, k)
			delete(u.uiData, k)
		}
	}
	for _, r := range registry {
		u.registry[r.Name] = r
	}
}

func (u *Cache) discoverAndRefreshRegistry() {
	registries, err := u.ds.ListRegistries(context.Background())
	if err != nil {
		log.Logger.Errorf("fail to get registry %v", err)
		return
	}
	u.putAddonRegistry2Cache(registries)

	for _, r := range registries {
		registryMeta, err := r.ListAddonMeta()
		if err != nil {
			log.Logger.Errorf("fail to list registry %s metadata,  %v", r.Name, err)
			continue
		}
		u.putAddonRegistryMeta2Cache(r.Name, registryMeta)
		uiData, err := r.ListUIData(registryMeta, UIMetaOptions)
		if err != nil {
			log.Logger.Errorf("fail to get addons from registry %s for cache updating, %v", r.Name, err)
			continue
		}
		u.putAddonUIMeta2Cache(r.Name, uiData)
	}
}
