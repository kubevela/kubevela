/*
Copyright 2024 The KubeVela Authors.

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

package multicluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newO11nTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = storagev1.AddToScheme(s)
	return s
}

func makeNode(name string, labels map[string]string, cpu, memory, pods string) *corev1.Node {
	q := func(s string) resource.Quantity { return resource.MustParse(s) }
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    q(cpu),
				corev1.ResourceMemory: q(memory),
				corev1.ResourcePods:   q(pods),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    q(cpu),
				corev1.ResourceMemory: q(memory),
				corev1.ResourcePods:   q(pods),
			},
		},
	}
}

func TestGetClusterInfo_LegacyMasterLabel(t *testing.T) {
	// Clusters running Kubernetes < 1.24 use node-role.kubernetes.io/master
	master := makeNode("master-0", map[string]string{
		"node-role.kubernetes.io/master": "",
	}, "4", "8Gi", "110")
	worker := makeNode("worker-0", nil, "4", "8Gi", "110")

	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(master, worker).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)
	assert.Equal(t, 1, info.MasterNumber)
	assert.Equal(t, 1, info.WorkerNumber)
}

func TestGetClusterInfo_ModernControlPlaneLabel(t *testing.T) {
	// Clusters running Kubernetes >= 1.24 use node-role.kubernetes.io/control-plane
	controlPlane := makeNode("cp-0", map[string]string{
		"node-role.kubernetes.io/control-plane": "",
	}, "4", "8Gi", "110")
	worker := makeNode("worker-0", nil, "4", "8Gi", "110")

	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controlPlane, worker).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)
	assert.Equal(t, 1, info.MasterNumber, "node-role.kubernetes.io/control-plane should be counted as master")
	assert.Equal(t, 1, info.WorkerNumber)
}

func TestGetClusterInfo_BothLabels(t *testing.T) {
	// Some clusters (e.g. kubeadm during transition) apply both labels to the same node
	controlPlane := makeNode("cp-0", map[string]string{
		"node-role.kubernetes.io/master":        "",
		"node-role.kubernetes.io/control-plane": "",
	}, "4", "8Gi", "110")
	worker := makeNode("worker-0", nil, "4", "8Gi", "110")

	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controlPlane, worker).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)
	// Node with both labels should be counted once as master, not double-counted
	assert.Equal(t, 1, info.MasterNumber)
	assert.Equal(t, 1, info.WorkerNumber)
}

func TestGetClusterInfo_AllWorkers(t *testing.T) {
	worker1 := makeNode("worker-0", nil, "4", "8Gi", "110")
	worker2 := makeNode("worker-1", nil, "4", "8Gi", "110")

	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(worker1, worker2).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)
	assert.Equal(t, 0, info.MasterNumber)
	assert.Equal(t, 2, info.WorkerNumber)
}

func TestGetClusterInfo_EmptyCluster(t *testing.T) {
	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)
	assert.Equal(t, 0, info.MasterNumber)
	assert.Equal(t, 0, info.WorkerNumber)
	assert.Equal(t, 0, len(info.Nodes.Items))
}

func TestGetClusterInfo_ResourceAggregation(t *testing.T) {
	worker1 := makeNode("worker-0", nil, "4", "8Gi", "110")
	worker2 := makeNode("worker-1", nil, "4", "8Gi", "110")

	scheme := newO11nTestScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(worker1, worker2).Build()

	info, err := GetClusterInfo(context.Background(), cli, "local")
	require.NoError(t, err)

	expectedCPU := resource.MustParse("8")
	expectedMemory := resource.MustParse("16Gi")
	assert.Equal(t, 0, info.CPUCapacity.Cmp(expectedCPU), "CPU capacity should be summed across nodes")
	assert.Equal(t, 0, info.MemoryCapacity.Cmp(expectedMemory), "Memory capacity should be summed across nodes")
}
