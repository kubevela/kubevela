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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	}

	assert.Equal(t, "nginx", params.Chart.Source)
	assert.Equal(t, "my-release", params.Release.Name)
	assert.Equal(t, 2, params.Values.(map[string]interface{})["replicaCount"])
}

// Note: Full integration tests would require:
// 1. Mocking Helm repository access
// 2. Providing test chart fixtures
// 3. Mocking Kubernetes client for Secret/ConfigMap access
// These would be added in a production implementation
