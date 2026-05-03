/*
Copyright 2026 The KubeVela Authors.

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

package cache

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// MissReason describes why a cache lookup missed.
type MissReason string

const (
	// MissReasonNone means the cache lookup found a value.
	MissReasonNone MissReason = ""
	// MissReasonNotFound means the key is not present in the cache.
	MissReasonNotFound MissReason = "not_found"
	// MissReasonExpired means the key was present but its TTL had elapsed.
	MissReasonExpired MissReason = "expired"
)

type lruByteEntry struct {
	data      []byte
	expiresAt time.Time
}

// LRUByteStore stores byte slices with a maximum byte budget and LRU eviction.
type LRUByteStore struct {
	mu       sync.Mutex
	maxBytes int64
	bytes    int64
	cache    *lru.Cache[string, *lruByteEntry]
	closed   bool
}

// NewLRUByteStore creates an LRUByteStore bounded by maxBytes.
func NewLRUByteStore(maxBytes int64) (*LRUByteStore, error) {
	store := &LRUByteStore{maxBytes: maxBytes}
	maxEntries := int(^uint(0) >> 1)
	c, err := lru.NewWithEvict[string, *lruByteEntry](maxEntries, func(_ string, entry *lruByteEntry) {
		store.bytes -= int64(len(entry.data))
	})
	if err != nil {
		return nil, err
	}
	store.cache = c
	return store, nil
}

// Put stores data for key until ttl elapses. Non-positive ttl means no expiry.
func (s *LRUByteStore) Put(key string, data []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.pruneExpiredLocked()

	size := int64(len(data))
	if s.maxBytes <= 0 || size > s.maxBytes {
		s.deleteLocked(key)
		return
	}

	s.deleteLocked(key)

	entry := &lruByteEntry{data: append([]byte(nil), data...)}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	s.cache.Add(key, entry)
	s.bytes += size

	for s.bytes > s.maxBytes {
		s.cache.RemoveOldest()
	}
}

// Get returns cached data for key.
func (s *LRUByteStore) Get(key string) ([]byte, bool, MissReason) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, false, MissReasonNotFound
	}

	entry, ok := s.cache.Get(key)
	if !ok {
		return nil, false, MissReasonNotFound
	}
	if entry.expired() {
		s.deleteLocked(key)
		return nil, false, MissReasonExpired
	}
	return append([]byte(nil), entry.data...), true, MissReasonNone
}

// Delete removes key from the store.
func (s *LRUByteStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(key)
}

// Bytes reports the current bytes held by the store.
func (s *LRUByteStore) Bytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked()
	return s.bytes
}

// Close releases all entries and ignores future Put calls.
func (s *LRUByteStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.Purge()
	s.bytes = 0
	s.closed = true
}

func (s *LRUByteStore) deleteLocked(key string) {
	if _, ok := s.cache.Peek(key); ok {
		s.cache.Remove(key)
	}
}

func (s *LRUByteStore) pruneExpiredLocked() {
	for _, key := range s.cache.Keys() {
		entry, ok := s.cache.Peek(key)
		if ok && entry.expired() {
			s.cache.Remove(key)
		}
	}
}

func (e *lruByteEntry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}
