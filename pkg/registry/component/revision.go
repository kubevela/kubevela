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
	goerrors "errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// IsPackageName reports whether name is usable as a single directory under a
// registry's configured path.
//
// A scoped listing joins the name onto that path to build a request, so an
// unchecked name escapes the registry's declared subtree: path.Join cleans
// "../.." away, turning a three-segment path into a one-segment one and
// listing a directory the platform team never exposed. Names come straight off
// an Application spec -- nothing between the CUE provider and here validates
// them -- so this is the boundary.
func IsPackageName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return name == path.Clean(name)
}

// ErrPackageNotExist means the named package is not in the registry. It is
// separate from ErrRegistryNotExist: the registry answered, and the answer was
// that it does not carry this package.
var ErrPackageNotExist = NewError("package does not exist in registry")

// defaultRateLimitHold is how long a source is left alone when it refused for
// rate limiting but did not say until when. GitHub always says, so this covers
// the sources that do not.
const defaultRateLimitHold = time.Minute

// maxRateLimitHold caps how long a refusal is honoured. The deadline is
// server-controlled -- a Retry-After of 31536000 is a year, and an HTTP date
// can name any future day -- and the gate refuses every read for that source
// until it passes, with nothing but a restart to undo it. An hour is GitHub's
// own reset window, so nothing legitimate needs longer; a misconfigured proxy
// asking for more gets an hour.
const maxRateLimitHold = time.Hour

// RateLimitedError says a source refused the request for rate limiting, and
// when it is worth asking again. The reset time is the part that matters: a
// reconcile loop that retries every second otherwise spends a request each
// time to be told the same thing, which keeps the limit exhausted through its
// own reset and turns a transient refusal into a permanent one.
type RateLimitedError struct {
	// Source identifies what refused, as owner/repo or registry host.
	Source string
	// Until is when the source said its limit resets. Zero if it did not say.
	Until time.Time
}

func (e *RateLimitedError) Error() string {
	switch {
	case e.Source != "" && !e.Until.IsZero():
		return fmt.Sprintf("%s: %s, resets at %s", e.Source, ErrRateLimit.Error(), e.Until.UTC().Format(time.RFC3339))
	case e.Source != "":
		return fmt.Sprintf("%s: %s", e.Source, ErrRateLimit.Error())
	case !e.Until.IsZero():
		return fmt.Sprintf("%s, resets at %s", ErrRateLimit.Error(), e.Until.UTC().Format(time.RFC3339))
	}
	return ErrRateLimit.Error()
}

// Unwrap reports ErrRateLimit so that every existing errors.Is test for the
// sentinel keeps matching now that the concrete error carries the reset time.
func (e *RateLimitedError) Unwrap() error { return ErrRateLimit }

// RetryAfter is how long to wait before asking the source again, or zero if it
// is worth asking now.
func (e *RateLimitedError) RetryAfter() time.Duration {
	if d := time.Until(e.Until); d > 0 {
		return d
	}
	return 0
}

// rateLimitGate remembers that a source refused for rate limiting and until
// when, so the refusal is served locally instead of spending a request to hear
// it again. It is process-wide on purpose: a git helper is built per call, so
// go-github's own per-client rate-limit memory never survives to the next one.
type rateLimitGate struct {
	mu    sync.Mutex
	until map[string]time.Time
}

var sourceRateLimit = &rateLimitGate{until: map[string]time.Time{}}

// blocked returns the error to answer with instead of calling the source, or
// nil if the source is worth calling.
func (g *rateLimitGate) blocked(key string) error {
	if key == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.until[key]
	if !ok {
		return nil
	}
	if !time.Now().Before(until) {
		delete(g.until, key)
		return nil
	}
	return &RateLimitedError{Source: key, Until: until}
}

func (g *rateLimitGate) trip(key string, until time.Time) {
	if key == "" {
		return
	}
	if latest := time.Now().Add(maxRateLimitHold); until.After(latest) {
		klog.Warningf("source %q asked to be left alone until %s; holding until %s instead",
			key, until.UTC().Format(time.RFC3339), latest.UTC().Format(time.RFC3339))
		until = latest
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.until[key]; ok && existing.After(until) {
		return
	}
	g.until[key] = until
}

// ResetRateLimitGate forgets every recorded rate limit. It exists for tests,
// which would otherwise leak a hold from one case into the next.
func ResetRateLimitGate() {
	sourceRateLimit.mu.Lock()
	defer sourceRateLimit.mu.Unlock()
	sourceRateLimit.until = map[string]time.Time{}
}

// rateLimitHold reads a deadline off an error this gate already produced, so a
// refusal that is re-wrapped keeps its original hold rather than extending it.
// A transport that recognises its own rate-limit error converts it to a
// RateLimitedError first; holdOCIThrottle is the one that does.
func rateLimitHold(err error) (time.Time, bool) {
	var already *RateLimitedError
	if goerrors.As(err, &already) {
		return already.Until, true
	}
	return time.Time{}, false
}
