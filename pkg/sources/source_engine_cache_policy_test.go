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

	"github.com/stretchr/testify/require"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

func TestSourceEngineCachePolicy(t *testing.T) {
	tmpl, _, err := cachekey.Stamp("settings", `
schema: {tier: string}
storage: {storageTTL: "90s", onStaleFailure: "fail"}
parameter: {name: string}
output: tier: "\(context.namespace)/\(parameter.name)"
`)
	require.NoError(t, err)
	policyIn := func(ctxValues map[string]interface{}) CachePolicy {
		engine, err := NewSourceEngine(SourceEngineOptions{
			Surface:   SurfaceComponent,
			Context:   ctxValues,
			Bindings:  map[string]map[string]interface{}{"cfg": {"name": "shop"}},
			Types:     map[string]string{"cfg": "settings"},
			Templates: map[string]string{"settings": tmpl},
			Compiler:  velacuex.SourceCompiler.Get(),
		})
		require.NoError(t, err)
		p, err := engine.CachePolicy(context.Background(), "cfg")
		require.NoError(t, err)
		return p
	}
	teamA := policyIn(map[string]interface{}{"namespace": "team-a", "appName": "shop", "cluster": "local"})
	require.Equal(t, 90*time.Second, teamA.TTL)
	require.Equal(t, "fail", teamA.OnStaleFailure)
	require.Equal(t, []string{"namespace"}, teamA.KeyInputs, "the template reads context.namespace and nothing else")
	require.Regexp(t, `^settings-`, teamA.Key)

	otherApp := policyIn(map[string]interface{}{"namespace": "team-a", "appName": "checkout", "cluster": "local"})
	require.Equal(t, teamA.Key, otherApp.Key, "an Application the template does not read shares the entry")
	teamB := policyIn(map[string]interface{}{"namespace": "team-b", "appName": "shop", "cluster": "local"})
	require.NotEqual(t, teamA.Key, teamB.Key, "another namespace is another entry")
}

// The key is the one a resolve uses, with the binding's expressions
// substituted first.
func TestSourceEngineCachePolicySubstitutesProperties(t *testing.T) {
	tmpl, _, err := cachekey.Stamp("echo", `
parameter: {ns: string}
output: ns: parameter.ns
`)
	require.NoError(t, err)
	engine, err := NewSourceEngine(SourceEngineOptions{
		Surface:   SurfaceComponent,
		Context:   map[string]interface{}{"namespace": "team-a", "cluster": "local"},
		Bindings:  map[string]map[string]interface{}{"cfg": {"ns": "$(context.namespace)"}},
		Types:     map[string]string{"cfg": "echo"},
		Templates: map[string]string{"echo": tmpl},
		Compiler:  velacuex.SourceCompiler.Get(),
	})
	require.NoError(t, err)
	res, err := engine.Resolve(context.Background(), map[string]interface{}{"o": "$(source.cfg)"})
	require.NoError(t, err)
	policy, err := engine.CachePolicy(context.Background(), "cfg")
	require.NoError(t, err)
	require.Equal(t, res.Statuses["cfg"].Config, policy.Key)
}

// keyedStore holds entries by key, all fresh or all stale, and records the
// key each source type was written under.
type keyedStore struct {
	entries map[string]map[string]interface{}
	written map[string]string
	stale   bool
}

func (s *keyedStore) Read(_ context.Context, cacheKey string, _ time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	v, ok := s.entries[cacheKey]
	return v, ok && s.stale, ok, time.Time{}, nil
}

func (s *keyedStore) Write(_ context.Context, cacheKey, sourceType string, data map[string]interface{}, _ velaprocess.SourceCacheWriteMeta) error {
	s.entries[cacheKey] = data
	s.written[sourceType] = cacheKey
	return nil
}

// A chained source is read through the store as a resolve reads it, so the
// key matches the one the resolve stores; and working it out writes nothing,
// even where the upstream source is fetched afresh and a resolve would write.
func TestSourceEngineCachePolicyMatchesTheStoredKey(t *testing.T) {
	cases := map[string]struct{ cached, stale bool }{
		"a fresh entry":  {cached: true},
		"a stale entry":  {cached: true, stale: true},
		"an empty cache": {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) { cachePolicyMatchesTheStoredKey(t, tc.cached, tc.stale) })
	}
}

func cachePolicyMatchesTheStoredKey(t *testing.T, cached, stale bool) {
	upTmpl, _, err := cachekey.Stamp("up", `
parameter: {}
output: name: "fresh"
`)
	require.NoError(t, err)
	downTmpl, _, err := cachekey.Stamp("down", `
parameter: {name: string}
output: name: parameter.name
`)
	require.NoError(t, err)
	store := &keyedStore{entries: map[string]map[string]interface{}{}, written: map[string]string{}, stale: stale}
	engine, err := NewSourceEngine(SourceEngineOptions{
		Surface: SurfaceComponent,
		Context: map[string]interface{}{"namespace": "team-a", "cluster": "local"},
		Bindings: map[string]map[string]interface{}{
			"u": {},
			"d": {"name": "$(source.u.name)"},
		},
		Types:     map[string]string{"u": "up", "d": "down"},
		Templates: map[string]string{"up": upTmpl, "down": downTmpl},
		Sensitive: map[string][]string{"down": {"name"}},
		Store:     store,
		Compiler:  velacuex.SourceCompiler.Get(),
	})
	require.NoError(t, err)
	if cached {
		up, err := engine.CachePolicy(context.Background(), "u")
		require.NoError(t, err)
		store.entries[up.Key] = map[string]interface{}{"name": "cached"}
	}

	down, err := engine.CachePolicy(context.Background(), "d")
	require.NoError(t, err)
	require.Empty(t, store.written, "working out a cache policy writes nothing")

	_, err = engine.Resolve(context.Background(), map[string]interface{}{"o": "$(source.d.name)"})
	require.NoError(t, err)
	require.Equal(t, store.written["down"], down.Key)
}

func TestSourceEngineCachePolicyNamesWhatIsMissing(t *testing.T) {
	engine, err := NewSourceEngine(SourceEngineOptions{
		Surface:   SurfaceComponent,
		Bindings:  map[string]map[string]interface{}{"cfg": {}},
		Types:     map[string]string{"cfg": "settings"},
		Templates: map[string]string{},
		Compiler:  velacuex.SourceCompiler.Get(),
	})
	require.NoError(t, err)
	_, err = engine.CachePolicy(context.Background(), "other")
	require.EqualError(t, err, `source "other" not found`)
	_, err = engine.CachePolicy(context.Background(), "cfg")
	require.EqualError(t, err, `source definition "settings" for source "cfg" is missing cue template`)
}
