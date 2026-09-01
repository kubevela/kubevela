/*
Copyright 2021 The KubeVela Authors.

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
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/repo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPutVersionedUIData2cache(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	u := NewCache(nil)
	u.putVersionedUIData2Cache("helm-repo", "fluxcd", "1.0.0", &uiData)
	assert.NotEmpty(t, u.versionedUIData)
	assert.NotEmpty(t, u.versionedUIData["helm-repo"])
	assert.NotEmpty(t, u.versionedUIData["helm-repo"]["fluxcd-1.0.0"])
	assert.Equal(t, u.versionedUIData["helm-repo"]["fluxcd-1.0.0"].Name, "fluxcd")
}

func TestPutAddonUIData2Cache(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	addons := []*UIData{&uiData}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonUIData2Cache(name, addons)
	assert.NotEmpty(t, u.uiData)
	assert.Equal(t, u.uiData[name], addons)
}

func TestListCachedUIData(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	addons := []*UIData{&uiData}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonUIData2Cache(name, addons)

	assert.Equal(t, u.listCachedUIData(name), addons)
}

var _ = Describe("Test addon cache", func() {
	vr := Registry{Name: "helm-repo", Helm: &HelmSource{URL: "http://127.0.0.1:18083/authReg", Username: "hello", Password: "hello"}}

	It("Test list addon helm repo UI data", func() {
		uiData := UIData{Meta: Meta{
			Name:        "fluxcd",
			Description: "Extended workload to do continuous and progressive delivery",
			Icon:        "https://raw.githubusercontent.com/fluxcd/flux/master/docs/_files/weave-flux.png",
			Version:     "1.0.0",
			Tags:        []string{"extended_workload", "gitops"},
		},
			AvailableVersions: []string{"1.0.0"},
			RegistryName:      "helm-repo"}
		addons := []*UIData{&uiData}
		u := NewCache(nil)
		uiDatas, err := u.ListUIData(vr)
		Expect(err).NotTo(HaveOccurred())
		Expect(uiDatas).To(Equal(addons))
	})
})

func TestListVersionRegistryCachedUIData(t *testing.T) {
	name := "fluxcd"
	version := "v1.0.1"
	uiData := &UIData{Meta: Meta{Name: name, Icon: "test.com/fluxcd.png", Version: version}}
	addons := []*UIData{uiData}
	vrName := "helm-repo"
	u := NewCache(nil)
	u.putVersionedUIData2Cache(vrName, name, version, uiData)
	u.putVersionedUIData2Cache(vrName, name, "latest", uiData)

	assert.Equal(t, u.listVersionRegistryCachedUIData(vrName), addons)
}

func TestPutAddonMeta2Cache(t *testing.T) {
	addonMeta := map[string]SourceMeta{
		"fluxcd": {
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/definitions/helm-release.yaml",
					name: "helm-release.yaml",
				},
			},
		},
	}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonMeta2Cache(name, addonMeta)
	assert.NotEmpty(t, u.registryMeta)
	assert.Equal(t, u.registryMeta[name], addonMeta)
}

func TestGetCachedAddonMeta(t *testing.T) {
	addonMeta := map[string]SourceMeta{
		"fluxcd": {
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/definitions/helm-release.yaml",
					name: "helm-release.yaml",
				},
			},
		},
	}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonMeta2Cache(name, addonMeta)

	assert.Equal(t, u.getCachedAddonMeta(name), addonMeta)
}

// registryStub serves a fixed listing and per-version data, standing in for a
// versioned registry without any network.
type registryStub struct {
	list    []*UIData
	perCall func(name, version string) *UIData
	err     error
}

func (s *registryStub) ListAddon() ([]*UIData, error) { return s.list, nil }
func (s *registryStub) GetAddonUIData(_ context.Context, name, version string) (*UIData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.perCall(name, version), nil
}
func (s *registryStub) GetAddonInstallPackage(context.Context, string, string) (*InstallPackage, error) {
	return nil, nil
}
func (s *registryStub) GetDetailedAddon(context.Context, string, string) (*WholeAddonPackage, error) {
	return nil, nil
}
func (s *registryStub) GetAddonAvailableVersion(string) ([]*repo.ChartVersion, error) {
	return nil, nil
}

// TestCacheVersionedUIDataKeepsAvailableVersions covers what the UI reads to
// offer version choices. The listing carries every version, but the per-version
// fetch it drives is pinned and need not, so caching the fetch result verbatim
// showed the addon as having a single version.
func TestCacheVersionedUIDataKeepsAvailableVersions(t *testing.T) {
	u := NewCache(nil)
	stub := &registryStub{
		list: []*UIData{{
			Meta:              Meta{Name: "fluxcd", Version: "3.0.2"},
			AvailableVersions: []string{"3.0.2", "3.0.1", "2.0.0"},
		}},
		perCall: func(name, version string) *UIData {
			// A pinned fetch: no version list, matching ociRegistry.loadAddon.
			return &UIData{Meta: Meta{Name: name, Version: version}}
		},
	}

	u.cacheVersionedUIData("ecr", stub, stub.list)

	cached := u.getCachedUIData(Registry{Name: "ecr", Helm: &HelmSource{URL: "oci://reg/addon"}}, "fluxcd", "3.0.2")
	require.NotNil(t, cached)
	assert.Equal(t, []string{"3.0.2", "3.0.1", "2.0.0"}, cached.AvailableVersions)

	latest := u.getCachedUIData(Registry{Name: "ecr", Helm: &HelmSource{URL: "oci://reg/addon"}}, "fluxcd", "")
	require.NotNil(t, latest)
	assert.Equal(t, []string{"3.0.2", "3.0.1", "2.0.0"}, latest.AvailableVersions)
}

func TestCacheVersionedUIDataEdgeCases(t *testing.T) {
	t.Run("skips addon on fetch error", func(t *testing.T) {
		u := NewCache(nil)
		stub := &registryStub{
			list: []*UIData{{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}}},
			err:  errors.New("network error"),
		}

		u.cacheVersionedUIData("test-reg", stub, stub.list)
		assert.Empty(t, u.versionedUIData["test-reg"])
	})

	t.Run("skips addon with empty name from chart", func(t *testing.T) {
		u := NewCache(nil)
		stub := &registryStub{
			list: []*UIData{{Meta: Meta{Name: "badchart", Version: "1.0.0"}}},
			perCall: func(_, version string) *UIData {
				return &UIData{Meta: Meta{Name: "", Version: version}}
			},
		}

		u.cacheVersionedUIData("test-reg", stub, stub.list)
		assert.Empty(t, u.versionedUIData["test-reg"])
	})

	t.Run("deletes stale entries no longer in listing", func(t *testing.T) {
		u := NewCache(nil)
		// Pre-populate with an addon that won't appear in the new listing.
		u.putVersionedUIData2Cache("test-reg", "old-addon", "1.0.0", &UIData{Meta: Meta{Name: "old-addon", Version: "1.0.0"}})
		u.putVersionedUIData2Cache("test-reg", "old-addon", "latest", &UIData{Meta: Meta{Name: "old-addon", Version: "1.0.0"}})
		require.NotNil(t, u.versionedUIData["test-reg"]["old-addon-1.0.0"])

		stub := &registryStub{
			list: []*UIData{{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}}},
			perCall: func(name, version string) *UIData {
				return &UIData{Meta: Meta{Name: name, Version: version}}
			},
		}

		u.cacheVersionedUIData("test-reg", stub, stub.list)

		assert.NotNil(t, u.versionedUIData["test-reg"]["fluxcd-1.0.0"], "new addon must be cached")
		assert.Nil(t, u.versionedUIData["test-reg"]["old-addon-1.0.0"], "stale addon must be deleted")
	})

	t.Run("preserves available versions when fetch already has them", func(t *testing.T) {
		u := NewCache(nil)
		stub := &registryStub{
			list: []*UIData{{
				Meta:              Meta{Name: "fluxcd", Version: "1.0.0"},
				AvailableVersions: []string{"1.0.0", "0.9.0"},
			}},
			perCall: func(name, version string) *UIData {
				return &UIData{Meta: Meta{Name: name, Version: version}, AvailableVersions: []string{"1.0.0"}}
			},
		}

		u.cacheVersionedUIData("test-reg", stub, stub.list)

		cached := u.versionedUIData["test-reg"]["fluxcd-1.0.0"]
		require.NotNil(t, cached)
		// The fetch already carried versions, so the listing's list must not override it.
		assert.Equal(t, []string{"1.0.0"}, cached.AvailableVersions)
	})
}

// TestPutRegistry2CacheClearsVersionedUIDataOnDelete pins a stale-cache bug: a
// deleted registry's versionedUIData entries survived putRegistry2Cache's
// cleanup, which only cleared registry/registryMeta/uiData. Recreating a
// registry with the same name would then serve the old registry's cached
// versioned addons instead of re-fetching.
func TestPutRegistry2CacheClearsVersionedUIDataOnDelete(t *testing.T) {
	u := NewCache(nil)
	u.putRegistry2Cache([]Registry{{Name: "ecr", Helm: &HelmSource{URL: "oci://reg/addon"}}})
	u.putVersionedUIData2Cache("ecr", "fluxcd", "1.0.0", &UIData{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}})
	require.NotNil(t, u.versionedUIData["ecr"]["fluxcd-1.0.0"])

	// "ecr" is no longer in the configured list, so it must be cleaned up.
	u.putRegistry2Cache(nil)

	assert.Nil(t, u.versionedUIData["ecr"], "versionedUIData for a deleted registry must not survive")
}

// TestCacheGetUIData covers the three shapes GetUIData now takes since it
// uses the chart-registry predicate: a cache hit
// short-circuits both branches, a versioned/OCI miss goes through
// ToVersionedRegistry, and a non-versioned registry with no source info
// fails building its reader.
func TestCacheGetUIData(t *testing.T) {
	t.Run("cache hit returns without touching the registry", func(t *testing.T) {
		c := NewCache(nil)
		c.putVersionedUIData2Cache("ecr", "fluxcd", "1.0.0", &UIData{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}})
		r := Registry{Name: "ecr", Helm: &HelmSource{URL: "oci://127.0.0.1:1/addon"}}

		data, err := c.GetUIData(r, "fluxcd", "1.0.0")
		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Equal(t, "fluxcd", data.Name)
	})

	t.Run("OCI registry cache miss resolves through ToVersionedRegistry and fails fast", func(t *testing.T) {
		c := NewCache(nil)
		r := Registry{Name: "ecr", Helm: &HelmSource{URL: "oci://127.0.0.1:1/addon"}}

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
// updated branch (IsVersionRegistry) for both a non-versioned and an
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
		Name: "ecr", Helm: &HelmSource{URL: "oci://127.0.0.1:1/addon"},
	}))

	c := NewCache(store)
	c.discoverAndRefreshRegistry()

	assert.Len(t, c.registry, 2, "registries are recorded in the cache even when their listings fail")
}
