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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestIsOCIRegistry(t *testing.T) {
	assert.True(t, IsOCIRegistry(Registry{OCI: &OCIAddonSource{URL: "oci://x/y"}}))
	assert.False(t, IsOCIRegistry(Registry{Helm: &HelmSource{URL: "http://x"}}))
	assert.False(t, IsOCIRegistry(Registry{Git: &GitAddonSource{URL: "http://x"}}))
	assert.False(t, IsOCIRegistry(Registry{}))
}

func TestOCIAddonSourceTokenSource(t *testing.T) {
	s := &OCIAddonSource{URL: "oci://reg/addon", Username: "AWS", Token: "secret"}
	// GetTokenSource should return the OCI source
	r := Registry{Name: "ecr", OCI: s}
	require.NotNil(t, r.GetTokenSource())
	assert.Equal(t, "secret", r.GetTokenSource().GetToken())

	// SetTokenSecretRef clears the inline token and records the ref
	s.SetTokenSecretRef("addon-registry-ecr")
	assert.Equal(t, "", s.GetToken())
	assert.Equal(t, "addon-registry-ecr", s.GetTokenSecretRef())

	// SetToken clears the ref
	s.SetToken("t2")
	assert.Equal(t, "t2", s.GetToken())
	assert.Equal(t, "", s.GetTokenSecretRef())

	// SafeCopy hides the token but keeps the ref
	s.SetTokenSecretRef("addon-registry-ecr")
	safe := s.SafeCopy()
	assert.Equal(t, "", safe.Token)
	assert.Equal(t, "addon-registry-ecr", safe.TokenSecretRef)
	assert.Equal(t, "oci://reg/addon", safe.URL)
}

func TestOCIRepoRef(t *testing.T) {
	cases := map[string]struct {
		url, addon         string
		wantRepo, wantHost string
	}{
		"with scheme": {
			url: "oci://reg.example.com/addon", addon: "fluxcd",
			wantRepo: "reg.example.com/addon/fluxcd", wantHost: "reg.example.com",
		},
		"no scheme": {
			url: "reg.example.com/addon", addon: "fluxcd",
			wantRepo: "reg.example.com/addon/fluxcd", wantHost: "reg.example.com",
		},
		"trailing slash": {
			url: "oci://reg.example.com/addon/", addon: "velaux",
			wantRepo: "reg.example.com/addon/velaux", wantHost: "reg.example.com",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo, host := ociRepoRef(tc.url, tc.addon)
			assert.Equal(t, tc.wantRepo, repo)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}

func TestListOCIRepositories(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "AWS", user)
		assert.Equal(t, "secret", pass)
		assert.Equal(t, "/v2/_catalog", req.URL.Path)

		if req.URL.Query().Get("last") == "" {
			rw.Header().Set("Link", fmt.Sprintf(`<%s/v2/_catalog?n=1000&last=addon%%2Ffluxcd>; rel="next"`, server.URL))
			_, _ = rw.Write([]byte(`{"repositories":["other/image","addon/fluxcd"]}`))
			return
		}
		_, _ = rw.Write([]byte(`{"repositories":["addon/velaux","addon/fluxcd"]}`))
	}))
	defer server.Close()

	originalClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() {
		http.DefaultClient = originalClient
	}()

	registryURL := "oci://" + strings.TrimPrefix(server.URL, "https://") + "/addon"
	addons, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")
	require.NoError(t, err)
	assert.Equal(t, []string{"fluxcd", "velaux"}, addons)
}

func TestDecodeOCIAddonCatalog(t *testing.T) {
	catalog, err := json.Marshal(OCIAddonCatalog{
		APIVersion: ociCatalogAPIVersion,
		Addons: []OCIAddonCatalogEntry{
			{Name: "velaux", Description: "UI", Versions: []string{"1.0.0"}},
			{Name: "fluxcd", Description: "Flux", Versions: []string{"3.0.2", "3.0.1"}},
		},
	})
	require.NoError(t, err)

	tmp := t.TempDir()
	archivePath, err := chartutil.Save(&chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       ociCatalogChartName,
			Version:    "1.0.0",
		},
		Files: []*chart.File{{Name: ociCatalogFileName, Data: catalog}},
	}, tmp)
	require.NoError(t, err)
	archive, err := os.ReadFile(filepath.Clean(archivePath))
	require.NoError(t, err)

	addons, err := decodeOCIAddonCatalog(archive)
	require.NoError(t, err)
	require.Len(t, addons, 2)
	assert.Equal(t, "fluxcd", addons[0].Name)
	assert.Equal(t, "Flux", addons[0].Description)
	assert.Equal(t, []string{"3.0.2", "3.0.1"}, addons[0].AvailableVersions)
	assert.Equal(t, "velaux", addons[1].Name)
}

func TestOCIRegistryPrefersPortableCatalog(t *testing.T) {
	reg := &ociRegistry{
		name: "portable",
		url:  "oci://reg.example.com/addon",
		catalogIndexFn: func(_ context.Context, registryURL, _, _ string) ([]*UIData, error) {
			assert.Equal(t, "oci://reg.example.com/addon", registryURL)
			return []*UIData{{
				Meta:              Meta{Name: "fluxcd", Description: "Flux"},
				AvailableVersions: []string{"3.0.2", "3.0.1"},
			}}, nil
		},
		catalogFn: func(_ context.Context, _, _, _ string) ([]string, error) {
			t.Fatal("registry catalog fallback must not be called when the portable catalog is available")
			return nil, nil
		},
	}

	addons, err := reg.ListAddon()
	require.NoError(t, err)
	require.Len(t, addons, 1)
	assert.Equal(t, "portable", addons[0].RegistryName)
}

// TestOCIRegistryLoadAddon injects a fake puller returning a real addon chart
// archive (a prebuilt fixture), so the OCI load path is exercised without any
// network and without writing artifacts to disk.
func TestOCIRegistryLoadAddon(t *testing.T) {
	data, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)

	// Empty version must resolve to the highest semver tag via the tag lister,
	// then pull that exact tag.
	reg := &ociRegistry{
		name:     "ecr",
		url:      "oci://reg.example.com/addon",
		username: "AWS",
		token:    "secret",
		tagsFn: func(_ context.Context, repoRef, host, user, pass string) ([]string, error) {
			assert.Equal(t, "reg.example.com/addon/fluxcd", repoRef)
			assert.Equal(t, "reg.example.com", host)
			// helm's Tags returns semver-sorted, highest first.
			return []string{"3.0.1", "2.0.0", "1.0.0"}, nil
		},
		pullFn: func(_ context.Context, ref, host, user, pass string) ([]byte, error) {
			assert.Equal(t, "reg.example.com/addon/fluxcd:3.0.1", ref)
			assert.Equal(t, "reg.example.com", host)
			assert.Equal(t, "AWS", user)
			assert.Equal(t, "secret", pass)
			return data, nil
		},
		catalogFn: func(_ context.Context, registryURL, user, pass string) ([]string, error) {
			assert.Equal(t, "oci://reg.example.com/addon", registryURL)
			assert.Equal(t, "AWS", user)
			assert.Equal(t, "secret", pass)
			return []string{"fluxcd"}, nil
		},
	}

	pkg, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	require.NotNil(t, pkg)
	assert.Equal(t, "fluxcd", pkg.Name)

	// GetDetailedAddon should stamp the registry name.
	whole, err := reg.GetDetailedAddon(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, "ecr", whole.RegistryName)

	addons, err := reg.ListAddon()
	require.NoError(t, err)
	require.Len(t, addons, 1)
	assert.Equal(t, "fluxcd", addons[0].Name)
	assert.Equal(t, "ecr", addons[0].RegistryName)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, addons[0].AvailableVersions)
}

// TestOCIRegistryExplicitVersion pins a version: no tag listing should happen,
// the exact tag is pulled.
func TestOCIRegistryExplicitVersion(t *testing.T) {
	data, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)

	reg := &ociRegistry{
		name: "ecr", url: "oci://reg.example.com/addon", username: "AWS", token: "secret",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			t.Fatalf("tag listing must not be called when a version is pinned")
			return nil, nil
		},
		pullFn: func(_ context.Context, ref, _, _, _ string) ([]byte, error) {
			assert.Equal(t, "reg.example.com/addon/fluxcd:3.0.1", ref)
			return data, nil
		},
	}
	pkg, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "3.0.1")
	require.NoError(t, err)
	assert.Equal(t, "fluxcd", pkg.Name)
}

// TestOCIRegistryNoTags errors clearly when no semver tags exist.
func TestOCIRegistryNoTags(t *testing.T) {
	reg := &ociRegistry{
		name: "ecr", url: "oci://reg.example.com/addon",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			return []string{}, nil
		},
	}
	_, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no")
}
