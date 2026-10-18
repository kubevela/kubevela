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
	"time"

	"k8s.io/utils/lru"
)

const (
	// refreshBackoffCacheSize bounds the failing cache keys remembered. A key is
	// forgotten by eviction as readily as by recovery, which only costs an early
	// retry.
	refreshBackoffCacheSize = 512
	// refreshBackoffBase is the first wait after a failed refresh: the in-process
	// layer's own window, so the earliest a hold can delay anything is the
	// earliest a value could have been re-read anyway.
	refreshBackoffBase = sourceCacheLRUTTL
)

// refreshBackoffs holds, by cache key, when a failing source may be refreshed
// again. In process and not persisted: it bounds what one controller sends a
// failing endpoint, and a restart costing one extra attempt is not worth a write
// on every failure.
var refreshBackoffs = lru.New(refreshBackoffCacheSize)

// refreshHold is how long a key waits, how many consecutive failures put it
// there, and what the last one was. The cause is kept so a refusal while the
// hold is in force says what is actually wrong rather than only that it is
// waiting.
type refreshHold struct {
	failures int
	retryAt  time.Time
	cause    error
}

// heldRefresh returns the hold on key if one is still being waited out, so a
// caller can serve the stored value or report why it is not asking again.
func heldRefresh(key string, now time.Time) (refreshHold, bool) {
	if key == "" {
		return refreshHold{}, false
	}
	hit, ok := refreshBackoffs.Get(key)
	if !ok {
		return refreshHold{}, false
	}
	hold, ok := hit.(refreshHold)
	if !ok || !now.Before(hold.retryAt) {
		return refreshHold{}, false
	}
	return hold, true
}

// backoffHoldsRefresh reports whether key is still waiting out its last failure.
func backoffHoldsRefresh(key string, now time.Time) bool {
	_, held := heldRefresh(key, now)
	return held
}

// recordRefreshFailure lengthens the wait for key after a refresh failed.
//
// servingStale says the binding still has a value. The wait grows only then,
// because the Application is running and the next attempt costs it nothing. With
// no value to serve the binding is down, and the attempt being held is the only
// way it comes back, so the wait stays at the shortest one however long the
// outage lasts.
func recordRefreshFailure(key string, ttl time.Duration, now time.Time, cause error, servingStale bool) {
	if key == "" {
		return
	}
	failures := 1
	if hit, ok := refreshBackoffs.Get(key); ok {
		if hold, ok := hit.(refreshHold); ok {
			failures = hold.failures + 1
		}
	}
	grown := failures
	if !servingStale {
		grown = 1
	}
	refreshBackoffs.Add(key, refreshHold{
		failures: failures,
		retryAt:  now.Add(refreshWait(grown, ttl)),
		cause:    cause,
	})
}

// clearRefreshBackoff forgets a key that refreshed, so a recovered source is
// read at its own pace again.
func clearRefreshBackoff(key string) {
	if key != "" {
		refreshBackoffs.Remove(key)
	}
}

// ForgetRefreshHolds drops every refresh hold in the process.
//
// A hold is controller-lifetime state, keyed by cache entry and shared by every
// Application that resolves to it, which is what rests a failing upstream
// between reconciles. A harness that evaluates independent cases in one process
// has to drop them between cases, or one case's failure refuses the next
// case's fetch of the same entry.
func ForgetRefreshHolds() { forgetRefreshBackoffs() }

// forgetRefreshBackoffs drops every hold, for tests that need a fresh start.
func forgetRefreshBackoffs() {
	refreshBackoffs.Clear()
}

// refreshWait doubles with each consecutive failure, so a long outage is not
// polled at the rate of a brief one, and stops at the freshness the definition
// asked for: a failing source then costs its upstream no more than a healthy one
// does, which is the most that can be asked of it.
func refreshWait(failures int, ttl time.Duration) time.Duration {
	wait := refreshBackoffBase
	for i := 1; i < failures && wait < ttl; i++ {
		wait *= 2
	}
	if ttl > 0 && wait > ttl {
		wait = ttl
	}
	return wait
}
