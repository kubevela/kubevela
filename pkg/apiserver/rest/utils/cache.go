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

import "time"

// MemoryCache memory cache, support time expired
type MemoryCache struct {
	data          interface{}
	cacheDuration time.Duration
	startTime     time.Time
}

// NewMemoryCache new memory cache instance
func NewMemoryCache(data interface{}, cacheDuration time.Duration) *MemoryCache {
	// prevent the cache to be broken, provide a minimal cache duration
	if cacheDuration < 30*time.Second {
		cacheDuration = 30 * time.Second
	}
	return &MemoryCache{data: data, cacheDuration: cacheDuration, startTime: time.Now()}
}

// IsExpired whether the cache data expires
func (m *MemoryCache) IsExpired() bool {
	return time.Now().After(m.startTime.Add(m.cacheDuration))
}

// GetData get cache data
func (m *MemoryCache) GetData() interface{} {
	return m.data
}
