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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/oam"
)

func cacheSecret(name string, props map[string]interface{}, created time.Time, anns map[string]string) *corev1.Secret {
	raw, _ := json.Marshal(props)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         CacheNamespace(),
			CreationTimestamp: metav1.NewTime(created),
			Annotations:       anns,
		},
		Data: map[string][]byte{sourceCacheDataKey: raw},
	}
}

func secretStore(objs ...client.Object) velaprocess.SourceCacheStore {
	return NewSecretSourceCacheStore(fake.NewClientBuilder().WithObjects(objs...).Build())
}

// No client means no cache, and the constructor must say so rather than hand
// back a store that swallows everything.
func TestNewSecretSourceCacheStoreIsNilWithoutAClient(t *testing.T) {
	require.Nil(t, NewSecretSourceCacheStore(nil))
	require.NotNil(t, NewSecretSourceCacheStore(fake.NewClientBuilder().Build()))
}

func TestSecretStoreReadMisses(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		_, _, found, _, err := secretStore().Read(context.Background(), "", time.Minute)
		require.NoError(t, err)
		require.False(t, found)
	})
	t.Run("no such entry", func(t *testing.T) {
		_, _, found, _, err := secretStore().Read(context.Background(), "absent", time.Minute)
		require.NoError(t, err, "a missing entry is a miss, not a failure")
		require.False(t, found)
	})
	t.Run("an entry with no payload", func(t *testing.T) {
		s := secretStore(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "empty", Namespace: CacheNamespace()}})
		_, _, found, _, err := s.Read(context.Background(), "empty", time.Minute)
		require.NoError(t, err)
		require.False(t, found, "an entry with nothing in it cannot be served")
	})
	t.Run("a corrupt payload is an error, not a silent miss", func(t *testing.T) {
		bad := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: CacheNamespace()},
			Data:       map[string][]byte{sourceCacheDataKey: []byte("{not json")},
		}
		_, _, found, _, err := secretStore(bad).Read(context.Background(), "bad", time.Minute)
		require.Error(t, err, "silently re-fetching would hide a corrupted entry forever")
		require.False(t, found)
	})
}

func TestSecretStoreReadFreshness(t *testing.T) {
	now := time.Now()
	props := map[string]interface{}{"image": "nginx"}

	t.Run("fresh", func(t *testing.T) {
		s := secretStore(cacheSecret("k", props, now.Add(-time.Minute), nil))
		data, stale, found, expires, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, found)
		require.False(t, stale)
		require.Equal(t, props, data)
		require.True(t, expires.After(now))
	})

	t.Run("stale", func(t *testing.T) {
		s := secretStore(cacheSecret("k", props, now.Add(-2*time.Hour), nil))
		_, stale, found, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, found, "still there to serve stale")
		require.True(t, stale)
	})

	t.Run("the sync annotation wins over creation", func(t *testing.T) {
		s := secretStore(cacheSecret("k", props, now.Add(-99*time.Hour),
			map[string]string{sourceCacheSyncAtKey: now.Add(-time.Minute).Format(time.RFC3339)}))
		_, stale, _, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, stale, "a refreshed entry is fresh however old the object is")
	})

	t.Run("an unparseable annotation falls back to creation", func(t *testing.T) {
		s := secretStore(cacheSecret("k", props, now.Add(-time.Minute),
			map[string]string{sourceCacheSyncAtKey: "nonsense"}))
		_, stale, _, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, stale)
	})

	t.Run("a non-positive ttl takes the default", func(t *testing.T) {
		s := secretStore(cacheSecret("k", props, now.Add(-sourceCacheTTL-time.Minute), nil))
		_, stale, _, _, err := s.Read(context.Background(), "k", 0)
		require.NoError(t, err)
		require.True(t, stale, "the default TTL applies rather than never expiring")
	})
}

func TestSecretStoreWriteCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	read := func(t *testing.T, cli client.Client) *corev1.Secret {
		t.Helper()
		got := &corev1.Secret{}
		require.NoError(t, cli.Get(ctx, ktypes.NamespacedName{
			Namespace: CacheNamespace(), Name: "k"}, got))
		return got
	}

	t.Run("creates when absent", func(t *testing.T) {
		cli := fake.NewClientBuilder().Build()
		s := NewSecretSourceCacheStore(cli)
		require.NoError(t, s.Write(ctx, "k", "http-get",
			map[string]interface{}{"body": "hi"}, velaprocess.SourceCacheWriteMeta{}))

		got := read(t, cli)
		require.Equal(t, corev1.SecretTypeOpaque, got.Type)
		require.JSONEq(t, `{"body":"hi"}`, string(got.Data[sourceCacheDataKey]))
		require.NotEmpty(t, got.Annotations[sourceCacheSyncAtKey])
		require.Equal(t, "http-get", got.Labels[apitypes.LabelConfigType])
		require.Equal(t, apitypes.VelaCoreConfig, got.Labels[apitypes.LabelConfigCatalog])
	})

	t.Run("updates in place, keeping labels other things set", func(t *testing.T) {
		existing := cacheSecret("k", map[string]interface{}{"body": "old"}, time.Now(), nil)
		existing.Labels = map[string]string{"someone.else/owns": "this"}
		cli := fake.NewClientBuilder().WithObjects(existing).Build()

		require.NoError(t, NewSecretSourceCacheStore(cli).Write(ctx, "k", "http-get",
			map[string]interface{}{"body": "new"}, velaprocess.SourceCacheWriteMeta{}))

		got := read(t, cli)
		require.JSONEq(t, `{"body":"new"}`, string(got.Data[sourceCacheDataKey]))
		require.Equal(t, "this", got.Labels["someone.else/owns"],
			"a rewrite must not strip labels it does not own")
	})

	t.Run("an entry with nil maps is filled in rather than panicking", func(t *testing.T) {
		bare := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "k", Namespace: CacheNamespace()}}
		cli := fake.NewClientBuilder().WithObjects(bare).Build()
		require.NoError(t, NewSecretSourceCacheStore(cli).Write(ctx, "k", "http-get",
			map[string]interface{}{"body": "hi"}, velaprocess.SourceCacheWriteMeta{}))
		require.NotEmpty(t, read(t, cli).Annotations[sourceCacheSyncAtKey])
	})

	t.Run("no key is a no-op", func(t *testing.T) {
		cli := fake.NewClientBuilder().Build()
		require.NoError(t, NewSecretSourceCacheStore(cli).Write(ctx, "", "http-get", nil,
			velaprocess.SourceCacheWriteMeta{}))
		list := &corev1.SecretList{}
		require.NoError(t, cli.List(ctx, list))
		require.Empty(t, list.Items)
	})

	t.Run("unmarshalable data is refused", func(t *testing.T) {
		err := NewSecretSourceCacheStore(fake.NewClientBuilder().Build()).Write(ctx, "k", "t",
			map[string]interface{}{"ch": make(chan int)}, velaprocess.SourceCacheWriteMeta{})
		require.Error(t, err)
	})
}

func TestSecretStoreTouch(t *testing.T) {
	ctx := context.Background()
	toucher := func(cli client.Client) velaprocess.SourceCacheToucher {
		return NewSecretSourceCacheStore(cli).(velaprocess.SourceCacheToucher)
	}

	t.Run("no key and no entry are both fine", func(t *testing.T) {
		cli := fake.NewClientBuilder().Build()
		require.NoError(t, toucher(cli).Touch(ctx, ""))
		require.NoError(t, toucher(cli).Touch(ctx, "absent"))
	})

	t.Run("an unmarked entry gets marked", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithObjects(
			cacheSecret("k", map[string]interface{}{}, time.Now(), map[string]string{})).Build()
		require.NoError(t, toucher(cli).Touch(ctx, "k"))
		got := &corev1.Secret{}
		require.NoError(t, cli.Get(ctx, ktypes.NamespacedName{
			Namespace: CacheNamespace(), Name: "k"}, got))
		require.NotEmpty(t, got.Annotations[sourceCacheAccessedKey])
	})

	t.Run("a recently marked entry is left alone", func(t *testing.T) {
		recent := time.Now().Format(time.RFC3339)
		cli := fake.NewClientBuilder().WithObjects(cacheSecret("k", map[string]interface{}{},
			time.Now(), map[string]string{sourceCacheAccessedKey: recent})).Build()
		require.NoError(t, toucher(cli).Touch(ctx, "k"))
		got := &corev1.Secret{}
		require.NoError(t, cli.Get(ctx, ktypes.NamespacedName{
			Namespace: CacheNamespace(), Name: "k"}, got))
		require.Equal(t, recent, got.Annotations[sourceCacheAccessedKey],
			"throttled, or every stale serve writes to the apiserver")
	})
}

// The cache namespace was a fixed "vela-system" while the chart granted access
// in the release namespace, so an install with a non-default
// systemDefinitionNamespace had every cache write, touch and sweep Forbidden.
func TestCacheNamespaceFollowsTheConfiguredSystemNamespace(t *testing.T) {
	restore := oam.SystemDefinitionNamespace
	defer func() { oam.SystemDefinitionNamespace = restore }()

	require.Equal(t, restore, CacheNamespace())
	oam.SystemDefinitionNamespace = "vela-core"
	require.Equal(t, "vela-core", CacheNamespace())
}
