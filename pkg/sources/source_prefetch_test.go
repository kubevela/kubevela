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
	gocontext "context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"cuelang.org/go/cue"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// slowCompiler counts compilations and holds each one open, so a sequential
// resolve takes n*delay and a concurrent one takes about delay.
type slowCompiler struct {
	inner    SourceCompiler
	delay    time.Duration
	calls    int32
	maxLive  int32
	live     int32
	liveLock sync.Mutex
}

func (c *slowCompiler) track() func() {
	atomic.AddInt32(&c.calls, 1)
	now := atomic.AddInt32(&c.live, 1)
	c.liveLock.Lock()
	if now > c.maxLive {
		c.maxLive = now
	}
	c.liveLock.Unlock()
	time.Sleep(c.delay)
	return func() { atomic.AddInt32(&c.live, -1) }
}

// prefetchContext builds n independent bindings of one type, plus a chained one
// that reads the first two.
func prefetchContext(t *testing.T, n int) wfprocess.Context {
	t.Helper()
	bindings := map[string]map[string]interface{}{}
	types := map[string]string{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("leaf%d", i)
		bindings[name] = map[string]interface{}{"which": name}
		types[name] = "echo"
	}
	bindings["chained"] = map[string]interface{}{"which": "$(source.leaf0.v)"}
	types["chained"] = "echo"

	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, bindings)
	pCtx.PushData(process.ContextAppSourceTypes, types)
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"echo": `
schema: {v: string}
$internal: {key: "echo", keyInputs: []}
parameter: {which: string}
output: {v: parameter.which}
`})
	return pCtx
}

// TestPrefetchResolvesIndependentBindingsConcurrently is the point of the
// change: bindings that read nothing from each other should not wait for each
// other.
func TestPrefetchResolvesIndependentBindingsConcurrently(t *testing.T) {
	const n = 4
	pCtx := prefetchContext(t, n)

	in := sourceInputsFromContext(pCtx)
	spy := &slowCompiler{inner: velacuex.WorkloadCompiler.Get(), delay: 150 * time.Millisecond}
	in.Compiler = spy

	r := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, in)

	props := map[string]interface{}{}
	for i := 0; i < n; i++ {
		props[fmt.Sprintf("p%d", i)] = fmt.Sprintf("$(source.leaf%d.v)", i)
	}

	start := time.Now()
	r.prefetch(props)
	elapsed := time.Since(start)

	// At least one compilation per binding. It is two in practice - the storage
	// block is evaluated with providers disabled before the cache is consulted,
	// then the template proper runs on a miss - so this asserts the floor rather
	// than pinning an implementation detail.
	assert.GreaterOrEqual(t, atomic.LoadInt32(&spy.calls), int32(n),
		"every leaf should have been resolved")
	assert.Greater(t, spy.maxLive, int32(1),
		"more than one binding must have been in flight at once")
	assert.Less(t, elapsed, time.Duration(n)*spy.delay,
		"concurrent resolution must beat doing them one after another")

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("leaf%d", i)
		require.Contains(t, r.resolved, name, "prefetched values must land in the memo")
		assert.Equal(t, name, r.resolved[name]["v"])
	}
}

// TestPrefetchSkipsChainedBindings pins which bindings take part. A chained one
// is resolved on demand, in order; the bindings it reads are still prefetched,
// because that is where the waiting is.
func TestPrefetchSkipsChainedBindings(t *testing.T) {
	pCtx := prefetchContext(t, 2)
	r := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, sourceInputsFromContext(pCtx))

	got := r.independentBindings(map[string]interface{}{
		"a": "$(source.chained.v)",
		"b": "$(source.leaf1.v)",
	})
	sort.Strings(got)
	assert.Equal(t, []string{"leaf0", "leaf1"}, got,
		"chained itself is excluded; leaf0, which it reads, is reached through its properties")
}

// TestPrefetchIsBehaviourNeutral is the safety property. Whatever the render
// produced before, it must produce now - including for bindings that fail.
func TestPrefetchIsBehaviourNeutral(t *testing.T) {
	pCtx := prefetchContext(t, 3)

	props := map[string]interface{}{
		"one":     "$(source.leaf0.v)",
		"two":     "$(source.leaf1.v)",
		"chained": "$(source.chained.v)",
	}

	withPrefetch, err := ResolveSourceExpressions(pCtx, props, SurfaceComponent)
	require.NoError(t, err)

	// The same work with the prefetch bypassed.
	fresh := prefetchContext(t, 3)
	r := newSourceResolver(fresh.GetCtx(), contextValuesFor(fresh), SurfaceComponent, sourceInputsFromContext(fresh))
	sequential, err := resolveSourceNode(props, r)
	require.NoError(t, err)

	assert.Equal(t, sequential, withPrefetch,
		"prefetching must not change what a render produces")
}

// TestPrefetchLeavesFailuresToTheLazyPath pins that a binding which cannot
// resolve is reported by the walk, with its own message, rather than being
// swallowed by the prefetch.
func TestPrefetchLeavesFailuresToTheLazyPath(t *testing.T) {
	pCtx := prefetchContext(t, 2)

	_, err := ResolveSourceExpressions(pCtx, map[string]interface{}{
		"ok":      "$(source.leaf0.v)",
		"missing": "$(source.nosuch.v)",
	}, SurfaceComponent)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nosuch",
		"the failure must name the binding, exactly as it did before prefetching")
}

func (c *slowCompiler) CompileString(ctx gocontext.Context, src string) (cue.Value, error) {
	done := c.track()
	defer done()
	return c.inner.CompileString(ctx, src)
}

func (c *slowCompiler) CompileStringWithOptions(ctx gocontext.Context, src string,
	opts ...upstreamcuex.CompileOption) (cue.Value, error) {
	done := c.track()
	defer done()
	return c.inner.CompileStringWithOptions(ctx, src, opts...)
}
