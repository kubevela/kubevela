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
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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

	versionedUIData map[string]*LRUCache

	mutex *sync.RWMutex

	ds RegistryDataStore
}

// NewCache will build a new cache instance
func NewCache(ds RegistryDataStore) *Cache {
	return &Cache{
		uiData:          make(map[string][]*UIData),
		registryMeta:    make(map[string]map[string]SourceMeta),
		registry:        make(map[string]Registry),
		versionedUIData: make(map[string]*LRUCache),
		mutex:           new(sync.RWMutex),
		ds:              ds,
	}
}

// DiscoverAndRefreshLoop will run a loop to automatically discovery and refresh addons from registry
func (u *Cache) DiscoverAndRefreshLoop(ctx context.Context, cacheTime time.Duration) {
	ticker := time.NewTicker(cacheTime)
	defer ticker.Stop()

	// This is infinite loop, we can receive a channel for close
	for {
		select {
		case <-ticker.C:
			u.discoverAndRefreshRegistry()
		case <-ctx.Done():
			return
		}
	}
}

// ListAddonMeta will list metadata from registry, if cache not found, it will find from source
func (u *Cache) ListAddonMeta(r Registry) (map[string]SourceMeta, error) {
	registryMeta := u.getCachedAddonMeta(r.Name)
	if registryMeta == nil {
		return r.ListAddonMeta()
	}
	return registryMeta, nil
}

// GetUIData get addon data for UI display from cache, if cache not found, it will find from source
func (u *Cache) GetUIData(r Registry, addonName, version string) (*UIData, error) {
	addon := u.getCachedUIData(r, addonName, version)
	if addon != nil {
		return addon, nil
	}
	var err error
	if !IsVersionRegistry(r) {
		registryMeta, err := u.ListAddonMeta(r)
		if err != nil {
			return nil, err
		}
		meta, ok := registryMeta[addonName]
		if !ok {
			return nil, ErrNotExist
		}
		addon, err = r.GetUIData(&meta, UIMetaOptions)
		if err != nil {
			return nil, err
		}
	} else {
		versionedRegistry := BuildVersionedRegistry(r.Name, r.Helm.URL, &common.HTTPOption{
			Username:        r.Helm.Username,
			Password:        r.Helm.Password,
			InsecureSkipTLS: r.Helm.InsecureSkipTLS,
		})
		addon, err = versionedRegistry.GetAddonUIData(context.Background(), addonName, version)
		if err != nil {
			klog.Errorf("fail to get addons from registry %s for cache updating, %v", utils.Sanitize(r.Name), err)
			return nil, err
		}
	}

	return addon, nil
}

// ListUIData will always list UIData from cache first, if not exist, read from source.
func (u *Cache) ListUIData(r Registry) ([]*UIData, error) {
	var err error
	var listAddons []*UIData
	if !IsVersionRegistry(r) {
		listAddons = u.listCachedUIData(r.Name)
		if listAddons != nil {
			return listAddons, nil
		}
		listAddons, err = u.listUIDataAndCache(r)
		if err != nil {
			return nil, err
		}
	} else {
		listAddons = u.listVersionRegistryCachedUIData(r.Name)
		if listAddons != nil {
			return listAddons, nil
		}
		listAddons, err = u.listVersionRegistryUIDataAndCache(r)
		if err != nil {
			return nil, err
		}
	}

	return listAddons, nil
}

func (u *Cache) getCachedUIData(registry Registry, addonName, version string) *UIData {
	if !IsVersionRegistry(registry) {
		addons := u.listCachedUIData(registry.Name)
		for _, a := range addons {
			if a.Name == addonName {
				return a
			}
		}
	} else {
		if len(version) == 0 {
			version = "latest"
		}
		if lru, ok := u.versionedUIData[registry.Name]; ok {
			val, _ := lru.get(fmt.Sprintf("%s-%s", addonName, version))
			return val
		}
		return nil
	}
	return nil
}

// listCachedUIData will get cached addons from specified registry in cache
func (u *Cache) listCachedUIData(name string) []*UIData {
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

// listVersionRegistryCachedUIData will get cached addons from specified VersionRegistry in cache
func (u *Cache) listVersionRegistryCachedUIData(name string) []*UIData {
	if u == nil {
		return nil
	}
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	lru, ok := u.versionedUIData[name]
	if !ok {
		return nil
	}
	var uiDatas []*UIData
	for _, k := range lru.keys() {
		if !strings.Contains(k, "-latest") {
			if val, ok := lru.get(k); ok {
				uiDatas = append(uiDatas, val)
			}
		}
	}

	return uiDatas
}

// getCachedAddonMeta will get cached registry meta from specified registry in cache
func (u *Cache) getCachedAddonMeta(name string) map[string]SourceMeta {
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

func (u *Cache) putAddonUIData2Cache(name string, addons []*UIData) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.uiData[name] = addons
}

func (u *Cache) putAddonMeta2Cache(name string, addonMeta map[string]SourceMeta) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.registryMeta[name] = addonMeta
}

func (u *Cache) putRegistry2Cache(registry []Registry) {
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

func (u *Cache) putVersionedUIData2Cache(registryName, addonName, version string, uiData *UIData) {
	if u == nil {
		return
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()

	if u.versionedUIData[registryName] == nil {
		u.versionedUIData[registryName] = newLRUCache(100)
	}
	u.versionedUIData[registryName].put(fmt.Sprintf("%s-%s", addonName, version), uiData)
}

func (u *Cache) discoverAndRefreshRegistry() {
	registries, err := u.ds.ListRegistries(context.Background())
	if err != nil {
		klog.Errorf("fail to get registry %v", err)
		return
	}
	u.putRegistry2Cache(registries)

	for _, r := range registries {
		if !IsVersionRegistry(r) {
			_, err = u.listUIDataAndCache(r)
			if err != nil {
				continue
			}
		} else {
			_, err = u.listVersionRegistryUIDataAndCache(r)
			if err != nil {
				continue
			}
		}
	}
}

func (u *Cache) listUIDataAndCache(r Registry) ([]*UIData, error) {
	registryMeta, err := r.ListAddonMeta()
	if err != nil {
		klog.Errorf("fail to list registry %s metadata,  %v", r.Name, err)
		return nil, err
	}
	u.putAddonMeta2Cache(r.Name, registryMeta)
	uiData, err := r.ListUIData(registryMeta, UIMetaOptions)
	if err != nil {
		klog.Errorf("fail to get addons from registry %s for cache updating, %v", r.Name, err)
		return nil, err
	}
	u.putAddonUIData2Cache(r.Name, uiData)
	return uiData, nil
}

func (u *Cache) listVersionRegistryUIDataAndCache(r Registry) ([]*UIData, error) {
	versionedRegistry := BuildVersionedRegistry(r.Name, r.Helm.URL, &common.HTTPOption{
		Username:        r.Helm.Username,
		Password:        r.Helm.Password,
		InsecureSkipTLS: r.Helm.InsecureSkipTLS,
	})
	uiDatas, err := versionedRegistry.ListAddon()
	if err != nil {
		klog.Errorf("fail to get addons from registry %s for cache updating, %v", r.Name, err)
		return nil, err
	}
	for _, addon := range uiDatas {
		uiData, err := versionedRegistry.GetAddonUIData(context.Background(), addon.Name, addon.Version)
		if err != nil {
			klog.Errorf("fail to get addon from versioned registry %s, addon %s version %s for cache updating, %v", r.Name, addon.Name, addon.Version, err)
			continue
		}
		// identity an addon from helm chart structure
		if uiData.Name == "" {
			addon.Name = ""
			continue
		}
		u.putVersionedUIData2Cache(r.Name, addon.Name, addon.Version, uiData)
		// we also no version key, if use get addonUIData without version will return this vale as latest data.
		u.putVersionedUIData2Cache(r.Name, addon.Name, "latest", uiData)
	}
	// delete the addon which has been deleted from the addonRegistryCache
	if lru, ok := u.versionedUIData[r.Name]; ok {
		for _, k := range lru.keys() {
			lastInd := strings.LastIndex(k, "-")
			var needDelete = true
			for _, addon := range uiDatas {
				if k[:lastInd] == addon.Name {
					needDelete = false
					break
				}
			}
			if needDelete {
				lru.delete(k)
			}
		}
	}
	return uiDatas, nil
}

// lruEntry holds the key and value for LRU eviction
type lruEntry struct {
	key   string
	value *UIData
}

// LRUCache is a thread-safe LRU cache for versioned UIData
type LRUCache struct {
	capacity int
	list     *list.List
	items    map[string]*list.Element
}

// newLRUCache creates a new LRUCache with given capacity
func newLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		list:     list.New(),
		items:    make(map[string]*list.Element),
	}
}

// get retrieves a value and marks it as recently used
func (c *LRUCache) get(key string) (*UIData, bool) {
	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		return el.Value.(*lruEntry).value, true
	}
	return nil, false
}

// put inserts or updates a value, evicting LRU entry if at capacity
func (c *LRUCache) put(key string, value *UIData) {
	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		el.Value.(*lruEntry).value = value
		return
	}
	if c.list.Len() >= c.capacity {
		oldest := c.list.Back()
		if oldest != nil {
			c.list.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry).key)
		}
	}
	el := c.list.PushFront(&lruEntry{key: key, value: value})
	c.items[key] = el
}

// delete removes a key from the cache
func (c *LRUCache) delete(key string) {
	if el, ok := c.items[key]; ok {
		c.list.Remove(el)
		delete(c.items, key)
	}
}

// keys returns all keys currently in cache
func (c *LRUCache) keys() []string {
	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	return keys
}
