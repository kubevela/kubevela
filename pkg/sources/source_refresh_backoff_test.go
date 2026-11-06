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
	"errors"
	"testing"
	"time"

	"cuelang.org/go/cue"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// failingCompiler counts the refreshes attempted and fails every one, standing
// in for the fetch a template performs against an endpoint that is down.
//
// Only the refresh fails. Reading the storage block compiles the same template
// with provider functions disabled, so it fetches nothing and is let through to
// the real compiler; failing it would stop the resolution before it reached the
// refresh this is about.
type failingCompiler struct{ attempts int }

func (c *failingCompiler) CompileString(context.Context, string) (cue.Value, error) {
	c.attempts++
	return cue.Value{}, errors.New("the endpoint is unreachable")
}

func (c *failingCompiler) CompileStringWithOptions(ctx context.Context, src string, opts ...upstreamcuex.CompileOption) (cue.Value, error) {
	return velacuex.SourceCompiler.Get().CompileStringWithOptions(ctx, src, opts...)
}

// staleStore always reports a stored value that is past its TTL, which is the
// state a source is left in once its upstream stops answering.
type staleStore struct{}

func (staleStore) Read(context.Context, string, time.Duration) (velaprocess.SourceCacheRead, error) {
	return velaprocess.SourceCacheRead{
		Data:      map[string]interface{}{"host": "db.internal"},
		Found:     true,
		Stale:     true,
		ExpiresAt: time.Now().Add(-time.Hour),
	}, nil
}

func (staleStore) Write(context.Context, string, string, map[string]interface{}, velaprocess.SourceCacheWriteMeta) error {
	return nil
}

const backoffTemplate = `
$internal: {key: "demo", keyInputs: []}
storage: {storageTTL: "30m", onStaleFailure: "use-stale"}
schema: {host: string}
output: {host: "db.internal"}
`

func backoffResolver(c SourceCompiler) *sourceResolver {
	return &sourceResolver{
		goCtx:           context.Background(),
		ctxValues:       map[string]interface{}{},
		surface:         SurfaceComponent,
		statuses:        map[string]SourceResolutionStatus{},
		sourceProps:     map[string]map[string]interface{}{"cfg": {}},
		sourceTypes:     map[string]string{"cfg": "demo"},
		sourceTemplates: map[string]string{"demo": backoffTemplate},
		sourceSchemas:   map[string]string{},
		sensitivePaths:  map[string][]string{},
		cacheStore:      staleStore{},
		compiler:        c,
		resolved:        map[string]map[string]interface{}{},
		resolving:       map[string]bool{},
	}
}

// A source whose upstream is down is retried no more often than a healthy one
// is re-read. Without a hold, every reconcile sends the dead endpoint the
// template's whole fetch again, for as long as it stays down.
func TestAFailedRefreshIsNotRetriedOnTheNextReconcile(t *testing.T) {
	forgetRefreshBackoffs()
	c := &failingCompiler{}

	first, err := backoffResolver(c).resolve("cfg")
	require.NoError(t, err, "the stale value is served")
	require.Equal(t, map[string]interface{}{"host": "db.internal"}, first)
	require.Equal(t, 1, c.attempts, "the first reconcile tries")

	second, err := backoffResolver(c).resolve("cfg")
	require.NoError(t, err, "the stale value is still served")
	require.Equal(t, map[string]interface{}{"host": "db.internal"}, second)
	require.Equal(t, 1, c.attempts, "the next reconcile does not try again")
}

// A source that refreshes is not held: the hold exists for a failing upstream,
// and must not delay the next read of a working one.
func TestASuccessfulRefreshClearsTheHold(t *testing.T) {
	forgetRefreshBackoffs()
	recordRefreshFailure("demo-abc", 30*time.Minute, time.Now(), errors.New("unreachable"), true)
	require.True(t, backoffHoldsRefresh("demo-abc", time.Now()))
	clearRefreshBackoff("demo-abc")
	require.False(t, backoffHoldsRefresh("demo-abc", time.Now()))
}

// The wait grows with consecutive failures so a long outage is not polled at
// the rate of a brief one, and never past the freshness the author asked for.
func TestTheHoldGrowsAndIsCappedByTheDeclaredTTL(t *testing.T) {
	forgetRefreshBackoffs()
	now := time.Now()
	ttl := 30 * time.Minute

	recordRefreshFailure("k", ttl, now, errors.New("unreachable"), true)
	require.False(t, backoffHoldsRefresh("k", now.Add(refreshBackoffBase)), "the first wait is the base")
	require.True(t, backoffHoldsRefresh("k", now.Add(refreshBackoffBase/2)))

	recordRefreshFailure("k", ttl, now, errors.New("unreachable"), true)
	require.True(t, backoffHoldsRefresh("k", now.Add(refreshBackoffBase)), "the second wait is longer")

	for i := 0; i < 20; i++ {
		recordRefreshFailure("k", ttl, now, errors.New("unreachable"), true)
	}
	require.False(t, backoffHoldsRefresh("k", now.Add(ttl)), "and never longer than the TTL")

	// A TTL shorter than the base is the cap, so a source asking to be fresh
	// every 5s is not held for 30.
	forgetRefreshBackoffs()
	recordRefreshFailure("short", 5*time.Second, now, errors.New("unreachable"), true)
	require.False(t, backoffHoldsRefresh("short", now.Add(5*time.Second)))
}

// flakyCompiler fails its first failUntil refreshes and then does the real
// thing, so a resolution can be driven through failure, recovery and failure
// again.
type flakyCompiler struct {
	attempts  int
	failUntil int
}

func (c *flakyCompiler) CompileString(ctx context.Context, src string) (cue.Value, error) {
	c.attempts++
	if c.attempts <= c.failUntil {
		return cue.Value{}, errors.New("the endpoint is unreachable")
	}
	return velacuex.SourceCompiler.Get().CompileString(ctx, src)
}

func (c *flakyCompiler) CompileStringWithOptions(ctx context.Context, src string, opts ...upstreamcuex.CompileOption) (cue.Value, error) {
	return velacuex.SourceCompiler.Get().CompileStringWithOptions(ctx, src, opts...)
}

// A short TTL, so the hold it caps expires inside a test rather than in thirty
// seconds.
const briefTemplate = `
$internal: {key: "brief", keyInputs: []}
storage: {storageTTL: "20ms", onStaleFailure: "use-stale"}
schema: {host: string}
output: {host: "db.internal"}
`

func briefResolver(c SourceCompiler) *sourceResolver {
	r := backoffResolver(c)
	r.sourceTemplates = map[string]string{"demo": briefTemplate}
	return r
}

// A source that recovers starts again from the shortest wait. Without the
// count being cleared, a binding that failed twice months ago would have its
// next blip held for the long wait it had reached back then.
func TestARecoveredSourceForgetsItsFailures(t *testing.T) {
	forgetRefreshBackoffs()
	c := &flakyCompiler{failUntil: 2}

	// The cache key is a prefix and a hash of the identity, so it is read back
	// from the status rather than assumed.
	key := func(r *sourceResolver) string { return r.statuses["cfg"].Config }
	failures := func(k string) (int, bool) {
		hit, ok := refreshBackoffs.Get(k)
		if !ok {
			return 0, false
		}
		hold, ok := hit.(refreshHold)
		return hold.failures, ok
	}

	var cacheKey string
	for i := 0; i < 2; i++ {
		r := briefResolver(c)
		_, err := r.resolve("cfg")
		require.NoError(t, err, "the stale value is served")
		cacheKey = key(r)
		require.NotEmpty(t, cacheKey)
		time.Sleep(40 * time.Millisecond)
	}
	n, ok := failures(cacheKey)
	require.True(t, ok)
	require.Equal(t, 2, n, "two in a row")

	// The third refresh works.
	r := briefResolver(c)
	_, err := r.resolve("cfg")
	require.NoError(t, err)
	_, stillHeld := failures(cacheKey)
	require.False(t, stillHeld, "a refresh that worked forgets the failures")

	// So the next failure is a first failure again.
	c.failUntil = c.attempts + 1
	time.Sleep(40 * time.Millisecond)
	_, err = briefResolver(c).resolve("cfg")
	require.NoError(t, err)
	n, ok = failures(cacheKey)
	require.True(t, ok)
	require.Equal(t, 1, n,
		"the count restarts, so a later blip is not held for the wait an old outage reached")
}

// A source that may not serve a stale value is still not asked again while its
// hold stands. The binding fails either way, so the only difference the fetch
// makes is to the upstream, and the failure that put the hold there is what the
// binding reports.
func TestAHeldRefreshDoesNotFetchUnderTheFailPolicy(t *testing.T) {
	forgetRefreshBackoffs()
	c := &failingCompiler{}
	resolver := func() *sourceResolver {
		r := backoffResolver(c)
		r.sourceTemplates = map[string]string{"demo": `
$internal: {key: "strict", keyInputs: []}
storage: {storageTTL: "30m", onStaleFailure: "fail"}
schema: {host: string}
output: {host: "db.internal"}
`}
		return r
	}

	_, err := resolver().resolve("cfg")
	require.Error(t, err, "a stale value may not be served, so the binding fails")
	require.Equal(t, 1, c.attempts)

	_, err = resolver().resolve("cfg")
	require.Error(t, err, "and still fails")
	require.Contains(t, err.Error(), "unreachable", "reporting what actually went wrong")
	require.Equal(t, 1, c.attempts, "without asking the upstream again")
}

// A binding with no value to serve is down, and the attempt being held is the
// only way it comes back. So the wait does not grow however long the outage
// lasts: an Application created before the ConfigMap it reads, or one whose
// host is away for ten minutes, must not be kept failed for the freshness
// window its definition happens to declare.
func TestAHoldDoesNotGrowWhileNothingCanBeServed(t *testing.T) {
	forgetRefreshBackoffs()
	now := time.Now()
	ttl := 24 * time.Hour
	cause := errors.New("unreachable")

	for i := 0; i < 8; i++ {
		recordRefreshFailure("down", ttl, now, cause, false)
		require.False(t, backoffHoldsRefresh("down", now.Add(refreshBackoffBase)),
			"after %d failures the wait is still the shortest one", i+1)
	}

	hit, ok := refreshBackoffs.Get("down")
	require.True(t, ok)
	require.Equal(t, 8, hit.(refreshHold).failures, "the failures are still counted")

	// Once a value can be served the Application is up, so the wait may grow.
	forgetRefreshBackoffs()
	recordRefreshFailure("serving", ttl, now, cause, true)
	recordRefreshFailure("serving", ttl, now, cause, true)
	require.True(t, backoffHoldsRefresh("serving", now.Add(refreshBackoffBase)),
		"a binding still serving its value backs off further")
}

// The whole point, through resolve: a source that can serve nothing is retried
// at the shortest interval no matter how many times it has failed.
func TestAFailingSourceWithNothingToServeKeepsTheShortestHold(t *testing.T) {
	forgetRefreshBackoffs()
	c := &failingCompiler{}
	resolver := func() *sourceResolver {
		r := backoffResolver(c)
		r.sourceTemplates = map[string]string{"demo": `
$internal: {key: "down", keyInputs: []}
storage: {storageTTL: "20ms", onStaleFailure: "fail"}
schema: {host: string}
output: {host: "db.internal"}
`}
		return r
	}

	var key string
	for i := 0; i < 3; i++ {
		r := resolver()
		_, err := r.resolve("cfg")
		require.Error(t, err)
		key = r.statuses["cfg"].Config
		time.Sleep(40 * time.Millisecond)
	}
	require.Equal(t, 3, c.attempts, "every attempt after the hold expires is made")

	hit, ok := refreshBackoffs.Get(key)
	require.True(t, ok)
	require.Equal(t, 3, hit.(refreshHold).failures)
	require.False(t, backoffHoldsRefresh(key, time.Now().Add(refreshWait(1, 20*time.Millisecond))),
		"the hold is still the shortest one after three failures")
}

// realCompiler lets the refresh through to the real compiler, so a template
// that resolves and reports its own `errs` is evaluated as the controller
// evaluates it.
type realCompiler struct{}

func (realCompiler) CompileString(ctx context.Context, src string) (cue.Value, error) {
	return velacuex.SourceCompiler.Get().CompileString(ctx, src)
}

func (realCompiler) CompileStringWithOptions(ctx context.Context, src string, opts ...upstreamcuex.CompileOption) (cue.Value, error) {
	return velacuex.SourceCompiler.Get().CompileStringWithOptions(ctx, src, opts...)
}

// A held refusal reports the failure the same way the fresh one did. The
// resolver's own error type carries the definition's `errs` entries separately
// from the message, and a caller matching on it gets them whichever attempt it
// is looking at.
func TestAHeldRefusalCarriesTheSameErrorShape(t *testing.T) {
	forgetRefreshBackoffs()
	resolver := func() *sourceResolver {
		r := backoffResolver(realCompiler{})
		r.sourceTemplates = map[string]string{"demo": `
$internal: {key: "errs", keyInputs: []}
storage: {storageTTL: "30m", onStaleFailure: "fail"}
schema: {host: string}
errs: ["the shop is closed"]
output: {host: "db.internal"}
`}
		return r
	}

	_, fresh := resolver().resolve("cfg")
	require.Error(t, fresh)
	var freshDef *DefinitionError
	require.ErrorAs(t, fresh, &freshDef, "the fresh failure carries the resolver's error type")
	require.Equal(t, []string{"the shop is closed"}, freshDef.User)

	_, held := resolver().resolve("cfg")
	require.Error(t, held)
	require.Contains(t, held.Error(), "next attempt in", "this one was refused by the hold")
	var heldDef *DefinitionError
	require.ErrorAs(t, held, &heldDef, "and so does the held one")
	require.Equal(t, freshDef.User, heldDef.User, "with the same entries a caller would read")
}

// A harness that runs independent cases in one process drops the holds between
// them, so the exported reset has to clear a hold that is still in force.
func TestForgetRefreshHoldsClearsAHoldInForce(t *testing.T) {
	forgetRefreshBackoffs()
	now := time.Now()
	recordRefreshFailure("entry-a", 30*time.Minute, now, errors.New("the endpoint is unreachable"), false)
	_, held := heldRefresh("entry-a", now)
	require.True(t, held, "the failure put a hold on the entry")

	ForgetRefreshHolds()
	_, held = heldRefresh("entry-a", now)
	require.False(t, held, "and the reset drops it")
}
