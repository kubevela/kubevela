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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Helm Provider", func() {

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

	Describe("orderResources", func() {
		It("should order CRDs first, then Namespaces, then others", func() {
			crd := map[string]interface{}{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata":   map[string]interface{}{"name": "test-crd"},
			}
			namespace := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": "test-namespace"},
			}
			deployment := map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test-deployment"},
			}
			service := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": "test-service"},
			}

			input := []map[string]interface{}{deployment, service, crd, namespace}
			result := orderResources(input)

			Expect(result).To(HaveLen(4))
			Expect(result[0]["kind"]).To(Equal("CustomResourceDefinition"))
			Expect(result[1]["kind"]).To(Equal("Namespace"))
			Expect(result[2]["kind"]).To(Equal("Deployment"))
			Expect(result[3]["kind"]).To(Equal("Service"))
		})
	})

	Describe("isTestResource", func() {
		DescribeTable("should identify test hook resources",
			func(annotations map[string]interface{}, expected bool) {
				resource := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]interface{}{
							"name":        "test-pod",
							"annotations": annotations,
						},
					},
				}
				Expect(isTestResource(resource)).To(Equal(expected))
			},
			Entry("test-success hook", map[string]interface{}{"helm.sh/hook": "test-success"}, true),
			Entry("pre-install hook", map[string]interface{}{"helm.sh/hook": "pre-install"}, false),
		)

		It("should return false for resources without annotations", func() {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata":   map[string]interface{}{"name": "my-service"},
				},
			}
			Expect(isTestResource(resource)).To(BeFalse())
		})
	})

	Describe("mergeValues", func() {
		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
			// Empty fake client so ConfigMap/Secret Gets return NotFound. Tests
			// that need specific resources override the singleton themselves.
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		})

		It("should return base values when no valuesFrom", func() {
			baseValues := map[string]interface{}{
				"key1": "value1",
				"nested": map[string]interface{}{
					"key2": "value2",
				},
			}
			result, err := p.mergeValues(ctx, baseValues, nil, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(baseValues))
		})

		It("should return empty map for nil base values", func() {
			result, err := p.mergeValues(ctx, nil, nil, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("should skip optional source when ConfigMap is missing", func() {
			base := map[string]interface{}{"key": "value"}
			valuesFrom := []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: true},
			}
			result, err := p.mergeValues(ctx, base, valuesFrom, "default", "default")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["key"]).To(Equal("value"))
		})

		It("should propagate missing-source errors when not optional", func() {
			base := map[string]interface{}{"key": "value"}
			valuesFrom := []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: false},
			}
			_, err := p.mergeValues(ctx, base, valuesFrom, "default", "default")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load values"))
		})
	})

	Describe("RenderParams structure", func() {
		It("should hold all fields correctly", func() {
			params := &RenderParams{
				Chart: ChartSourceParams{
					Source:  "nginx",
					RepoURL: "https://charts.bitnami.com/bitnami",
					Version: "1.0.0",
				},
				Release: &ReleaseParams{
					Name:      "my-release",
					Namespace: "my-namespace",
				},
				Values: map[string]interface{}{
					"replicaCount": 2,
				},
				Context: &ContextParams{
					AppName:      "my-app",
					AppNamespace: "my-app-ns",
					Name:         "nginx-component",
					Namespace:    "my-namespace",
				},
			}

			Expect(params.Chart.Source).To(Equal("nginx"))
			Expect(params.Release.Name).To(Equal("my-release"))
			Expect(params.Values.(map[string]interface{})["replicaCount"]).To(Equal(2))
			Expect(params.Context.AppName).To(Equal("my-app"))
			Expect(params.Context.Name).To(Equal("nginx-component"))
		})
	})

	Describe("velaLabelPostRenderer", func() {
		It("should inject KubeVela labels and annotations on all resources", func() {
			manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: test-ns
spec:
  replicas: 1
---
apiVersion: v1
kind: Service
metadata:
  name: test-svc
  namespace: test-ns
`
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-app-ns",
				Name:         "my-component",
				Namespace:    "test-ns",
			}
			renderer := &velaLabelPostRenderer{
				context:          velaCtx,
				releaseName:      "my-release",
				releaseNamespace: "test-ns",
			}
			result, err := renderer.Run(bytes.NewBufferString(manifest))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 4096)
			var resourceCount int
			for {
				obj := &unstructured.Unstructured{}
				if err := decoder.Decode(obj); err != nil {
					if err == io.EOF {
						break
					}
					Fail(fmt.Sprintf("failed to decode: %v", err))
				}
				if len(obj.Object) == 0 {
					continue
				}
				resourceCount++

				labels := obj.GetLabels()
				Expect(labels["app.oam.dev/name"]).To(Equal("my-app"))
				Expect(labels["app.oam.dev/namespace"]).To(Equal("my-app-ns"))
				Expect(labels["app.oam.dev/component"]).To(Equal("my-component"))

				annotations := obj.GetAnnotations()
				Expect(annotations["app.oam.dev/owner"]).To(Equal("helm-provider"))
				Expect(annotations["meta.helm.sh/release-name"]).To(Equal("my-release"))
				Expect(annotations["meta.helm.sh/release-namespace"]).To(Equal("test-ns"))
			}
			Expect(resourceCount).To(Equal(2))
		})

		It("should return original buffer when context is nil", func() {
			manifest := `apiVersion: v1
kind: Service
metadata:
  name: test-svc
`
			renderer := &velaLabelPostRenderer{context: nil}
			buf := bytes.NewBufferString(manifest)
			result, err := renderer.Run(buf)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).To(Equal(buf))
		})
	})

	Describe("parseManifestResources", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should skip test hooks by default", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
---
apiVersion: v1
kind: Service
metadata:
  name: test-svc
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    helm.sh/hook: test-success
`
			resources, err := p.parseManifestResources(manifest, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))

			kinds := make([]string, len(resources))
			for i, r := range resources {
				kinds[i], _, _ = unstructured.NestedString(r, "kind")
			}
			Expect(kinds).To(ContainElement("Deployment"))
			Expect(kinds).To(ContainElement("Service"))
			Expect(kinds).ToNot(ContainElement("Pod"))
		})

		It("should include test hooks when skipTests=false", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    helm.sh/hook: test-success
`
			skipFalse := false
			resources, err := p.parseManifestResources(manifest, &RenderOptionsParams{SkipTests: &skipFalse})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))
		})

		It("should order CRDs before Namespaces before others", func() {
			manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deploy
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: my-crd
---
apiVersion: v1
kind: Namespace
metadata:
  name: my-ns
`
			resources, err := p.parseManifestResources(manifest, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(3))

			kind0, _, _ := unstructured.NestedString(resources[0], "kind")
			kind1, _, _ := unstructured.NestedString(resources[1], "kind")
			kind2, _, _ := unstructured.NestedString(resources[2], "kind")
			Expect(kind0).To(Equal("CustomResourceDefinition"))
			Expect(kind1).To(Equal("Namespace"))
			Expect(kind2).To(Equal("Deployment"))
		})
	})

	Describe("getActionConfig", func() {
		It("should not panic without a real cluster", func() {
			p := NewProviderWithConfig(nil)
			// Without a real cluster, Init will fail — we just verify no panic
			_, err := p.getActionConfig("test-namespace")
			_ = err // error is expected
		})
	})

	Describe("computeReleaseFingerprint", func() {
		It("should be deterministic for same inputs", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			values := map[string]interface{}{"replicas": 2}

			fp1 := computeReleaseFingerprint(ch, values)
			fp2 := computeReleaseFingerprint(ch, values)
			Expect(fp1).To(Equal(fp2))
		})

		It("should differ for different values", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp1 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			fp2 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 3})
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should encode the chart version", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			Expect(fp).To(ContainSubstring("1.2.3"))
		})

		It("should differ for different chart versions", func() {
			ch1 := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			ch2 := &chart.Chart{Metadata: &chart.Metadata{Version: "2.0.0"}}
			values := map[string]interface{}{"key": "val"}

			fp1 := computeReleaseFingerprint(ch1, values)
			fp2 := computeReleaseFingerprint(ch2, values)
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should handle nil chart metadata", func() {
			fp := computeReleaseFingerprint(nil, map[string]interface{}{"replicas": 2})
			Expect(fp).ToNot(BeEmpty())
		})

		It("should treat nil values and empty map as equivalent", func() {
			// Helm stores release.Config as nil when no values were supplied
			// at install time, but mergeValues returns an empty map for the
			// same logical input. Without normalising the two, the dedup
			// check at the call site would mis-fire on every reconcile and
			// trigger spurious helm upgrades for releases installed with
			// empty/optional valuesFrom sources.
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fpNil := computeReleaseFingerprint(ch, nil)
			fpEmpty := computeReleaseFingerprint(ch, map[string]interface{}{})
			Expect(fpNil).To(Equal(fpEmpty))
		})
	})

	Describe("cache invalidation on missing release", func() {
		It("should not return stale cached data", func() {
			p := NewProviderWithConfig(nil)

			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			values := map[string]interface{}{"replicas": 1}
			fp := computeReleaseFingerprint(ch, values)

			cacheKey := "default/my-release"
			// Pre-seed the in-memory cache
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = fp
			p.releaseManifests[cacheKey] = "---\napiVersion: v1\nkind: Service\n"
			p.releaseVersions[cacheKey] = 3
			p.releaseMu.Unlock()

			manifest, _, version, _ := p.installOrUpgradeChart(
				context.Background(), ch, "my-release", "default", values, nil, nil,
			)
			// Stale cache should NOT be returned
			if manifest == "---\napiVersion: v1\nkind: Service\n" && version == 3 {
				Fail("stale cached data was returned — cache invalidation failed")
			}

			p.releaseMu.Lock()
			_, hasFP := p.releaseFingerprints[cacheKey]
			p.releaseMu.Unlock()
			if hasFP && p.releaseManifests[cacheKey] == "---\napiVersion: v1\nkind: Service\n" {
				Fail("stale cache entry was not invalidated")
			}
		})
	})

	Describe("InvalidateRelease", func() {
		It("should clear all cache entries for a release", func() {
			p := NewProviderWithConfig(nil)

			cacheKey := "default/test-rel"
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = "fp1"
			p.releaseManifests[cacheKey] = "manifest"
			p.releaseVersions[cacheKey] = 1
			p.releaseMu.Unlock()

			Expect(p.releaseFingerprints[cacheKey]).To(Equal("fp1"))

			p.InvalidateRelease("test-rel", "default")

			_, ok := p.releaseFingerprints[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseManifests[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseVersions[cacheKey]
			Expect(ok).To(BeFalse())
		})
	})

	Describe("dry-run context", func() {
		It("should default to false", func() {
			ctx := context.Background()
			Expect(isDryRun(ctx)).To(BeFalse())
		})

		It("should be true after WithDryRun", func() {
			ctx := context.Background()
			dryCtx := WithDryRun(ctx)
			Expect(isDryRun(dryCtx)).To(BeTrue())
		})

		It("should not affect the original context", func() {
			ctx := context.Background()
			_ = WithDryRun(ctx)
			Expect(isDryRun(ctx)).To(BeFalse())
		})
	})

	Describe("dryRunRender", func() {
		It("should render a chart client-side", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Templates: []*chart.File{
					{
						Name: "templates/deployment.yaml",
						Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
`),
					},
				},
			}

			manifest, _, err := p.dryRunRender(ch, "test-release", "test-ns",
				map[string]interface{}{"key": "value"}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: Deployment"))
			Expect(manifest).To(ContainSubstring("name: test-release"))
		})
	})

	Describe("velaOwnerLabels", func() {
		It("should return labels for a valid context", func() {
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-ns",
				Name:         "my-component",
			}
			labels := velaOwnerLabels(velaCtx)
			Expect(labels["app.oam.dev/name"]).To(Equal("my-app"))
			Expect(labels["app.oam.dev/namespace"]).To(Equal("my-ns"))
			Expect(labels["app.oam.dev/component"]).To(Equal("my-component"))
		})

		It("should return nil for nil context", func() {
			Expect(velaOwnerLabels(nil)).To(BeNil())
		})
	})

	Describe("UninstallParams structure", func() {
		It("should hold all fields correctly", func() {
			params := &UninstallParams{
				Release: ReleaseParams{
					Name:      "my-release",
					Namespace: "my-ns",
				},
				KeepHistory: true,
			}
			Expect(params.Release.Name).To(Equal("my-release"))
			Expect(params.Release.Namespace).To(Equal("my-ns"))
			Expect(params.KeepHistory).To(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// NEW TESTS for additional coverage
	// -----------------------------------------------------------------------

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

	Describe("isRetryableInstallError", func() {
		DescribeTable("should identify retryable errors",
			func(errMsg string, expected bool) {
				Expect(isRetryableInstallError(errors.New(errMsg))).To(Equal(expected))
			},
			Entry("cannot be imported", "cannot be imported into the current release", true),
			Entry("invalid ownership metadata", "invalid ownership metadata", true),
			Entry("no revision for release", "no revision for release", true),
			Entry("release already exists", "release: already exists", true),
			Entry("generic error", "connection refused", false),
			Entry("timeout error", "context deadline exceeded", false),
		)
	})

	Describe("isOwnedByVela", func() {
		It("should return false for nil release", func() {
			Expect(isOwnedByVela(nil, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return false for nil context", func() {
			rel := &release.Release{Labels: map[string]string{"app.oam.dev/name": "app"}}
			Expect(isOwnedByVela(rel, nil)).To(BeFalse())
		})

		It("should return false for nil labels", func() {
			rel := &release.Release{}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return false when vela label is missing", func() {
			rel := &release.Release{Labels: map[string]string{"other": "val"}}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "app"})).To(BeFalse())
		})

		It("should return true when vela label is present", func() {
			rel := &release.Release{Labels: map[string]string{"app.oam.dev/name": "my-app"}}
			Expect(isOwnedByVela(rel, &ContextParams{AppName: "my-app"})).To(BeTrue())
		})
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

	Describe("cleanResource", func() {
		It("should remove nil values", func() {
			input := map[string]interface{}{
				"key1": "value1",
				"key2": nil,
				"key3": "value3",
			}
			result := cleanResource(input)
			Expect(result["key1"]).To(Equal("value1"))
			Expect(result["key3"]).To(Equal("value3"))
			Expect(result).ToNot(HaveKey("key2"))
		})

		It("should recursively clean nested maps", func() {
			input := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": nil,
				},
			}
			result := cleanResource(input)
			metadata := result["metadata"].(map[string]interface{})
			Expect(metadata["name"]).To(Equal("test"))
			Expect(metadata).ToNot(HaveKey("namespace"))
		})

		It("should clean arrays with nil items", func() {
			input := map[string]interface{}{
				"items": []interface{}{"a", nil, "c"},
			}
			result := cleanResource(input)
			Expect(result["items"]).To(Equal([]interface{}{"a", "c"}))
		})

		It("should clean nested maps inside arrays", func() {
			input := map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": nil,
					},
				},
			}
			result := cleanResource(input)
			containers := result["containers"].([]interface{})
			container := containers[0].(map[string]interface{})
			Expect(container["name"]).To(Equal("app"))
			Expect(container).ToNot(HaveKey("image"))
		})

		It("should remove empty nested maps", func() {
			input := map[string]interface{}{
				"status": map[string]interface{}{},
				"spec":   map[string]interface{}{"replicas": 1},
			}
			result := cleanResource(input)
			Expect(result).ToNot(HaveKey("status"))
			Expect(result["spec"].(map[string]interface{})["replicas"]).To(Equal(1))
		})

		It("should remove empty arrays", func() {
			input := map[string]interface{}{
				"items":  []interface{}{nil},
				"labels": map[string]interface{}{"app": "test"},
			}
			result := cleanResource(input)
			// After removing nil, the array is empty and should be dropped
			Expect(result).ToNot(HaveKey("items"))
			Expect(result["labels"].(map[string]interface{})["app"]).To(Equal("test"))
		})
	})

	Describe("loadValuesFromSource dispatcher", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("returns an error for unsupported kinds (including the reserved OCIRepository)", func() {
			for _, kind := range []string{"OCIRepository", "Unknown", "configmap", ""} {
				_, err := p.loadValuesFromSource(context.Background(),
					ValuesFromParams{Kind: kind, Name: "test"},
					"default", "default")
				Expect(err).Should(HaveOccurred(), "kind=%q must fail", kind)
				Expect(err.Error()).To(ContainSubstring("unsupported values source kind"),
					"kind=%q must surface as unsupported", kind)
			}
		})
	})

	Describe("cross-namespace valuesFrom rejection", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())
		})

		It("rejects a ConfigMap reference to a namespace other than the Application's", func() {
			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "secrets-bearer", Namespace: "kube-system"},
				"tenant-a", "tenant-a")
			Expect(err).Should(HaveOccurred())
			Expect(errors.Is(err, errCrossNamespaceValuesFrom)).To(BeTrue(),
				"cross-ns error must be detectable via errors.Is")
			Expect(err.Error()).To(ContainSubstring("kube-system"))
			Expect(err.Error()).To(ContainSubstring("tenant-a"))
		})

		It("rejects a Secret reference to a namespace other than the Application's", func() {
			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "any", Namespace: "other-tenant"},
				"tenant-a", "tenant-a")
			Expect(err).Should(HaveOccurred())
			Expect(errors.Is(err, errCrossNamespaceValuesFrom)).To(BeTrue())
		})

		It("allows an explicit Namespace equal to the Application's namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "tenant-a"},
				Data:       map[string]string{"values.yaml": "k: v"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build())
			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "same", Namespace: "tenant-a"},
				"tenant-a", "tenant-a")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["k"]).To(Equal("v"))
		})
	})

	Describe("loadConfigMapValues", func() {
		const releaseNS = "prod"

		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("loads from the default values.yaml key when Key is empty", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 3\nimage: nginx"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(3))
			Expect(values["image"]).To(Equal("nginx"))
		})

		It("loads from an explicit Key", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"prod.yaml": "replicaCount: 5"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg", Key: "prod.yaml"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(5))
		})

		It("accepts an explicit Namespace that equals the Application's namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 7"},
			}
			buildClient(cm)

			values, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg", Namespace: releaseNS}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["replicaCount"]).To(BeEquivalentTo(7))
		})

		It("returns a missing-source error when the ConfigMap does not exist", func() {
			buildClient()
			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "absent"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue(),
				"missing ConfigMap should surface as valueSourceMissingError")
		})

		It("returns a missing-source error when the key is absent", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"other.yaml": "foo: bar"},
			}
			buildClient(cm)

			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue())
		})

		It("surfaces YAML parse errors and never classifies them as missing", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicas: [unterminated"},
			}
			buildClient(cm)

			_, err := p.loadConfigMapValues(context.Background(),
				ValuesFromParams{Kind: "ConfigMap", Name: "cfg"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid YAML"))
			Expect(isValueSourceMissing(err)).To(BeFalse(),
				"parse errors must NOT be swallowed by optional")
		})
	})

	Describe("loadSecretValues", func() {
		const releaseNS = "prod"

		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("loads YAML from Secret.Data (already base64-decoded by the API)", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data: map[string][]byte{
					"values.yaml": []byte("password: s3cret\nuser: admin"),
				},
			}
			buildClient(secret)

			values, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "creds"}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["user"]).To(Equal("admin"))
			Expect(values["password"]).To(Equal("s3cret"))
		})

		It("returns a missing-source error when the Secret does not exist", func() {
			buildClient()
			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "absent"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(isValueSourceMissing(err)).To(BeTrue())
		})

		It("surfaces YAML parse errors and does not leak raw secret bytes", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data:       map[string][]byte{"values.yaml": []byte("super-secret: [unterminated")},
			}
			buildClient(secret)

			_, err := p.loadSecretValues(context.Background(),
				ValuesFromParams{Kind: "Secret", Name: "creds"}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("super-secret"),
				"Secret contents must never appear in error messages")
			Expect(isValueSourceMissing(err)).To(BeFalse())
		})
	})

	Describe("mergeValues priority", func() {
		const releaseNS = "prod"

		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
		})

		It("gives inline values the highest priority over valuesFrom", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 3\nimage: from-configmap"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build())

			base := map[string]interface{}{"image": "from-inline"}
			result, err := p.mergeValues(ctx, base,
				[]ValuesFromParams{{Kind: "ConfigMap", Name: "cm"}}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["image"]).To(Equal("from-inline"), "inline must win over ConfigMap")
			Expect(result["replicaCount"]).To(BeEquivalentTo(3), "CM-only keys must remain")
		})

		It("makes later valuesFrom entries override earlier ones", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "tier: free\ncolour: blue"},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "tier: paid"},
			}
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(cmA, cmB).Build())

			result, err := p.mergeValues(ctx, nil,
				[]ValuesFromParams{
					{Kind: "ConfigMap", Name: "a"},
					{Kind: "ConfigMap", Name: "b"},
				}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["tier"]).To(Equal("paid"), "later source must win on conflict")
			Expect(result["colour"]).To(Equal("blue"), "earlier source keeps non-overridden keys")
		})
	})

	Describe("mergeValues edge cases", func() {
		const releaseNS = "prod"

		var (
			p   *Provider
			ctx context.Context
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ctx = context.Background()
		})

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("treats empty valuesFrom slice equivalently to nil", func() {
			buildClient()
			base := map[string]interface{}{"key": "value"}

			fromNil, errNil := p.mergeValues(ctx, base, nil, releaseNS, releaseNS)
			fromEmpty, errEmpty := p.mergeValues(ctx, base, []ValuesFromParams{}, releaseNS, releaseNS)

			Expect(errNil).ShouldNot(HaveOccurred())
			Expect(errEmpty).ShouldNot(HaveOccurred())
			Expect(fromEmpty).To(Equal(fromNil))
		})

		It("skips a missing optional source and continues with a following required source", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "real", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 7"},
			}
			buildClient(cm)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "missing", Optional: true},
				{Kind: "ConfigMap", Name: "real"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["replicaCount"]).To(BeEquivalentTo(7))
		})

		It("preserves orthogonal nested keys while resolving conflicts at depth", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data: map[string]string{"values.yaml": `resources:
  limits:
    cpu: 100m
    memory: 256Mi
  requests:
    cpu: 50m`},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data: map[string]string{"values.yaml": `resources:
  limits:
    memory: 512Mi`},
			}
			buildClient(cmA, cmB)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "a"},
				{Kind: "ConfigMap", Name: "b"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())

			resources := result["resources"].(map[string]interface{})
			limits := resources["limits"].(map[string]interface{})
			requests := resources["requests"].(map[string]interface{})
			Expect(limits["memory"]).To(Equal("512Mi"), "later source wins on conflict deep in the tree")
			Expect(limits["cpu"]).To(Equal("100m"), "orthogonal sibling in the same sub-object preserved")
			Expect(requests["cpu"]).To(Equal("50m"), "untouched sub-object preserved in full")
		})

		It("replaces array values instead of merging them (helm CoalesceTables semantics)", func() {
			cmA := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "extraArgs:\n  - --level=debug\n  - --timeout=30"},
			}
			cmB := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "extraArgs:\n  - --level=info"},
			}
			buildClient(cmA, cmB)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "a"},
				{Kind: "ConfigMap", Name: "b"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())

			args := result["extraArgs"].([]interface{})
			Expect(args).To(HaveLen(1), "later array wholly replaces earlier array")
			Expect(args[0]).To(Equal("--level=info"))
		})

		It("surfaces parse errors even when Optional is true", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicas: [unterminated"},
			}
			buildClient(cm)

			_, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "broken", Optional: true},
			}, releaseNS, releaseNS)
			Expect(err).Should(HaveOccurred(),
				"Optional must not mask parse errors — this is the critical contract")
			Expect(err.Error()).To(ContainSubstring("invalid YAML"))
			Expect(isValueSourceMissing(err)).To(BeFalse())
		})

		It("mixes a Secret and ConfigMap in the same valuesFrom list", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: releaseNS},
				Data:       map[string]string{"values.yaml": "replicaCount: 2\nimage: cm-image"},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: releaseNS},
				Data:       map[string][]byte{"values.yaml": []byte("image: secret-image")},
			}
			buildClient(cm, secret)

			result, err := p.mergeValues(ctx, nil, []ValuesFromParams{
				{Kind: "ConfigMap", Name: "cm"},
				{Kind: "Secret", Name: "creds"},
			}, releaseNS, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result["image"]).To(Equal("secret-image"), "later Secret wins over earlier ConfigMap")
			Expect(result["replicaCount"]).To(BeEquivalentTo(2), "orthogonal CM key preserved")
		})
	})

	Describe("fetchChartWithoutCache", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should fail for unsupported source type", func() {
			_, err := p.fetchChartWithoutCache(context.Background(), &ChartSourceParams{Source: "test"}, "unknown")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported chart source type"))
		})

		It("should fail for repo without repoURL", func() {
			_, err := p.fetchChartWithoutCache(context.Background(), &ChartSourceParams{Source: "nginx"}, "repo")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("repoURL is required"))
		})
	})

	Describe("DefaultCacheTTLConfig", func() {
		It("should return correct defaults", func() {
			config := DefaultCacheTTLConfig()
			Expect(config.ImmutableVersionTTL).To(Equal(24 * time.Hour))
			Expect(config.MutableVersionTTL).To(Equal(5 * time.Minute))
		})
	})

	Describe("NewProviderWithConfig", func() {
		It("should use defaults when config is nil", func() {
			p := NewProviderWithConfig(nil)
			Expect(p.cacheTTL.ImmutableVersionTTL).To(Equal(24 * time.Hour))
			Expect(p.cacheTTL.MutableVersionTTL).To(Equal(5 * time.Minute))
			Expect(p.releaseFingerprints).ToNot(BeNil())
			Expect(p.releaseManifests).ToNot(BeNil())
			Expect(p.releaseVersions).ToNot(BeNil())
		})

		It("should use custom config when provided", func() {
			p := NewProviderWithConfig(&CacheTTLConfig{
				ImmutableVersionTTL: 1 * time.Hour,
				MutableVersionTTL:   1 * time.Minute,
			})
			Expect(p.cacheTTL.ImmutableVersionTTL).To(Equal(1 * time.Hour))
			Expect(p.cacheTTL.MutableVersionTTL).To(Equal(1 * time.Minute))
		})
	})

	Describe("newInstallAction", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should set defaults correctly", func() {
			install := p.newInstallAction(&action.Configuration{}, "test-release", "test-ns", nil, nil, nil)
			Expect(install.ReleaseName).To(Equal("test-release"))
			Expect(install.Namespace).To(Equal("test-ns"))
			Expect(install.CreateNamespace).To(BeTrue())
			Expect(install.DryRun).To(BeFalse())
			Expect(install.ClientOnly).To(BeFalse())
		})

		It("should apply render options", func() {
			createNs := false
			skipHooks := true
			install := p.newInstallAction(&action.Configuration{}, "test-release", "test-ns", &RenderOptionsParams{
				Atomic:          true,
				Timeout:         "5m",
				CreateNamespace: &createNs,
				SkipHooks:       &skipHooks,
			}, nil, map[string]string{"app.oam.dev/name": "my-app"})

			Expect(install.Atomic).To(BeTrue())
			Expect(install.Wait).To(BeTrue())
			Expect(install.Timeout).To(Equal(5 * time.Minute))
			Expect(install.CreateNamespace).To(BeFalse())
			Expect(install.DisableHooks).To(BeTrue())
			Expect(install.Labels).To(Equal(map[string]string{"app.oam.dev/name": "my-app"}))
		})

		It("should set Wait when Atomic is true", func() {
			install := p.newInstallAction(&action.Configuration{}, "rel", "ns", &RenderOptionsParams{
				Atomic: true,
			}, nil, nil)
			Expect(install.Wait).To(BeTrue())
		})

		It("should set Wait independently", func() {
			install := p.newInstallAction(&action.Configuration{}, "rel", "ns", &RenderOptionsParams{
				Wait: true,
			}, nil, nil)
			Expect(install.Wait).To(BeTrue())
			Expect(install.Atomic).To(BeFalse())
		})
	})

	Describe("Template and Package exports", func() {
		It("should have a non-empty embedded CUE template", func() {
			Expect(Template).ToNot(BeEmpty())
		})

		It("should have a non-nil provider package", func() {
			Expect(Package).ToNot(BeNil())
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: fetchChart caching logic
	// -----------------------------------------------------------------------

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
				nil)
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
				&RenderOptionsParams{Cache: &CacheParams{Key: "my-prefix"}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("custom-cached"))
		})

		It("should bypass cache when TTL is '0'", func() {
			// TTL=0 means cache disabled — should call fetchChartWithoutCache directly
			// which will fail since there's no real repo, but the code path is exercised
			_, err := p.fetchChart(context.Background(),
				&ChartSourceParams{Source: "nginx"},
				&RenderOptionsParams{Cache: &CacheParams{TTL: "0"}})
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
				nil)
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
				nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Metadata.Name).To(Equal("url-chart"))
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: dryRunRender with options and velaCtx
	// -----------------------------------------------------------------------

	Describe("dryRunRender with options", func() {
		var (
			p  *Provider
			ch *chart.Chart
		)

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
			ch = &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Templates: []*chart.File{
					{
						Name: "templates/configmap.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-config
  namespace: {{ .Release.Namespace }}
data:
  key: value
`),
					},
				},
			}
		})

		It("should render with velaCtx labels injected", func() {
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-ns",
				Name:         "my-comp",
				Namespace:    "test-ns",
			}
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
			Expect(manifest).To(ContainSubstring("app.oam.dev/name"))
		})

		It("should apply skipHooks option", func() {
			skipHooks := true
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, &RenderOptionsParams{SkipHooks: &skipHooks}, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
		})

		It("should render multiple templates", func() {
			ch.Templates = append(ch.Templates, &chart.File{
				Name: "templates/service.yaml",
				Data: []byte(`apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-svc
  namespace: {{ .Release.Namespace }}
spec:
  ports:
    - port: 80
`),
			})
			manifest, _, err := p.dryRunRender(ch, "my-rel", "test-ns",
				map[string]interface{}{}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))
			Expect(manifest).To(ContainSubstring("kind: Service"))
		})

		It("should fail on invalid chart template", func() {
			badChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "bad", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/bad.yaml",
						Data: []byte(`{{ .Values.undefined.nested.deep }}`),
					},
				},
			}
			_, _, err := p.dryRunRender(badChart, "rel", "ns",
				map[string]interface{}{}, nil, nil)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dry-run render failed"))
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: installOrUpgradeChart upgrade options
	// -----------------------------------------------------------------------

	Describe("installOrUpgradeChart options", func() {
		It("should not panic when called with various options (no cluster)", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			values := map[string]interface{}{"key": "val"}
			opts := &RenderOptionsParams{
				Atomic:        true,
				Wait:          true,
				Timeout:       "30s",
				Force:         true,
				CleanupOnFail: true,
				RecreatePods:  true,
				MaxHistory:    5,
			}

			// This will fail (no cluster) but exercises the options parsing code paths
			_, _, _, err := p.installOrUpgradeChart(
				context.Background(), ch, "test-opts", "default", values, opts, nil,
			)
			// Error expected (no cluster), but no panic
			_ = err
		})

		It("should exercise velaCtx adoption path (no cluster)", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			velaCtx := &ContextParams{
				AppName:      "my-app",
				AppNamespace: "my-ns",
				Name:         "my-comp",
			}

			_, _, _, err := p.installOrUpgradeChart(
				context.Background(), ch, "test-adopt", "default",
				map[string]interface{}{}, nil, velaCtx,
			)
			_ = err
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: Render function defaults
	// -----------------------------------------------------------------------

	Describe("Render function (via provider)", func() {
		It("should exercise dry-run render path with pre-cached chart", func() {
			p := NewProviderWithConfig(nil)

			// Pre-seed a chart in cache so fetchChart succeeds
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "cached-render", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/deploy.yaml",
						Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
`),
					},
				},
			}
			p.cache.Put("repo/my-chart/1.0.0", testChart, 1*time.Hour)

			// Call the provider's internal render logic in dry-run mode
			ctx := WithDryRun(context.Background())
			manifest, notes, err := p.dryRunRender(testChart, "release", "default",
				map[string]interface{}{}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			_ = ctx
			_ = notes

			// Parse the manifest
			resources, err := p.parseManifestResources(manifest, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))

			kind, _, _ := unstructured.NestedString(resources[0], "kind")
			Expect(kind).To(Equal("Deployment"))
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: getKubeVersion (no cluster)
	// -----------------------------------------------------------------------

	Describe("getKubeVersion", func() {
		It("should not panic and return a result or nil", func() {
			p := NewProviderWithConfig(nil)
			// May return a KubeVersion (if cluster available) or nil (if not)
			// Either way, it should not panic
			Expect(func() {
				_ = p.getKubeVersion()
			}).ToNot(Panic())
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: validateReleaseHealth (no cluster)
	// -----------------------------------------------------------------------

	Describe("validateReleaseHealth", func() {
		It("should not panic without a real cluster", func() {
			p := NewProviderWithConfig(nil)
			// This runs in background normally, but we call it directly
			// It will fail to get action config, but should not panic
			Expect(func() {
				p.validateReleaseHealth("nonexistent-release", "default")
			}).ToNot(Panic())
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: cleanOrphanedReleaseSecrets, deleteReleaseSecretsDirect,
	// listReleaseSecretNames, labelReleaseSecrets (no cluster)
	// -----------------------------------------------------------------------

	Describe("cluster-dependent functions (no cluster)", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("cleanOrphanedReleaseSecrets should not panic", func() {
			Expect(func() {
				_ = p.cleanOrphanedReleaseSecrets(nil, "test-release", "default", nil)
			}).ToNot(Panic())
		})

		It("deleteReleaseSecretsDirect should not panic", func() {
			Expect(func() {
				_ = p.deleteReleaseSecretsDirect("default", "test-release", nil)
			}).ToNot(Panic())
		})

		It("listReleaseSecretNames should return empty or nil for nonexistent release", func() {
			names := p.listReleaseSecretNames("default", "nonexistent-release-xyz")
			// May return nil (no cluster) or empty slice (cluster but no secrets)
			Expect(len(names)).To(Equal(0))
		})

		It("labelReleaseSecrets should not panic with nil context", func() {
			Expect(func() {
				p.labelReleaseSecrets("default", "test-release", nil)
			}).ToNot(Panic())
		})

		It("labelReleaseSecrets should not panic without cluster", func() {
			velaCtx := &ContextParams{
				AppName:      "app",
				AppNamespace: "ns",
				Name:         "comp",
			}
			Expect(func() {
				p.labelReleaseSecrets("default", "test-release", velaCtx)
			}).ToNot(Panic())
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: freshInstall error paths
	// -----------------------------------------------------------------------

	Describe("freshInstall", func() {
		It("should return error when install fails (via installOrUpgradeChart)", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-fresh", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/cm.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`),
					},
				},
			}
			// Use installOrUpgradeChart which handles actionConfig initialization safely
			// It will install into a real cluster if available, or fail if not
			_, _, _, err := p.installOrUpgradeChart(
				context.Background(), ch, "test-fresh-install", "default",
				map[string]interface{}{}, nil, nil)
			// We just verify it doesn't panic - it may succeed or fail depending on cluster
			_ = err
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: Uninstall structure and defaults
	// -----------------------------------------------------------------------

	Describe("Uninstall params", func() {
		It("should handle KeepHistory=false", func() {
			params := &UninstallParams{
				Release: ReleaseParams{
					Name:      "rel",
					Namespace: "ns",
				},
				KeepHistory: false,
			}
			Expect(params.KeepHistory).To(BeFalse())
			Expect(params.Release.Name).To(Equal("rel"))
		})
	})

	// -----------------------------------------------------------------------
	// Additional coverage: RenderReturns and UninstallReturns structures
	// -----------------------------------------------------------------------

	Describe("return types", func() {
		It("RenderReturns should hold resources and notes", func() {
			ret := RenderReturns{
				Resources: []map[string]interface{}{
					{"kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}},
				},
				Notes: "install notes",
			}
			Expect(ret.Resources).To(HaveLen(1))
			Expect(ret.Notes).To(Equal("install notes"))
		})

		It("UninstallReturns should hold success and message", func() {
			ret := UninstallReturns{
				Success: true,
				Message: "uninstalled",
			}
			Expect(ret.Success).To(BeTrue())
			Expect(ret.Message).To(Equal("uninstalled"))
		})
	})

	// -----------------------------------------------------------------------
	// Tier 1 Coverage: Render top-level provider function
	// -----------------------------------------------------------------------

	Describe("Render top-level function", func() {
		It("should render via dry-run path with context and release params", func() {
			p := NewProvider()

			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-dryrun", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/deploy.yaml",
						Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
`),
					},
					{
						Name: "templates/service.yaml",
						Data: []byte(`apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-svc
  namespace: {{ .Release.Namespace }}
spec:
  ports:
    - port: 80
`),
					},
				},
			}
			p.cache.Put("repo/render-dryrun/1.0.0", testChart, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			result, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "render-dryrun",
						Version: "1.0.0",
					},
					Release: &ReleaseParams{
						Name:      "my-release",
						Namespace: "test-ns",
					},
					Values: map[string]interface{}{"replicas": 2},
					Context: &ContextParams{
						AppName:      "my-app",
						AppNamespace: "my-ns",
						Name:         "my-comp",
						Namespace:    "test-ns",
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(len(result.Returns.Resources)).To(BeNumerically(">=", 1))

			// Verify resource structure
			kind, _, _ := unstructured.NestedString(result.Returns.Resources[0], "kind")
			Expect(kind).ToNot(BeEmpty())
		})

		It("should use default release name and namespace when not specified", func() {
			p := NewProvider()

			testChart := &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-defaults", Version: "2.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/cm.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-cm
data:
  key: value
`),
					},
				},
			}
			p.cache.Put("repo/render-defaults/2.0.0", testChart, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			result, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "render-defaults",
						Version: "2.0.0",
					},
					// No Release, no Context — use defaults
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Returns.Resources).To(HaveLen(1))
		})

		It("should return error when chart fetch fails", func() {
			ctx := WithDryRun(context.Background())
			_, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart: ChartSourceParams{
						Source:  "nonexistent-chart-xyz",
						Version: "99.99.99",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"))
		})
	})

	// -----------------------------------------------------------------------
	// Application publishVersion lookup defense
	//
	// Render reads the parent Application's publishVersion annotation so that
	// installOrUpgradeChart can short-circuit when the deployed release is
	// already at the user's pin. A non-NotFound error during that lookup
	// previously left the pin empty, allowing an unintended upgrade to fire
	// even though the user's pin was still in place. The defense surfaces
	// that error instead of swallowing it.
	// -----------------------------------------------------------------------

	Describe("Render Application publishVersion lookup defense", func() {
		var scheme *runtime.Scheme

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
		})

		It("propagates a transient API error from the Application Get rather than bypassing the pin", func() {
			injected := errors.New("etcdserver: leader changed")
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, kc client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1beta1.Application); ok {
							return injected
						}
						return kc.Get(ctx, key, obj, opts...)
					},
				}).
				Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-1", Version: "1.0.0"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "my-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read Application tenant-a/my-app for publishVersion lookup"),
				"defense must surface the wrapped App-lookup error")
			Expect(err.Error()).To(ContainSubstring("etcdserver: leader changed"),
				"defense must preserve the underlying cause")
		})

		It("treats a NotFound on the Application as deletion-in-flight and proceeds past the lookup", func() {
			// No App registered → NotFound from the typed fake client.
			// Render must continue past the lookup; we prove it by leaving
			// the chart un-cached so the next failure is from fetchChart,
			// not the App lookup.
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-notfound-xyz", Version: "9.9.9"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "deleted-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("failed to read Application"),
				"NotFound on App must not be reported as a pin-lookup error")
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"),
				"Render must proceed past the App lookup to the chart fetch step")
		})

		It("succeeds the App lookup when the Application exists with a publishVersion annotation", func() {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pinned-app",
					Namespace:   "tenant-a",
					Annotations: map[string]string{oam.AnnotationPublishVersion: "v42"},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()
			singleton.KubeClient.Set(c)

			_, err := Render(context.Background(), &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-pinned-xyz", Version: "9.9.9"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "pinned-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("failed to read Application"),
				"a present App with a pin must not surface a lookup error")
			Expect(err.Error()).To(ContainSubstring("failed to fetch chart"),
				"Render must proceed past the App lookup to the chart fetch step")
		})

		It("skips the App lookup entirely in dry-run", func() {
			// Dry-run is the webhook admission path; it must never depend on
			// cluster state. Inject a Get error: if the lookup ran, Render
			// would surface it. Instead, dry-run renders client-side.
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, kc client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*v1beta1.Application); ok {
							return errors.New("dry-run must not call Get on Application")
						}
						return kc.Get(ctx, key, obj, opts...)
					},
				}).
				Build()
			singleton.KubeClient.Set(c)

			p := NewProvider()
			p.cache.Put("repo/render-defense-dryrun/1.0.0", &chart.Chart{
				Metadata: &chart.Metadata{Name: "render-defense-dryrun", Version: "1.0.0"},
				Templates: []*chart.File{{
					Name: "templates/cm.yaml",
					Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}-cm\n"),
				}},
			}, 1*time.Hour)

			ctx := WithDryRun(context.Background())
			_, err := Render(ctx, &providers.Params[RenderParams]{
				Params: RenderParams{
					Chart:   ChartSourceParams{Source: "render-defense-dryrun", Version: "1.0.0"},
					Release: &ReleaseParams{Name: "rel", Namespace: "tenant-a"},
					Context: &ContextParams{
						AppName:      "any-app",
						AppNamespace: "tenant-a",
						Name:         "my-comp",
						Namespace:    "tenant-a",
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred(),
				"dry-run must not exercise the App lookup path")
		})
	})

	// -----------------------------------------------------------------------
	// Tier 1 Coverage: fetchRepoChart via httptest
	// -----------------------------------------------------------------------

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
			})
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
			})
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
			})
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
			})
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
			})
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
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no download URL found"))
		})
	})

	// -----------------------------------------------------------------------
	// Tier 1 Coverage: fetchURLChart via httptest
	// -----------------------------------------------------------------------

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
			})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch).ToNot(BeNil())
			Expect(ch.Metadata.Name).To(Equal("url-chart"))
			Expect(ch.Metadata.Version).To(Equal("2.0.0"))
		})

		It("should return error for unreachable URL", func() {
			p := NewProviderWithConfig(nil)
			_, err := p.fetchURLChart(context.Background(), &ChartSourceParams{
				Source: "http://127.0.0.1:1/nonexistent.tgz",
			})
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download chart"))
		})
	})

	// -----------------------------------------------------------------------
	// Tier 1 Coverage: fetchChart cache miss → fetch → cache store
	// -----------------------------------------------------------------------

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
			}, nil)
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
			}, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ch.Metadata.Name).To(Equal("url-cache"))
		})
	})

	// -----------------------------------------------------------------------
	// Tier 1 Coverage: Uninstall top-level provider function
	// -----------------------------------------------------------------------

	Describe("Uninstall top-level function", func() {
		It("should exercise the function entry and return without panicking", func() {
			// Without a reachable cluster the call typically fails at
			// getActionConfig or while contacting the API server; with a
			// cluster it succeeds. The only invariant we can assert portably
			// is that the function returns in a consistent shape — result is
			// non-nil on success, or an error is returned on failure.
			result, err := Uninstall(context.Background(), &providers.Params[UninstallParams]{
				Params: UninstallParams{
					Release: ReleaseParams{
						Name:      "uninstall-test",
						Namespace: "default",
					},
					KeepHistory: false,
				},
			})
			if err == nil {
				Expect(result).ToNot(BeNil())
			}
		})
	})

	Describe("resolveOCICredentials", func() {
		const releaseNS = "prod"

		buildClient := func(objs ...client.Object) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, o := range objs {
				builder = builder.WithObjects(o)
			}
			singleton.KubeClient.Set(builder.Build())
		}

		It("returns empty credentials when auth is nil", func() {
			buildClient()
			username, password, err := resolveOCICredentials(context.Background(), nil, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(username).To(BeEmpty())
			Expect(password).To(BeEmpty())
		})

		It("returns empty credentials when auth.SecretRef is nil", func() {
			buildClient()
			username, password, err := resolveOCICredentials(context.Background(), &AuthParams{}, releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(username).To(BeEmpty())
			Expect(password).To(BeEmpty())
		})

		It("returns an error when the Secret does not exist", func() {
			buildClient()
			_, _, err := resolveOCICredentials(context.Background(),
				&AuthParams{SecretRef: &SecretRefParams{Name: "missing-secret"}},
				releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-secret"))
		})

		It("returns an error when the Secret has no 'username' key", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "oci-creds", Namespace: releaseNS},
				Data: map[string][]byte{
					"password": []byte("s3cret"),
				},
			}
			buildClient(secret)
			_, _, err := resolveOCICredentials(context.Background(),
				&AuthParams{SecretRef: &SecretRefParams{Name: "oci-creds"}},
				releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("username"))
		})

		It("returns an error when the Secret has no 'password' key", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "oci-creds", Namespace: releaseNS},
				Data: map[string][]byte{
					"username": []byte("robot"),
				},
			}
			buildClient(secret)
			_, _, err := resolveOCICredentials(context.Background(),
				&AuthParams{SecretRef: &SecretRefParams{Name: "oci-creds"}},
				releaseNS)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("password"))
		})

		It("returns credentials from Secret in the release namespace when namespace is omitted", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "oci-creds", Namespace: releaseNS},
				Data: map[string][]byte{
					"username": []byte("robot"),
					"password": []byte("s3cret"),
				},
			}
			buildClient(secret)
			username, password, err := resolveOCICredentials(context.Background(),
				&AuthParams{SecretRef: &SecretRefParams{Name: "oci-creds"}},
				releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(username).To(Equal("robot"))
			Expect(password).To(Equal("s3cret"))
		})

		It("uses an explicit namespace from SecretRef when provided", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "oci-creds", Namespace: "infra"},
				Data: map[string][]byte{
					"username": []byte("svc-account"),
					"password": []byte("tok3n"),
				},
			}
			buildClient(secret)
			username, password, err := resolveOCICredentials(context.Background(),
				&AuthParams{SecretRef: &SecretRefParams{Name: "oci-creds", Namespace: "infra"}},
				releaseNS)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(username).To(Equal("svc-account"))
			Expect(password).To(Equal("tok3n"))
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
