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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClusterLabels retrieves the cluster identity labels from the well-known
// ConfigMap in the vela-system namespace.
//
// The ConfigMap is expected to be named "vela-cluster-identity" and contain
// labels as key-value pairs in its Data field.
//
// Returns an empty map (not an error) if the ConfigMap does not exist,
// allowing definitions without placement constraints to work on any cluster.
func GetClusterLabels(ctx context.Context, c client.Client) (map[string]string, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      ClusterIdentityConfigMapName,
		Namespace: ClusterIdentityNamespace,
	}

	if err := c.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap doesn't exist - return empty labels
			// This allows definitions without placement constraints to work
			return map[string]string{}, nil
		}
		return nil, errors.Wrapf(err, "failed to get cluster identity ConfigMap %s/%s",
			ClusterIdentityNamespace, ClusterIdentityConfigMapName)
	}

	if cm.Data == nil {
		return map[string]string{}, nil
	}

	// Return a copy to prevent mutation
	labels := make(map[string]string, len(cm.Data))
	for k, v := range cm.Data {
		labels[k] = v
	}

	return labels, nil
}

// ClusterLabelsExist checks if the cluster identity ConfigMap exists.
// This can be used to warn users that cluster labels are not configured.
func ClusterLabelsExist(ctx context.Context, c client.Client) (bool, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      ClusterIdentityConfigMapName,
		Namespace: ClusterIdentityNamespace,
	}

	if err := c.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to check cluster identity ConfigMap")
	}

	return true, nil
}

// FormatClusterLabels returns a formatted string representation of cluster labels
// suitable for display in CLI output.
func FormatClusterLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "(no labels)"
	}

	result := ""
	first := true
	for k, v := range labels {
		if !first {
			result += ", "
		}
		result += k + "=" + v
		first = false
	}
	return result
}
