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

package sources

import (
	"context"
	"time"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// readOnlySourceCacheStore reads through to a real store and discards writes.
//
// The admission dry-run resolves sources for real, because it has to type-check
// the result against the consuming parameter, but a validation must leave
// nothing behind. Writing from there would also produce entries labelled by
// source type via the Secret store, where a reconcile writes them through the
// Config API labelled with the ConfigTemplate name.
//
// Reads still go through. Skipping them would make every admission repeat the
// source's live I/O against a webhook timeout, and have validation see different
// data than the render that follows. The type check never depends on cached
// values: TypeOf builds sentinels from the schema.
type readOnlySourceCacheStore struct {
	delegate velaprocess.SourceCacheStore
}

// NewReadOnlySourceCacheStore wraps delegate so reads are served and writes are
// dropped. Returns nil if delegate is nil, matching the other constructors.
func NewReadOnlySourceCacheStore(delegate velaprocess.SourceCacheStore) velaprocess.SourceCacheStore {
	if delegate == nil {
		return nil
	}
	return &readOnlySourceCacheStore{delegate: delegate}
}

func (s *readOnlySourceCacheStore) Read(ctx context.Context, cacheKey string, ttl time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	if s == nil || s.delegate == nil {
		return nil, false, false, time.Time{}, nil
	}
	return s.delegate.Read(ctx, cacheKey, ttl)
}

// Write is a no-op. A validation render resolves to check types, not to populate
// the cache the cluster shares.
func (s *readOnlySourceCacheStore) Write(_ context.Context, _, _ string, _ map[string]interface{}, _ velaprocess.SourceCacheWriteMeta) error {
	return nil
}

// discardsWrites lets a layer above this one - the process LRU, which wraps it
// rather than the other way round - see that a write it just forwarded went
// nowhere, and decline to memoise it.
func (s *readOnlySourceCacheStore) discardsWrites() bool { return true }

// Touch is a no-op for the same reason: last-accessed drives the GC sweep, and a
// validation is not a use.
func (s *readOnlySourceCacheStore) Touch(_ context.Context, _ string) error {
	return nil
}
