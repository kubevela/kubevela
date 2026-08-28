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
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
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
		assert.True(t, isOCIRepositoryAbsentError(err), "expected absent for: %v", err)
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
		assert.False(t, isOCIRepositoryAbsentError(err), "expected not-absent for: %v", err)
	}
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
		rw.Header().Set("Link", fmt.Sprintf(`<%s/v2/_catalog?n=1000&last=x>; rel="next"`, attacker.URL))
		_, _ = rw.Write([]byte(`{"repositories":["addon/fluxcd"]}`))
	}))
	defer registry.Close()

	originalClient := http.DefaultClient
	http.DefaultClient = registry.Client()
	defer func() { http.DefaultClient = originalClient }()

	registryURL := "oci://" + strings.TrimPrefix(registry.URL, "https://") + "/addon"
	_, err := listOCIRepositories(context.Background(), registryURL, "AWS", "secret")

	require.Error(t, err, "a pagination link pointing at another host must be refused")
	assert.Contains(t, err.Error(), "refusing OCI catalog pagination link")
	assert.Zero(t, atomic.LoadInt32(&attackerHits), "credentials must never be sent to the foreign host")
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

	originalClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = originalClient }()

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
	t.Run("a pull failure is a fetch error", func(t *testing.T) {
		reg := &ociRegistry{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			pullFn: func(context.Context, string, string, string, string) ([]byte, error) {
				return nil, errors.New("unauthorized: authentication required")
			},
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return []string{"1.0.0"}, nil
			},
		}
		_, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.True(t, isSkippableRegistryError(err), "got %v", err)
		assert.Contains(t, err.Error(), "unauthorized", "the underlying cause must stay visible")
	})

	t.Run("no semver tags means the addon does not exist here", func(t *testing.T) {
		reg := &ociRegistry{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, nil
			},
		}
		_, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotExist)
		assert.True(t, isSkippableRegistryError(err))
	})

	t.Run("a tag listing failure is a fetch error", func(t *testing.T) {
		reg := &ociRegistry{
			name: "ecr",
			url:  "oci://registry.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
		}
		_, err := reg.GetAddonInstallPackage(context.Background(), "fluxcd", "")
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

			originalClient := http.DefaultClient
			http.DefaultClient = server.Client()
			defer func() { http.DefaultClient = originalClient }()

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
		reg := &ociRegistry{
			name:           "ecr",
			url:            "oci://registry.example.com/addon",
			catalogIndexFn: func(context.Context, string, string, string) ([]*UIData, error) { return nil, absent },
			catalogFn:      func(context.Context, string, string, string) ([]string, error) { return nil, absent },
		}
		_, err := reg.ListAddon()
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
				reg := &ociRegistry{
					name:           "ecr",
					url:            "oci://registry.example.com/addon",
					catalogIndexFn: func(context.Context, string, string, string) ([]*UIData, error) { return nil, idxErr },
					catalogFn:      func(context.Context, string, string, string) ([]string, error) { return nil, catErr },
				}
				_, err := reg.ListAddon()
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
		client, err := newOCIClientWithPlainHTTP("reg.example.com", "", "", plainHTTP)
		require.NoError(t, err)
		assert.NotNil(t, client)
	}
}

// TestNewOCIClientWithPlainHTTPLoginFailure covers the credentialed branch:
// non-empty credentials trigger a real Login call, which fails fast and
// deterministically against a closed loopback port.
func TestNewOCIClientWithPlainHTTPLoginFailure(t *testing.T) {
	_, err := newOCIClientWithPlainHTTP(closedPortHost, "AWS", "secret", false)
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
		_, err := pullOCIChartWithTransport(closedPortRepoRef+":1.0.0", closedPortHost, "", "", plainHTTP)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to pull addon chart")
	}
}

// TestPullOCIChartWrappers covers pullOCIChart and pullOCIChartWithPlainHTTP,
// which only select a transport before delegating.
func TestPullOCIChartWrappers(t *testing.T) {
	_, err := pullOCIChart(context.Background(), closedPortRepoRef+":1.0.0", closedPortHost, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart")

	_, err = pullOCIChartWithPlainHTTP(context.Background(), closedPortRepoRef+":1.0.0", closedPortHost, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull addon chart")
}

// TestListOCITagsWithTransportDialFailure exercises the real Helm registry
// client against a closed port for both transports.
func TestListOCITagsWithTransportDialFailure(t *testing.T) {
	for _, plainHTTP := range []bool{true, false} {
		_, err := listOCITagsWithTransport(closedPortRepoRef, closedPortHost, "", "", plainHTTP)
		require.Error(t, err)
	}
}

// TestListOCITagsWrappers covers listOCITags and listOCITagsWithPlainHTTP.
func TestListOCITagsWrappers(t *testing.T) {
	_, err := listOCITags(context.Background(), closedPortRepoRef, closedPortHost, "", "")
	require.Error(t, err)

	_, err = listOCITagsWithPlainHTTP(context.Background(), closedPortRepoRef, closedPortHost, "", "")
	require.Error(t, err)
}

// TestGetAddonAvailableVersion covers both the success path (a real tagsFn
// producing chart versions) and the tagsFn-error passthrough.
func TestGetAddonAvailableVersion(t *testing.T) {
	t.Run("returns a chart version per tag", func(t *testing.T) {
		reg := &ociRegistry{
			url: "oci://reg.example.com/addon",
			tagsFn: func(_ context.Context, repoRef, host, _, _ string) ([]string, error) {
				assert.Equal(t, "reg.example.com/addon/fluxcd", repoRef)
				assert.Equal(t, "reg.example.com", host)
				return []string{"2.0.0", "1.0.0"}, nil
			},
		}
		versions, err := reg.GetAddonAvailableVersion("fluxcd")
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, "fluxcd", versions[0].Name)
		assert.Equal(t, "2.0.0", versions[0].Version)
		assert.Equal(t, "1.0.0", versions[1].Version)
	})

	t.Run("propagates a tag listing failure", func(t *testing.T) {
		reg := &ociRegistry{
			url: "oci://reg.example.com/addon",
			tagsFn: func(context.Context, string, string, string, string) ([]string, error) {
				return nil, errors.New("dial tcp: i/o timeout")
			},
		}
		_, err := reg.GetAddonAvailableVersion("fluxcd")
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

	reg := &ociRegistry{
		name: "ecr", url: "oci://reg.example.com/addon",
		tagsFn: func(_ context.Context, _, _, _, _ string) ([]string, error) {
			return []string{"3.0.1", "2.0.0", "1.0.0"}, nil
		},
		pullFn: func(_ context.Context, _, _, _, _ string) ([]byte, error) { return data, nil },
	}

	ui, err := reg.GetAddonUIData(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, ui.AvailableVersions)

	whole, err := reg.GetDetailedAddon(context.Background(), "fluxcd", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"3.0.1", "2.0.0", "1.0.0"}, whole.AvailableVersions)
}
