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

package component

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cacheProbe counts the calls a RevisionCache makes to its revision and read
// callbacks, so a test can state exactly how many registry round trips a
// sequence of Loads costs.
type cacheProbe struct {
	revisionCalls atomic.Int32
	readCalls     atomic.Int32
	// lastKnown records the revision the cache offered for revalidation.
	lastKnown string
	// readAt records the revision the cache asked to read at.
	readAt string
}

func (p *cacheProbe) revision(current string, err error) func(string) (string, error) {
	return func(lastKnown string) (string, error) {
		p.revisionCalls.Add(1)
		p.lastKnown = lastKnown
		return current, err
	}
}

func (p *cacheProbe) read(value string, err error) func(string) (string, error) {
	return func(revision string) (string, error) {
		p.readCalls.Add(1)
		p.readAt = revision
		return value, err
	}
}

func TestNewRevisionCacheAppliesDefaults(t *testing.T) {
	c := NewRevisionCache[string](0)
	require.NotNil(t, c.store, "a size below one must still build a store, at the default size")
	assert.Equal(t, DefaultRevisionTTL, c.ttl)
	assert.Equal(t, DefaultFailureTTL, c.failureTTL)

	custom := newRevisionCache[string](context.Background(), 4, 0)
	assert.Equal(t, DefaultRevisionTTL, custom.ttl, "a non-positive ttl must fall back to the default")
	tuned := newRevisionCache[string](context.Background(), 4, time.Minute)
	assert.Equal(t, time.Minute, tuned.ttl)
}

func TestRevisionCacheServesUnchangedRevisionWithoutReading(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	got, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
	assert.Equal(t, "", p.lastKnown, "a miss has nothing to revalidate against")
	assert.Equal(t, "r1", p.readAt, "the read must happen at the revision that was checked")
	assert.EqualValues(t, 1, p.readCalls.Load())

	got, err = c.Load("widget", p.revision("r1", nil), p.read("v2", nil))
	require.NoError(t, err)
	assert.Equal(t, "v1", got, "an unchanged revision must serve the cached value")
	assert.Equal(t, "r1", p.lastKnown, "the cached revision is offered for conditional revalidation")
	assert.EqualValues(t, 1, p.readCalls.Load(), "no read when the revision has not moved")
	assert.EqualValues(t, 2, p.revisionCalls.Load())
}

func TestRevisionCacheReadsAgainWhenRevisionMoves(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	_, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)

	got, err := c.Load("widget", p.revision("r2", nil), p.read("v2", nil))
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
	assert.Equal(t, "r2", p.readAt)
	assert.EqualValues(t, 2, p.readCalls.Load())

	// And the new revision is what is cached now.
	got, err = c.Load("widget", p.revision("r2", nil), p.read("v3", nil))
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
	assert.EqualValues(t, 2, p.readCalls.Load())
}

func TestRevisionCacheReadsEveryTimeWhenRevisionUnsupported(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	for i := 0; i < 3; i++ {
		got, err := c.Load("widget", p.revision("", ErrRevisionUnsupported), p.read("v1", nil))
		require.NoError(t, err)
		assert.Equal(t, "v1", got)
		assert.Equal(t, "", p.readAt, "nothing to pin the read to")
	}
	assert.EqualValues(t, 3, p.readCalls.Load(), "a source without revisions is read on every call")

	// Nothing was cached: a source that later learns to report a revision
	// still reads once.
	_, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)
	assert.EqualValues(t, 4, p.readCalls.Load())
}

func TestRevisionCacheServesCachedValueWhenProbeFails(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	_, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)

	got, err := c.Load("widget", p.revision("", errors.New("registry unreachable")), p.read("v2", nil))
	require.NoError(t, err, "a failed probe must not fail a package the cache already holds")
	assert.Equal(t, "v1", got)
	assert.EqualValues(t, 1, p.readCalls.Load(), "the cached value is served, not re-read")
}

func TestRevisionCacheReadsWhenProbeFailsAndNothingIsCached(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	got, err := c.Load("widget", p.revision("", errors.New("registry unreachable")), p.read("v1", nil))
	require.NoError(t, err, "the probe is an optimisation; its failure must not stand in for the read")
	assert.Equal(t, "v1", got)
	assert.Equal(t, "", p.readAt)

	// The read's own error is the one reported when it fails too.
	readErr := errors.New("package does not compile")
	_, err = c.Load("widget", p.revision("", errors.New("registry unreachable")), p.read("", readErr))
	assert.ErrorIs(t, err, readErr)

	// Nothing was stored for a read at no revision.
	_, err = c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)
	assert.EqualValues(t, 3, p.readCalls.Load())
}

func TestRevisionCacheRemembersFailedReadAtRevision(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}
	readErr := errors.New("_module.cue does not compile")

	got, err := c.Load("widget", p.revision("r1", nil), p.read("", readErr))
	assert.ErrorIs(t, err, readErr)
	assert.Equal(t, "", got)

	_, err = c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	assert.ErrorIs(t, err, readErr, "the same revision fails the same way; it must not be crawled again")
	assert.EqualValues(t, 1, p.readCalls.Load())

	// A new revision is read, and may well succeed.
	got, err = c.Load("widget", p.revision("r2", nil), p.read("v2", nil))
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
	assert.EqualValues(t, 2, p.readCalls.Load())
}

func TestRevisionCacheDoesNotRememberFailureWithoutRevision(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}
	readErr := errors.New("boom")

	_, err := c.Load("widget", p.revision("", nil), p.read("", readErr))
	assert.ErrorIs(t, err, readErr)
	_, err = c.Load("widget", p.revision("", nil), p.read("", readErr))
	assert.ErrorIs(t, err, readErr)
	assert.EqualValues(t, 2, p.readCalls.Load(), "with no revision to file it under, nothing is cached")

	// The same holds for a successful read at an empty revision.
	got, err := c.Load("widget", p.revision("", nil), p.read("v1", nil))
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
	_, err = c.Load("widget", p.revision("", nil), p.read("v1", nil))
	require.NoError(t, err)
	assert.EqualValues(t, 4, p.readCalls.Load())
}

func TestRevisionCacheResetForgetsEverything(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	p := &cacheProbe{}

	_, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
	require.NoError(t, err)
	c.Reset()

	got, err := c.Load("widget", p.revision("r1", nil), p.read("v2", nil))
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
	assert.EqualValues(t, 2, p.readCalls.Load())
}

func TestRevisionCacheCollapsesConcurrentReads(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	var reads, probes atomic.Int32
	release := make(chan struct{})
	read := func(string) (string, error) {
		reads.Add(1)
		<-release
		return "v1", nil
	}
	// Every caller probes the revision after missing the cache and before
	// entering the shared read. Counting the probes is therefore proof that
	// every caller has missed: the cache is only filled once the read returns,
	// and the read is held until the count is complete.
	revision := func(string) (string, error) { probes.Add(1); return "r1", nil }

	const callers = 5
	var wg sync.WaitGroup
	results := make([]string, callers)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := c.Load("widget", revision, read)
			assert.NoError(t, err)
			results[i] = got
		}(i)
	}
	require.Eventually(t, func() bool { return probes.Load() == callers }, 5*time.Second, time.Millisecond,
		"every caller must have missed the cache before the read is released")
	close(release)
	wg.Wait()

	assert.EqualValues(t, 1, reads.Load(), "%d callers that all missed must share one registry read", callers)
	for _, got := range results {
		assert.Equal(t, "v1", got)
	}
}

// TestRevisionCacheLateCallerFindsWinnerResult covers the caller that missed
// the cache, was slow to learn the revision, and by the time it reads finds
// that another caller already filled the entry for that same revision.
func TestRevisionCacheLateCallerFindsWinnerResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		readErr error
	}{
		{name: "winner succeeded"},
		{name: "winner failed", readErr: errors.New("boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newRevisionCache[string](context.Background(), 8, time.Minute)
			var reads atomic.Int32
			var probes atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})
			revision := func(string) (string, error) {
				if probes.Add(1) == 1 {
					// The late caller: it has already missed the cache, and now
					// waits until the winner is done before it learns the revision.
					close(started)
					<-release
				}
				return "r1", nil
			}
			read := func(string) (string, error) {
				reads.Add(1)
				return "v1", tc.readErr
			}

			done := make(chan struct{})
			var lateGot string
			var lateErr error
			go func() {
				defer close(done)
				lateGot, lateErr = c.Load("widget", revision, read)
			}()
			<-started

			winnerGot, winnerErr := c.Load("widget", revision, read)
			close(release)
			<-done

			assert.EqualValues(t, 1, reads.Load(), "the late caller must reuse the winner's read")
			if tc.readErr != nil {
				assert.ErrorIs(t, winnerErr, tc.readErr)
				assert.ErrorIs(t, lateErr, tc.readErr)
				return
			}
			require.NoError(t, winnerErr)
			require.NoError(t, lateErr)
			assert.Equal(t, "v1", winnerGot)
			assert.Equal(t, "v1", lateGot)
		})
	}
}

// TestRevisionCacheWithoutStoreReadsEveryTime pins the degraded mode: a cache
// whose store could not be built stays correct by never caching.
func TestRevisionCacheWithoutStoreReadsEveryTime(t *testing.T) {
	c := &RevisionCache[string]{ttl: time.Minute, failureTTL: time.Minute}
	p := &cacheProbe{}

	c.Reset() // must not panic
	c.put("widget", "r1", "v1")
	c.putFailure("widget", "r1", errors.New("boom"))
	_, ok := c.get("widget")
	assert.False(t, ok)

	for i := 0; i < 2; i++ {
		got, err := c.Load("widget", p.revision("r1", nil), p.read("v1", nil))
		require.NoError(t, err)
		assert.Equal(t, "v1", got)
	}
	assert.EqualValues(t, 2, p.readCalls.Load())
}

func TestRevisionCachePutFailureIgnoresNilError(t *testing.T) {
	c := newRevisionCache[string](context.Background(), 8, time.Minute)
	c.putFailure("widget", "r1", nil)
	_, ok := c.get("widget")
	assert.False(t, ok, "a nil error is not a failure to remember")
}
