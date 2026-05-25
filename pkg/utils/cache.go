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
	"container/list"
	"context"
	"sync"
	"time"
)

type memoryCache struct {
	data          interface{}
	cacheDuration time.Duration
	startTime     time.Time
}

func newMemoryCache(data interface{}, cacheDuration time.Duration) *memoryCache {
	return &memoryCache{data: data, cacheDuration: cacheDuration, startTime: time.Now()}
}

func (m *memoryCache) IsExpired() bool {
	if m.cacheDuration <= 0 {
		return false
	}
	return time.Now().After(m.startTime.Add(m.cacheDuration))
}

func (m *memoryCache) GetData() interface{} {
	return m.data
}

// MemoryCacheStore is a TTL-based memory cache. Expired cleanup has a 3-second window.
// Use NewMemoryCacheStoreWithMaxSize to bound memory via LRU eviction.
type MemoryCacheStore struct {
	mu      sync.Mutex
	items   map[interface{}]*list.Element
	lru     *list.List
	maxSize int
}

type cacheEntry struct {
	key   interface{}
	value *memoryCache
}

// NewMemoryCacheStore creates an unbounded cache (backward compatible).
func NewMemoryCacheStore(ctx context.Context) *MemoryCacheStore {
	mcs := &MemoryCacheStore{
		items:   make(map[interface{}]*list.Element),
		lru:     list.New(),
		maxSize: 0,
	}
	go mcs.run(ctx)
	return mcs
}

// NewMemoryCacheStoreWithMaxSize creates a cache that evicts the least-recently-used
// entry when the number of items exceeds maxSize. maxSize must be > 0.
func NewMemoryCacheStoreWithMaxSize(ctx context.Context, maxSize int) *MemoryCacheStore {
	if maxSize <= 0 {
		return NewMemoryCacheStore(ctx)
	}
	mcs := &MemoryCacheStore{
		items:   make(map[interface{}]*list.Element),
		lru:     list.New(),
		maxSize: maxSize,
	}
	go mcs.run(ctx)
	return mcs
}

func (m *MemoryCacheStore) run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			for key, el := range m.items {
				if el.Value.(*cacheEntry).value.IsExpired() {
					m.lru.Remove(el)
					delete(m.items, key)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Put stores a value. If maxSize is set and the cache is full, the least-recently-used
// entry is evicted first.
func (m *MemoryCacheStore) Put(key, value interface{}, cacheDuration time.Duration) {
	mc := newMemoryCache(value, cacheDuration)
	m.mu.Lock()
	defer m.mu.Unlock()

	if el, ok := m.items[key]; ok {
		m.lru.MoveToFront(el)
		el.Value.(*cacheEntry).value = mc
		return
	}

	if m.maxSize > 0 && m.lru.Len() >= m.maxSize {
		oldest := m.lru.Back()
		if oldest != nil {
			m.lru.Remove(oldest)
			delete(m.items, oldest.Value.(*cacheEntry).key)
		}
	}

	el := m.lru.PushFront(&cacheEntry{key: key, value: mc})
	m.items[key] = el
}

// Delete removes a key from the cache.
func (m *MemoryCacheStore) Delete(key interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[key]; ok {
		m.lru.Remove(el)
		delete(m.items, key)
	}
}

// Get returns the cached value, or nil if missing or expired.
// A cache hit moves the entry to the front of the LRU list.
func (m *MemoryCacheStore) Get(key interface{}) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[key]
	if !ok {
		return nil
	}
	entry := el.Value.(*cacheEntry)
	if entry.value.IsExpired() {
		m.lru.Remove(el)
		delete(m.items, key)
		return nil
	}
	m.lru.MoveToFront(el)
	return entry.value.GetData()
}
