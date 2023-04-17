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
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	kfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	restfake "k8s.io/client-go/rest/fake"
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
	ctx := context.Background()
	ClusterGatewaySecretNamespace = "default"
	fakeClient := NewFakeClient(fake.NewClientBuilder().
		WithScheme(common.Scheme).
		WithRuntimeObjects(FakeManagedCluster("managed-cluster")).
		WithObjects(FakeSecret(NormalClusterName), FakeSecret(DisconnectedClusterName)).
		Build())
	fakeK := &fakeStaticClient{kfake.NewSimpleClientset()}

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

	mgr, err := NewClusterStatusMgr(ctx, fakeClient, fakeK, 15*time.Second)
	assert.NilError(t, err)

	_ = mgr.Refresh()
	// TODO fix
	/*
		assert.NilError(t, err)

		clusterClient := NewClusterClient(fakeClient)
		clusters, err := clusterClient.List(ctx)
		assert.NilError(t, err)

		for _, cluster := range clusters.Items {
			assertClusterMetrics(t, &cluster)
		}

		disCluster, err := clusterClient.Get(ctx, DisconnectedClusterName)
		assert.NilError(t, err)
		assertClusterMetrics(t, disCluster)

		norCluster, err := clusterClient.Get(ctx, NormalClusterName)
		assert.NilError(t, err)
		assertClusterMetrics(t, norCluster)

		exportMetrics(disCluster)
		exportMetrics(norCluster)
	*/
}

//func assertClusterMetrics(t *testing.T, cluster *v1alpha1.VirtualCluster) {
//	status := cluster.Status
//	switch cluster.Name {
//	case DisconnectedClusterName:
//		assert.Equal(t, status.Healthy, false)
//		//assert.Assert(t, status.Resources == nil)
//	case NormalClusterName:
//		assert.Equal(t, status.Healthy, true)
//
//		assert.Assert(t, resource.MustParse("15").Equal(status.Resources.Capacity[corev1.ResourceCPU]))
//		assert.Assert(t, resource.MustParse(strconv.FormatInt(48*1024*1024*1024, 10)).Equal(status.Resources.Capacity[corev1.ResourceMemory]))
//		assert.Assert(t, resource.MustParse("15").Equal(status.Resources.Allocatable[corev1.ResourceCPU]))
//		assert.Assert(t, resource.MustParse(strconv.FormatInt(48*1024*1024*1024, 10)).Equal(status.Resources.Allocatable[corev1.ResourceMemory]))
//
//		assert.Assert(t, resource.MustParse("5").Equal(status.Resources.Usage[corev1.ResourceCPU]))
//		assert.Assert(t, resource.MustParse(strconv.FormatInt(11*1024*1024*1024, 10)).Equal(status.Resources.Usage[corev1.ResourceMemory]))
//	}
//}

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
		clustercommon.LabelKeyClusterEndpointType:   "127.0.0.1",
	}
	return secret
}

func FakeManagedCluster(name string) *clusterv1.ManagedCluster {
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = name
	managedCluster.Spec.ManagedClusterClientConfigs = []clusterv1.ClientConfig{{}}
	return managedCluster
}

type disconnectedClient struct {
	client.Client
}

func (cli *disconnectedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return errors.New("no such host")
}

type fakeStaticClient struct {
	kubernetes.Interface
}

func (in *fakeStaticClient) Discovery() discovery.DiscoveryInterface {
	return &fakeDiscovery{}
}

type fakeDiscovery struct {
	discovery.DiscoveryInterface
}

func (in *fakeDiscovery) RESTClient() rest.Interface {
	return &restfake.RESTClient{
		Resp: &http.Response{
			StatusCode: 200,
			Body:       &fakeReaderCloser{bytes.NewReader([]byte(`{}`))},
		},
	}
}

type fakeReaderCloser struct {
	io.Reader
}

func (in *fakeReaderCloser) Close() error {
	return nil
}
