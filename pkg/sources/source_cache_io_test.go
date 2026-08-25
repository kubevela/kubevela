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

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

func resolverWithStore(store velaprocess.SourceCacheStore, ctxValues map[string]interface{}) *sourceResolver {
	return newSourceResolver(context.Background(), ctxValues, SurfaceComponent,
		sourceInputs{Store: store})
}

// Caching off, or no key to cache under: both are no-ops rather than errors.
func TestSourceCacheIOIsANoOpWithoutAStoreOrKey(t *testing.T) {
	store := &fakeSourceCacheStore{}

	_, _, found, _, err := resolverWithStore(nil, nil).readSourceCache("k", time.Minute)
	require.NoError(t, err)
	require.False(t, found)

	_, _, found, _, err = resolverWithStore(store, nil).readSourceCache("", time.Minute)
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, 0, store.reads)

	require.NoError(t, resolverWithStore(nil, nil).writeSourceCache("k", "t", nil, 0, nil, identityInputs{}))
	require.NoError(t, resolverWithStore(store, nil).writeSourceCache("", "t", nil, 0, nil, identityInputs{}))
	require.Equal(t, 0, store.writes)
}

// The write carries the identity the entry is addressed by. Losing any of it
// means an entry that outlives the thing that produced it.
func TestWriteSourceCacheCarriesTheIdentity(t *testing.T) {
	store := &fakeSourceCacheStore{}
	r := resolverWithStore(store, map[string]interface{}{velaprocess.ContextNamespace: "prod"})

	identity := identityInputs{
		Template:   "abc123",
		Properties: map[string]interface{}{"url": "https://example.com"},
		Context:    map[string]interface{}{"cluster": "eu-west"},
	}
	require.NoError(t, r.writeSourceCache("http-get-abc", "http-get",
		map[string]interface{}{"body": "hi"}, 5*time.Minute, []string{"cluster"}, identity))

	require.Equal(t, 1, store.writes)
	require.Equal(t, 5*time.Minute, store.lastMeta.TTL)
	require.Equal(t, "http-get", store.lastMeta.SourceDefName)
	require.Equal(t, "prod", store.lastMeta.SourceDefNamespace,
		"the definition's namespace comes from the render context")
	require.Equal(t, []string{"cluster"}, store.lastMeta.KeyInputs)
	require.Equal(t, identity.Context, store.lastMeta.Context)
	require.Equal(t, identity.Properties, store.lastMeta.Properties)
	require.Equal(t, "abc123", store.lastMeta.TemplateHash)
}

// A render context with no namespace must still write, with an empty one,
// rather than failing on the type assertion.
func TestWriteSourceCacheToleratesAMissingNamespace(t *testing.T) {
	store := &fakeSourceCacheStore{}
	r := resolverWithStore(store, map[string]interface{}{velaprocess.ContextNamespace: 42})
	require.NoError(t, r.writeSourceCache("k", "t", nil, 0, nil, identityInputs{}))
	require.Equal(t, "", store.lastMeta.SourceDefNamespace)
}

func TestTouchSourceCache(t *testing.T) {
	t.Run("no store or key", func(t *testing.T) {
		resolverWithStore(nil, nil).touchSourceCache("k")
		toucher := &touchableCacheStore{}
		resolverWithStore(toucher, nil).touchSourceCache("")
		require.Equal(t, 0, toucher.touched)
	})

	t.Run("a store that cannot touch is skipped", func(t *testing.T) {
		// No panic and no error: touching is best-effort.
		resolverWithStore(&fakeSourceCacheStore{}, nil).touchSourceCache("k")
	})

	t.Run("a touching store is forwarded to", func(t *testing.T) {
		toucher := &touchableCacheStore{}
		resolverWithStore(toucher, nil).touchSourceCache("k")
		require.Equal(t, 1, toucher.touched)
	})

	t.Run("a failed touch is logged, not raised", func(t *testing.T) {
		// A missed touch only risks the sweep collecting an entry one cycle
		// early, which the next render re-creates. Failing the render over it
		// would be far worse than the thing it protects against.
		toucher := &touchableCacheStore{touchErr: context.DeadlineExceeded}
		resolverWithStore(toucher, nil).touchSourceCache("k")
		require.Equal(t, 1, toucher.touched)
	})
}

func TestFormatExpiryIsSilentRatherThanWrong(t *testing.T) {
	require.Equal(t, "", formatExpiry(time.Time{}),
		"a source with no TTL has no expiry, and a zero date would read as one")
	at := time.Date(2026, 8, 22, 10, 30, 0, 0, time.UTC)
	require.Equal(t, "2026-08-22T10:30:00Z", formatExpiry(at))
}
