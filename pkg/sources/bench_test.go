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

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// These guard the steady-state cost of a source: what an Application pays on
// every render for a binding whose value has not changed and is already stored.
//
// Before the policy cache that was 490,826 ns per source, of which 468,842 ns
// was resolveCachePolicy - which runs before the store is consulted, because it
// is what says which entry to look for. If BenchmarkSteadyStateSource regresses
// by two orders of magnitude, that cache has stopped being hit.

// alwaysHit answers every read as a fresh hit, which is the steady state.
type alwaysHit struct{ data map[string]interface{} }

func (a alwaysHit) Read(_ context.Context, _ string, _ time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	return a.data, false, true, time.Now().Add(time.Hour), nil
}

func (a alwaysHit) Write(_ context.Context, _, _ string, _ map[string]interface{}, _ velaprocess.SourceCacheWriteMeta) error {
	return nil
}

const benchTemplate = `
schema: {host: string}
$internal: {key: "bench-\(context.namespace)", keyInputs: ["namespace"]}
storage: {storageTTL: "15m", onStaleFailure: "use-stale"}
parameter: {name: string}
output: {host: "example.com"}
`

func benchContext(tb testing.TB) (wfprocess.Context, sourceInputs) {
	tb.Helper()
	pCtx := velaprocess.NewContext(velaprocess.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(velaprocess.ContextAppSources, map[string]map[string]interface{}{"cfg": {"name": "some-cm"}})
	pCtx.PushData(velaprocess.ContextAppSourceTypes, map[string]string{"cfg": "bench"})
	pCtx.PushData(velaprocess.ContextAppSourceTemplates, map[string]string{"bench": benchTemplate})
	in := sourceInputsFromContext(pCtx)
	in.Store = alwaysHit{data: map[string]interface{}{"host": "example.com"}}
	return pCtx, in
}

// One already-cached source, resolved on a fresh render.
func BenchmarkSteadyStateSource(b *testing.B) {
	pCtx, in := benchContext(b)
	ctxVals := contextValuesFor(pCtx)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := newSourceResolver(pCtx.GetCtx(), ctxVals, SurfaceComponent, in)
		if _, err := r.resolve("cfg"); err != nil {
			b.Fatal(err)
		}
	}
}

// The policy derivation on its own, which was 95% of the above.
func BenchmarkResolveCachePolicy(b *testing.B) {
	pCtx, in := benchContext(b)
	r := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, in)
	props := map[string]interface{}{"name": "some-cm"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.resolveCachePolicy("cfg", "bench", benchTemplate, props); err != nil {
			b.Fatal(err)
		}
	}
}

// A render reading one field of that source, end to end through the public
// entry point - the number an Application actually pays.
func BenchmarkRenderOneExpression(b *testing.B) {
	pCtx, _ := benchContext(b)
	pCtx.PushData(velaprocess.ContextAppSourceCacheStore,
		velaprocess.SourceCacheStore(alwaysHit{data: map[string]interface{}{"host": "example.com"}}))
	props := map[string]interface{}{"h": "$(source.cfg.host)"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ResolveSourceExpressions(pCtx, props, SurfaceComponent); err != nil {
			b.Fatal(err)
		}
	}
}
