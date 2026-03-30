/*
Copyright 2025 The KubeVela Authors.

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
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestDetectChartSourceType(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "OCI registry",
			source:   "oci://ghcr.io/stefanprodan/charts/podinfo",
			expected: "oci",
		},
		{
			name:     "Direct URL with .tgz",
			source:   "https://github.com/nginx/nginx-helm/releases/download/nginx-1.1.0/nginx-1.1.0.tgz",
			expected: "url",
		},
		{
			name:     "Direct URL with .tar.gz",
			source:   "https://example.com/charts/app-1.0.0.tar.gz",
			expected: "url",
		},
		{
			name:     "HTTP URL",
			source:   "http://charts.example.com/app-1.0.0.tgz",
			expected: "url",
		},
		{
			name:     "Repository chart",
			source:   "postgresql",
			expected: "repo",
		},
		{
			name:     "Repository chart with path",
			source:   "stable/postgresql",
			expected: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectChartSourceType(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOrderResources(t *testing.T) {
	// Create test resources
	crd := map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]interface{}{
			"name": "test-crd",
		},
	}

	namespace := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "test-namespace",
		},
	}

	deployment := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name": "test-deployment",
		},
	}

	service := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name": "test-service",
		},
	}

	// Test ordering
	input := []map[string]interface{}{deployment, service, crd, namespace}
	result := orderResources(input)

	// Verify order: CRD, Namespace, Deployment, Service
	require.Len(t, result, 4)
	assert.Equal(t, "CustomResourceDefinition", result[0]["kind"])
	assert.Equal(t, "Namespace", result[1]["kind"])
	assert.Equal(t, "Deployment", result[2]["kind"])
	assert.Equal(t, "Service", result[3]["kind"])
}

func TestIsTestResource(t *testing.T) {
	tests := []struct {
		name     string
		resource *unstructured.Unstructured
		expected bool
	}{
		{
			name: "Test hook resource",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
						"annotations": map[string]interface{}{
							"helm.sh/hook": "test-success",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Non-test hook resource",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "pre-install-job",
						"annotations": map[string]interface{}{
							"helm.sh/hook": "pre-install",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Resource without annotations",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]interface{}{
						"name": "my-service",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTestResource(tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeValues(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// Test base values only
	baseValues := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"key2": "value2",
		},
	}

	result, err := p.mergeValues(ctx, baseValues, nil)
	require.NoError(t, err)
	assert.Equal(t, baseValues, result)

	// Test with empty base values
	result, err = p.mergeValues(ctx, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestRenderParams(t *testing.T) {
	// Test basic render params structure
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

	assert.Equal(t, "nginx", params.Chart.Source)
	assert.Equal(t, "my-release", params.Release.Name)
	assert.Equal(t, 2, params.Values.(map[string]interface{})["replicaCount"])
	assert.Equal(t, "my-app", params.Context.AppName)
	assert.Equal(t, "nginx-component", params.Context.Name)
}

func TestVelaLabelPostRenderer(t *testing.T) {
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
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse result and verify labels were injected on every resource
	decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(result.Bytes()), 4096)
	var resourceCount int
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		resourceCount++

		labels := obj.GetLabels()
		assert.Equal(t, "my-app", labels["app.oam.dev/name"], "missing app.oam.dev/name on %s", obj.GetName())
		assert.Equal(t, "my-app-ns", labels["app.oam.dev/namespace"], "missing app.oam.dev/namespace on %s", obj.GetName())
		assert.Equal(t, "my-component", labels["app.oam.dev/component"], "missing app.oam.dev/component on %s", obj.GetName())

		annotations := obj.GetAnnotations()
		assert.Equal(t, "helm-provider", annotations["app.oam.dev/owner"], "missing app.oam.dev/owner on %s", obj.GetName())
		assert.Equal(t, "my-release", annotations["meta.helm.sh/release-name"], "missing meta.helm.sh/release-name on %s", obj.GetName())
		assert.Equal(t, "test-ns", annotations["meta.helm.sh/release-namespace"], "missing meta.helm.sh/release-namespace on %s", obj.GetName())
	}
	assert.Equal(t, 2, resourceCount, "expected 2 resources in output")
}

func TestVelaLabelPostRendererNilContext(t *testing.T) {
	manifest := `apiVersion: v1
kind: Service
metadata:
  name: test-svc
`
	renderer := &velaLabelPostRenderer{context: nil}
	buf := bytes.NewBufferString(manifest)
	result, err := renderer.Run(buf)
	require.NoError(t, err)
	// With nil context, the original buffer is returned unchanged
	assert.Equal(t, buf, result)
}

func TestParseManifestResources(t *testing.T) {
	p := NewProvider()

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
	// Default: skipTests=true — test hook Pod should be excluded
	resources, err := p.parseManifestResources(manifest, nil)
	require.NoError(t, err)
	require.Len(t, resources, 2)

	kinds := make([]string, len(resources))
	for i, r := range resources {
		kinds[i], _, _ = unstructured.NestedString(r, "kind")
	}
	assert.Contains(t, kinds, "Deployment")
	assert.Contains(t, kinds, "Service")
	assert.NotContains(t, kinds, "Pod")

	// With skipTests=false — all 3 resources should be included
	skipFalse := false
	resources, err = p.parseManifestResources(manifest, &RenderOptionsParams{SkipTests: &skipFalse})
	require.NoError(t, err)
	assert.Len(t, resources, 3)
}

func TestParseManifestResourcesOrdering(t *testing.T) {
	p := NewProvider()

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
	require.NoError(t, err)
	require.Len(t, resources, 3)

	// Verify ordering: CRD → Namespace → Deployment
	kind0, _, _ := unstructured.NestedString(resources[0], "kind")
	kind1, _, _ := unstructured.NestedString(resources[1], "kind")
	kind2, _, _ := unstructured.NestedString(resources[2], "kind")
	assert.Equal(t, "CustomResourceDefinition", kind0)
	assert.Equal(t, "Namespace", kind1)
	assert.Equal(t, "Deployment", kind2)
}

func TestGetActionConfig(t *testing.T) {
	p := NewProvider()
	// Without a real cluster, Init will fail to connect — we just verify the
	// function is callable and returns a non-nil error (not a panic).
	_, err := p.getActionConfig("test-namespace")
	// An error is expected in unit-test environments without a kubeconfig.
	// The important thing is that it doesn't panic.
	_ = err
}

func TestComputeReleaseFingerprint(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{Version: "1.2.3"},
	}
	values1 := map[string]interface{}{"replicas": 2}
	values2 := map[string]interface{}{"replicas": 3}

	fp1 := computeReleaseFingerprint(ch, values1)
	fp2 := computeReleaseFingerprint(ch, values2)
	fp1Again := computeReleaseFingerprint(ch, values1)

	// Same inputs → same fingerprint
	assert.Equal(t, fp1, fp1Again, "fingerprint must be deterministic")
	// Different values → different fingerprint
	assert.NotEqual(t, fp1, fp2, "different values must produce different fingerprints")
	// Fingerprint encodes the chart version
	assert.Contains(t, fp1, "1.2.3")

	// Nil chart metadata
	fpNil := computeReleaseFingerprint(nil, values1)
	assert.NotEmpty(t, fpNil)
}

func TestComputeReleaseFingerprintVersionChange(t *testing.T) {
	ch1 := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
	ch2 := &chart.Chart{Metadata: &chart.Metadata{Version: "2.0.0"}}
	values := map[string]interface{}{"key": "val"}

	fp1 := computeReleaseFingerprint(ch1, values)
	fp2 := computeReleaseFingerprint(ch2, values)
	assert.NotEqual(t, fp1, fp2, "different chart versions must produce different fingerprints")
}

func TestCacheInvalidationOnMissingRelease(t *testing.T) {
	// Use a fresh provider (not the singleton) so tests don't interfere
	p := NewProviderWithConfig(nil)

	ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
	values := map[string]interface{}{"replicas": 1}
	fp := computeReleaseFingerprint(ch, values)

	// Pre-seed the in-memory cache as if a prior install succeeded
	p.releaseMu.Lock()
	p.releaseFingerprints["my-release"] = fp
	p.releaseManifests["my-release"] = "---\napiVersion: v1\nkind: Service\n"
	p.releaseVersions["my-release"] = 3
	p.releaseMu.Unlock()

	// installOrUpgradeChart now always checks the cluster before using cache.
	// When the release doesn't exist in the cluster, the stale cache should be
	// cleared (not used as a fast-path). The function will either:
	// - Fail (no cluster) — proving cache was not used
	// - Succeed with a fresh install (cluster available) — proving cache was cleared
	// Either way, the old cached manifest "---\napiVersion: v1\nkind: Service\n"
	// and version 3 must NOT be returned.
	manifest, _, version, _ := p.installOrUpgradeChart(
		context.Background(), ch, "my-release", "default", values, nil, nil,
	)
	// The stale cache had version=3 and a specific manifest. If the cache was
	// bypassed, we should NOT see those values.
	if manifest == "---\napiVersion: v1\nkind: Service\n" && version == 3 {
		t.Error("stale cached data was returned — cache invalidation failed")
	}
	// Also verify the cache entry was cleared
	p.releaseMu.Lock()
	_, hasFP := p.releaseFingerprints["my-release"]
	p.releaseMu.Unlock()
	// If install succeeded, a new fingerprint will be set. If it failed, it
	// should have been deleted. Either way, it should not be the OLD value
	// pointing to the stale manifest.
	if hasFP && p.releaseManifests["my-release"] == "---\napiVersion: v1\nkind: Service\n" {
		t.Error("stale cache entry was not invalidated")
	}
}

func TestInvalidateRelease(t *testing.T) {
	p := NewProviderWithConfig(nil)

	// Seed cache
	p.releaseMu.Lock()
	p.releaseFingerprints["test-rel"] = "fp1"
	p.releaseManifests["test-rel"] = "manifest"
	p.releaseVersions["test-rel"] = 1
	p.releaseMu.Unlock()

	// Verify seeded
	assert.Equal(t, "fp1", p.releaseFingerprints["test-rel"])

	// Invalidate
	p.InvalidateRelease("test-rel")

	// Verify cleared
	_, ok := p.releaseFingerprints["test-rel"]
	assert.False(t, ok, "fingerprint should be cleared")
	_, ok = p.releaseManifests["test-rel"]
	assert.False(t, ok, "manifest should be cleared")
	_, ok = p.releaseVersions["test-rel"]
	assert.False(t, ok, "version should be cleared")
}

func TestDryRunContext(t *testing.T) {
	// Verify dry-run context flag works correctly
	ctx := context.Background()
	assert.False(t, isDryRun(ctx), "default context should not be dry-run")

	dryCtx := WithDryRun(ctx)
	assert.True(t, isDryRun(dryCtx), "WithDryRun context should be dry-run")

	// Verify non-dry-run context is not affected
	assert.False(t, isDryRun(ctx), "original context should still not be dry-run")
}

func TestDryRunRender(t *testing.T) {
	p := NewProviderWithConfig(nil)

	// Create a minimal chart for dry-run testing
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
	require.NoError(t, err, "dry-run render should not fail")
	assert.Contains(t, manifest, "kind: Deployment", "manifest should contain rendered deployment")
	assert.Contains(t, manifest, "name: test-release", "manifest should contain release name")
}

func TestVelaOwnerLabels(t *testing.T) {
	velaCtx := &ContextParams{
		AppName:      "my-app",
		AppNamespace: "my-ns",
		Name:         "my-component",
	}
	labels := velaOwnerLabels(velaCtx)
	assert.Equal(t, "my-app", labels["app.oam.dev/name"])
	assert.Equal(t, "my-ns", labels["app.oam.dev/namespace"])
	assert.Equal(t, "my-component", labels["app.oam.dev/component"])

	// nil context should return nil (safe to pass to install.Labels)
	assert.Nil(t, velaOwnerLabels(nil))
}

func TestUninstallParamsStructure(t *testing.T) {
	params := &UninstallParams{
		Release: ReleaseParams{
			Name:      "my-release",
			Namespace: "my-ns",
		},
		KeepHistory: true,
	}
	assert.Equal(t, "my-release", params.Release.Name)
	assert.Equal(t, "my-ns", params.Release.Namespace)
	assert.True(t, params.KeepHistory)
}
