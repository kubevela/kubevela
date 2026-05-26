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

const defaultSweepInterval = 3 * time.Second

// Option defines the functional option type for configuring MemoryCacheStore.
type Option func(*MemoryCacheStore)

// WithSweepInterval allows configuring a custom background sweep frequency.
func WithSweepInterval(d time.Duration) Option {
	return func(m *MemoryCacheStore) {
		if d > 0 {
			m.sweepInterval = d
		}
	}
}

// memoryCache memory cache, support time expired
type memoryCache struct {
	data       interface{}
	expiration time.Time
}

// newMemoryCache new memory cache instance
func newMemoryCache(data interface{}, cacheDuration time.Duration) *memoryCache {
	var exp time.Time
	if cacheDuration > 0 {
		exp = time.Now().Add(cacheDuration)
	}
	return &memoryCache{data: data, expiration: exp}
}

// IsExpired whether the cache data expires
func (c *memoryCache) IsExpired(now time.Time) bool {
	if c.expiration.IsZero() {
		return false
	}
	return now.After(c.expiration)
}

// GetData get cache data
func (m *memoryCache) GetData() interface{} {
	return m.data
}

// MemoryCacheStore a sample memory cache instance, if data set cache duration, will auto clear after timeout.
// But, Expired cleanup is not necessarily accurate, it has a 3-second window.
type MemoryCacheStore struct {
	store         sync.Map
	done          chan struct{}
	wg            sync.WaitGroup
	sweepInterval time.Duration
	closeOnce     sync.Once
}

// NewMemoryCacheStore memory cache store
func NewMemoryCacheStore(parent context.Context, opts ...Option) *MemoryCacheStore {
	if parent == nil {
		parent = context.Background()
	}

	m := &MemoryCacheStore{
		sweepInterval: defaultSweepInterval,
		done:          make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(m.sweepInterval)
		defer ticker.Stop()

		for {
			select {
			case <-parent.Done():
				return
			case <-m.done:
				return
			case <-ticker.C:
				now := time.Now()
				m.store.Range(func(k, v interface{}) bool {
					if item, ok := v.(*memoryCache); ok && item.IsExpired(now) {
						m.store.Delete(k)
					}
					return true
				})
			}
		}
	}()

	return m
}

// Close gracefully stops the background sweeper goroutine. It is safe to call concurrently.
func (m *MemoryCacheStore) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
		m.wg.Wait()
	})
}

// Put cache data, if cacheDuration>0, store will clear data after timeout.
func (m *MemoryCacheStore) Put(key, value interface{}, cacheDuration time.Duration) {
	m.store.Store(key, newMemoryCache(value, cacheDuration))
}

// Delete cache data from store
func (m *MemoryCacheStore) Delete(key interface{}) {
	m.store.Delete(key)
}

// Get cache data from store, if not exist or timeout, will return nil
func (m *MemoryCacheStore) Get(key interface{}) (value interface{}) {
	v, exists := m.store.Load(key)
	if !exists {
		return nil
	}

	item, ok := v.(*memoryCache)
	if !ok {
		return nil
	}

	if item.IsExpired(time.Now()) {
		m.store.Delete(key)
		return nil
	}

	return item.GetData()
}
