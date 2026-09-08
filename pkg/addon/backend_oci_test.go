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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	registryauth "oras.land/oras-go/pkg/registry/remote/auth"
)

// useCatalogHTTPClient points the /v2/_catalog probe at a test HTTP client for
// the duration of the test.
//
// These tests used to swap http.DefaultClient instead. That is process-global
// state shared with every other caller in the binary: it happens to work while
// no test in this package calls t.Parallel, but it makes the probe's transport
// an implicit dependency on test ordering rather than an injected one.
func useCatalogHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()
	original := ociCatalogHTTPClient
	t.Cleanup(func() { ociCatalogHTTPClient = original })
	ociCatalogHTTPClient = client
}

// resetOCIClientCache empties the process-wide OCI client cache for one test,
// and empties it again afterward so the clients this test created are not left
// for the next one to reuse.
func resetOCIClientCache(t *testing.T) {
	t.Helper()
	component.ResetOCIClientCache()
	t.Cleanup(component.ResetOCIClientCache)
}

// clientTrusting builds an HTTP client that trusts exactly the given httptest
// TLS servers, so a test can let a request reach more than one of them.
func clientTrusting(servers ...*httptest.Server) *http.Client {
	pool := x509.NewCertPool()
	for _, server := range servers {
		pool.AddCert(server.Certificate())
	}
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
}

// ociFacade wraps a backend in the shared registry facade, which is what
// production callers hold. The tests below drive real VersionedRegistry calls
// while still injecting the transport seams on the backend.
func ociFacade(b *ociHelmBackend) VersionedRegistry {
	return &helmRegistry{name: b.name, backend: b}
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
			repo, host := component.OCIRepoRef(tc.url, tc.addon)
			assert.Equal(t, tc.wantRepo, repo)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}

// TestListOCIRepositories covers the basic-auth challenge flow and pagination.
// The probe sends no credentials until the registry asks for them, so the
// server answers the unauthenticated request with a Basic challenge and only
// then serves the catalog.
func TestListOCIRepositories(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/v2/_catalog", req.URL.Path)

		user, pass, ok := req.BasicAuth()
		if !ok {
			rw.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, "AWS", user)
		assert.Equal(t, "secret", pass)

		if req.URL.Query().Get("last") == "" {
			rw.Header().Set("Link", fmt.Sprintf(`<%s/v2/_catalog?n=1000&last=addon%%2Ffluxcd>; rel="next"`, server.URL))
			_, _ = rw.Write([]byte(`{"repositories":["other/image","addon/fluxcd"]}`))
			return
		}
		_, _ = rw.Write([]byte(`{"repositories":["addon/velaux","addon/fluxcd"]}`))
	}))
	defer server.Close()

	useCatalogHTTPClient(t, server.Client())

	registryURL := "oci://" + strings.TrimPrefix(server.URL, "https://") + "/addon"
	addons, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")
	require.NoError(t, err)
	assert.Equal(t, []string{"fluxcd", "velaux"}, addons)
}

// TestListOCIRepositoriesCompletesBearerChallenge covers the token-auth
// registries (Docker Hub, GHCR, ECR, Harbor): they answer /v2/_catalog with
// 401 and a Bearer challenge, and expect the caller to exchange it at the
// challenge's realm for a registry:catalog:*-scoped token. Sending BasicAuth
// unconditionally, as this probe used to, gets a second 401 and reports the
// catalog as unreadable.
func TestListOCIRepositoriesCompletesBearerChallenge(t *testing.T) {
	const issuedToken = "issued-catalog-token"

	var tokenRequests int32
	tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&tokenRequests, 1)
		user, pass, ok := req.BasicAuth()
		assert.True(t, ok, "the token exchange must present the registry credentials")
		assert.Equal(t, "AWS", user)
		assert.Equal(t, "secret", pass)
		assert.Equal(t, registryauth.ScopeRegistryCatalog, req.URL.Query().Get("scope"),
			"the exchange must ask for catalog scope, or the minted token cannot list the catalog")
		_, _ = rw.Write([]byte(`{"token":"` + issuedToken + `"}`))
	}))
	defer tokenServer.Close()

	registryServer := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/v2/_catalog", req.URL.Path)
		if req.Header.Get("Authorization") != "Bearer "+issuedToken {
			assert.Empty(t, req.Header.Get("Authorization"), "basic auth must not be sent to a bearer registry")
			rw.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registry"`, tokenServer.URL))
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = rw.Write([]byte(`{"repositories":["addon/fluxcd"]}`))
	}))
	defer registryServer.Close()

	useCatalogHTTPClient(t, clientTrusting(registryServer, tokenServer))

	registryURL := "oci://" + strings.TrimPrefix(registryServer.URL, "https://") + "/addon"
	addons, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")
	require.NoError(t, err)
	assert.Equal(t, []string{"fluxcd"}, addons)
	assert.Equal(t, int32(1), atomic.LoadInt32(&tokenRequests), "the challenge must be exchanged exactly once")
}

func TestListOCIRepositoriesWithPlainHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/v2/_catalog", req.URL.Path)
		_, _ = rw.Write([]byte(`{"repositories":["addon/sample"]}`))
	}))
	defer server.Close()

	registryURL := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/addon"
	addons, err := listOCIRepositoriesWithPlainHTTP(context.Background(), registryURL, "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"sample"}, addons)
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

	// Version has to carry the newest version. The shared addon cache keys
	// versioned UIData by it, so an empty value writes a dead "<name>-" entry.
	assert.Equal(t, "3.0.2", addons[0].Version)
	assert.Equal(t, "1.0.0", addons[1].Version)
}

func TestDecodeOCIAddonCatalogErrors(t *testing.T) {
	buildArchive := func(t *testing.T, fileName string, fileData []byte) []byte {
		t.Helper()
		tmp := t.TempDir()
		archivePath, err := chartutil.Save(&chart.Chart{
			Metadata: &chart.Metadata{
				APIVersion: chart.APIVersionV2,
				Name:       ociCatalogChartName,
				Version:    "1.0.0",
			},
			Files: []*chart.File{{Name: fileName, Data: fileData}},
		}, tmp)
		require.NoError(t, err)
		archive, err := os.ReadFile(filepath.Clean(archivePath))
		require.NoError(t, err)
		return archive
	}

	t.Run("archive decode failure", func(t *testing.T) {
		_, err := decodeOCIAddonCatalog([]byte("not-a-chart-archive"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load portable OCI addon catalog archive")
	})

	t.Run("missing catalog file", func(t *testing.T) {
		archive := buildArchive(t, "README.md", []byte("no catalog here"))
		_, err := decodeOCIAddonCatalog(archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain "+ociCatalogFileName)
	})

	t.Run("invalid catalog JSON", func(t *testing.T) {
		archive := buildArchive(t, ociCatalogFileName, []byte("{"))
		_, err := decodeOCIAddonCatalog(archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode portable OCI addon catalog")
	})

	t.Run("unsupported api version", func(t *testing.T) {
		catalogData, err := json.Marshal(OCIAddonCatalog{APIVersion: "addons.kubevela.io/v9", Addons: nil})
		require.NoError(t, err)
		archive := buildArchive(t, ociCatalogFileName, catalogData)
		_, err = decodeOCIAddonCatalog(archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported OCI addon catalog API version")
	})

	t.Run("addon entry without a name", func(t *testing.T) {
		catalogData, err := json.Marshal(OCIAddonCatalog{
			APIVersion: ociCatalogAPIVersion,
			Addons: []OCIAddonCatalogEntry{{
				Name:     "   ",
				Versions: []string{"1.0.0"},
			}},
		})
		require.NoError(t, err)
		archive := buildArchive(t, ociCatalogFileName, catalogData)
		_, err = decodeOCIAddonCatalog(archive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without a name")
	})
}

func TestNewestOCICatalogVersion(t *testing.T) {
	assert.Equal(t, "3.0.2", newestOCICatalogVersion([]string{"3.0.2", "3.0.1"}))
	// Order in the catalog is not trusted.
	assert.Equal(t, "3.0.10", newestOCICatalogVersion([]string{"3.0.2", "3.0.10"}))
	// A release outranks its own prerelease.
	assert.Equal(t, "1.0.0", newestOCICatalogVersion([]string{"1.0.0-rc.1", "1.0.0"}))
	// Nothing parses: fall back rather than reporting no version at all.
	assert.Equal(t, "nightly", newestOCICatalogVersion([]string{"nightly", "edge"}))
	assert.Equal(t, "", newestOCICatalogVersion(nil))
}

// TestIsOCIRepositoryAbsentError pins the classifier that decides whether the
// first push to a registry may bootstrap a catalog. oras-go v1.2.5 keeps its
// error types in an internal package, so the status code is only reachable
// through the message -- these are the shapes it actually produces.
func TestIsOCIRepositoryAbsentError(t *testing.T) {
	absent := []error{
		errors.New(`GET "https://reg.example.com/v2/addon/kubevela-addon-catalog/tags/list": unexpected status code 404: name unknown: repository name not known to registry`),
		fmt.Errorf("wrapped: %w", errors.New(`unexpected status code 404: name unknown: The repository with name 'addon/kubevela-addon-catalog' does not exist in the registry`)),
	}
	for _, err := range absent {
		assert.True(t, component.IsOCIRepositoryAbsentError(err), "expected absent for: %v", err)
	}

	notAbsent := []error{
		nil,
		errors.New(`unexpected status code 401: unauthorized: authentication required`),
		errors.New(`unexpected status code 403: denied`),
		errors.New(`dial tcp: i/o timeout`),
		// A bare 404 carries no error code, so it cannot be told apart from a
		// proxy or a registry that does not serve the tag-list route. Reading it
		// as an absence would rebuild the catalog from empty and drop every
		// addon already published, so it stays on the conservative branch.
		errors.New(`unexpected status code 404: Not Found`),
	}
	for _, err := range notAbsent {
		assert.False(t, component.IsOCIRepositoryAbsentError(err), "expected not-absent for: %v", err)
	}
}

func TestOCIRegistryPrefersPortableCatalog(t *testing.T) {
	reg := &ociHelmBackend{
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

	addons, err := ociFacade(reg).ListAddon()
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
	reg := &ociHelmBackend{
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

	pkg, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	require.NotNil(t, pkg)
	assert.Equal(t, "fluxcd", pkg.Name)

	// GetDetailedAddon should stamp the registry name.
	whole, err := ociFacade(reg).GetDetailedAddon(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, "ecr", whole.RegistryName)

	addons, err := ociFacade(reg).ListAddon()
	require.NoError(t, err)
	require.Len(t, addons, 1)
	assert.Equal(t, "fluxcd", addons[0].Name)
	assert.Equal(t, "ecr", addons[0].RegistryName)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, addons[0].AvailableVersions)
}

// TestOCIRegistryLoadFiles verifies resolve returns the chart's raw buffered
// files (chart-name-prefixed) without the addon-specific parse, the reuse the
// module fetch (PullOCIChartFiles) depends on. Same fixture + seams.
func TestOCIRegistryLoadFiles(t *testing.T) {
	data, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)

	reg := &ociHelmBackend{
		name:     "ecr",
		url:      "oci://reg.example.com/addon",
		username: "AWS",
		token:    "secret",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			return []string{"1.0.0"}, nil
		},
		pullFn: func(_ context.Context, ref, _, _, _ string) ([]byte, error) {
			assert.Equal(t, "reg.example.com/addon/fluxcd:1.0.0", ref)
			return data, nil
		},
	}

	resolved, err := reg.resolve(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	require.NotEmpty(t, resolved.files)

	var hasPrefixed bool
	for _, f := range resolved.files {
		if strings.HasPrefix(f.Name, "fluxcd/") {
			hasPrefixed = true
			break
		}
	}
	assert.True(t, hasPrefixed, "chart files should be prefixed with the chart name")
}

// TestOCIRegistryExplicitVersion pins a version: no tag listing should happen,
// the exact tag is pulled.
func TestOCIRegistryExplicitVersion(t *testing.T) {
	data, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)

	reg := &ociHelmBackend{
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
	pkg, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "3.0.1")
	require.NoError(t, err)
	assert.Equal(t, "fluxcd", pkg.Name)
}

// TestOCIRegistryNoTags errors clearly when no semver tags exist.
func TestOCIRegistryNoTags(t *testing.T) {
	reg := &ociHelmBackend{
		name: "ecr", url: "oci://reg.example.com/addon",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			return []string{}, nil
		},
	}
	_, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no")
}

// TestListOCIRepositoriesRefusesForeignPaginationLink covers the credential-leak
// path: the Link header is registry-supplied, url.Parse resolves an absolute URL
// by replacing scheme and host outright, and every request in the pagination loop
// attaches the configured BasicAuth. Following a foreign link would hand the
// registry's credentials to a host we were never configured to talk to.
func TestListOCIRepositoriesRefusesForeignPaginationLink(t *testing.T) {
	var attackerHits int32
	attacker := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		_, _ = rw.Write([]byte(`{"repositories":["addon/pwned"]}`))
	}))
	defer attacker.Close()

	registry := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok {
			rw.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, "AWS", user)
		assert.Equal(t, "secret", pass)
		rw.Header().Set("Link", fmt.Sprintf(`<%s/v2/_catalog?n=1000&last=x>; rel="next"`, attacker.URL))
		_, _ = rw.Write([]byte(`{"repositories":["addon/fluxcd"]}`))
	}))
	defer registry.Close()

	// Trust the attacker's certificate as well as the registry's. With only the
	// registry's CA trusted, a link-following implementation would fail the TLS
	// handshake before sending anything, so attackerHits would stay zero
	// whether or not the link was refused -- the assertion below could not
	// catch the leak it is written to catch.
	useCatalogHTTPClient(t, clientTrusting(registry, attacker))

	registryURL := "oci://" + strings.TrimPrefix(registry.URL, "https://") + "/addon"
	_, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")

	require.Error(t, err, "a pagination link pointing at another host must be refused")
	assert.Contains(t, err.Error(), "refusing OCI catalog pagination link")
	assert.Zero(t, atomic.LoadInt32(&attackerHits), "the foreign host must never be contacted")
}

// TestListOCIRepositoriesRefusesPlaintextPaginationLink covers the downgrade
// variant: a link that keeps the host but drops to http would send BasicAuth in
// the clear.
func TestListOCIRepositoriesRefusesPlaintextPaginationLink(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		host := strings.TrimPrefix(server.URL, "https://")
		rw.Header().Set("Link", fmt.Sprintf(`<http://%s/v2/_catalog?n=1000&last=x>; rel="next"`, host))
		_, _ = rw.Write([]byte(`{"repositories":["addon/fluxcd"]}`))
	}))
	defer server.Close()

	useCatalogHTTPClient(t, server.Client())

	registryURL := "oci://" + strings.TrimPrefix(server.URL, "https://") + "/addon"
	_, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")

	require.Error(t, err, "an http downgrade in the pagination link must be refused")
	assert.Contains(t, err.Error(), "expected an https link")
}

// TestOCILoadFailuresAreSkippable pins the contract installDependency relies on:
// isSkippableRegistryError must recognise OCI failures, otherwise a dependency
// missing from an OCI registry aborts resolution instead of falling through to
// the remaining registries.
func TestOCILoadFailuresAreSkippable(t *testing.T) {
	t.Run("an unreachable-registry pull failure is a fetch error", func(t *testing.T) {
		reg := &ociHelmBackend{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			pullFn: func(context.Context, string, string, string, string) ([]byte, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return []string{"1.0.0"}, nil
			},
		}
		_, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.True(t, isSkippableRegistryError(err), "got %v", err)
		assert.Contains(t, err.Error(), "i/o timeout", "the underlying cause must stay visible")
	})

	// The complement of the case above, and the reason classify does not wrap
	// everything: a rejected credential or an artifact that is not a chart is a
	// fault in this registry that no other registry can substitute for.
	// Reporting it as ErrFetch spends the remaining registries on a request
	// that cannot succeed and then tells the operator the addon does not
	// exist anywhere, with the 401 nowhere in the message.
	t.Run("a rejected credential is not skippable", func(t *testing.T) {
		for _, cause := range []string{
			"unauthorized: authentication required",
			"denied: requested access to the resource is denied",
			"addon chart registry.example.com/addon/fluxcd:1.0.0 has no chart layer",
		} {
			reg := &ociHelmBackend{
				name: "ecr",
				url:  "oci://registry.example.com/addon",
				pullFn: func(context.Context, string, string, string, string) ([]byte, error) {
					return nil, errors.New(cause)
				},
				tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
					return []string{"1.0.0"}, nil
				},
			}
			_, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
			require.Error(t, err)
			assert.False(t, isSkippableRegistryError(err), "got %v", err)
			assert.Contains(t, err.Error(), cause, "the underlying cause must stay visible")
			assert.Contains(t, err.Error(), "OCI registry ecr", "the failing registry must be named")
		}
	})

	t.Run("no semver tags means the addon does not exist here", func(t *testing.T) {
		reg := &ociHelmBackend{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, nil
			},
		}
		_, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotExist)
		assert.True(t, isSkippableRegistryError(err))
	})

	t.Run("a tag listing failure is a fetch error", func(t *testing.T) {
		reg := &ociHelmBackend{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
		}
		_, err := ociFacade(reg).GetAddonInstallPackage(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.True(t, isSkippableRegistryError(err), "got %v", err)
	})
}

// TestOCICatalogAbsenceIsDistinguishable pins the discriminator updateOCIAddonCatalog
// depends on. Rebuilding the catalog from an empty list is only safe when there is
// genuinely no catalog; doing it after a transient read failure would publish a
// catalog containing one addon and silently drop every other entry.
func TestOCICatalogAbsenceIsDistinguishable(t *testing.T) {
	newServer := func(status int, body string) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(status)
			_, _ = rw.Write([]byte(body))
		}))
	}

	cases := map[string]struct {
		status     int
		wantAbsent bool
	}{
		"404 means enumeration is unsupported":         {status: http.StatusNotFound, wantAbsent: true},
		"405 means enumeration is unsupported":         {status: http.StatusMethodNotAllowed, wantAbsent: true},
		"501 means enumeration is unsupported":         {status: http.StatusNotImplemented, wantAbsent: true},
		"401 is a read failure, not an absent catalog": {status: http.StatusUnauthorized, wantAbsent: false},
		"500 is a read failure, not an absent catalog": {status: http.StatusInternalServerError, wantAbsent: false},
		"503 is a read failure, not an absent catalog": {status: http.StatusServiceUnavailable, wantAbsent: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := newServer(tc.status, `{}`)
			defer server.Close()

			useCatalogHTTPClient(t, server.Client())

			registryURL := "oci://" + strings.TrimPrefix(server.URL, "https://") + "/addon"
			_, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")
			require.Error(t, err)
			assert.Equal(t, tc.wantAbsent, errors.Is(err, ErrOCICatalogAbsent), "got %v", err)
		})
	}
}

// TestOCIListAddonKeepsReadFailuresDistinct covers the combined path: ListAddon
// may report an absent catalog only when both sources agree it is absent.
func TestOCIListAddonKeepsReadFailuresDistinct(t *testing.T) {
	absent := errors.Wrap(ErrOCICatalogAbsent, "no tags")
	readFail := errors.New("dial tcp: i/o timeout")

	t.Run("both absent reports absent", func(t *testing.T) {
		reg := &ociHelmBackend{
			name:           "ecr",
			url:            "oci://registry.example.com/addon",
			catalogIndexFn: func(context.Context, string, string, string) ([]*UIData, error) { return nil, absent },
			catalogFn:      func(context.Context, string, string, string) ([]string, error) { return nil, absent },
		}
		_, err := ociFacade(reg).ListAddon()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOCICatalogAbsent)
	})

	t.Run("a read failure on either side is not absent", func(t *testing.T) {
		for name, pair := range map[string][2]error{
			"index read failure":   {readFail, absent},
			"catalog read failure": {absent, readFail},
			"both read failures":   {readFail, readFail},
		} {
			t.Run(name, func(t *testing.T) {
				idxErr, catErr := pair[0], pair[1]
				reg := &ociHelmBackend{
					name:           "ecr",
					url:            "oci://registry.example.com/addon",
					catalogIndexFn: func(context.Context, string, string, string) ([]*UIData, error) { return nil, idxErr },
					catalogFn:      func(context.Context, string, string, string) ([]string, error) { return nil, catErr },
				}
				_, err := ociFacade(reg).ListAddon()
				require.Error(t, err)
				assert.NotErrorIs(t, err, ErrOCICatalogAbsent,
					"a read failure must never be reported as an absent catalog")
			})
		}
	})
}

// TestClassifyCatalogAbsenceProbe pins the gate that authorises overwriting a
// published catalog. Only a registry stating that the repository does not exist
// may pass; every other answer is a refusal, because a wrong "absent" silently
// drops every addon already published while a wrong "present" only refuses a
// push with a message the operator can act on.
func TestClassifyCatalogAbsenceProbe(t *testing.T) {
	const repo = "reg.example.com/addon/kubevela-addon-catalog"

	t.Run("a confirmed missing repository is the only pass", func(t *testing.T) {
		err := classifyCatalogAbsenceProbe(repo, nil,
			errors.New(`unexpected status code 404: name unknown: repository name not known to registry`))
		assert.NoError(t, err)
	})

	t.Run("a bare 404 is refused", func(t *testing.T) {
		// A proxy, a gateway, or a registry that does not serve the tag-list
		// route answers this way for a repository that does exist.
		err := classifyCatalogAbsenceProbe(repo, nil, errors.New(`unexpected status code 404: Not Found`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot confirm whether")
	})

	t.Run("a transient failure is refused", func(t *testing.T) {
		err := classifyCatalogAbsenceProbe(repo, nil, errors.New("dial tcp: i/o timeout"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot confirm whether")
	})

	t.Run("an auth failure is refused", func(t *testing.T) {
		err := classifyCatalogAbsenceProbe(repo, nil,
			errors.New(`unexpected status code 401: unauthorized: authentication required`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot confirm whether")
	})

	t.Run("a published tag that could not be read is refused", func(t *testing.T) {
		err := classifyCatalogAbsenceProbe(repo, []string{"0.0.4", "0.0.3"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `holds catalog tag "0.0.4"`)
	})

	t.Run("an existing repository with no semver tag is refused", func(t *testing.T) {
		// helm's tag listing drops anything that is not strict semver, so an
		// empty result does not mean the repository is empty -- a catalog tagged
		// "latest" or "v0.0.1" is invisible here.
		err := classifyCatalogAbsenceProbe(repo, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exposes no semver-tagged catalog")
	})
}

// TestNewOCIClientWithPlainHTTP covers client construction for both transports.
// Empty credentials skip Login, so this exercises the constructor without any
// network call.
func TestNewOCIClientWithPlainHTTP(t *testing.T) {
	for _, plainHTTP := range []bool{true, false} {
		client, err := component.NewOCIClientWithPlainHTTP("reg.example.com", "", "", plainHTTP)
		require.NoError(t, err)
		assert.NotNil(t, client)
	}
}

// TestNewOCIClientWithPlainHTTPLoginFailure covers the credentialed branch:
// non-empty credentials trigger a real Login call, which fails fast and
// deterministically against a closed loopback port.
func TestNewOCIClientWithPlainHTTPLoginFailure(t *testing.T) {
	_, err := component.NewOCIClientWithPlainHTTP(closedPortHost, "AWS", "secret", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to login to OCI registry")
}

// closedPortRepoRef and closedPortHost point at a loopback port nothing is
// listening on, so the dial fails immediately and deterministically without
// any real network dependency.
const (
	closedPortHost    = "127.0.0.1:1"
	closedPortRepoRef = "127.0.0.1:1/addon/fluxcd"
)

// TestPullOCIChartWithTransportDialFailure exercises the real Helm registry
// client against a closed port for both transports, pinning the wrap text
// callers rely on.
func TestPullOCIChartWithTransportDialFailure(t *testing.T) {
	for _, plainHTTP := range []bool{true, false} {
		_, err := component.PullOCIChartWithTransport(closedPortRepoRef+":1.0.0", closedPortHost, "", "", plainHTTP)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to pull addon chart")
	}
}

// TestPullOCIChartWrappers covers component.PullOCIChart and component.PullOCIChartWithPlainHTTP,
// which only select a transport before delegating.
func TestPullOCIChartWrappers(t *testing.T) {
	_, err := component.PullOCIChart(context.Background(), closedPortRepoRef+":1.0.0", closedPortHost, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart")

	_, err = component.PullOCIChartWithPlainHTTP(context.Background(), closedPortRepoRef+":1.0.0", closedPortHost, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart")
}

// TestListOCITagsWithTransportDialFailure exercises the real Helm registry
// client against a closed port for both transports.
func TestListOCITagsWithTransportDialFailure(t *testing.T) {
	for _, plainHTTP := range []bool{true, false} {
		_, err := component.ListOCITagsWithTransport(closedPortRepoRef, closedPortHost, "", "", plainHTTP)
		require.Error(t, err)
	}
}

// TestListOCITagsWrappers covers component.ListOCITags and component.ListOCITagsWithPlainHTTP.
func TestListOCITagsWrappers(t *testing.T) {
	_, err := component.ListOCITags(context.Background(), closedPortRepoRef, closedPortHost, "", "")
	require.Error(t, err)

	_, err = component.ListOCITagsWithPlainHTTP(context.Background(), closedPortRepoRef, closedPortHost, "", "")
	require.Error(t, err)
}

// TestGetAddonAvailableVersion covers both the success path (a real tagsFn
// producing chart versions) and the tagsFn-error passthrough.
func TestGetAddonAvailableVersion(t *testing.T) {
	t.Run("returns a chart version per tag", func(t *testing.T) {
		reg := &ociHelmBackend{
			url: "oci://reg.example.com/addon",
			tagsFn: func(_ context.Context, repoRef, host, _, _ string) ([]string, error) {
				assert.Equal(t, "reg.example.com/addon/fluxcd", repoRef)
				assert.Equal(t, "reg.example.com", host)
				return []string{"2.0.0", "1.0.0"}, nil
			},
		}
		versions, err := ociFacade(reg).GetAddonAvailableVersion("fluxcd")
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, "fluxcd", versions[0].Name)
		assert.Equal(t, "2.0.0", versions[0].Version)
		assert.Equal(t, "1.0.0", versions[1].Version)
	})

	t.Run("propagates a tag listing failure", func(t *testing.T) {
		reg := &ociHelmBackend{
			url: "oci://reg.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
		}
		_, err := ociFacade(reg).GetAddonAvailableVersion("fluxcd")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "i/o timeout")
	})
}

// TestOCIRegistryGetAddonUIDataCarriesAvailableVersions covers the field the UI
// and the shared addon cache read to offer version choices. loadAddon builds the
// package from the chart archive alone, which carries no notion of sibling tags,
// so the tag list it already fetched to resolve "latest" has to be attached.
func TestOCIRegistryGetAddonUIDataCarriesAvailableVersions(t *testing.T) {
	data, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)

	reg := &ociHelmBackend{
		name: "ecr", url: "oci://reg.example.com/addon",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			return []string{"3.0.1", "2.0.0", "1.0.0"}, nil
		},
		pullFn: func(_ context.Context, _, _, _, _ string) ([]byte, error) { return data, nil },
	}

	ui, err := ociFacade(reg).GetAddonUIData(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, ui.AvailableVersions)

	whole, err := ociFacade(reg).GetDetailedAddon(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, whole.AvailableVersions)
}

// TestOCIPullNormalizesBuildMetadataTag pins the tag round-trip for versions
// carrying SemVer build metadata.
//
// OCI tags cannot contain "+", so Helm stores such a version with "_" and
// Client.Tags converts it back to "+" when listing. That makes the version this
// code carries around ("1.0.0+build.5") differ from the tag actually published
// ("1.0.0_build.5"), which would pull a nonexistent tag if the reference were
// used verbatim. Client.Pull applies the same substitution as Client.Push, so
// the reference is normalized on the way out and both directions agree. This
// test fails if that ever stops being true.
func TestOCIPullNormalizesBuildMetadataTag(t *testing.T) {
	resetOCIClientCache(t)

	var requested []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		mu.Lock()
		requested = append(requested, req.URL.Path)
		mu.Unlock()
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	// The pull fails: the point is the reference the client puts on the wire,
	// not the response.
	_, err := component.PullOCIChartWithTransport(host+"/addon/fluxcd:1.0.0+build.5", host, "", "", true)
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, requested, "the client must have reached the registry")
	for _, path := range requested {
		assert.NotContains(t, path, "+", "a plus sign is not a legal OCI tag character")
		assert.NotContains(t, path, "%2B", "the plus must be substituted, not percent-encoded")
	}
	assert.Contains(t, strings.Join(requested, " "), "1.0.0_build.5",
		"the published tag spelling must be what is requested")
}
