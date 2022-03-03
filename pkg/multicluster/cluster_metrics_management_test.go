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

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	NormalClusterName       = "normal-cluster"
	DisconnectedClusterName = "disconnected-cluster"
)

func TestRefresh(t *testing.T) {
	fakeClient := NewFakeClient(fake.NewClientBuilder().
		WithScheme(common.Scheme).
		WithRuntimeObjects(FakeManagedCluster("managed-cluster")).
		WithObjects(FakeSecret("normal-cluster"), FakeSecret("disconnected-cluster")).
		Build())

	normalCluster := fake.NewClientBuilder().
		WithScheme(common.Scheme).
		WithObjects(FakeNode("node-1", "8", strconv.FormatInt(16*1024*1024*1024, 10)), FakeNode("node-2", "7", strconv.FormatInt(32*1024*1024*1024, 10))).
		Build()

	disconnectedCluster := &disconnectedClient{}

	fakeClient.AddCluster("normal-cluster", normalCluster)
	fakeClient.AddCluster("disconnected-cluster", disconnectedCluster)
	mgr := NewClusterMetricsMgr(fakeClient)
	err := mgr.Refresh()
	assert.NilError(t, err)

	var clusterName []string
	var detail MetricsDetail

	// assert isConnected
	clusterName, detail, err = mgr.IsConnected()
	assert.NilError(t, err)
	assert.Equal(t, clusterName[0], DisconnectedClusterName)
	assert.Equal(t, clusterName[1], NormalClusterName)
	assert.Equal(t, detail.value[0], "false")
	assert.Equal(t, detail.value[1], "true")
	assert.Equal(t, detail.description, IsConnectedDescription)

	// assert cpu
	clusterName, detail, err = mgr.CPUResources()
	assert.NilError(t, err)
	assert.Equal(t, clusterName[0], DisconnectedClusterName)
	assert.Equal(t, clusterName[1], NormalClusterName)
	assert.Equal(t, detail.value[0], "0")
	assert.Equal(t, detail.value[1], "15000")
	assert.Equal(t, detail.description, CPUResourceDescription)

	// assert memory
	clusterName, detail, err = mgr.MemoryResources()
	assert.NilError(t, err)
	assert.Equal(t, clusterName[0], DisconnectedClusterName)
	assert.Equal(t, clusterName[1], NormalClusterName)
	assert.Equal(t, detail.value[0], "0")
	assert.Equal(t, detail.value[1], strconv.FormatInt(48*1024, 10))
	assert.Equal(t, detail.description, MemoryResourceDescription)
}

func FakeNode(name string, cpu string, memory string) *corev1.Node {
	node := &corev1.Node{}
	node.Name = name
	node.Kind = "node"
	node.APIVersion = "v1"
	node.Status = corev1.NodeStatus{Allocatable: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
	}}
	return node
}

func FakeSecret(name string) *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = name
	secret.Kind = "Secret"
	secret.APIVersion = "v1"
	secret.Labels = map[string]string{
		v1alpha1.LabelKeyClusterCredentialType: "ServiceAccountToken",
	}
	return secret
}

func FakeManagedCluster(name string) *clusterv1.ManagedCluster {
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = name
	managedCluster.Kind = "ManagedCluster"
	managedCluster.APIVersion = "v1"
	return managedCluster
}

type disconnectedClient struct {
	client.Client
}

func (cli *disconnectedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return errors.New("no such host")
}
