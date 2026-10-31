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

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

const resolver_cache_entry_template = `
$internal: {
	key: "cache-entry"
	keyInputs: []
}
storage: {
	storageTTL: "1h"
}
output: {
  value: parameter.value
}
parameter: {
  value: string
}
`

const resolver_cache_entry_stale_template = `
$internal: {
	key: "cache-entry-stale"
	keyInputs: []
}
storage: {
	storageTTL: "1h"
	onStaleFailure: "use-stale"
}
output: {
  value: parameter.value
}
parameter: {
  value: string
}
`

// scriptedCacheStore answers reads with readErr while it is set, and otherwise from what
// was written; a successful write clears readErr, as a rewrite heals the entry it replaces.
// readErrData is returned with readErr, for a store that hands back data with its error.
type scriptedCacheStore struct {
	readErr     error
	readErrData map[string]interface{}
	writeErr    error
	entries     map[string]map[string]interface{}
	writes      int
}

func (s *scriptedCacheStore) Read(_ context.Context, cacheKey string, _ time.Duration) (process.SourceCacheRead, error) {
	if s.readErr != nil {
		return process.SourceCacheRead{Data: s.readErrData, Found: s.readErrData != nil, Stale: true}, s.readErr
	}
	data, ok := s.entries[cacheKey]
	if !ok {
		return process.SourceCacheRead{}, nil
	}
	return process.SourceCacheRead{Data: data, Found: true, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (s *scriptedCacheStore) Write(_ context.Context, cacheKey, _ string, data map[string]interface{}, _ process.SourceCacheWriteMeta) error {
	s.writes++
	if s.writeErr != nil {
		return s.writeErr
	}
	s.entries[cacheKey] = data
	s.readErr = nil
	return nil
}

// resolveWithStore resolves source s of template against store, with properties props,
// and returns the output and how many times the source was compiled (a fetch).
func resolveWithStore(t *testing.T, store process.SourceCacheStore, template string, props map[string]interface{}) (map[string]interface{}, int, error) {
	t.Helper()
	ctx := process.NewContext(process.ContextData{})
	ctx.PushData(process.ContextAppSourceCacheStore, store)
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	spy := &recordingCompiler{inner: velacuex.WorkloadCompiler.Get()}
	resolver.compiler = spy
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{"t": template}
	resolver.sourceProps = map[string]map[string]interface{}{"s": props}
	out, err := resolver.resolve("s")
	if err == nil {
		require.Equal(t, PhaseResolved, resolver.statuses["s"].Phase)
	}
	return out, spy.called, err
}

var freshProps = map[string]interface{}{"value": "fresh"}

// An entry the store cannot serve (the config store's answer for an entry with no
// input-properties) is a miss: the source is fetched, the entry is written again, and the
// next resolution is a hit, which writes nothing.
func TestResolveHealsAnEntryTheStoreCannotRead(t *testing.T) {
	store := &scriptedCacheStore{
		readErr: errors.New("secret vela-system/k has no input-properties"),
		entries: map[string]map[string]interface{}{},
	}
	out, compiles, err := resolveWithStore(t, store, resolver_cache_entry_template, freshProps)
	require.NoError(t, err)
	require.Equal(t, "fresh", out["value"])
	require.Positive(t, compiles, "fetched")
	require.Equal(t, 1, store.writes, "the entry is written again")

	out, compiles, err = resolveWithStore(t, store, resolver_cache_entry_template, freshProps)
	require.NoError(t, err)
	require.Equal(t, "fresh", out["value"])
	require.Zero(t, compiles, "a hit: nothing fetched")
	require.Equal(t, 1, store.writes, "nothing written")
}

// A Secret the cache did not write sits under the key: the read refuses it, and the write
// that follows the fetch fails on it (the factory refuses a Secret it did not make). The
// render uses the fresh value and does not fail; each resolution fetches again.
func TestResolveUsesTheFreshValueOverAnEntryItCannotWrite(t *testing.T) {
	store := &scriptedCacheStore{
		readErr:  errors.New("secret vela-system/k is not a source cache entry"),
		writeErr: errors.New("the template of the config can not be changed"),
		entries:  map[string]map[string]interface{}{},
	}
	for i := 1; i <= 2; i++ {
		out, _, err := resolveWithStore(t, store, resolver_cache_entry_template, freshProps)
		require.NoError(t, err)
		require.Equal(t, "fresh", out["value"])
		require.Equal(t, i, store.writes)
	}
}

// A read the store refused is no stored value: when the fetch then fails, a source that
// would serve a stale value fails instead, whatever the store returned with its error.
func TestResolveServesNoStaleValueFromARefusedRead(t *testing.T) {
	store := &scriptedCacheStore{
		readErr:     errors.New("secret vela-system/k is not a source cache entry"),
		readErrData: map[string]interface{}{"value": "planted"},
		entries:     map[string]map[string]interface{}{},
	}
	// a number where the source wants a string: the fetch fails
	out, _, err := resolveWithStore(t, store, resolver_cache_entry_stale_template, map[string]interface{}{"value": 1})
	require.Error(t, err)
	require.NotEqual(t, "planted", out["value"])
}

// The Secret-backed store with a Secret of the key's name it did not write: the read refuses
// it, the write leaves it alone, and the resolution uses the fresh value.
func TestResolveWithTheSecretStoreLeavesAPlantedSecretAlone(t *testing.T) {
	key, err := cacheIdentity("cache-entry", identityInputs{
		Template:   templateFingerprint(resolver_cache_entry_template),
		Properties: freshProps,
	})
	require.NoError(t, err)
	planted := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: key, Namespace: CacheNamespace()},
		Data:       map[string][]byte{sourceCacheDataKey: []byte(`{"value":"planted"}`)},
	}
	cli := fake.NewClientBuilder().WithObjects(planted).Build()

	out, compiles, err := resolveWithStore(t, NewSecretSourceCacheStore(cli), resolver_cache_entry_template, freshProps)
	require.NoError(t, err)
	require.Equal(t, "fresh", out["value"])
	require.Positive(t, compiles)
	var got corev1.Secret
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(planted), &got))
	require.JSONEq(t, `{"value":"planted"}`, string(got.Data[sourceCacheDataKey]))
	require.Empty(t, got.Labels)
}
