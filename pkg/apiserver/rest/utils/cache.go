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

package utils

import (
	"context"
	"sync"
	"time"
)

// memoryCache memory cache, support time expired
type memoryCache struct {
	data          interface{}
	cacheDuration time.Duration
	startTime     time.Time
}

// NewMemoryCache new memory cache instance
func newMemoryCache(data interface{}, cacheDuration time.Duration) *memoryCache {
	mc := &memoryCache{data: data, cacheDuration: cacheDuration, startTime: time.Now()}
	return mc
}

// IsExpired whether the cache data expires
func (m *memoryCache) IsExpired() bool {
	if m.cacheDuration <= 0 {
		return false
	}
	return time.Now().After(m.startTime.Add(m.cacheDuration))
}

// GetData get cache data
func (m *memoryCache) GetData() interface{} {
	return m.data
}

// MemoryCacheStore a sample memory cache instance, if data set cache duration, will auto clear after timeout.
// But, Expired cleanup is not necessarily accurate, it has a 3-second window.
type MemoryCacheStore struct {
	store sync.Map
}

// NewMemoryCacheStore memory cache store
func NewMemoryCacheStore(ctx context.Context) *MemoryCacheStore {
	mcs := &MemoryCacheStore{
		store: sync.Map{},
	}
	go mcs.run(ctx)
	return mcs
}

func (m *MemoryCacheStore) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.store.Range(func(key, value interface{}) bool {
				if value.(*memoryCache).IsExpired() {
					m.store.Delete(key)
				}
				return true
			})
		}
	}
}

// Put cache data, if cacheDuration>0, store will clear data after timeout.
func (m *MemoryCacheStore) Put(key, value interface{}, cacheDuration time.Duration) {
	mc := newMemoryCache(value, cacheDuration)
	m.store.Store(key, mc)
}

// Delete cache data from store
func (m *MemoryCacheStore) Delete(key interface{}) {
	m.store.Delete(key)
}

// Get cache data from store, if not exist or timeout, will return nil
func (m *MemoryCacheStore) Get(key interface{}) (value interface{}) {
	mc, ok := m.store.Load(key)
	if ok && !mc.(*memoryCache).IsExpired() {
		return mc.(*memoryCache).GetData()
	}
	return nil
}
