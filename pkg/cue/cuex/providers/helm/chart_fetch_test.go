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

package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/singleton"
)

var _ = Describe("chart_fetch", func() {

	Describe("detectChartSourceType", func() {
		DescribeTable("should detect the correct chart source type",
			func(source, expected string) {
				Expect(detectChartSourceType(source)).To(Equal(expected))
			},
			Entry("OCI registry", "oci://ghcr.io/stefanprodan/charts/podinfo", "oci"),
			Entry("Direct URL with .tgz", "https://github.com/nginx/nginx-helm/releases/download/nginx-1.1.0/nginx-1.1.0.tgz", "url"),
			Entry("Direct URL with .tar.gz", "https://example.com/charts/app-1.0.0.tar.gz", "url"),
			Entry("HTTP URL", "http://charts.example.com/app-1.0.0.tgz", "url"),
			Entry("Repository chart", "postgresql", "repo"),
			Entry("Repository chart with path", "stable/postgresql", "repo"),
		)
	})

	Describe("isMutableVersion", func() {
		DescribeTable("should classify mutable vs immutable versions",
			func(version string, expected bool) {
				Expect(isMutableVersion(version)).To(Equal(expected))
			},
			// Mutable tags
			Entry("latest", "latest", true),
			Entry("dev", "dev", true),
			Entry("develop", "develop", true),
			Entry("main", "main", true),
			Entry("master", "master", true),
			Entry("edge", "edge", true),
			Entry("canary", "canary", true),
			Entry("nightly", "nightly", true),
			Entry("case insensitive LATEST", "LATEST", true),
			Entry("SNAPSHOT suffix", "1.0.0-SNAPSHOT", true),
			Entry("dev suffix", "1.0.0-dev", true),
			Entry("alpha suffix", "1.0.0-alpha", true),
			Entry("beta suffix", "1.0.0-beta", true),
			Entry("rc suffix", "1.0.0-rc", true),
			Entry("unknown string defaults to mutable", "my-custom-branch", true),

			// Immutable versions
			Entry("semver 1.2.3", "1.2.3", false),
			Entry("semver v1.2.3", "v1.2.3", false),
			Entry("semver short 1.0", "1.0", false),
		)
	})

	Describe("determineCacheTTL", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(&CacheTTLConfig{
				ImmutableVersionTTL: 24 * time.Hour,
				MutableVersionTTL:   5 * time.Minute,
			})
		})

		It("should use default immutable TTL for semver", func() {
			Expect(p.determineCacheTTL("1.2.3", nil)).To(Equal(24 * time.Hour))
		})

		It("should use default mutable TTL for latest", func() {
			Expect(p.determineCacheTTL("latest", nil)).To(Equal(5 * time.Minute))
		})

		It("should use explicit TTL from options", func() {
			ttl := p.determineCacheTTL("1.2.3", &RenderOptionsParams{
				Cache: &CacheParams{TTL: "10m"},
			})
			Expect(ttl).To(Equal(10 * time.Minute))
		})

		It("should use explicit immutable TTL from options", func() {
			ttl := p.determineCacheTTL("1.2.3", &RenderOptionsParams{
				Cache: &CacheParams{ImmutableTTL: "48h"},
			})
			Expect(ttl).To(Equal(48 * time.Hour))
		})

		It("should use explicit mutable TTL from options", func() {
			ttl := p.determineCacheTTL("latest", &RenderOptionsParams{
				Cache: &CacheParams{MutableTTL: "1m"},
			})
			Expect(ttl).To(Equal(1 * time.Minute))
		})

		It("should fall back to default for invalid TTL string", func() {
			ttl := p.determineCacheTTL("1.2.3", &RenderOptionsParams{
				Cache: &CacheParams{TTL: "invalid"},
			})
			Expect(ttl).To(Equal(24 * time.Hour))
		})

		It("should fall back to default when TTL is '0'", func() {
			// TTL=0 is handled by fetchChart; determineCacheTTL falls through
			ttl := p.determineCacheTTL("1.2.3", &RenderOptionsParams{
				Cache: &CacheParams{TTL: "0"},
			})
			Expect(ttl).To(Equal(24 * time.Hour))
		})

		It("should fall back to default for invalid mutable TTL string", func() {
			ttl := p.determineCacheTTL("latest", &RenderOptionsParams{
				Cache: &CacheParams{MutableTTL: "bad"},
			})
			Expect(ttl).To(Equal(5 * time.Minute))
		})

		It("should fall back to default for invalid immutable TTL string", func() {
			ttl := p.determineCacheTTL("1.2.3", &RenderOptionsParams{
				Cache: &CacheParams{ImmutableTTL: "bad"},
			})
			Expect(ttl).To(Equal(24 * time.Hour))
		})
	})

	Describe("fetchChartWithoutCache", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should fail for unsupported source type", func() {
			_, err := p.fetchChartWithoutCache(context.Background(), &ChartSourceParams{Source: "test"}, "unknown", "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported chart source type"))
		})

		It("should fail for repo without repoURL", func() {
			_, err := p.fetchChartWithoutCache(context.Background(), &ChartSourceParams{Source: "nginx"}, "repo", "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("repoURL is required"))
		})
	})

	Describe("fetchChart", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should return a cached chart on cache hit", func() {
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "cached-chart", Version: "1.0.0"},
			}
			// Pre-seed the cache with the expected key format: <sourceType>/<source>/<version>
			cacheKey := "repo/nginx/1.0.0"
			p.cache.Put(cacheKey, testChart, 1*time.Hour)

			result, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "nginx", Version: "1.0.0"},
				nil, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("cached-chart"))
		})

		It("should return a cached chart with custom cache key prefix", func() {
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "custom-cached", Version: "2.0.0"},
			}
			// With custom cache key: <cache_key_prefix>/<sourceType>/<source>/<version>
			cacheKey := "my-prefix/repo/myapp/2.0.0"
			p.cache.Put(cacheKey, testChart, 1*time.Hour)

			result, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "myapp", Version: "2.0.0"},
				&RenderOptionsParams{Cache: &CacheParams{Key: "my-prefix"}}, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("custom-cached"))
		})

		It("should bypass cache when TTL is '0'", func() {
			// TTL=0 means cache disabled — should call fetchChartWithoutCache directly
			// which will fail since there's no real repo, but the code path is exercised
			_, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "nginx"},
				&RenderOptionsParams{Cache: &CacheParams{TTL: "0"}}, "", "")
			Expect(err).Should(HaveOccurred())
			// The error should come from fetchChartWithoutCache, not from cache logic
			Expect(err.Error()).To(ContainSubstring("repoURL is required"))
		})

		It("should build correct cache key for OCI sources", func() {
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "oci-chart", Version: "3.0.0"},
			}
			// OCI source: oci://ghcr.io/example/chart
			// After replacing "://" with "-" and "/" with "-": oci-ghcr.io-example-chart
			cacheKey := "oci/oci-ghcr.io-example-chart/3.0.0"
			p.cache.Put(cacheKey, testChart, 1*time.Hour)

			result, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "oci://ghcr.io/example/chart", Version: "3.0.0"},
				nil, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("oci-chart"))
		})

		It("should build correct cache key for URL sources", func() {
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "url-chart", Version: "1.0.0"},
			}
			// URL source: https://example.com/chart.tgz
			// After replacing "://" with "-" and "/" with "-": https-example.com-chart.tgz
			cacheKey := "url/https-example.com-chart.tgz/1.0.0"
			p.cache.Put(cacheKey, testChart, 1*time.Hour)

			result, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "https://example.com/chart.tgz", Version: "1.0.0"},
				nil, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("url-chart"))
		})
	})

	Describe("fetchRepoChart with httptest", func() {
		It("should fetch chart from a repo server", func() {
			chartArchive := createMinimalChartArchive("test-repo-chart", "1.0.0")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.yaml":
					w.Header().Set("Content-Type", "application/x-yaml")
					_, _ = w.Write([]byte(`apiVersion: v1
entries:
  test-repo-chart:
    - name: test-repo-chart
      version: 1.0.0
      urls:
        - charts/test-repo-chart-1.0.0.tgz
`))
				case "/charts/test-repo-chart-1.0.0.tgz":
					w.Header().Set("Content-Type", "application/gzip")
					_, _ = w.Write(chartArchive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			ch, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "test-repo-chart",
				RepoURL: server.URL,
				Version: "1.0.0",
			}, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch).ToNot(BeNil())
			Expect(ch.Metadata.Name).To(Equal("test-repo-chart"))
			Expect(ch.Metadata.Version).To(Equal("1.0.0"))
		})

		It("should use first version when no version specified", func() {
			chartArchive := createMinimalChartArchive("no-ver-chart", "3.0.0")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = w.Write([]byte(`apiVersion: v1
entries:
  no-ver-chart:
    - name: no-ver-chart
      version: 3.0.0
      urls:
        - no-ver-chart-3.0.0.tgz
`))
				case "/no-ver-chart-3.0.0.tgz":
					_, _ = w.Write(chartArchive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			ch, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "no-ver-chart",
				RepoURL: server.URL,
			}, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch.Metadata.Name).To(Equal("no-ver-chart"))
		})

		It("should return error when chart not found in index", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`apiVersion: v1
entries:
  other-chart:
    - name: other-chart
      version: 1.0.0
      urls:
        - other-chart-1.0.0.tgz
`))
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			_, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "missing-chart",
				RepoURL: server.URL,
			}, "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in repository"))
		})

		It("should return error when version not found", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`apiVersion: v1
entries:
  my-chart:
    - name: my-chart
      version: 1.0.0
      urls:
        - my-chart-1.0.0.tgz
`))
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			_, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "my-chart",
				RepoURL: server.URL,
				Version: "99.0.0",
			}, "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should return error when index is invalid YAML", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{invalid yaml!!!`))
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			_, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "test",
				RepoURL: server.URL,
			}, "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse repository index"))
		})

		It("should return error when chart version has no URLs", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`apiVersion: v1
entries:
  empty-urls:
    - name: empty-urls
      version: 1.0.0
      urls: []
`))
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			_, err := p.fetchRepoChart(context.Background(), &ChartSourceParams{
				Source:  "empty-urls",
				RepoURL: server.URL,
				Version: "1.0.0",
			}, "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no download URL found"))
		})
	})

	Describe("fetchURLChart with httptest", func() {
		It("should fetch chart from a direct URL", func() {
			chartArchive := createMinimalChartArchive("url-chart", "2.0.0")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/gzip")
				_, _ = w.Write(chartArchive)
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			ch, err := p.fetchURLChart(context.Background(), &ChartSourceParams{
				Source: server.URL + "/url-chart-2.0.0.tgz",
			}, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch).ToNot(BeNil())
			Expect(ch.Metadata.Name).To(Equal("url-chart"))
			Expect(ch.Metadata.Version).To(Equal("2.0.0"))
		})

		It("should return error for unreachable URL", func() {
			p := NewProviderWithConfig(nil)
			_, err := p.fetchURLChart(context.Background(), &ChartSourceParams{
				Source: "http://127.0.0.1:1/nonexistent.tgz",
			}, "", "")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download chart"))
		})
	})

	Describe("fetchChart cache miss and store", func() {
		It("should fetch repo chart on miss and cache it", func() {
			chartArchive := createMinimalChartArchive("cache-miss", "1.0.0")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = w.Write([]byte(`apiVersion: v1
entries:
  cache-miss:
    - name: cache-miss
      version: 1.0.0
      urls:
        - cache-miss-1.0.0.tgz
`))
				case "/cache-miss-1.0.0.tgz":
					_, _ = w.Write(chartArchive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			ch, err := p.fetchChart(context.Background(), &ChartSourceParams{
				Source:  "cache-miss",
				RepoURL: server.URL,
				Version: "1.0.0",
			}, nil, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch.Metadata.Name).To(Equal("cache-miss"))

			// Verify it's now cached
			cached := p.cache.Get("repo/cache-miss/1.0.0")
			Expect(cached).ToNot(BeNil())
		})

		It("should fetch URL chart on miss and cache it", func() {
			chartArchive := createMinimalChartArchive("url-cache", "1.0.0")

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(chartArchive)
			}))
			defer server.Close()

			p := NewProviderWithConfig(nil)
			ch, err := p.fetchChart(context.Background(), &ChartSourceParams{
				Source:  server.URL + "/url-cache-1.0.0.tgz",
				Version: "1.0.0",
			}, nil, "", "")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch.Metadata.Name).To(Equal("url-cache"))
		})

		It("re-runs the auth resolver on a cache hit when the source declares auth.secretRef", func() {
			// Pre-warm the cache with a chart that was previously fetched
			// without auth, then make a follow-up request that references
			// a missing Secret. The resolver must fail rather than letting
			// the cached chart bytes paper over a missing/invalid Secret.
			chartArchive := createMinimalChartArchive("auth-cache", "1.0.0")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = w.Write([]byte(`apiVersion: v1
entries:
  auth-cache:
    - name: auth-cache
      version: 1.0.0
      urls:
        - auth-cache-1.0.0.tgz
`))
				case "/auth-cache-1.0.0.tgz":
					_, _ = w.Write(chartArchive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			origKube := singleton.KubeClient.Get()
			singleton.KubeClient.Set(c)
			defer singleton.KubeClient.Set(origKube)

			p := NewProviderWithConfig(nil)
			// First call: no auth, populates the cache.
			_, err := p.fetchChart(context.Background(), &ChartSourceParams{
				Source:  "auth-cache",
				RepoURL: server.URL,
				Version: "1.0.0",
			}, nil, "ns-app", "ns-rel")
			Expect(err).ShouldNot(HaveOccurred())

			// Second call: same source/version (cache hit), but with an
			// auth.secretRef pointing at a Secret that does not exist.
			_, err = p.fetchChart(context.Background(), &ChartSourceParams{
				Source:  "auth-cache",
				RepoURL: server.URL,
				Version: "1.0.0",
				Auth: &AuthParams{
					SecretRef: &SecretRefParams{Name: "missing-secret"},
				},
			}, nil, "ns-app", "ns-rel")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-secret"))
		})
	})

})

// createMinimalChartArchive creates a minimal valid Helm chart .tgz archive
// for testing. The archive contains Chart.yaml and a simple ConfigMap template.
func createMinimalChartArchive(name, version string) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	chartYaml := fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", name, version)
	_ = tarWriter.WriteHeader(&tar.Header{
		Name: name + "/Chart.yaml",
		Size: int64(len(chartYaml)),
		Mode: 0644,
	})
	_, _ = tarWriter.Write([]byte(chartYaml))

	tmpl := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
`
	_ = tarWriter.WriteHeader(&tar.Header{
		Name: name + "/templates/configmap.yaml",
		Size: int64(len(tmpl)),
		Mode: 0644,
	})
	_, _ = tarWriter.Write([]byte(tmpl))

	_ = tarWriter.Close()
	_ = gzWriter.Close()

	return buf.Bytes()
}

var _ = Describe("fetchURLChart with auth", func() {
	var (
		scheme         *runtime.Scheme
		origKubeClient client.Client
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		// Capture the package-global KubeClient so the per-spec
		// singleton.KubeClient.Set() calls below cannot leak fake
		// clients into later tests in this package.
		origKubeClient = singleton.KubeClient.Get()
	})
	AfterEach(func() {
		singleton.KubeClient.Set(origKubeClient)
	})

	It("sends Authorization: Basic when params.Auth references a basic-auth Secret", func() {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			tgz := createMinimalChartArchive("auth-chart", "1.0.0")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(tgz)
		}))
		defer server.Close()

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "rel-ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{corev1.BasicAuthUsernameKey: []byte("alice"), corev1.BasicAuthPasswordKey: []byte("wonderland")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build())

		p := NewProvider()
		params := &ChartSourceParams{
			Source: server.URL + "/x.tgz",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "creds"}},
		}
		_, err := p.fetchURLChart(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(gotAuth).To(HavePrefix("Basic "))
	})
})

var _ = Describe("fetchRepoChart with auth", func() {
	var (
		scheme         *runtime.Scheme
		origKubeClient client.Client
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		origKubeClient = singleton.KubeClient.Get()
	})
	AfterEach(func() {
		singleton.KubeClient.Set(origKubeClient)
	})

	It("authenticates both index.yaml and chart-tarball fetches", func() {
		var indexAuth, chartAuth string
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/index.yaml" {
				indexAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`apiVersion: v1
entries:
  podinfo:
  - name: podinfo
    version: 1.0.0
    urls:
    - ` + server.URL + `/podinfo-1.0.0.tgz
`))
				return
			}
			if r.URL.Path == "/podinfo-1.0.0.tgz" {
				chartAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(createMinimalChartArchive("podinfo", "1.0.0"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "rel-ns"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{corev1.BasicAuthUsernameKey: []byte("u"), corev1.BasicAuthPasswordKey: []byte("p")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build())

		p := NewProvider()
		params := &ChartSourceParams{
			Source:  "podinfo",
			RepoURL: server.URL,
			Version: "1.0.0",
			Auth:    &AuthParams{SecretRef: &SecretRefParams{Name: "creds"}},
		}
		_, err := p.fetchRepoChart(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).NotTo(HaveOccurred())
		Expect(indexAuth).To(HavePrefix("Basic "))
		Expect(chartAuth).To(HavePrefix("Basic "))
		Expect(indexAuth).To(Equal(chartAuth))
	})
})

var _ = Describe("fetchOCIChart with auth", func() {
	var (
		scheme         *runtime.Scheme
		origKubeClient client.Client
	)
	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		// Capture the package-global KubeClient so the per-spec
		// singleton.KubeClient.Set() calls below cannot leak fake
		// clients into later tests in this package.
		origKubeClient = singleton.KubeClient.Get()
	})
	AfterEach(func() {
		singleton.KubeClient.Set(origKubeClient)
	})

	It("rejects a user-supplied bearer token on an OCI source", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "rel-ns"},
			Data:       map[string][]byte{"token": []byte("abc.def.ghi")},
		}
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build())

		p := NewProvider()
		params := &ChartSourceParams{
			Source: "oci://ghcr.io/foo/podinfo",
			Auth:   &AuthParams{SecretRef: &SecretRefParams{Name: "t"}},
		}
		_, err := p.fetchOCIChart(context.Background(), params, "app-ns", "rel-ns")
		Expect(err).To(MatchError(ContainSubstring(`user-supplied bearer tokens MUST NOT be used with OCI sources`)))
	})
})
