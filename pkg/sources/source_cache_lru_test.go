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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

type fakeSourceCacheStore struct {
	reads    int
	writes   int
	data     map[string]interface{}
	stale    bool
	found    bool
	expires  time.Time
	writeErr error
	lastMeta velaprocess.SourceCacheWriteMeta
}

func (f *fakeSourceCacheStore) Read(_ context.Context, _ string, _ time.Duration) (velaprocess.SourceCacheRead, error) {
	f.reads++
	return velaprocess.SourceCacheRead{Data: f.data, Found: f.found, Stale: f.stale, ExpiresAt: f.expires}, nil
}

func (f *fakeSourceCacheStore) Write(_ context.Context, _, _ string, data map[string]interface{}, meta velaprocess.SourceCacheWriteMeta) error {
	f.writes++
	f.lastMeta = meta
	if f.writeErr != nil {
		return f.writeErr
	}
	f.data = data
	f.found = true
	f.stale = false
	return nil
}

func newTestLRUStore(delegate *fakeSourceCacheStore, ttl time.Duration) *lruSourceCacheStore {
	return &lruSourceCacheStore{
		delegate: delegate,
		cache:    sharedSourceLRU,
		ttl:      ttl,
	}
}

func TestLRUSourceCacheReadServesFreshFromMemory(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"region": "us-east-1"},
		found:   true,
		stale:   false,
		expires: time.Now().Add(time.Hour),
	}
	store := newTestLRUStore(delegate, time.Minute)

	// First read populates Layer 1 from the delegate.
	entry, err := store.Read(context.Background(), "key-a", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.False(t, entry.Stale)
	assert.Equal(t, "us-east-1", entry.Data["region"])
	assert.Equal(t, 1, delegate.reads)

	// Second read is served from Layer 1; the delegate is not consulted again.
	entry, err = store.Read(context.Background(), "key-a", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.False(t, entry.Stale)
	assert.Equal(t, "us-east-1", entry.Data["region"])
	assert.Equal(t, 1, delegate.reads, "delegate should not be read on Layer 1 hit")
}

func TestLRUSourceCacheInMemoryExpiryFallsThrough(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"region": "us-east-1"},
		found:   true,
		expires: time.Now().Add(time.Hour),
	}
	// Zero in-memory TTL means every entry is immediately expired.
	store := newTestLRUStore(delegate, 0)

	_, _ = store.Read(context.Background(), "key-b", time.Hour)
	_, _ = store.Read(context.Background(), "key-b", time.Hour)
	assert.Equal(t, 2, delegate.reads, "expired Layer 1 entry must fall through to delegate")
}

func TestLRUSourceCacheTTLCappedByStorageTTL(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"value": 1},
		found:   true,
		expires: time.Now().Add(time.Hour),
	}
	// Long in-memory window, but a tiny storageTTL passed by the caller: the
	// entry must expire per the storageTTL so a short-TTL source is not masked.
	store := newTestLRUStore(delegate, time.Hour)
	_, _ = store.Read(context.Background(), "key-c", time.Nanosecond)
	time.Sleep(time.Millisecond)
	_, _ = store.Read(context.Background(), "key-c", time.Nanosecond)
	assert.Equal(t, 2, delegate.reads, "storageTTL must cap the in-memory window")
}

func TestEffectiveTTL(t *testing.T) {
	s := &lruSourceCacheStore{ttl: 30 * time.Second}
	assert.Equal(t, 10*time.Second, s.effectiveTTL(10*time.Second), "shorter storageTTL wins")
	assert.Equal(t, 30*time.Second, s.effectiveTTL(time.Minute), "longer storageTTL capped at LRU ttl")
	assert.Equal(t, 30*time.Second, s.effectiveTTL(0), "zero storageTTL falls back to LRU ttl")
}

func TestLRUSourceCacheDoesNotPromoteStale(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"region": "old"},
		found:   true,
		stale:   true,
		expires: time.Now().Add(-time.Minute),
	}
	store := newTestLRUStore(delegate, time.Minute)

	entry, err := store.Read(context.Background(), "key-c", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.True(t, entry.Stale)
	// Stale value must not be cached in Layer 1; a follow-up read hits delegate again.
	_, _ = store.Read(context.Background(), "key-c", time.Hour)
	assert.Equal(t, 2, delegate.reads, "stale delegate value must not be promoted to Layer 1")
}

func TestLRUSourceCacheWritePopulatesMemory(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{}
	store := newTestLRUStore(delegate, time.Minute)

	err := store.Write(context.Background(), "key-d", "source-x", map[string]interface{}{"region": "eu-west-1"}, velaprocess.SourceCacheWriteMeta{TTL: time.Minute})
	assert.NoError(t, err)
	assert.Equal(t, 1, delegate.writes)
	assert.Equal(t, time.Minute, delegate.lastMeta.TTL)

	// A subsequent read is served from Layer 1 without touching the delegate.
	entry, err := store.Read(context.Background(), "key-d", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.False(t, entry.Stale)
	assert.Equal(t, "eu-west-1", entry.Data["region"])
	assert.Equal(t, 0, delegate.reads, "write should populate Layer 1 so no delegate read is needed")
}

func TestLRUSourceCacheSharedAcrossStores(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate1 := &fakeSourceCacheStore{
		data:    map[string]interface{}{"region": "shared"},
		found:   true,
		expires: time.Now().Add(time.Hour),
	}
	// Simulates a second Application's render in the same process.
	delegate2 := &fakeSourceCacheStore{
		data:    map[string]interface{}{"region": "should-not-be-read"},
		found:   true,
		expires: time.Now().Add(time.Hour),
	}
	store1 := newTestLRUStore(delegate1, time.Minute)
	store2 := newTestLRUStore(delegate2, time.Minute)

	_, _ = store1.Read(context.Background(), "shared-key", time.Hour)
	entry, err := store2.Read(context.Background(), "shared-key", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.Equal(t, "shared", entry.Data["region"], "second store should hit the shared Layer 1 entry")
	assert.Equal(t, 0, delegate2.reads, "shared Layer 1 hit must avoid the second delegate")
}

// A toucher delegate, to prove Touch is forwarded only when the delegate can
// take it.
type touchableCacheStore struct {
	fakeSourceCacheStore
	touched  int
	touchErr error
}

func (f *touchableCacheStore) Touch(_ context.Context, _ string) error {
	f.touched++
	return f.touchErr
}

// No delegate means source caching is off, and the wrapper must be nil rather
// than an object that silently swallows every read and write.
func TestNewLRUSourceCacheStoreIsNilWithoutADelegate(t *testing.T) {
	assert.Nil(t, NewLRUSourceCacheStore(nil))
	assert.NotNil(t, NewLRUSourceCacheStore(&fakeSourceCacheStore{}))
}

// An empty key is not an identity, so it must not become a shared entry that
// every unkeyed source reads from.
func TestLRUStoreIgnoresAnEmptyKey(t *testing.T) {
	delegate := &fakeSourceCacheStore{found: true, data: map[string]interface{}{"a": 1}}
	s := NewLRUSourceCacheStore(delegate)

	entry, err := s.Read(context.Background(), "", time.Minute)
	assert.NoError(t, err)
	assert.False(t, entry.Found)
	assert.Equal(t, 0, delegate.reads, "an empty key must not reach the store")

	assert.NoError(t, s.Write(context.Background(), "", "t", map[string]interface{}{"a": 1},
		velaprocess.SourceCacheWriteMeta{}))
	assert.Equal(t, 0, delegate.writes)
}

// The wrapper has to keep offering Touch, or wrapping a store silently loses
// last-accessed stamping and the GC sweep starts reaping entries in use.
func TestLRUStoreStaysTouchable(t *testing.T) {
	wrapped, ok := NewLRUSourceCacheStore(&fakeSourceCacheStore{}).(velaprocess.SourceCacheToucher)
	assert.True(t, ok, "wrapping must not hide Touch")
	assert.NotNil(t, wrapped)
}

func TestLRUStoreTouchForwardsOnlyToATouchingDelegate(t *testing.T) {
	toucher := func(d velaprocess.SourceCacheStore) velaprocess.SourceCacheToucher {
		return NewLRUSourceCacheStore(d).(velaprocess.SourceCacheToucher)
	}

	assert.NoError(t, toucher(&fakeSourceCacheStore{}).Touch(context.Background(), "k"),
		"a delegate that cannot touch is not an error")

	backing := &touchableCacheStore{}
	assert.NoError(t, toucher(backing).Touch(context.Background(), "k"))
	assert.Equal(t, 1, backing.touched)

	assert.NoError(t, toucher(&touchableCacheStore{}).Touch(context.Background(), ""),
		"an empty key is not touched")
}

func TestLRUStoreTouchPropagatesTheDelegateError(t *testing.T) {
	backing := &touchableCacheStore{touchErr: assert.AnError}
	wrapped := NewLRUSourceCacheStore(backing).(velaprocess.SourceCacheToucher)
	assert.Error(t, wrapped.Touch(context.Background(), "k"))
}

// A failed write must not seed the in-memory layer, or the process serves a
// value the store never accepted.
func TestLRUStoreDoesNotCacheAFailedWrite(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{writeErr: assert.AnError}
	s := NewLRUSourceCacheStore(delegate)

	assert.Error(t, s.Write(context.Background(), "k-failed-write", "t",
		map[string]interface{}{"a": 1}, velaprocess.SourceCacheWriteMeta{}))

	delegate.writeErr = nil
	entry, err := s.Read(context.Background(), "k-failed-write", time.Minute)
	assert.NoError(t, err)
	assert.False(t, entry.Found, "nothing was written, so nothing may be served")
	assert.Equal(t, 1, delegate.reads, "the read had to go to the store")
}

// A write cached the value for the fixed in-memory window regardless of the
// source's own storageTTL, so a source asking to be refreshed every second was
// served from memory for thirty. Read already capped the window; Write did not.
func TestLRUSourceCacheWriteHonoursTheSourceTTL(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{}
	store := newTestLRUStore(delegate, time.Minute)

	err := store.Write(context.Background(), "key-ttl", "source-x",
		map[string]interface{}{"region": "eu-west-1"},
		velaprocess.SourceCacheWriteMeta{TTL: 10 * time.Millisecond})
	assert.NoError(t, err)

	time.Sleep(30 * time.Millisecond)

	_, err = store.Read(context.Background(), "key-ttl", 10*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 1, delegate.reads, "the in-memory entry must expire with the source's own TTL")
}

// The write left storeExpiresAt zero, so a Layer 1 hit reported no expiry at all
// and the resolver cleared the deadline it had just published.
func TestLRUSourceCacheWriteReportsThePersistentDeadline(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{}
	store := newTestLRUStore(delegate, time.Minute)

	before := time.Now()
	err := store.Write(context.Background(), "key-exp", "source-x",
		map[string]interface{}{"region": "eu-west-1"},
		velaprocess.SourceCacheWriteMeta{TTL: time.Hour})
	assert.NoError(t, err)

	entry, err := store.Read(context.Background(), "key-exp", time.Hour)
	assert.NoError(t, err)
	assert.True(t, entry.Found)
	assert.False(t, entry.ExpiresAt.IsZero(), "a Layer 1 hit must report the persistent deadline")
	assert.WithinDuration(t, before.Add(time.Hour), entry.ExpiresAt, time.Minute)
}

// Admission resolves through a read-only store, but the LRU is layered outside
// it, so a discarded write still populated the process-wide cache: a validation
// left an entry behind after all, and the reconcile that followed served it
// instead of writing the persistent one.
func TestLRUSourceCacheDoesNotSeedFromADiscardedWrite(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{}
	store := &lruSourceCacheStore{
		delegate: NewReadOnlySourceCacheStore(delegate),
		cache:    sharedSourceLRU,
		ttl:      time.Minute,
	}

	err := store.Write(context.Background(), "key-ro", "source-x",
		map[string]interface{}{"region": "eu-west-1"},
		velaprocess.SourceCacheWriteMeta{TTL: time.Minute})
	assert.NoError(t, err)
	assert.Equal(t, 0, delegate.writes, "the read-only layer must not persist")

	entry, err := store.Read(context.Background(), "key-ro", time.Minute)
	assert.NoError(t, err)
	assert.False(t, entry.Found, "a discarded write must not seed the shared LRU")
	assert.Equal(t, 1, delegate.reads, "the read must fall through")
}

// Promotion is bounded by what is left of the persistent deadline, not by the
// whole storageTTL. Write states the rule: a value is held for the in-memory
// window or its own TTL, whichever is shorter, because serving it past its
// persistent expiry is what a short storageTTL asks not to happen. A value
// promoted on read is already part-way through that TTL, so the full TTL is
// too long a window for it.
func TestLRUPromotionIsCappedByWhatIsLeftOfTheStoreDeadline(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"value": 1},
		found:   true,
		expires: time.Now().Add(40 * time.Millisecond),
	}
	// A ten-minute storageTTL, so the window is the in-memory one, but this
	// value has 40ms of its deadline left.
	store := newTestLRUStore(delegate, time.Hour)
	_, _ = store.Read(context.Background(), "key-remaining", 10*time.Minute)
	assert.Equal(t, 1, delegate.reads)

	time.Sleep(120 * time.Millisecond)
	got, err := store.Read(context.Background(), "key-remaining", 10*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 2, delegate.reads,
		"the store is asked again once the value's own deadline has passed")
	assert.False(t, got.ExpiresAt.Before(time.Now().Add(-time.Second)),
		"so no hit reports itself fresh with an expiry already behind it")
}

// A store that records no expiry still gets promoted: there is no deadline to
// cap against, and refusing to promote would put every such read through to
// Layer 2.
func TestLRUPromotesAValueWithNoRecordedExpiry(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{data: map[string]interface{}{"value": 1}, found: true}
	store := newTestLRUStore(delegate, time.Hour)
	_, _ = store.Read(context.Background(), "key-noexpiry", 10*time.Minute)
	_, _ = store.Read(context.Background(), "key-noexpiry", 10*time.Minute)
	assert.Equal(t, 1, delegate.reads, "the second read is served from memory")
}

// A fresh hit whose deadline has already passed is not promoted at all. The
// delegate decides what is stale, so this is a shape it should not report, and
// holding it for the default window would be the longest hold of the three.
func TestLRUDoesNotPromoteASpentDeadline(t *testing.T) {
	sharedSourceLRU.Clear()
	delegate := &fakeSourceCacheStore{
		data:    map[string]interface{}{"value": 1},
		found:   true,
		expires: time.Now().Add(-time.Minute),
	}
	store := newTestLRUStore(delegate, time.Hour)
	_, _ = store.Read(context.Background(), "key-spent", 10*time.Minute)
	_, _ = store.Read(context.Background(), "key-spent", 10*time.Minute)
	assert.Equal(t, 2, delegate.reads, "nothing was promoted, so the store answers both")
}
