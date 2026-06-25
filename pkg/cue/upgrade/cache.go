/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package upgrade

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CompatibilityCacheSize is the maximum number of cache entries. Default 2000
// Overridden at startup via --cue-compatibility-cache-size.
var CompatibilityCacheSize = 2000

// compatCache stores the result of each template compatibility check, keyed by SHA-256 of the raw template.
var compatCache atomic.Pointer[lruCache]

// compatCacheCancel cancels the eviction goroutine of the current compatCache instance.
var compatCacheCancel context.CancelFunc

// compatCacheMu serialises concurrent calls to InitCompatibilityCache.
var compatCacheMu sync.Mutex

func init() {
	compatCache.Store(newLRUCache(CompatibilityCacheSize))
}

// CacheEntryTTL is how long an unaccessed entry lives before being swept.
var CacheEntryTTL = 1 * time.Hour

// InitCompatibilityCache reinitialises the cache with the given size and starts background TTL eviction.
// Safe to call multiple times (e.g. in tests): the previous eviction goroutine is stopped before the
// new cache is installed.
func InitCompatibilityCache(ctx context.Context, size int) {
	compatCacheMu.Lock()
	defer compatCacheMu.Unlock()
	if compatCacheCancel != nil {
		compatCacheCancel()
	}
	CompatibilityCacheSize = size
	c := newLRUCache(CompatibilityCacheSize)
	if size > 0 {
		cacheCtx, cancel := context.WithCancel(ctx)
		compatCacheCancel = cancel
		c.startEvictionLoop(cacheCtx)
	} else {
		compatCacheCancel = nil
	}
	compatCache.Store(c)
}

// templateHash returns the SHA-256 hex digest of s, used as the cache key.
func templateHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

// compatEntry is the cached result for a single template.
// If requiresUpgrade is false, upgraded is empty and the original template is returned as-is.
type compatEntry struct {
	requiresUpgrade bool
	upgraded        string
}

// lruCache is a goroutine-safe LRU cache with TTL eviction.
type lruCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[string]*list.Element
}

type lruEntry struct {
	key        string
	value      compatEntry
	lastAccess time.Time
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (c *lruCache) startEvictionLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(CacheEntryTTL / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.evictStale()
			}
		}
	}()
}

func (c *lruCache) evictStale() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, el := range c.items {
		if now.Sub(el.Value.(*lruEntry).lastAccess) > CacheEntryTTL {
			c.ll.Remove(el)
			delete(c.items, key)
			CUECompatCacheEvictionsTotal.WithLabelValues("ttl").Inc()
		}
	}
}

func (c *lruCache) get(key string) (compatEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return compatEntry{}, false
	}
	entry := el.Value.(*lruEntry)
	entry.lastAccess = time.Now()
	c.ll.MoveToFront(el)
	return entry.value, true
}

func (c *lruCache) put(key string, value compatEntry) {
	if c.capacity <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		entry := el.Value.(*lruEntry)
		entry.value = value
		entry.lastAccess = time.Now()
		c.ll.MoveToFront(el)
		return
	}
	if c.ll.Len() >= c.capacity {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry).key)
			CUECompatCacheEvictionsTotal.WithLabelValues("capacity").Inc()
		}
	}
	el := c.ll.PushFront(&lruEntry{key: key, value: value, lastAccess: time.Now()})
	c.items[key] = el
}

func (c *lruCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
