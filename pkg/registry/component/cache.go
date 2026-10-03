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
	goerrors "errors"
	"fmt"
	"time"

	velacache "github.com/kubevela/pkg/cache"
	"golang.org/x/sync/singleflight"
	"k8s.io/klog/v2"
)

// DefaultRevisionCacheSize is how many packages one cache keeps. A cluster
// reads a handful of modules and addons from a handful of registries, so this
// is a bound, not a target.
const DefaultRevisionCacheSize = 64

// DefaultRevisionTTL is a backstop, not the cache's working lifetime. Entries
// are normally kept or replaced by comparing revisions, which is exact; the TTL
// only bounds how long a package can be served when the registry has been
// unreachable for revision checks that whole time.
const DefaultRevisionTTL = 12 * time.Hour

// DefaultFailureTTL is how long a failed read is remembered.
//
// A read that failed at a revision fails the same way at that revision, and
// without remembering it the expensive part runs again on every reconcile: a
// module whose _module.cue does not compile, or an addon whose metadata cannot
// be parsed, crawls the whole registry, fails downstream of the crawl, and is
// requeued a second later by the workflow. The rate-limit gate does not help,
// because the registry is answering perfectly well. It is kept short so that
// fixing the cause does not need a push to the registry to take effect.
const DefaultFailureTTL = time.Minute

type revisionEntry[T any] struct {
	revision string
	value    T
	// err is set when the read at this revision failed, in which case value is
	// the zero value and must not be used.
	err error
}

// RevisionCache holds what a registry yielded for a package, keyed by the
// revision it was read at, so a reconcile that finds the revision unchanged
// spends nothing reading it again.
//
// This is the whole point of the type. Reading one module from a git registry
// costs one API request per directory in the registry plus one per file of the
// module, every time, and the render path runs on every reconcile of every
// Application that names it. Several applications retrying on failure once a
// second is enough to spend a 5000-request hour in minutes, and once the hour
// is spent every render fails, which makes every Application retry, which
// spends the next hour too.
type RevisionCache[T any] struct {
	// store is nil when it could not be built, and the cache then reads every
	// time -- slower and dearer, but never wrong.
	store      *velacache.LRUStore[string, revisionEntry[T]]
	ttl        time.Duration
	failureTTL time.Duration
	flight     singleflight.Group
}

// NewRevisionCache returns a cache bounded to size entries, holding each for at
// most ttl. A size or ttl below one gets the package default.
//
// The sweeper that expires entries runs for the life of the process, which is
// the life of this cache: one is built per package-level variable at startup,
// not per request.
func NewRevisionCache[T any](size int) *RevisionCache[T] {
	return newRevisionCache[T](context.Background(), size, DefaultRevisionTTL)
}

func newRevisionCache[T any](ctx context.Context, size int, ttl time.Duration) *RevisionCache[T] {
	if size < 1 {
		size = DefaultRevisionCacheSize
	}
	if ttl <= 0 {
		ttl = DefaultRevisionTTL
	}
	cache := &RevisionCache[T]{ttl: ttl, failureTTL: DefaultFailureTTL}
	store, err := velacache.NewLRUStore[string, revisionEntry[T]](ctx, velacache.Options[string, revisionEntry[T]]{
		MaxSize: size,
	})
	if err != nil {
		// Only a byte budget without a sizer, or a non-positive size, can fail
		// here, and neither is reachable from these options. Degrading to no
		// caching keeps the caller correct if that ever changes.
		klog.Errorf("cannot build the registry package cache, every read will go to the registry: %v", err)
		return cache
	}
	cache.store = store
	return cache
}

func (c *RevisionCache[T]) get(key string) (revisionEntry[T], bool) {
	if c.store == nil {
		return revisionEntry[T]{}, false
	}
	return c.store.Get(key)
}

func (c *RevisionCache[T]) put(key, revision string, value T) {
	if c.store == nil {
		return
	}
	c.store.Put(key, revisionEntry[T]{revision: revision, value: value}, c.ttl)
}

// putFailure remembers that reading this revision failed, briefly.
func (c *RevisionCache[T]) putFailure(key, revision string, readErr error) {
	if c.store == nil || readErr == nil {
		return
	}
	c.store.Put(key, revisionEntry[T]{revision: revision, err: readErr}, c.failureTTL)
}

// Reset empties the cache. It exists for tests, which would otherwise carry a
// package from one case into the next.
func (c *RevisionCache[T]) Reset() {
	if c.store == nil {
		return
	}
	c.store.Purge()
}

// Load returns the package stored under key, reading it through read only when
// the source says its revision has moved.
//
// revision is given the revision the cache holds so the source can revalidate
// conditionally, which for GitHub means a 304 that does not count against the
// rate limit at all, and for an OCI registry a manifest HEAD instead of a pull.
// A source that cannot report a revision returns ErrRevisionUnsupported and
// every call reads.
//
// read is handed the revision it must read at, empty when there is none to
// pin to. Reading at the checked revision is what keeps the entry honest: the
// source can move between the check and the read, and a read that returned
// older content than the revision it is filed under would be served for as
// long as that revision stayed current.
//
// When the revision probe itself fails and the cache holds a value, that value
// is served and the failure logged. This is deliberate. The cached value is
// what the registry served, not a guess, and refusing to use it is how one
// exhausted rate limit takes down every Application that was running fine. A
// caller that must not serve an unrevalidated package should probe the revision
// itself rather than going through the cache.
func (c *RevisionCache[T]) Load(key string, revision func(lastKnown string) (string, error), read func(revision string) (T, error)) (T, error) {
	cached, hasCached := c.get(key)

	current, err := revision(cached.revision)
	switch {
	case err == nil && hasCached && current != "" && current == cached.revision:
		if cached.err != nil {
			// The same read failed at this same revision moments ago. Running
			// it again would crawl the registry to reach the identical
			// failure, which is what the reconcile loop does once a second.
			return zeroOf[T](), cached.err
		}
		return cached.value, nil
	case goerrors.Is(err, ErrRevisionUnsupported):
		// Nothing to compare, so nothing to cache against either. Reading
		// every time is what this source always did.
		return read("")
	case err != nil:
		if hasCached && cached.err == nil {
			klog.Warningf("serving cached package %q read at revision %q: cannot check for a newer one: %v",
				key, cached.revision, err)
			return cached.value, nil
		}
		// Nothing cached, so there is nothing the probe can save. It is only an
		// optimisation, and its failure must not stand in for the read's own:
		// the read may well succeed where the probe could not (a blocked
		// manifest route, a source with no revision endpoint), and when it
		// fails too, its error is the one that names what the caller asked for.
		// A source that is rate limited fails the read locally at the gate, so
		// falling through spends no request.
		klog.V(4).Infof("cannot check the revision of package %q, reading it: %v", key, err)
		return read("")
	}

	// A miss, or a revision that moved. Collapse concurrent identical reads:
	// every Application naming this package reconciles on its own schedule, and
	// they all land together after a restart.
	value, err, _ := c.flight.Do(key+"@"+current, func() (interface{}, error) {
		// The winner of the race may have filled the cache while this call
		// waited inside Do.
		if entry, ok := c.get(key); ok && current != "" && entry.revision == current {
			if entry.err != nil {
				return nil, entry.err
			}
			return entry.value, nil
		}
		// Read at exactly the revision that was checked, so what is stored
		// cannot be older than the key it is stored under.
		fresh, err := read(current)
		if err != nil {
			if current != "" {
				c.putFailure(key, current, err)
			}
			return nil, err
		}
		if current != "" {
			c.put(key, current, fresh)
		}
		return fresh, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	typed, ok := value.(T)
	if !ok {
		// Unreachable while only read's own result is stored, but returning a
		// zero value with a nil error would hand the caller an empty package
		// and call it a success.
		var zero T
		return zero, fmt.Errorf("registry package cache holds %T for %q, not the expected type", value, key)
	}
	return typed, nil
}

// zeroOf is the zero value of T, for returning alongside an error.
func zeroOf[T any]() T {
	var zero T
	return zero
}
