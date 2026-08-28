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

package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestCacheGetUIData covers the three shapes GetUIData now takes since it
// switched from IsVersionRegistry to isVersionCapableRegistry: a cache hit
// short-circuits both branches, a versioned/OCI miss goes through
// ToVersionedRegistry, and a non-versioned registry with no source info
// fails building its reader.
func TestCacheGetUIData(t *testing.T) {
	t.Run("cache hit returns without touching the registry", func(t *testing.T) {
		c := NewCache(nil)
		c.putVersionedUIData2Cache("ecr", "fluxcd", "1.0.0", &UIData{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}})
		r := Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://127.0.0.1:1/addon"}}

		data, err := c.GetUIData(r, "fluxcd", "1.0.0")
		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, "fluxcd", data.Name)
	})

	t.Run("OCI registry cache miss resolves through ToVersionedRegistry and fails fast", func(t *testing.T) {
		c := NewCache(nil)
		r := Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://127.0.0.1:1/addon"}}

		_, err := c.GetUIData(r, "fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("non-versioned registry with no source info fails fast", func(t *testing.T) {
		c := NewCache(nil)
		r := Registry{Name: "bare"}

		_, err := c.GetUIData(r, "fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enough info")
	})
}

// TestCacheDiscoverAndRefreshRegistry exercises discoverAndRefreshRegistry's
// updated branch (isVersionCapableRegistry) for both a non-versioned and an
// OCI registry. Both listings fail fast against an unreachable host, so the
// call completes deterministically without a real network dependency, while
// still recording every registry in the cache via putRegistry2Cache.
func TestCacheDiscoverAndRefreshRegistry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	store := NewRegistryDataStore(kubeClient)
	require.NoError(t, store.AddRegistry(context.Background(), Registry{Name: "bare"}))
	require.NoError(t, store.AddRegistry(context.Background(), Registry{
		Name: "ecr", OCI: &OCIAddonSource{URL: "oci://127.0.0.1:1/addon"},
	}))

	c := NewCache(store)
	c.discoverAndRefreshRegistry()

	assert.Len(t, c.registry, 2, "registries are recorded in the cache even when their listings fail")
}
