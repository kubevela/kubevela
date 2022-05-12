/*
Copyright 2020-2022 The KubeVela Authors.

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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	prismclusterv1alpha1 "github.com/kubevela/prism/pkg/apis/cluster/v1alpha1"
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/types"
)

var _ = Describe("Test Virtual Cluster", func() {

	It("Test Virtual Cluster", func() {
		ClusterGatewaySecretNamespace = "vela-system"
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ClusterGatewaySecretNamespace}})).Should(Succeed())

		By("Initialize Secrets")
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
					clustercommon.LabelKeyClusterEndpointType:   string(v1alpha1.ClusterEndpointTypeConst),
					"key": "value",
				},
				Annotations: map[string]string{types.AnnotationClusterAlias: "test-alias"},
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-no-label",
				Namespace: ClusterGatewaySecretNamespace,
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
				},
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-invalid",
				Namespace: ClusterGatewaySecretNamespace,
			},
		})).Should(Succeed())

		By("Test Get Virtual Cluster From Cluster Secret")
		vc, err := GetVirtualCluster(ctx, k8sClient, "test-cluster")
		Expect(err).Should(Succeed())
		Expect(vc.Type).Should(Equal(v1alpha1.CredentialTypeX509Certificate))
		Expect(vc.Labels["key"]).Should(Equal("value"))

		_, err = GetVirtualCluster(ctx, k8sClient, "cluster-not-found")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("no such cluster"))

		_, err = GetVirtualCluster(ctx, k8sClient, "cluster-invalid")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("not a valid cluster"))

		By("Add OCM ManagedCluster")
		Expect(k8sClient.Create(ctx, &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ocm-bad-cluster",
				Namespace: ClusterGatewaySecretNamespace,
			},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "ocm-cluster",
				Namespace:   ClusterGatewaySecretNamespace,
				Labels:      map[string]string{"key": "value"},
				Annotations: map[string]string{types.AnnotationClusterAlias: "ocm-alias"},
			},
			Spec: clusterv1.ManagedClusterSpec{
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{{URL: "test-url"}},
			},
		})).Should(Succeed())

		By("Test Get Virtual Cluster From OCM")

		_, err = GetVirtualCluster(ctx, k8sClient, "ocm-bad-cluster")
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("has no client config"))

		vc, err = GetVirtualCluster(ctx, k8sClient, "ocm-cluster")
		Expect(err).Should(Succeed())
		Expect(vc.Type).Should(Equal(types.CredentialTypeOCMManagedCluster))

		By("Test List Virtual Clusters")

		vcs, err := ListVirtualClusters(ctx, k8sClient)
		Expect(err).Should(Succeed())
		Expect(len(vcs)).Should(Equal(4))

		vcs, err = FindVirtualClustersByLabels(ctx, k8sClient, map[string]string{"key": "value"})
		Expect(err).Should(Succeed())
		Expect(len(vcs)).Should(Equal(2))

		By("Test prism cluster list for clusterNameMapper")
		cli := fakeClient{Client: k8sClient}
		cnm, err := NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Succeed())
		Expect(cnm.GetClusterName("example")).Should(Equal("example (example-alias)"))
		Expect(cnm.GetClusterName("no-alias")).Should(Equal("no-alias"))
		cli.returnBadRequest = true
		_, err = NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Satisfy(errors.IsBadRequest))
		cli.returnBadRequest = false
		cli.prismNotRegistered = true
		cnm, err = NewClusterNameMapper(ctx, cli)
		Expect(err).Should(Succeed())
		Expect(cnm.GetClusterName("example")).Should(Equal("example"))
		Expect(cnm.GetClusterName("test-cluster")).Should(Equal("test-cluster (test-alias)"))
		Expect(cnm.GetClusterName("ocm-cluster")).Should(Equal("ocm-cluster (ocm-alias)"))
		cli.returnBadRequest = true
		cli.prismNotRegistered = true
		_, err = NewClusterNameMapper(ctx, cli)
		Expect(err).ShouldNot(Succeed())
	})

})

type fakeClient struct {
	client.Client
	returnBadRequest   bool
	prismNotRegistered bool
}

func (c fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if !c.prismNotRegistered && c.returnBadRequest {
		return errors.NewBadRequest("")
	}
	if src, ok := list.(*prismclusterv1alpha1.ClusterList); ok {
		if c.prismNotRegistered {
			return runtime.NewNotRegisteredErrForKind("", schema.GroupVersionKind{})
		}
		objs := &prismclusterv1alpha1.ClusterList{Items: []prismclusterv1alpha1.Cluster{{
			ObjectMeta: metav1.ObjectMeta{Name: "example"},
			Spec:       prismclusterv1alpha1.ClusterSpec{Alias: "example-alias"},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "no-alias"},
		}}}
		objs.DeepCopyInto(src)
		return nil
	}
	if c.returnBadRequest {
		return errors.NewBadRequest("")
	}
	return c.Client.List(ctx, list, opts...)
}
