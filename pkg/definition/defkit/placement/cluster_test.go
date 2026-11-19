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

package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetClusterLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name           string
		configMap      *corev1.ConfigMap
		expectedLabels map[string]string
		expectError    bool
	}{
		{
			name: "configmap exists with labels",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClusterIdentityConfigMapName,
					Namespace: ClusterIdentityNamespace,
				},
				Data: map[string]string{
					"provider":     "aws",
					"cluster-type": "eks",
					"environment":  "production",
				},
			},
			expectedLabels: map[string]string{
				"provider":     "aws",
				"cluster-type": "eks",
				"environment":  "production",
			},
			expectError: false,
		},
		{
			name: "configmap exists with empty data",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClusterIdentityConfigMapName,
					Namespace: ClusterIdentityNamespace,
				},
				Data: map[string]string{},
			},
			expectedLabels: map[string]string{},
			expectError:    false,
		},
		{
			name: "configmap exists with nil data",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClusterIdentityConfigMapName,
					Namespace: ClusterIdentityNamespace,
				},
				Data: nil,
			},
			expectedLabels: map[string]string{},
			expectError:    false,
		},
		{
			name:           "configmap does not exist",
			configMap:      nil,
			expectedLabels: map[string]string{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.configMap != nil {
				objs = append(objs, tt.configMap)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			labels, err := GetClusterLabels(context.Background(), client)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLabels, labels)
			}
		})
	}
}

func TestGetClusterLabels_ReturnsCopy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterIdentityConfigMapName,
			Namespace: ClusterIdentityNamespace,
		},
		Data: map[string]string{
			"provider": "aws",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(cm).
		Build()

	labels1, err := GetClusterLabels(context.Background(), client)
	require.NoError(t, err)

	// Modify the returned map
	labels1["provider"] = "modified"
	labels1["new-key"] = "new-value"

	// Get labels again - should not be affected by modification
	labels2, err := GetClusterLabels(context.Background(), client)
	require.NoError(t, err)

	assert.Equal(t, "aws", labels2["provider"])
	_, hasNewKey := labels2["new-key"]
	assert.False(t, hasNewKey, "modification should not affect subsequent calls")
}

func TestClusterLabelsExist(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name        string
		configMap   *corev1.ConfigMap
		expected    bool
		expectError bool
	}{
		{
			name: "configmap exists",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClusterIdentityConfigMapName,
					Namespace: ClusterIdentityNamespace,
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name:        "configmap does not exist",
			configMap:   nil,
			expected:    false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.configMap != nil {
				objs = append(objs, tt.configMap)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			exists, err := ClusterLabelsExist(context.Background(), client)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, exists)
			}
		})
	}
}

func TestFormatClusterLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "(no labels)",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "(no labels)",
		},
		{
			name: "single label",
			labels: map[string]string{
				"provider": "aws",
			},
			expected: "provider=aws",
		},
		{
			name: "multiple labels",
			labels: map[string]string{
				"provider":    "aws",
				"environment": "production",
			},
			// Note: map iteration order is not guaranteed, so we check contains
			expected: "", // Will check contains instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatClusterLabels(tt.labels)

			if tt.name == "multiple labels" {
				// For multiple labels, check that all labels are present
				assert.Contains(t, result, "provider=aws")
				assert.Contains(t, result, "environment=production")
				assert.Contains(t, result, ", ")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	assert.Equal(t, "vela-cluster-identity", ClusterIdentityConfigMapName)
	assert.Equal(t, "vela-system", ClusterIdentityNamespace)
}
