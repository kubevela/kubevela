/*
 Copyright 2022. The KubeVela Authors.

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

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/oam"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

func TestBuildResourceArray(t *testing.T) {
	t.Parallel()
	// Define common objects used across tests
	pod1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "pod1",
			},
		},
	}
	pod2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "pod2",
				"annotations": map[string]interface{}{
					oam.AnnotationPublishVersion: "v2.0.0-pod",
					oam.AnnotationDeployVersion:  "rev2-pod",
				},
			},
		},
	}
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "my-app",
			},
		},
	}

	// Define common tree nodes
	parentWorkloadNode := &querytypes.ResourceTreeNode{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "my-app",
		Namespace:  "default",
		Object:     deployment,
	}

	pod1Node := &querytypes.ResourceTreeNode{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "pod1",
		Namespace:  "default",
		Object:     pod1,
	}

	pod2Node := &querytypes.ResourceTreeNode{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "pod2",
		Namespace:  "default",
		Object:     pod2,
	}

	replicaSetNode := &querytypes.ResourceTreeNode{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "my-app-rs",
		Namespace:  "default",
		LeafNodes:  []*querytypes.ResourceTreeNode{pod1Node, pod2Node},
	}

	// Define test cases
	testCases := map[string]struct {
		res        querytypes.AppliedResource
		parent     *querytypes.ResourceTreeNode
		node       *querytypes.ResourceTreeNode
		kind       string
		apiVersion string
		expected   []querytypes.ResourceItem
	}{
		"simple case with one matching pod": {
			res: querytypes.AppliedResource{
				Cluster:        "local",
				Component:      "my-comp",
				PublishVersion: "v1.0.0",
				DeployVersion:  "rev1",
			},
			parent:     parentWorkloadNode,
			node:       pod1Node,
			kind:       "Pod",
			apiVersion: "v1",
			expected: []querytypes.ResourceItem{
				{
					Cluster:   "local",
					Component: "my-comp",
					Workload: querytypes.Workload{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "my-app",
						Namespace:  "default",
					},
					Object:         pod1,
					PublishVersion: "v1.0.0",
					DeployVersion:  "rev1",
				},
			},
		},
		"nested case with multiple matching pods": {
			res: querytypes.AppliedResource{
				Cluster:        "remote",
				Component:      "my-comp-2",
				PublishVersion: "v2.0.0",
				DeployVersion:  "rev2",
			},
			parent:     parentWorkloadNode,
			node:       replicaSetNode,
			kind:       "Pod",
			apiVersion: "v1",
			expected: []querytypes.ResourceItem{
				{
					Cluster:   "remote",
					Component: "my-comp-2",
					Workload: querytypes.Workload{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       "my-app-rs",
						Namespace:  "default",
					},
					Object:         pod1,
					PublishVersion: "v2.0.0",
					DeployVersion:  "rev2",
				},
				{
					Cluster:   "remote",
					Component: "my-comp-2",
					Workload: querytypes.Workload{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       "my-app-rs",
						Namespace:  "default",
					},
					Object:         pod2,
					PublishVersion: "v2.0.0-pod", // From annotation
					DeployVersion:  "rev2-pod",   // From annotation
				},
			},
		},
		"no matching nodes": {
			res:        querytypes.AppliedResource{},
			parent:     parentWorkloadNode,
			node:       pod1Node,
			kind:       "Service",
			apiVersion: "v1",
			expected:   nil,
		},
		"empty node": {
			res:        querytypes.AppliedResource{},
			parent:     parentWorkloadNode,
			node:       &querytypes.ResourceTreeNode{},
			kind:       "Pod",
			apiVersion: "v1",
			expected:   nil,
		},
		"complex tree with mixed resources": {
			res: querytypes.AppliedResource{
				Cluster:        "local",
				Component:      "my-comp",
				PublishVersion: "v1.0.0",
				DeployVersion:  "rev1",
			},
			parent: parentWorkloadNode,
			node: &querytypes.ResourceTreeNode{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       "my-app-rs",
				Namespace:  "default",
				LeafNodes: []*querytypes.ResourceTreeNode{
					pod1Node,
					{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       "my-service",
					},
					pod2Node,
				},
			},
			kind:       "Pod",
			apiVersion: "v1",
			expected: []querytypes.ResourceItem{
				{
					Cluster:   "local",
					Component: "my-comp",
					Workload: querytypes.Workload{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       "my-app-rs",
						Namespace:  "default",
					},
					Object:         pod1,
					PublishVersion: "v1.0.0",
					DeployVersion:  "rev1",
				},
				{
					Cluster:   "local",
					Component: "my-comp",
					Workload: querytypes.Workload{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       "my-app-rs",
						Namespace:  "default",
					},
					Object:         pod2,
					PublishVersion: "v2.0.0-pod",
					DeployVersion:  "rev2-pod",
				},
			},
		},
		"case-insensitive matching": {
			res: querytypes.AppliedResource{
				Cluster:        "local",
				Component:      "my-comp",
				PublishVersion: "v1.0.0",
				DeployVersion:  "rev1",
			},
			parent:     parentWorkloadNode,
			node:       pod1Node,
			kind:       "pod",
			apiVersion: "V1",
			expected:   nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := buildResourceArray(tc.res, tc.parent, tc.node, tc.kind, tc.apiVersion)
			assert.ElementsMatch(t, tc.expected, result, "The returned resource items should match the expected ones")
		})
	}
}

func TestBuildResourceItem(t *testing.T) {
	t.Parallel()
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
		},
	}
	podWithAnnotations := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod-annotated",
				"annotations": map[string]interface{}{
					oam.AnnotationPublishVersion: "v2.0.0-annotated",
					oam.AnnotationDeployVersion:  "rev2-annotated",
				},
			},
		},
	}
	res := querytypes.AppliedResource{
		Cluster:        "test-cluster",
		Component:      "test-comp",
		PublishVersion: "v1.0.0-res",
		DeployVersion:  "rev1-res",
	}
	workload := querytypes.Workload{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test-workload",
		Namespace:  "test-ns",
	}

	t.Run("without annotations", func(t *testing.T) {
		t.Parallel()
		item := buildResourceItem(res, workload, pod)
		assert.Equal(t, "test-cluster", item.Cluster)
		assert.Equal(t, "test-comp", item.Component)
		assert.Equal(t, workload, item.Workload)
		assert.Equal(t, pod, item.Object)
		assert.Equal(t, "v1.0.0-res", item.PublishVersion)
		assert.Equal(t, "rev1-res", item.DeployVersion)
	})

	t.Run("with annotations", func(t *testing.T) {
		t.Parallel()
		item := buildResourceItem(res, workload, podWithAnnotations)
		assert.Equal(t, "test-cluster", item.Cluster)
		assert.Equal(t, "test-comp", item.Component)
		assert.Equal(t, workload, item.Workload)
		assert.Equal(t, podWithAnnotations, item.Object)
		assert.Equal(t, "v2.0.0-annotated", item.PublishVersion)
		assert.Equal(t, "rev2-annotated", item.DeployVersion)
	})

	t.Run("annotation override", func(t *testing.T) {
		item := buildResourceItem(res, workload, podWithAnnotations)
		assert.Equal(t, "v2.0.0-annotated", item.PublishVersion)
		assert.Equal(t, "rev2-annotated", item.DeployVersion)
	})
}
