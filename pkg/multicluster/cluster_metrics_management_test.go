/*
Copyright 2022 The KubeVela Authors.

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
	"errors"
	"strconv"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	NormalClusterName       = "normal-cluster"
	DisconnectedClusterName = "disconnected-cluster"

	NodeName1 = "node-1"
	NodeName2 = "node-2"
)

func TestRefresh(t *testing.T) {
	ClusterGatewaySecretNamespace = "default"
	fakeClient := NewFakeClient(fake.NewClientBuilder().
		WithScheme(common.Scheme).
		WithRuntimeObjects(FakeManagedCluster("managed-cluster")).
		WithObjects(FakeSecret(NormalClusterName), FakeSecret(DisconnectedClusterName)).
		Build())

	normalCluster := fake.NewClientBuilder().
		WithScheme(common.Scheme).
		WithObjects(FakeNode(NodeName1, "8", strconv.FormatInt(16*1024*1024*1024, 10)),
			FakeNode(NodeName2, "7", strconv.FormatInt(32*1024*1024*1024, 10)),
			FakeNodeMetrics(NodeName1, "4", strconv.FormatInt(8*1024*1024*1024, 10)),
			FakeNodeMetrics(NodeName2, "1", strconv.FormatInt(3*1024*1024*1024, 10))).
		Build()

	disconnectedCluster := &disconnectedClient{}

	fakeClient.AddCluster(NormalClusterName, normalCluster)
	fakeClient.AddCluster(DisconnectedClusterName, disconnectedCluster)

	mgr, err := NewClusterMetricsMgr(context.Background(), fakeClient, 15*time.Second)
	assert.NilError(t, err)

	_, err = mgr.Refresh()
	assert.NilError(t, err)

	clusters, err := ListVirtualClusters(context.Background(), fakeClient)
	assert.NilError(t, err)

	for _, cluster := range clusters {
		assertClusterMetrics(t, &cluster)
	}

	disCluster, err := GetVirtualCluster(context.Background(), fakeClient, DisconnectedClusterName)
	assert.NilError(t, err)
	assertClusterMetrics(t, disCluster)

	norCluster, err := GetVirtualCluster(context.Background(), fakeClient, NormalClusterName)
	assert.NilError(t, err)
	assertClusterMetrics(t, norCluster)

	exportMetrics(disCluster.Metrics, disCluster.Name)
	exportMetrics(norCluster.Metrics, norCluster.Name)
}

func assertClusterMetrics(t *testing.T, cluster *VirtualCluster) {
	metrics := cluster.Metrics
	switch cluster.Name {
	case DisconnectedClusterName:
		assert.Equal(t, metrics.IsConnected, false)
		assert.Assert(t, metrics.ClusterInfo == nil)
		assert.Assert(t, metrics.ClusterUsageMetrics == nil)
	case NormalClusterName:
		assert.Equal(t, metrics.IsConnected, true)

		assert.Assert(t, resource.MustParse("15").Equal(metrics.ClusterInfo.CPUCapacity))
		assert.Assert(t, resource.MustParse(strconv.FormatInt(48*1024*1024*1024, 10)).Equal(metrics.ClusterInfo.MemoryCapacity))
		assert.Assert(t, resource.MustParse("15").Equal(metrics.ClusterInfo.CPUAllocatable))
		assert.Assert(t, resource.MustParse(strconv.FormatInt(48*1024*1024*1024, 10)).Equal(metrics.ClusterInfo.MemoryAllocatable))

		assert.Assert(t, resource.MustParse("5").Equal(metrics.ClusterUsageMetrics.CPUUsage))
		assert.Assert(t, resource.MustParse(strconv.FormatInt(11*1024*1024*1024, 10)).Equal(metrics.ClusterUsageMetrics.MemoryUsage))
	}
}

func FakeNodeMetrics(name string, cpu string, memory string) *metricsV1beta1api.NodeMetrics {
	nodeMetrics := &metricsV1beta1api.NodeMetrics{}
	nodeMetrics.Name = name
	nodeMetrics.Usage = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
	}
	return nodeMetrics
}

func FakeNode(name string, cpu string, memory string) *corev1.Node {
	node := &corev1.Node{}
	node.Name = name
	node.Status = corev1.NodeStatus{
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
	}
	return node
}

func FakeSecret(name string) *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = name
	secret.Namespace = ClusterGatewaySecretNamespace
	secret.Labels = map[string]string{
		clustercommon.LabelKeyClusterCredentialType: "ServiceAccountToken",
	}
	return secret
}

func FakeManagedCluster(name string) *clusterv1.ManagedCluster {
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = name
	return managedCluster
}

type disconnectedClient struct {
	client.Client
}

func (cli *disconnectedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return errors.New("no such host")
}
